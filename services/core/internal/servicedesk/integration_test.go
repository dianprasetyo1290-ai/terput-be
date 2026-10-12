package servicedesk_test

// Integration tests against Postgres (TEST_PG_DSN, all core migrations
// applied) with httptest fakes for identity, notification and ops (T9).

import (
	"bytes"
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"skci/core/internal/client"
	"skci/core/internal/common"
	"skci/core/internal/config"
	"skci/core/internal/handler"
	"skci/core/internal/model"
	"skci/core/internal/servicedesk"
	"skci/core/internal/storage"
	"skci/core/internal/store"
)

const (
	jwtSecret   = "test-jwt-secret-sd"
	internalKey = "test-internal-key-sd"
)

func newID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	h := hex.EncodeToString(b)
	return h[0:8] + "-" + h[8:12] + "-4" + h[13:16] + "-a" + h[17:20] + "-" + h[20:32]
}

// fakes records what the downstream services received.
type fakes struct {
	mu          sync.Mutex
	features    map[string]bool             // school → service_desk on
	guardians   map[string]bool             // guardian|student
	roleUsers   map[string][]client.UserRef // role → users
	notifyKeys  []string                    // Idempotency-Key per notification
	woKeys      []string                    // Idempotency-Key per work-order POST
	woByKey     map[string]client.WorkOrder // ops idempotency store
	woFailFirst bool                        // create the order but answer 500 once
	opsDown     bool
}

func (f *fakes) notifications(substr string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, k := range f.notifyKeys {
		if strings.Contains(k, substr) {
			n++
		}
	}
	return n
}

type env struct {
	t      *testing.T
	db     *sql.DB
	sd     *servicedesk.Service
	router http.Handler
	f      *fakes
	core   *store.PGStore
	now    time.Time
}

func writeData(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"data": data})
}

func setup(t *testing.T) *env {
	t.Helper()
	dsn := os.Getenv("TEST_PG_DSN")
	if dsn == "" {
		t.Skip("TEST_PG_DSN not set, skipping service desk integration test")
	}
	db, err := store.Open(dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	f := &fakes{features: map[string]bool{}, guardians: map[string]bool{}, roleUsers: map[string][]client.UserRef{},
		woByKey: map[string]client.WorkOrder{}}
	identity := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		p := r.URL.Path
		switch {
		case strings.HasPrefix(p, "/schools/") && strings.HasSuffix(p, "/features"):
			school := strings.Split(p, "/")[2]
			writeData(w, 200, []map[string]any{{"feature_key": "service_desk", "enabled": f.features[school]}})
		case strings.HasPrefix(p, "/internal/guardians/"):
			parts := strings.Split(p, "/") // /internal/guardians/{g}/students/{s}/access
			if f.guardians[parts[3]+"|"+parts[5]] {
				writeData(w, 200, map[string]bool{"allowed": true})
				return
			}
			w.WriteHeader(404)
		case p == "/users":
			if role := r.URL.Query().Get("role"); role != "" {
				out := []map[string]string{}
				for _, u := range f.roleUsers[role] {
					out = append(out, map[string]string{"id": u.ID, "name": u.Name})
				}
				writeData(w, 200, out)
				return
			}
			writeData(w, 200, []any{})
		default:
			w.WriteHeader(404)
		}
	}))
	t.Cleanup(identity.Close)
	notif := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.notifyKeys = append(f.notifyKeys, r.Header.Get("Idempotency-Key"))
		f.mu.Unlock()
		writeData(w, 201, map[string]string{"id": "n"})
	}))
	t.Cleanup(notif.Close)
	ops := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		if f.opsDown {
			w.WriteHeader(503)
			return
		}
		if r.Header.Get("X-Internal-Key") != internalKey {
			w.WriteHeader(401)
			return
		}
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/internal/facility/work-orders":
			var in client.WorkOrderInput
			_ = json.NewDecoder(r.Body).Decode(&in)
			key := r.Header.Get("Idempotency-Key")
			f.woKeys = append(f.woKeys, key)
			if in.IdempotencyKey != key || in.SchoolID == "" || in.TicketRef == "" || in.Priority == "" {
				w.WriteHeader(422)
				return
			}
			if wo, ok := f.woByKey[key]; ok {
				writeData(w, 200, wo)
				return
			}
			wo := client.WorkOrder{ID: newID(), Code: fmt.Sprintf("WO-2026-%04d", len(f.woByKey)+1), Status: "dilaporkan"}
			f.woByKey[key] = wo
			if f.woFailFirst {
				f.woFailFirst = false
				w.WriteHeader(500)
				return
			}
			writeData(w, 201, wo)
		case r.Method == http.MethodGet && r.URL.Path == "/internal/facility/work-orders":
			out := []client.WorkOrder{}
			for _, wo := range f.woByKey {
				wo.AssigneeName = "Teknisi A"
				out = append(out, wo)
			}
			writeData(w, 200, out)
		default:
			w.WriteHeader(404)
		}
	}))
	t.Cleanup(ops.Close)

	cfg := config.Config{JWTSecret: jwtSecret, InternalKey: internalKey, CORSOrigins: []string{"http://localhost"},
		MaxUploadBytes: 1 << 20, SchoolTimezone: "Asia/Jakarta", SelfInternalURL: "http://core.test"}
	cl := client.New(notif.URL, identity.URL, "", internalKey)
	cl.OpsURL = ops.URL
	core := store.NewPGStore(db)
	sd, err := servicedesk.New(db, core, cl, storage.NewLocal(t.TempDir(), cfg.MaxUploadBytes), cfg)
	if err != nil {
		t.Fatal(err)
	}
	e := &env{t: t, db: db, sd: sd, f: f, core: core}
	// 2026-10-12 is a Monday.
	e.now = time.Date(2026, 10, 12, 10, 0, 0, 0, sd.Loc)
	sd.Now = func() time.Time { return e.now }
	h := handler.New(core, cfg, cl)
	h.ServiceDesk = sd
	e.router = handler.NewRouter(h, cfg)
	return e
}

