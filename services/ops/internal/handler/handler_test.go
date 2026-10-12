package handler

import (
	"bytes"
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"skci/ops/internal/client"
	"skci/ops/internal/config"
	"skci/ops/internal/facility"
	"skci/ops/internal/storage"
	"skci/ops/internal/store"
)

// DB-backed handler tests. They need a migrated ops database:
//
//	TEST_PG_DSN=postgres://$USER@localhost:5432/skci_t9_ops?sslmode=disable go test ./...
//
// Identity, core and notification are replaced by one httptest fake that
// follows the written contracts (identity features/users, T1 §3).

const (
	testSecret = "test-jwt-secret"
	testKey    = "test-internal-key"
)

var jkt, _ = time.LoadLocation("Asia/Jakarta")

func uuid() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}

// fake is the identity + core + notification + ticket-callback stand-in.
type fake struct {
	mu            sync.Mutex
	features      map[string]map[string]bool // school → key → on
	technicians   map[string][]client.User   // school → teknisi
	approvalFail  bool
	approvals     []client.ApprovalRequest
	cancelled     []string
	notifications []string // idempotency keys
	callbacks     []string // idempotency keys
	callbackFail  bool
	exceptions    []string
	audits        int
}

func (f *fake) handler(t *testing.T) http.Handler {
	mux := http.NewServeMux()
	writeData := func(w http.ResponseWriter, status int, data any) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(map[string]any{"data": data})
	}
	mux.HandleFunc("GET /schools/{id}/features", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Internal-Key") != testKey {
			t.Errorf("features call without internal key")
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		out := []map[string]any{}
		for k, v := range f.features[r.PathValue("id")] {
			out = append(out, map[string]any{"feature_key": k, "enabled": v})
		}
		writeData(w, 200, out)
	})
	mux.HandleFunc("GET /users", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		if r.URL.Query().Get("role") != "teknisi" {
			writeData(w, 200, []client.User{})
			return
		}
		writeData(w, 200, f.technicians[r.URL.Query().Get("school_id")])
	})
	mux.HandleFunc("POST /internal/approvals", func(w http.ResponseWriter, r *http.Request) {
		var req client.ApprovalRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		f.mu.Lock()
		defer f.mu.Unlock()
		if f.approvalFail {
			w.WriteHeader(422)
			_, _ = w.Write([]byte(`{"message":"alur approval room_booking belum dikonfigurasi"}`))
			return
		}
		f.approvals = append(f.approvals, req)
		writeData(w, 201, map[string]any{"id": uuid(), "status": "pending", "current_seq": 1})
	})
	mux.HandleFunc("POST /internal/approvals/{id}/cancel", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.cancelled = append(f.cancelled, r.PathValue("id"))
		f.mu.Unlock()
		writeData(w, 200, nil)
	})
	mux.HandleFunc("POST /internal/notifications", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.notifications = append(f.notifications, r.Header.Get("Idempotency-Key"))
		f.mu.Unlock()
		writeData(w, 201, nil)
	})
	mux.HandleFunc("POST /internal/audit", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.audits++
		f.mu.Unlock()
		writeData(w, 202, nil)
	})
	mux.HandleFunc("POST /internal/exceptions", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.exceptions = append(f.exceptions, r.Header.Get("Idempotency-Key"))
		f.mu.Unlock()
		writeData(w, 201, nil)
	})
	mux.HandleFunc("POST /ticket-callback", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		f.mu.Lock()
		defer f.mu.Unlock()
		if f.callbackFail {
			w.WriteHeader(500)
			return
		}
		if body["work_order_id"] == nil || body["status"] == nil {
			t.Errorf("callback body missing fields: %v", body)
		}
		f.callbacks = append(f.callbacks, r.Header.Get("Idempotency-Key"))
		writeData(w, 200, nil)
	})
	return mux
}

func (f *fake) count(list *[]string, key string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, k := range *list {
		if k == key {
			n++
		}
	}
	return n
}

type env struct {
	t      *testing.T
	srv    *httptest.Server
	fake   *fake
	app    *App
	db     *sql.DB
	school string
}

func newEnv(t *testing.T) *env {
	t.Helper()
	dsn := os.Getenv("TEST_PG_DSN")
	if dsn == "" {
		t.Skip("TEST_PG_DSN not set")
	}
	db, err := store.Open(dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	f := &fake{features: map[string]map[string]bool{}, technicians: map[string][]client.User{}}
	up := httptest.NewServer(f.handler(t))
	t.Cleanup(up.Close)
	cfg := &config.Config{JWTSecret: testSecret, InternalKey: testKey, CORSOrigins: []string{"*"}, OpsPublicInternalURL: "http://ops"}
	cli := client.New(up.URL, up.URL, up.URL, testKey, time.Millisecond)
	fac := &facility.Handler{Store: facility.NewStore(db, jkt), Client: cli,
		Storage: storage.NewLocal(t.TempDir(), 1<<20, facility.PhotoMIMEs...), MaxUpload: 1 << 20, CallbackBase: "http://ops"}
	app := &App{DB: db, Config: cfg, Client: cli, Facility: fac}
	srv := httptest.NewServer(app.Router())
	t.Cleanup(srv.Close)
	school := uuid()
	f.features[school] = map[string]bool{"facility": true}
	return &env{t: t, srv: srv, fake: f, app: app, db: db, school: school}
}

type user struct {
	id, role, school, name string
}

func (e *env) user(role string) user {
	return user{id: uuid(), role: role, school: e.school, name: "Pengguna " + role}
}

func (u user) token(t *testing.T) string {
	claims := jwt.MapClaims{"sub": u.id, "school_id": u.school, "active_role": u.role, "roles": []string{u.role},
		"name": u.name, "exp": time.Now().Add(time.Hour).Unix()}
	s, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(testSecret))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

type resp struct {
	Status  int
	Data    json.RawMessage
	Meta    json.RawMessage
	Message string
	Raw     []byte
}

func (r resp) into(t *testing.T, v any) {
	t.Helper()
	if err := json.Unmarshal(r.Data, v); err != nil {
		t.Fatalf("decode %s: %v", r.Data, err)
	}
}

func (e *env) send(req *http.Request) resp {
	e.t.Helper()
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		e.t.Fatal(err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	var envl struct {
		Data    json.RawMessage `json:"data"`
		Meta    json.RawMessage `json:"meta"`
		Message string          `json:"message"`
	}
	_ = json.Unmarshal(raw, &envl)
	return resp{Status: res.StatusCode, Data: envl.Data, Meta: envl.Meta, Message: envl.Message, Raw: raw}
}

// call sends a JWT request as u (zero user = no auth).
func (e *env) call(u user, method, path string, body any, headers ...string) resp {
	e.t.Helper()
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, e.srv.URL+path, rd)
	req.Header.Set("Content-Type", "application/json")
	if u.id != "" {
		req.Header.Set("Authorization", "Bearer "+u.token(e.t))
	}
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}
	return e.send(req)
}