// waitFor polls a background side effect (menu 81 notifies in a goroutine).
func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	for i := 0; i < 100; i++ {
		if cond() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("kondisi tidak tercapai")
}

func token(t *testing.T, userID, schoolID, role string) string {
	t.Helper()
	claims := common.Claims{UserID: userID, SchoolID: schoolID, ActiveRole: role, Roles: []string{role},
		RegisteredClaims: jwt.RegisteredClaims{ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour))}}
	s, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(jwtSecret))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

type resp struct {
	Code int
	Body map[string]any
	Raw  string
}

func (e *env) do(method, path, tok string, body any) resp {
	e.t.Helper()
	var rd *bytes.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	} else {
		rd = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, rd)
	req.Header.Set("Content-Type", "application/json")
	if tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	return e.serve(req)
}

func (e *env) multipart(path, tok string, fields map[string]string, fileName, mime string, content []byte) resp {
	e.t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	for k, v := range fields {
		_ = mw.WriteField(k, v)
	}
	if fileName != "" {
		h := make(map[string][]string)
		h["Content-Disposition"] = []string{fmt.Sprintf(`form-data; name="files"; filename="%s"`, fileName)}
		h["Content-Type"] = []string{mime}
		part, _ := mw.CreatePart(h)
		_, _ = part.Write(content)
	}
	_ = mw.Close()
	req := httptest.NewRequest(http.MethodPost, path, &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+tok)
	return e.serve(req)
}

func (e *env) serve(req *http.Request) resp {
	rec := httptest.NewRecorder()
	e.router.ServeHTTP(rec, req)
	out := resp{Code: rec.Code, Raw: rec.Body.String()}
	_ = json.Unmarshal(rec.Body.Bytes(), &out.Body)
	return out
}

func (r resp) data() map[string]any {
	m, _ := r.Body["data"].(map[string]any)
	return m
}

func (r resp) list() []any {
	l, _ := r.Body["data"].([]any)
	return l
}

func (e *env) must(r resp, code int) resp {
	e.t.Helper()
	if r.Code != code {
		e.t.Fatalf("want %d got %d: %s", code, r.Code, r.Raw)
	}
	return r
}