// internal sends an X-Internal-Key request.
func (e *env) internal(method, path string, body any) resp {
	e.t.Helper()
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, e.srv.URL+path, rd)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Internal-Key", testKey)
	return e.send(req)
}

func mustStatus(t *testing.T, r resp, want int) {
	t.Helper()
	if r.Status != want {
		t.Fatalf("status %d want %d: %s", r.Status, want, r.Raw)
	}
}

func idOf(t *testing.T, r resp) string {
	t.Helper()
	var v struct {
		ID string `json:"id"`
	}
	r.into(t, &v)
	return v.ID
}

// campus seeds one building/floor with a lab (cap 40, 07–17), an aula in
// maintenance and a fixed-use classroom.
type campus struct {
	building, floor, lab, aula, kelas string
}

func (e *env) seedCampus(officer user) campus {
	t := e.t
	var c campus
	r := e.call(officer, "POST", "/facility/buildings", map[string]any{"code": "A", "name": "Gedung A"})
	mustStatus(t, r, 201)
	c.building = idOf(t, r)
	r = e.call(officer, "POST", "/facility/floors", map[string]any{"building_id": c.building, "level": 1, "name": "Lantai 1"})
	mustStatus(t, r, 201)
	c.floor = idOf(t, r)
	r = e.call(officer, "POST", "/facility/rooms", map[string]any{"floor_id": c.floor, "code": "LAB-1", "name": "Lab Komputer",
		"kind": "lab", "capacity": 40, "usage": "bookable", "available_from": "07:00", "available_to": "17:00"})
	mustStatus(t, r, 201)
	c.lab = idOf(t, r)
	r = e.call(officer, "POST", "/facility/rooms", map[string]any{"floor_id": c.floor, "code": "AULA", "name": "Aula",
		"kind": "aula", "capacity": 300, "usage": "bookable", "status": "maintenance"})
	mustStatus(t, r, 201)
	c.aula = idOf(t, r)
	r = e.call(officer, "POST", "/facility/rooms", map[string]any{"floor_id": c.floor, "code": "7A", "name": "Kelas 7A",
		"kind": "kelas", "capacity": 32, "usage": "fixed"})
	mustStatus(t, r, 201)
	c.kelas = idOf(t, r)
	return c
}

// day is a future Monday so "start in the past" never trips.
const day = "2030-01-07"

func ts(hm string) string {
	t, _ := time.ParseInLocation("2006-01-02 15:04", day+" "+hm, jkt)
	return t.Format(time.RFC3339)
}

// tsq is ts escaped for a query string ("+07:00").
func tsq(hm string) string { return url.QueryEscape(ts(hm)) }

func booking(room, from, to string, participants int) map[string]any {
	return map[string]any{"room_id": room, "starts_at": ts(from), "ends_at": ts(to), "purpose": "Praktik TIK", "participants": participants}
}

// ---------------------------------------------------------------------------

func TestHealthz(t *testing.T) {
	e := newEnv(t)
	r := e.call(user{}, "GET", "/healthz", nil)
	mustStatus(t, r, 200)
}

func TestRequireFeatureOff(t *testing.T) {
	e := newEnv(t)
	e.fake.features[e.school]["facility"] = false
	r := e.call(e.user("guru"), "GET", "/facility/rooms", nil)
	mustStatus(t, r, 403)
	if r.Message != "Modul Fasilitas dinonaktifkan untuk sekolah ini" {
		t.Errorf("message %q", r.Message)
	}
	// Portal roles never reach facility.
	mustStatus(t, e.call(e.user("siswa"), "GET", "/facility/rooms", nil), 403)
}