// school bundles one tenant with its desk staff.
type school struct {
	id, manager, agent, otherAgent, supervisor string
	mTok, aTok, oTok                           string
	teamID, otherTeamID                        string
}

func (e *env) newSchool(enabled bool) *school {
	s := &school{id: newID(), manager: newID(), agent: newID(), otherAgent: newID(), supervisor: newID()}
	e.f.mu.Lock()
	e.f.features[s.id] = enabled
	e.f.mu.Unlock()
	s.mTok = token(e.t, s.manager, s.id, servicedesk.RoleManager)
	s.aTok = token(e.t, s.agent, s.id, servicedesk.RoleAgent)
	s.oTok = token(e.t, s.otherAgent, s.id, servicedesk.RoleAgent)
	if !enabled {
		return s
	}
	team := e.must(e.do("POST", "/service-desk/teams", s.mTok, map[string]any{
		"name": "Tim Keuangan", "manager_user_id": s.supervisor, "member_ids": []string{s.agent}}), 200).data()
	s.teamID = team["id"].(string)
	other := e.must(e.do("POST", "/service-desk/teams", s.mTok, map[string]any{
		"name": "Tim Sarpras", "member_ids": []string{s.otherAgent}}), 200).data()
	s.otherTeamID = other["id"].(string)
	// Senin–Jumat 07:00–15:00
	hours := []map[string]any{}
	for d := 1; d <= 5; d++ {
		hours = append(hours, map[string]any{"weekday": d, "opens_at": "07:00", "closes_at": "15:00"})
	}
	e.must(e.do("PUT", "/service-desk/business-hours", s.mTok, hours), 200)
	return s
}

func (e *env) category(s *school, body map[string]any) string {
	return e.must(e.do("POST", "/service-desk/categories", s.mTok, body), 200).data()["id"].(string)
}

func TestBRSD01_ServiceRequestBecomesTicket(t *testing.T) {
	e := setup(t)
	s := e.newSchool(true)
	ctx := context.Background()
	srCat, err := e.core.CreateServiceRequestCategory(ctx, model.ServiceRequestCategory{
		SchoolID: s.id, Name: "Surat Keterangan", HandlerRole: "admin", RequiresStudent: false, Active: true})
	if err != nil {
		t.Fatal(err)
	}
	catID := e.category(s, map[string]any{"name": "Tagihan", "team_id": s.teamID, "reporter_kinds": []string{"orang_tua"},
		"service_request_category_id": srCat.ID, "active": true})
	e.must(e.do("POST", "/service-desk/sla-policies", s.mTok, map[string]any{
		"category_id": catID, "priority": "normal", "response_value": 4, "response_unit": "business_hours",
		"resolution_value": 2, "resolution_unit": "business_days", "warn_bp": 8000, "effective_from": "2026-01-01",
	}), 200)

	e.f.mu.Lock()
	e.f.roleUsers["admin"] = []client.UserRef{{ID: newID(), Name: "Admin TU"}}
	e.f.mu.Unlock()
	parent := newID()
	pTok := token(t, parent, s.id, "orang_tua")
	sr := e.must(e.do("POST", "/parent/service-requests", pTok, map[string]any{
		"category_id": srCat.ID, "subject": "Surat aktif", "description": "Untuk beasiswa"}), 201).data()
	srID := sr["id"].(string)

	link := e.must(e.do("GET", "/service-desk/service-requests/"+srID+"/ticket", pTok, nil), 200).data()
	if n := e.f.notifications("service_request:" + srID); n != 0 {
		t.Fatalf("SR bertiket tidak boleh mengirim notifikasi menu 81 terpisah, got %d", n)
	}
	ticketID := link["ticket_id"].(string)
	if !strings.HasPrefix(link["number"].(string), "SD-2026-") {
		t.Fatalf("nomor tiket %v", link["number"])
	}
	tk := e.must(e.do("GET", "/service-desk/tickets/"+ticketID, s.aTok, nil), 200).data()["ticket"].(map[string]any)
	if tk["channel"] != "parent_portal" || tk["service_request_id"] != srID || tk["resolution_due_at"] == nil {
		t.Fatalf("tiket tidak tertaut / tanpa SLA: %v", tk)
	}
	if e.f.notifications("sd:"+ticketID+":new:"+s.agent) != 1 {
		t.Fatalf("anggota tim harus diberi tahu tiket baru: %v", e.f.notifyKeys)
	}

	// Ticket status mirrors to the menu 81 request.
	e.must(e.do("POST", "/service-desk/tickets/"+ticketID+"/assign", s.aTok, map[string]any{"user_id": s.agent}), 200)
	srNow, _ := e.core.GetServiceRequest(ctx, s.id, srID)
	if srNow.Status != "accepted" {
		t.Fatalf("SR status = %s, want accepted", srNow.Status)
	}
	e.must(e.do("POST", "/service-desk/tickets/"+ticketID+"/status", s.aTok, map[string]any{"to": "in_progress"}), 200)
	srNow, _ = e.core.GetServiceRequest(ctx, s.id, srID)
	if srNow.Status != "in_progress" {
		t.Fatalf("SR status = %s, want in_progress", srNow.Status)
	}

	// Menu 81 handler can no longer change it.
	adminTok := token(t, newID(), s.id, "admin")
	r := e.must(e.do("POST", "/service-requests/"+srID+"/status", adminTok, map[string]any{"to": "done"}), 409)
	if !strings.Contains(r.Body["message"].(string), "Service Desk") {
		t.Fatalf("pesan 409: %v", r.Body["message"])
	}
	e.must(e.do("POST", "/service-requests/"+srID+"/result", adminTok, map[string]any{"result_attachments": []any{}}), 409)

	// Agent result document (public comment with file) → SR result_attachments on resolve.
	e.must(e.multipart("/service-desk/tickets/"+ticketID+"/comments", s.aTok,
		map[string]string{"body": "Surat terlampir"}, "surat.pdf", "application/pdf", []byte("%PDF-1.4 test")), 200)
	e.must(e.multipart("/service-desk/tickets/"+ticketID+"/comments", s.aTok,
		map[string]string{"body": "catatan internal: cek arsip", "internal": "true"}, "", "", nil), 200)
	e.must(e.do("POST", "/service-desk/tickets/"+ticketID+"/status", s.aTok, map[string]any{"to": "resolved", "note": "Selesai"}), 200)
	srNow, _ = e.core.GetServiceRequest(ctx, s.id, srID)
	if srNow.Status != "done" || !strings.Contains(string(srNow.ResultAttachments), "surat.pdf") {
		t.Fatalf("SR = %s %s", srNow.Status, srNow.ResultAttachments)
	}
	events, _ := e.core.ListServiceRequestEvents(ctx, s.id, srID)
	synced := 0
	for _, ev := range events {
		if strings.HasPrefix(ev.Note, "Sinkron dari tiket SD-") {
			synced++
		}
	}
	if synced != 3 { // accepted, in_progress, done
		t.Fatalf("event sinkron SR tidak ada: %+v", events)
	}

	// Reporter view: no internal comment, sees public file, can rate once.
	mine := e.must(e.do("GET", "/me/tickets/"+ticketID, pTok, nil), 200).data()
	raw, _ := json.Marshal(mine["events"])
	if strings.Contains(string(raw), "catatan internal") {
		t.Fatal("komentar internal bocor ke pelapor")
	}
	if mine["ticket"].(map[string]any)["sla_snapshot"] != nil {
		t.Fatal("snapshot SLA tidak untuk pelapor")
	}
	if e.f.notifications("sd:"+ticketID+":") < 4 {
		t.Fatalf("pelapor harus diberi tahu setiap perubahan: %v", e.f.notifyKeys)
	}
	e.must(e.do("POST", "/me/tickets/"+ticketID+"/satisfaction", pTok, map[string]any{"score": 5, "comment": "Cepat"}), 200)
	e.must(e.do("POST", "/me/tickets/"+ticketID+"/satisfaction", pTok, map[string]any{"score": 4}), 422)

	// Another parent and an agent of another team cannot see it.
	e.must(e.do("GET", "/me/tickets/"+ticketID, token(t, newID(), s.id, "orang_tua"), nil), 404)
	e.must(e.do("GET", "/service-desk/tickets/"+ticketID, s.oTok, nil), 404)
	if l := e.must(e.do("GET", "/service-desk/tickets", s.oTok, nil), 200).list(); len(l) != 0 {
		t.Fatalf("agent tim lain melihat %d tiket", len(l))
	}
	// Other school cannot reach it either.
	other := e.newSchool(true)
	e.must(e.do("GET", "/service-desk/tickets/"+ticketID, other.mTok, nil), 404)

	// Report: one ticket, met, satisfaction 5.00.
	rep := e.must(e.do("GET", "/service-desk/reports?from=2026-10-01&to=2026-10-31&group_by=category", s.mTok, nil), 200).list()
	row := rep[0].(map[string]any)
	if row["total"].(float64) != 1 || row["met"].(float64) != 1 || row["compliance_bp"].(float64) != 10000 || row["avg_satisfaction_x100"].(float64) != 500 {
		t.Fatalf("laporan %v", row)
	}
	e.must(e.do("GET", "/service-desk/reports?from=2026-10-01&to=2026-10-31", s.aTok, nil), 403)
	e.must(e.do("GET", "/service-desk/reports?from=2026-10-01&to=2026-10-31", token(t, newID(), s.id, "kepala_sekolah"), nil), 200)
}