func TestRoomMasterAndBookableSearch(t *testing.T) {
	e := newEnv(t)
	officer, guru := e.user("facility_officer"), e.user("guru")
	c := e.seedCampus(officer)

	// Guru cannot manage the master.
	mustStatus(t, e.call(guru, "POST", "/facility/buildings", map[string]any{"code": "B", "name": "B"}), 403)
	// Duplicate code → 409.
	mustStatus(t, e.call(officer, "POST", "/facility/buildings", map[string]any{"code": "A", "name": "Lagi"}), 409)
	// Fixed room cannot require approval.
	mustStatus(t, e.call(officer, "POST", "/facility/rooms", map[string]any{"floor_id": c.floor, "code": "X", "name": "X",
		"kind": "kelas", "capacity": 1, "usage": "fixed", "requires_approval": true}), 422)

	// AC-116-01 + FR-116-002: only the active bookable lab shows up.
	r := e.call(guru, "GET", "/facility/rooms?bookable=true", nil)
	mustStatus(t, r, 200)
	var rooms []facility.Room
	r.into(t, &rooms)
	if len(rooms) != 1 || rooms[0].ID != c.lab {
		t.Fatalf("bookable rooms = %+v", rooms)
	}
	if !strings.Contains(string(r.Meta), `"total":1`) {
		t.Errorf("meta %s", r.Meta)
	}
	// All rooms (picker) still lists three.
	r = e.call(guru, "GET", "/facility/rooms", nil)
	r.into(t, &rooms)
	if len(rooms) != 3 {
		t.Errorf("all rooms = %d", len(rooms))
	}
	// Facilities replace.
	mustStatus(t, e.call(officer, "PUT", "/facility/rooms/"+c.lab+"/facilities",
		map[string]any{"facilities": []map[string]any{{"name": "PC", "qty": 40}, {"name": "Proyektor", "qty": 1}}}), 200)
	r = e.call(guru, "GET", "/facility/rooms/"+c.lab, nil)
	var lab facility.Room
	r.into(t, &lab)
	if len(lab.Facilities) != 2 || lab.Building.Name != "Gedung A" {
		t.Errorf("lab = %+v", lab)
	}
	// Floor with rooms cannot be deleted; soft-deleted room disappears.
	mustStatus(t, e.call(officer, "DELETE", "/facility/floors/"+c.floor, nil), 409)
	mustStatus(t, e.call(officer, "DELETE", "/facility/rooms/"+c.kelas, nil), 200)
	mustStatus(t, e.call(guru, "GET", "/facility/rooms/"+c.kelas, nil), 404)
	var deletedAt *time.Time
	_ = e.db.QueryRow(`SELECT deleted_at FROM rooms WHERE id=$1`, c.kelas).Scan(&deletedAt)
	if deletedAt == nil {
		t.Error("room was hard-deleted")
	}

	// Cross-school: another school's guru gets 404.
	other := user{id: uuid(), role: "guru", school: uuid()}
	e.fake.features[other.school] = map[string]bool{"facility": true}
	mustStatus(t, e.call(other, "GET", "/facility/rooms/"+c.lab, nil), 404)

	// Every mutation is audited.
	var n int
	_ = e.db.QueryRow(`SELECT count(*) FROM audit_events WHERE school_id=$1`, e.school).Scan(&n)
	if n < 7 {
		t.Errorf("audit rows = %d", n)
	}
}

func TestBookingConflictCapacityAndNearest(t *testing.T) {
	e := newEnv(t)
	officer, guru, guru2 := e.user("facility_officer"), e.user("guru"), e.user("guru")
	c := e.seedCampus(officer)

	r := e.call(guru, "POST", "/facility/bookings", booking(c.lab, "10:00", "12:00", 30))
	mustStatus(t, r, 201)
	var b facility.Booking
	r.into(t, &b)
	if b.Status != "confirmed" {
		t.Fatalf("status %s", b.Status)
	}

	// AC-117-01: 11–13 refused, nearest free slots offered nearest first.
	r = e.call(guru2, "POST", "/facility/bookings", booking(c.lab, "11:00", "13:00", 30))
	mustStatus(t, r, 409)
	var rej struct {
		Error        string              `json:"error"`
		NearestSlots []facility.Interval `json:"nearest_slots"`
	}
	r.into(t, &rej)
	if len(rej.NearestSlots) != 2 || rej.NearestSlots[0].Start.In(jkt).Format("15:04") != "12:00" ||
		rej.NearestSlots[1].Start.In(jkt).Format("15:04") != "08:00" || !strings.Contains(rej.Error, "Bentrok dengan") {
		t.Fatalf("rejection = %+v", rej)
	}
	// Guru picks 12–14: accepted.
	mustStatus(t, e.call(guru2, "POST", "/facility/bookings", booking(c.lab, "12:00", "14:00", 30)), 201)

	// BR-FAC-01 capacity, outside hours, maintenance room, fixed room.
	mustStatus(t, e.call(guru, "POST", "/facility/bookings", booking(c.lab, "15:00", "16:00", 41)), 422)
	mustStatus(t, e.call(guru, "POST", "/facility/bookings", booking(c.lab, "16:00", "18:00", 10)), 422)
	mustStatus(t, e.call(guru, "POST", "/facility/bookings", booking(c.aula, "10:00", "11:00", 10)), 422)
	mustStatus(t, e.call(guru, "POST", "/facility/bookings", booking(c.kelas, "10:00", "11:00", 10)), 422)

	// Fixed schedule on Mondays 07–08 blocks the slot.
	mustStatus(t, e.call(officer, "POST", "/facility/rooms/"+c.lab+"/fixed-schedules",
		map[string]any{"weekday": 1, "starts_time": "07:00", "ends_time": "08:00", "label": "Praktikum IPA", "valid_from": "2029-07-01"}), 201)
	r = e.call(guru, "POST", "/facility/bookings", booking(c.lab, "07:30", "08:30", 10))
	mustStatus(t, r, 409)
	if !strings.Contains(r.Message, "Praktikum IPA") {
		t.Errorf("message %q", r.Message)
	}

	// Search with period hides the busy lab (FR-117-002 at search time).
	r = e.call(guru, "GET", "/facility/rooms?bookable=true&from="+tsq("10:30")+"&to="+tsq("11:00"), nil)
	var rooms []facility.Room
	r.into(t, &rooms)
	if len(rooms) != 0 {
		t.Errorf("busy lab still listed: %+v", rooms)
	}

	// FR-117-004 calendar shows bookings + fixed schedule.
	r = e.call(guru, "GET", "/facility/rooms/"+c.lab+"/calendar?from="+tsq("00:00")+"&to="+tsq("23:59"), nil)
	mustStatus(t, r, 200)
	var cal struct {
		Busy []facility.BusySlot `json:"busy"`
	}
	r.into(t, &cal)
	if len(cal.Busy) != 3 {
		t.Errorf("calendar busy = %d", len(cal.Busy))
	}

	// Mine + cancel own; another guru cannot cancel it; officer needs reason.
	r = e.call(guru, "GET", "/facility/bookings/mine", nil)
	var mine []facility.Booking
	r.into(t, &mine)
	if len(mine) != 1 {
		t.Fatalf("mine = %d", len(mine))
	}
	mustStatus(t, e.call(guru2, "POST", "/facility/bookings/"+b.ID+"/cancel", map[string]any{"reason": "x"}), 403)
	mustStatus(t, e.call(officer, "POST", "/facility/bookings/"+b.ID+"/cancel", map[string]any{}), 422)
	mustStatus(t, e.call(guru, "POST", "/facility/bookings/"+b.ID+"/cancel", map[string]any{"reason": "batal"}), 200)
	// Freed slot can be booked again.
	mustStatus(t, e.call(guru2, "POST", "/facility/bookings", booking(c.lab, "10:00", "12:00", 5)), 201)

	// Admin listing for officer/kepsek; guru forbidden.
	mustStatus(t, e.call(e.user("kepala_sekolah"), "GET", "/facility/bookings", nil), 200)
	mustStatus(t, e.call(guru, "GET", "/facility/bookings", nil), 403)
}