func TestBRSD01_FeatureOffOrUnmappedKeepsMenu81(t *testing.T) {
	e := setup(t)
	ctx := context.Background()

	e.f.mu.Lock()
	e.f.roleUsers["admin"] = []client.UserRef{{ID: newID(), Name: "Admin TU"}}
	e.f.mu.Unlock()
	off := e.newSchool(false)
	srCat, _ := e.core.CreateServiceRequestCategory(ctx, model.ServiceRequestCategory{
		SchoolID: off.id, Name: "Surat", HandlerRole: "admin", Active: true})
	pTok := token(t, newID(), off.id, "orang_tua")
	sr := e.must(e.do("POST", "/parent/service-requests", pTok, map[string]any{
		"category_id": srCat.ID, "subject": "A", "description": "B"}), 201).data()
	e.must(e.do("GET", "/service-desk/service-requests/"+sr["id"].(string)+"/ticket", pTok, nil), 404)
	waitFor(t, func() bool { return e.f.notifications("service_request:"+sr["id"].(string)) == 1 })
	e.must(e.do("GET", "/service-desk/tickets", off.mTok, nil), 403)
	e.must(e.do("GET", "/me/tickets", pTok, nil), 403)
	// menu 81 still works
	e.must(e.do("POST", "/service-requests/"+sr["id"].(string)+"/status", token(t, newID(), off.id, "admin"),
		map[string]any{"to": "accepted"}), 200)

	// Feature on, but no mapping and no fallback → no ticket.
	on := e.newSchool(true)
	srCat2, _ := e.core.CreateServiceRequestCategory(ctx, model.ServiceRequestCategory{
		SchoolID: on.id, Name: "Surat", HandlerRole: "admin", Active: true})
	p2 := token(t, newID(), on.id, "orang_tua")
	sr2 := e.must(e.do("POST", "/parent/service-requests", p2, map[string]any{
		"category_id": srCat2.ID, "subject": "A", "description": "B"}), 201).data()
	e.must(e.do("GET", "/service-desk/service-requests/"+sr2["id"].(string)+"/ticket", p2, nil), 404)

	// Fallback category catches it; exception retry endpoint is idempotent.
	e.category(on, map[string]any{"name": "Umum", "team_id": on.teamID, "reporter_kinds": []string{"orang_tua"},
		"is_fallback": true, "active": true})
	req := httptest.NewRequest("POST", "/internal/service-desk/from-service-request/"+sr2["id"].(string)+"?school_id="+on.id, nil)
	req.Header.Set("X-Internal-Key", internalKey)
	first := e.must(e.serve(req), 200).data()
	req = httptest.NewRequest("POST", "/internal/service-desk/from-service-request/"+sr2["id"].(string)+"?school_id="+on.id, nil)
	req.Header.Set("X-Internal-Key", internalKey)
	second := e.must(e.serve(req), 200).data()
	if first["id"] != second["id"] {
		t.Fatal("retry harus idempoten (satu tiket per SR)")
	}
	// A second fallback is rejected.
	e.must(e.do("POST", "/service-desk/categories", on.mTok, map[string]any{"name": "Umum 2", "team_id": on.teamID,
		"reporter_kinds": []string{"orang_tua"}, "is_fallback": true, "active": true}), 409)
}

func TestAC13701_TicketToWorkOrder(t *testing.T) {
	e := setup(t)
	s := e.newSchool(true)
	facility := e.category(s, map[string]any{"name": "Fasilitas", "team_id": s.teamID, "reporter_kinds": []string{"pegawai"},
		"is_facility": true, "active": true,
		"form_schema": []map[string]any{{"key": "lokasi", "label": "Lokasi", "type": "text", "required": true}}})
	billing := e.category(s, map[string]any{"name": "Tagihan", "team_id": s.teamID, "reporter_kinds": []string{"orang_tua"}, "active": true})

	guru := token(t, newID(), s.id, "guru")
	// FR-137-002: category without 'pegawai' rejects employees; form validation (FR-137-001).
	data, _ := json.Marshal(map[string]any{"category_id": billing, "subject": "x", "description": "y"})
	e.must(e.multipart("/me/tickets", guru, map[string]string{"data": string(data)}, "", "", nil), 422)
	data, _ = json.Marshal(map[string]any{"category_id": facility, "subject": "AC kelas 8A mati", "description": "Tidak dingin"})
	r := e.must(e.multipart("/me/tickets", guru, map[string]string{"data": string(data)}, "", "", nil), 422)
	if !strings.Contains(r.Raw, "lokasi") {
		t.Fatalf("galat per field: %s", r.Raw)
	}
	data, _ = json.Marshal(map[string]any{"category_id": facility, "subject": "AC kelas 8A mati", "description": "Tidak dingin",
		"form_answers": map[string]any{"lokasi": "8A"}})
	tk := e.must(e.multipart("/me/tickets", guru, map[string]string{"data": string(data)}, "foto.png", "image/png", []byte("\x89PNG fake")), 201).data()
	ticketID := tk["id"].(string)
	if tk["channel"] != "employee_portal" {
		t.Fatalf("channel %v", tk["channel"])
	}

	// ops answers 500 after creating → 502 + exception; retry reuses the key and links the same order.
	e.f.woFailFirst = true
	e.must(e.do("POST", "/service-desk/tickets/"+ticketID+"/work-order", s.aTok, nil), 502)
	var excs int
	_ = e.db.QueryRow(`SELECT count(*) FROM integration_exceptions WHERE school_id=$1 AND kind='callback'`, s.id).Scan(&excs)
	if excs != 1 {
		t.Fatalf("exception callback = %d", excs)
	}
	got := e.must(e.do("POST", "/service-desk/tickets/"+ticketID+"/work-order", s.aTok, map[string]any{}), 200).data()
	if got["work_order_id"] == nil || got["work_order_number"] != "WO-2026-0001" {
		t.Fatalf("tautan PK tidak tersimpan: %v", got)
	}
	again := e.must(e.do("POST", "/service-desk/tickets/"+ticketID+"/work-order", s.aTok, nil), 200).data()
	if again["work_order_id"] != got["work_order_id"] {
		t.Fatal("konversi kedua harus mengembalikan PK yang sama")
	}
	if len(e.f.woKeys) != 2 || e.f.woKeys[0] != e.f.woKeys[1] || e.f.woKeys[0] != "sd_ticket:"+ticketID+":work_order" {
		t.Fatalf("Idempotency-Key ke ops: %v", e.f.woKeys)
	}
	detail := e.must(e.do("GET", "/service-desk/tickets/"+ticketID, s.aTok, nil), 200).data()
	if wo, _ := detail["work_order"].(map[string]any); wo == nil || wo["assignee_name"] != "Teknisi A" {
		t.Fatalf("status PK tidak ditarik dari ops: %v", detail["work_order"])
	}

	// Non-facility ticket → 422.
	pTok := token(t, newID(), s.id, "orang_tua")
	data, _ = json.Marshal(map[string]any{"category_id": billing, "subject": "SPP", "description": "Salah nominal"})
	bt := e.must(e.multipart("/me/tickets", pTok, map[string]string{"data": string(data)}, "", "", nil), 201).data()
	e.must(e.do("POST", "/service-desk/tickets/"+bt["id"].(string)+"/work-order", s.aTok, nil), 422)

	// Attachment download: reporter yes, other user no.
	att := tk["id"].(string)
	evs := e.must(e.do("GET", "/me/tickets/"+att, guru, nil), 200).data()["events"].([]any)
	key := evs[0].(map[string]any)["attachments"].([]any)[0].(map[string]any)["key"].(string)
	req := httptest.NewRequest("GET", "/me/tickets/"+att+"/files/"+key, nil)
	req.Header.Set("Authorization", "Bearer "+guru)
	if rr := e.serve(req); rr.Code != 200 || !strings.Contains(rr.Raw, "PNG") {
		t.Fatalf("unduh lampiran: %d", rr.Code)
	}
	req = httptest.NewRequest("GET", "/me/tickets/"+att+"/files/"+key, nil)
	req.Header.Set("Authorization", "Bearer "+pTok)
	if rr := e.serve(req); rr.Code != 404 {
		t.Fatalf("pelapor lain harus 404, got %d", rr.Code)
	}
}