func TestBookingRaceAndIdempotency(t *testing.T) {
	e := newEnv(t)
	officer := e.user("facility_officer")
	c := e.seedCampus(officer)

	// Same Idempotency-Key → same booking, 200 on replay.
	g := e.user("guru")
	r1 := e.call(g, "POST", "/facility/bookings", booking(c.lab, "08:00", "09:00", 5), "Idempotency-Key", "k1")
	r2 := e.call(g, "POST", "/facility/bookings", booking(c.lab, "08:00", "09:00", 5), "Idempotency-Key", "k1")
	mustStatus(t, r1, 201)
	mustStatus(t, r2, 200)
	if idOf(t, r1) != idOf(t, r2) {
		t.Fatal("replay created a second booking")
	}

	// Two parallel bookings of the same slot → exactly one wins.
	var wg sync.WaitGroup
	statuses := make([]int, 6)
	for i := range statuses {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			statuses[i] = e.call(e.user("guru"), "POST", "/facility/bookings", booking(c.lab, "13:00", "15:00", 5)).Status
		}(i)
	}
	wg.Wait()
	sort.Ints(statuses)
	if statuses[0] != 201 || statuses[1] != 409 || statuses[5] != 409 {
		t.Fatalf("parallel statuses = %v", statuses)
	}
}