func TestAC13801_SLABreachEscalatesOnce(t *testing.T) {
	e := setup(t)
	s := e.newSchool(true)
	catID := e.category(s, map[string]any{"name": "Tagihan", "team_id": s.teamID, "reporter_kinds": []string{"orang_tua"}, "active": true})
	e.must(e.do("POST", "/service-desk/sla-policies", s.mTok, map[string]any{
		"category_id": catID, "priority": "normal", "response_value": 4, "response_unit": "business_hours",
		"resolution_value": 2, "resolution_unit": "business_days", "warn_bp": 8000, "effective_from": "2026-01-01",
		"escalation_rules": []map[string]any{
			{"trigger": "warning", "level": 1, "target_kind": "team_manager"},
			{"trigger": "breach", "level": 2, "target_kind": "team_manager"},
		},
	}), 200)
	pTok := token(t, newID(), s.id, "orang_tua")
	data, _ := json.Marshal(map[string]any{"category_id": catID, "subject": "Tagihan ganda", "description": "SPP Oktober dua kali"})
	tk := e.must(e.multipart("/me/tickets", pTok, map[string]string{"data": string(data)}, "", "", nil), 201).data()
	id := tk["id"].(string)
	want := time.Date(2026, 10, 14, 10, 0, 0, 0, e.sd.Loc)
	due, _ := time.Parse(time.RFC3339, tk["resolution_due_at"].(string))
	if !due.Equal(want) {
		t.Fatalf("resolution_due_at %s want %s", due, want)
	}
	// Agent answers so only the resolution SLA matters.
	e.must(e.multipart("/service-desk/tickets/"+id+"/comments", s.aTok, map[string]string{"body": "Kami cek"}, "", "", nil), 200)

	// Policy change after creation is not retroactive (snapshot).
	pols := e.must(e.do("GET", "/service-desk/sla-policies", s.mTok, nil), 200).list()
	pol := pols[0].(map[string]any)
	pol["resolution_value"] = 10
	e.must(e.do("PUT", "/service-desk/sla-policies/"+pol["id"].(string), s.mTok, pol), 200)

	ctx := context.Background()
	e.now = time.Date(2026, 10, 12, 11, 0, 0, 0, e.sd.Loc)
	if err := e.sd.RunSLAJob(ctx); err != nil {
		t.Fatal(err)
	}
	if n := e.f.notifications(":esc:"); n != 0 {
		t.Fatalf("belum waktunya eskalasi, got %d", n)
	}

	e.now = want.Add(time.Minute) // 2 hari kerja + 1 menit
	for i := 0; i < 2; i++ {
		if err := e.sd.RunSLAJob(ctx); err != nil {
			t.Fatal(err)
		}
	}
	var breachedAt *time.Time
	var level int
	_ = e.db.QueryRow(`SELECT resolution_breached_at, escalation_level FROM sd_tickets WHERE id=$1`, id).Scan(&breachedAt, &level)
	if breachedAt == nil || level != 2 {
		t.Fatalf("breached_at=%v level=%d", breachedAt, level)
	}
	var escEvents, breachEvents int
	_ = e.db.QueryRow(`SELECT count(*) FILTER (WHERE kind='escalation'), count(*) FILTER (WHERE kind='sla_breach') FROM sd_ticket_events WHERE ticket_id=$1`, id).
		Scan(&escEvents, &breachEvents)
	if escEvents != 1 || breachEvents != 1 {
		t.Fatalf("escalation=%d sla_breach=%d (want 1/1)", escEvents, breachEvents)
	}
	if n := e.f.notifications("sd:" + id + ":esc:breach:2:" + s.supervisor); n != 1 {
		t.Fatalf("notifikasi eskalasi ke supervisor = %d: %v", n, e.f.notifyKeys)
	}
	list := e.must(e.do("GET", "/service-desk/tickets?sla=breached", s.aTok, nil), 200).list()
	if len(list) != 1 || list[0].(map[string]any)["sla_state"] != "breached" {
		t.Fatalf("filter melanggar SLA: %v", list)
	}
	// Reporter never sees escalation internals.
	mine := e.must(e.do("GET", "/me/tickets/"+id, pTok, nil), 200).data()
	raw, _ := json.Marshal(mine)
	if strings.Contains(string(raw), "Eskalasi") || strings.Contains(string(raw), "escalation_level\":2") {
		t.Fatal("detail eskalasi bocor ke pelapor")
	}
}