func TestBookingApprovalFlow(t *testing.T) {
	e := newEnv(t)
	officer, guru := e.user("facility_officer"), e.user("guru")
	c := e.seedCampus(officer)
	r := e.call(officer, "POST", "/facility/rooms", map[string]any{"floor_id": c.floor, "code": "AULA2", "name": "Aula Utama",
		"kind": "aula", "capacity": 200, "usage": "bookable", "requires_approval": true})
	mustStatus(t, r, 201)
	aula := idOf(t, r)

	// Core without a configured flow → booking fails with core's message.
	e.fake.approvalFail = true
	r = e.call(guru, "POST", "/facility/bookings", booking(aula, "09:00", "10:00", 50))
	mustStatus(t, r, 422)
	if !strings.Contains(r.Message, "belum dikonfigurasi") {
		t.Errorf("message %q", r.Message)
	}
	var n int
	_ = e.db.QueryRow(`SELECT count(*) FROM room_bookings WHERE room_id=$1`, aula).Scan(&n)
	if n != 0 {
		t.Fatalf("booking persisted despite core 422")
	}
	e.fake.approvalFail = false

	// FR-117-003: pending + core room_booking request.
	r = e.call(guru, "POST", "/facility/bookings", booking(aula, "09:00", "10:00", 50))
	mustStatus(t, r, 201)
	var b facility.Booking
	r.into(t, &b)
	if b.Status != "pending_approval" || b.ApprovalID == nil {
		t.Fatalf("booking %+v", b)
	}
	ap := e.fake.approvals[len(e.fake.approvals)-1]
	if ap.RequestType != "room_booking" || ap.SourceService != "ops" || ap.SourceID != b.ID ||
		ap.IdempotencyKey != "room_booking:"+b.ID || ap.CallbackURL != "http://ops/internal/approval-callbacks" || ap.MakerUserID != guru.id {
		t.Errorf("approval request %+v", ap)
	}
	// Pending slot blocks others.
	mustStatus(t, e.call(e.user("guru"), "POST", "/facility/bookings", booking(aula, "09:30", "10:30", 10)), 409)

	// Callback approved twice → confirmed once, one notification.
	cb := map[string]any{"approval_id": *b.ApprovalID, "source_id": b.ID, "request_type": "room_booking", "decision": "approved"}
	mustStatus(t, e.internal("POST", "/internal/approval-callbacks", cb), 200)
	mustStatus(t, e.internal("POST", "/internal/approval-callbacks", cb), 200)
	got, _ := e.app.Facility.Store.GetBooking(context.Background(), e.school, b.ID)
	if got.Status != "confirmed" {
		t.Fatalf("status after approve = %s", got.Status)
	}
	if n := e.fake.count(&e.fake.notifications, "booking:"+b.ID+":confirmed"); n != 1 {
		t.Errorf("notifications = %d", n)
	}

	// Rejected → slot freed.
	r = e.call(guru, "POST", "/facility/bookings", booking(aula, "11:00", "12:00", 50))
	r.into(t, &b)
	cb = map[string]any{"approval_id": *b.ApprovalID, "source_id": b.ID, "request_type": "room_booking", "decision": "rejected"}
	mustStatus(t, e.internal("POST", "/internal/approval-callbacks", cb), 200)
	mustStatus(t, e.internal("POST", "/internal/approval-callbacks", cb), 200)
	got, _ = e.app.Facility.Store.GetBooking(context.Background(), e.school, b.ID)
	if got.Status != "rejected" {
		t.Fatalf("status after reject = %s", got.Status)
	}
	mustStatus(t, e.call(e.user("guru"), "POST", "/facility/bookings", booking(aula, "11:00", "12:00", 10)), 201)

	// Cancel while pending withdraws the core approval.
	r = e.call(guru, "POST", "/facility/bookings", booking(aula, "14:00", "15:00", 50))
	r.into(t, &b)
	mustStatus(t, e.call(guru, "POST", "/facility/bookings/"+b.ID+"/cancel", map[string]any{"reason": "tidak jadi"}), 200)
	if e.fake.count(&e.fake.cancelled, *b.ApprovalID) != 1 {
		t.Error("core approval not cancelled")
	}

	// Callback without internal key / unknown type.
	req, _ := http.NewRequest("POST", e.srv.URL+"/internal/approval-callbacks", strings.NewReader(`{}`))
	mustStatus(t, e.send(req), 401)
	mustStatus(t, e.internal("POST", "/internal/approval-callbacks", map[string]any{"source_id": b.ID, "request_type": "x", "decision": "approved"}), 400)
}

func pngBytes() []byte {
	return []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR\x00\x00\x00\x01\x00\x00\x00\x01\x08\x06\x00\x00\x00\x1f\x15\xc4\x89")
}

func (e *env) report(u user, fields map[string]string, photo []byte) resp {
	e.t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	for k, v := range fields {
		_ = mw.WriteField(k, v)
	}
	if photo != nil {
		fw, _ := mw.CreateFormFile("photos", "foto.png")
		_, _ = fw.Write(photo)
	}
	_ = mw.Close()
	req, _ := http.NewRequest("POST", e.srv.URL+"/facility/work-orders", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+u.token(e.t))
	return e.send(req)
}

func TestWorkOrderLifecycle(t *testing.T) {
	e := newEnv(t)
	officer, guru := e.user("facility_officer"), e.user("guru")
	tech, otherTech := e.user("teknisi"), e.user("teknisi")
	e.fake.technicians[e.school] = []client.User{{ID: tech.id, Name: "Teknisi Satu"}, {ID: otherTech.id, Name: "Teknisi Dua"}}
	c := e.seedCampus(officer)

	// FR-118-001: report with photo + priority.
	r := e.report(guru, map[string]string{"room_id": c.lab, "title": "AC rusak", "priority": "tinggi", "description": "AC tidak dingin"}, pngBytes())
	mustStatus(t, r, 201)
	var wo facility.WorkOrder
	r.into(t, &wo)
	if wo.Status != "dilaporkan" || !strings.HasPrefix(wo.Code, "WO-") || len(wo.Photos) != 1 || len(wo.Photos[0].SHA256) != 64 {
		t.Fatalf("wo %+v", wo)
	}
	// Non-image upload rejected; missing target rejected.
	mustStatus(t, e.report(guru, map[string]string{"room_id": c.lab, "title": "x", "priority": "rendah"}, []byte("plain text, not an image")), 422)
	mustStatus(t, e.report(guru, map[string]string{"title": "x", "priority": "rendah"}, nil), 422)
	// Hook nil: asset rejected.
	r = e.report(guru, map[string]string{"asset_id": uuid(), "title": "x", "priority": "rendah"}, nil)
	mustStatus(t, r, 422)
	if r.Message != "Modul aset belum aktif" {
		t.Errorf("asset message %q", r.Message)
	}

	// Photo through authenticated endpoint.
	mustStatus(t, e.call(guru, "GET", "/facility/work-orders/"+wo.ID+"/photos/"+wo.Photos[0].ID, nil), 200)
	mustStatus(t, e.call(otherTech, "GET", "/facility/work-orders/"+wo.ID+"/photos/"+wo.Photos[0].ID, nil), 403)

	// Teknisi cannot assign; assignee must be a teknisi of this school.
	mustStatus(t, e.call(tech, "POST", "/facility/work-orders/"+wo.ID+"/assign", map[string]any{"assignee_user_id": tech.id}), 403)
	mustStatus(t, e.call(officer, "POST", "/facility/work-orders/"+wo.ID+"/assign", map[string]any{"assignee_user_id": guru.id}), 422)
	r = e.call(officer, "POST", "/facility/work-orders/"+wo.ID+"/assign", map[string]any{"assignee_user_id": tech.id})
	mustStatus(t, r, 200)
	r.into(t, &wo)
	if wo.Status != "ditugaskan" || wo.AssigneeName == nil || *wo.AssigneeName != "Teknisi Satu" {
		t.Fatalf("assigned %+v", wo)
	}
	if e.fake.count(&e.fake.notifications, "wo:"+wo.ID+":assigned:"+tech.id) != 1 {
		t.Error("assignee not notified")
	}

	// Teknisi lists only their WOs; other teknisi cannot touch this one.
	r = e.call(otherTech, "GET", "/facility/work-orders", nil)
	var list []facility.WorkOrder
	r.into(t, &list)
	if len(list) != 0 {
		t.Errorf("other teknisi sees %d", len(list))
	}
	mustStatus(t, e.call(otherTech, "POST", "/facility/work-orders/"+wo.ID+"/status", map[string]any{"to": "diproses"}), 403)
	mustStatus(t, e.call(otherTech, "GET", "/facility/work-orders/"+wo.ID, nil), 403)

	// Invalid jump → 422; valid flow.
	mustStatus(t, e.call(tech, "POST", "/facility/work-orders/"+wo.ID+"/status", map[string]any{"to": "selesai", "note": "x"}), 422)
	mustStatus(t, e.call(tech, "POST", "/facility/work-orders/"+wo.ID+"/status", map[string]any{"to": "diproses"}), 200)

	// FR-118-004 costs and total.
	mustStatus(t, e.call(tech, "POST", "/facility/work-orders/"+wo.ID+"/costs", map[string]any{"kind": "material", "description": "Freon", "amount_idr": 150000}), 201)
	r = e.call(officer, "POST", "/facility/work-orders/"+wo.ID+"/costs", map[string]any{"kind": "jasa", "description": "Servis", "amount_idr": 75000})
	mustStatus(t, r, 201)
	mustStatus(t, e.call(otherTech, "POST", "/facility/work-orders/"+wo.ID+"/costs", map[string]any{"kind": "jasa", "description": "x", "amount_idr": 1}), 403)
	// Hook nil: purchase request → 409.
	r = e.call(officer, "POST", "/facility/work-orders/"+wo.ID+"/purchase-request", map[string]any{"lines": []map[string]any{{"description": "Freon", "qty": 1, "est_amount_idr": 100000}}})
	mustStatus(t, r, 409)
	if r.Message != "Modul pengadaan belum aktif" {
		t.Errorf("purchase message %q", r.Message)
	}

	// Selesai requires a completion note.
	mustStatus(t, e.call(tech, "POST", "/facility/work-orders/"+wo.ID+"/status", map[string]any{"to": "selesai"}), 422)
	mustStatus(t, e.call(tech, "POST", "/facility/work-orders/"+wo.ID+"/status", map[string]any{"to": "selesai", "note": "Freon diisi ulang"}), 200)
	if e.fake.count(&e.fake.notifications, "wo:"+wo.ID+":selesai") != 1 {
		t.Error("reporter not notified")
	}

	// Reporter sees status + total, then closes.
	r = e.call(guru, "GET", "/facility/work-orders/"+wo.ID, nil)
	mustStatus(t, r, 200)
	r.into(t, &wo)
	if wo.Status != "selesai" || wo.TotalCost == nil || *wo.TotalCost != 225000 || wo.CompletionNote == nil || len(wo.Events) != 4 {
		t.Fatalf("detail %+v", wo)
	}
	mustStatus(t, e.call(guru, "POST", "/facility/work-orders/"+wo.ID+"/close", map[string]any{}), 200)
	r = e.call(guru, "GET", "/facility/work-orders/mine", nil)
	r.into(t, &list)
	if len(list) != 1 || list[0].Status != "ditutup" {
		t.Errorf("mine %+v", list)
	}

	// Cross-school 404.
	other := user{id: uuid(), role: "facility_officer", school: uuid()}
	e.fake.features[other.school] = map[string]bool{"facility": true}
	mustStatus(t, e.call(other, "GET", "/facility/work-orders/"+wo.ID, nil), 404)
}