func TestAuthorization(t *testing.T) {
	e := setup(t)
	s := e.newSchool(true)
	guru := token(t, newID(), s.id, "guru")
	e.must(e.do("POST", "/service-desk/teams", s.aTok, map[string]any{"name": "X"}), 403)
	e.must(e.do("POST", "/service-desk/categories", guru, map[string]any{"name": "X"}), 403)
	e.must(e.do("GET", "/service-desk/tickets", guru, nil), 403)
	e.must(e.do("PUT", "/service-desk/business-hours", s.aTok, []any{}), 403)
	e.must(e.do("PUT", "/service-desk/business-hours", s.mTok, []map[string]any{{"weekday": 1, "opens_at": "15:00", "closes_at": "07:00"}}), 422)
	e.must(e.do("POST", "/service-desk/sla-policies", s.mTok, map[string]any{"priority": "normal", "response_value": 1,
		"response_unit": "business_hours", "resolution_value": 1, "resolution_unit": "business_hours", "effective_from": "2026-01-01"}), 422) // warn_bp wajib
	// Guardian must be linked to the student named on a ticket.
	cat := e.category(s, map[string]any{"name": "Umum", "team_id": s.teamID, "reporter_kinds": []string{"orang_tua"}, "active": true})
	parent, student := newID(), newID()
	pTok := token(t, parent, s.id, "orang_tua")
	data, _ := json.Marshal(map[string]any{"category_id": cat, "subject": "a", "description": "b", "student_user_id": student})
	e.must(e.multipart("/me/tickets", pTok, map[string]string{"data": string(data)}, "", "", nil), 403)
	e.f.mu.Lock()
	e.f.guardians[parent+"|"+student] = true
	e.f.mu.Unlock()
	e.must(e.multipart("/me/tickets", pTok, map[string]string{"data": string(data)}, "", "", nil), 201)
	// Assignment only to team members.
	tl := e.must(e.do("GET", "/me/tickets", pTok, nil), 200).list()
	tid := tl[0].(map[string]any)["id"].(string)
	e.must(e.do("POST", "/service-desk/tickets/"+tid+"/assign", s.aTok, map[string]any{"user_id": s.otherAgent}), 422)
	e.must(e.do("POST", "/service-desk/tickets/"+tid+"/status", s.aTok, map[string]any{"to": "rejected"}), 422)
	e.must(e.do("POST", "/service-desk/tickets/"+tid+"/status", s.aTok, map[string]any{"to": "resolved"}), 422)
	e.must(e.do("POST", "/me/tickets/"+tid+"/reopen", pTok, nil), 422)
}