func TestPreventiveMaintenanceJob(t *testing.T) {
	e := newEnv(t)
	officer, tech := e.user("facility_officer"), e.user("teknisi")
	e.fake.technicians[e.school] = []client.User{{ID: tech.id, Name: "Teknisi"}}
	c := e.seedCampus(officer)
	today := time.Now().In(jkt).Format("2006-01-02")

	// AC-118-01: AC ruang guru every 3 months, due today, default teknisi.
	r := e.call(officer, "POST", "/facility/maintenance-schedules", map[string]any{"room_id": c.lab, "title": "Servis AC",
		"priority": "sedang", "interval_unit": "month", "interval_count": 3, "next_due_date": today,
		"default_assignee_user_id": tech.id, "active": true})
	mustStatus(t, r, 201)
	sid := idOf(t, r)
	// Asset-based schedule without asset hook → 422.
	mustStatus(t, e.call(officer, "POST", "/facility/maintenance-schedules", map[string]any{"asset_id": uuid(), "title": "x",
		"interval_unit": "month", "interval_count": 1, "next_due_date": today, "active": true}), 422)

	for range 2 {
		if err := e.app.Facility.RunPreventive(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	var n int
	var status, assignee string
	_ = e.db.QueryRow(`SELECT count(*), max(status), max(assignee_user_id::text) FROM work_orders WHERE schedule_id=$1`, sid).Scan(&n, &status, &assignee)
	if n != 1 || status != "ditugaskan" || assignee != tech.id {
		t.Fatalf("preventive WOs n=%d status=%s assignee=%s", n, status, assignee)
	}
	var next string
	_ = e.db.QueryRow(`SELECT to_char(next_due_date,'YYYY-MM-DD') FROM maintenance_schedules WHERE id=$1`, sid).Scan(&next)
	todayT, _ := time.Parse("2006-01-02", today)
	if want := facility.NextDue(todayT, "month", 3).Format("2006-01-02"); next != want {
		t.Errorf("next due %s want %s", next, want)
	}
	var woID string
	_ = e.db.QueryRow(`SELECT id FROM work_orders WHERE schedule_id=$1`, sid).Scan(&woID)
	if e.fake.count(&e.fake.notifications, "wo:"+woID+":assigned:"+tech.id) != 1 {
		t.Error("teknisi not notified")
	}

	// Overdue by two periods catches up one period per run.
	r = e.call(officer, "POST", "/facility/maintenance-schedules", map[string]any{"room_id": c.lab, "title": "Servis Genset",
		"interval_unit": "month", "interval_count": 1, "next_due_date": todayT.AddDate(0, -2, 0).Format("2006-01-02"), "active": true})
	sid2 := idOf(t, r)
	_ = e.app.Facility.RunPreventive(context.Background())
	_ = e.db.QueryRow(`SELECT count(*) FROM work_orders WHERE schedule_id=$1`, sid2).Scan(&n)
	if n != 1 {
		t.Errorf("overdue run created %d WOs", n)
	}
	_ = e.db.QueryRow(`SELECT status FROM work_orders WHERE schedule_id=$1`, sid2).Scan(&status)
	if status != "dilaporkan" {
		t.Errorf("unassigned preventive status = %s", status)
	}
}

func keysOf(t *testing.T, raw json.RawMessage) []string {
	t.Helper()
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("decode %s: %v", raw, err)
	}
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func sameKeys(t *testing.T, raw json.RawMessage, want ...string) {
	t.Helper()
	sort.Strings(want)
	if got := keysOf(t, raw); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("keys %v want %v", got, want)
	}
}

func TestInternalContracts(t *testing.T) {
	e := newEnv(t)
	officer := e.user("facility_officer")
	c := e.seedCampus(officer)
	mustStatus(t, e.call(officer, "PUT", "/facility/rooms/"+c.lab+"/facilities",
		map[string]any{"facilities": []map[string]any{{"name": "PC", "qty": 40}}}), 200)
	q := "?school_id=" + e.school

	// Auth + mandatory school_id.
	req, _ := http.NewRequest("GET", e.srv.URL+"/internal/facility/rooms"+q, nil)
	mustStatus(t, e.send(req), 401)
	mustStatus(t, e.internal("GET", "/internal/facility/rooms", nil), 400)

	// GET buildings.
	r := e.internal("GET", "/internal/facility/buildings"+q, nil)
	mustStatus(t, r, 200)
	var bs []json.RawMessage
	r.into(t, &bs)
	if len(bs) != 1 {
		t.Fatalf("buildings %d", len(bs))
	}
	sameKeys(t, bs[0], "id", "code", "name", "floors")
	var b struct {
		Floors []json.RawMessage `json:"floors"`
	}
	_ = json.Unmarshal(bs[0], &b)
	sameKeys(t, b.Floors[0], "id", "level", "name")

	// GET rooms (shape exact) + filters.
	r = e.internal("GET", "/internal/facility/rooms"+q+"&bookable=true", nil)
	var rooms []json.RawMessage
	r.into(t, &rooms)
	if len(rooms) != 1 {
		t.Fatalf("bookable rooms %d", len(rooms))
	}
	sameKeys(t, rooms[0], "id", "code", "name", "kind", "capacity", "usage", "status", "requires_approval", "building", "floor", "facilities")
	var room struct {
		Building   json.RawMessage   `json:"building"`
		Floor      json.RawMessage   `json:"floor"`
		Facilities []json.RawMessage `json:"facilities"`
	}
	_ = json.Unmarshal(rooms[0], &room)
	sameKeys(t, room.Building, "id", "name")
	sameKeys(t, room.Floor, "id", "level", "name")
	sameKeys(t, room.Facilities[0], "name", "qty")
	r = e.internal("GET", "/internal/facility/rooms"+q+"&ids="+c.aula+","+c.kelas, nil)
	r.into(t, &rooms)
	if len(rooms) != 2 {
		t.Errorf("ids filter %d", len(rooms))
	}

	// GET rooms/{id}: other school → 404.
	mustStatus(t, e.internal("GET", "/internal/facility/rooms/"+c.lab+q, nil), 200)
	mustStatus(t, e.internal("GET", "/internal/facility/rooms/"+c.lab+"?school_id="+uuid(), nil), 404)
	r = e.internal("GET", "/internal/facility/rooms?school_id="+uuid(), nil)
	r.into(t, &rooms)
	if len(rooms) != 0 {
		t.Error("cross-school rooms leaked")
	}

	// POST bookings: 201, replay 200, conflict 409 with nearest, capacity 422.
	body := booking(c.lab, "10:00", "12:00", 30)
	body["school_id"], body["requester_user_id"], body["source_service"], body["source_ref"], body["idempotency_key"] =
		e.school, uuid(), "studentlife", "event-1", "event-1"
	r = e.internal("POST", "/internal/facility/bookings", body)
	mustStatus(t, r, 201)
	sameKeys(t, r.Data, "id", "status")
	bookingID := idOf(t, r)
	r = e.internal("POST", "/internal/facility/bookings", body)
	mustStatus(t, r, 200)
	if idOf(t, r) != bookingID {
		t.Error("replay returned another booking")
	}
	body2 := booking(c.lab, "11:00", "13:00", 30)
	body2["school_id"], body2["requester_user_id"], body2["source_service"], body2["idempotency_key"] = e.school, uuid(), "studentlife", "event-2"
	r = e.internal("POST", "/internal/facility/bookings", body2)
	mustStatus(t, r, 409)
	var rej map[string]json.RawMessage
	r.into(t, &rej)
	if rej["error"] == nil || rej["nearest_slots"] == nil {
		t.Errorf("409 body %s", r.Data)
	}
	body2["participants"], body2["starts_at"], body2["ends_at"] = 500, ts("14:00"), ts("15:00")
	mustStatus(t, e.internal("POST", "/internal/facility/bookings", body2), 422)

	// Availability.
	r = e.internal("GET", "/internal/facility/availability"+q+"&room_ids="+c.lab+"&from="+tsq("00:00")+"&to="+tsq("23:59"), nil)
	mustStatus(t, r, 200)
	var av []json.RawMessage
	r.into(t, &av)
	if len(av) != 1 {
		t.Fatalf("availability %s", r.Data)
	}
	sameKeys(t, av[0], "room_id", "busy")
	var avb struct {
		Busy []json.RawMessage `json:"busy"`
	}
	_ = json.Unmarshal(av[0], &avb)
	if len(avb.Busy) != 1 {
		t.Fatalf("busy %d", len(avb.Busy))
	}
	// booked_by_name is optional ("?") and absent for internal bookings
	// that carry no requester name.
	sameKeys(t, avb.Busy[0], "starts_at", "ends_at", "kind", "label", "booking_id")
	r = e.internal("GET", "/internal/facility/availability?school_id="+uuid()+"&room_ids="+c.lab+"&from="+tsq("00:00")+"&to="+tsq("23:59"), nil)
	r.into(t, &av)
	if len(av) != 0 {
		t.Error("cross-school availability leaked")
	}

	// Cancel booking.
	mustStatus(t, e.internal("POST", "/internal/facility/bookings/"+bookingID+"/cancel", map[string]any{"school_id": uuid(), "reason": "x"}), 404)
	mustStatus(t, e.internal("POST", "/internal/facility/bookings/"+bookingID+"/cancel", map[string]any{"school_id": e.school, "actor_user_id": uuid(), "reason": "event batal"}), 200)

	// POST work-orders (ticket) + idempotent + list + callback on change.
	wob := map[string]any{"school_id": e.school, "room_id": c.lab, "title": "AC kelas 8A mati", "description": "dari tiket",
		"priority": "tinggi", "reporter_user_id": uuid(), "ticket_ref": "TKT-1", "callback_url": e.fakeURL() + "/ticket-callback",
		"idempotency_key": "TKT-1"}
	r = e.internal("POST", "/internal/facility/work-orders", wob)
	mustStatus(t, r, 201)
	sameKeys(t, r.Data, "id", "code", "status")
	woID := idOf(t, r)
	r = e.internal("POST", "/internal/facility/work-orders", wob)
	mustStatus(t, r, 200)
	if idOf(t, r) != woID {
		t.Error("ticket replay created a second WO")
	}
	r = e.internal("GET", "/internal/facility/work-orders"+q+"&status=active&ticket_ref=TKT-1", nil)
	var wos []json.RawMessage
	r.into(t, &wos)
	if len(wos) != 1 {
		t.Fatalf("active WOs %d", len(wos))
	}
	sameKeys(t, wos[0], "id", "code", "room_id", "asset_id", "title", "priority", "status", "assignee_name", "updated_at")
	r = e.internal("GET", "/internal/facility/work-orders?school_id="+uuid()+"&status=active", nil)
	r.into(t, &wos)
	if len(wos) != 0 {
		t.Error("cross-school WOs leaked")
	}

	tech := e.user("teknisi")
	e.fake.technicians[e.school] = []client.User{{ID: tech.id, Name: "T"}}
	mustStatus(t, e.call(officer, "POST", "/facility/work-orders/"+woID+"/assign", map[string]any{"assignee_user_id": tech.id}), 200)
	if e.fake.count(&e.fake.callbacks, "wo:"+woID+":ditugaskan") != 1 {
		t.Error("ticket callback not sent")
	}
	// Failing callback lands in core exceptions.
	e.fake.callbackFail = true
	mustStatus(t, e.call(tech, "POST", "/facility/work-orders/"+woID+"/status", map[string]any{"to": "diproses"}), 200)
	if e.fake.count(&e.fake.exceptions, "callback:wo:"+woID+":diproses") != 1 {
		t.Error("failed callback not recorded as exception")
	}

	// Feature off → internal mutations refused.
	e.fake.features[e.school]["facility"] = false
	wob["idempotency_key"] = "TKT-2"
	mustStatus(t, e.internal("POST", "/internal/facility/work-orders", wob), 403)
}

func (e *env) fakeURL() string { return e.app.Client.CoreURL }

func TestForwardAudit(t *testing.T) {
	e := newEnv(t)
	e.seedCampus(e.user("facility_officer"))
	if err := e.app.ForwardAudit(context.Background(), e.db); err != nil {
		t.Fatal(err)
	}
	var n int
	_ = e.db.QueryRow(`SELECT count(*) FROM audit_events WHERE school_id=$1 AND forwarded_at IS NULL`, e.school).Scan(&n)
	if n != 0 || e.fake.audits == 0 {
		t.Errorf("pending=%d forwarded=%d", n, e.fake.audits)
	}
}
