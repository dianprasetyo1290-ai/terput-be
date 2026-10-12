package campaign

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/golang-jwt/jwt/v5"
	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	_ "github.com/jackc/pgx/v5/stdlib"

	"skci/notification/internal/common"
	"skci/notification/internal/provider"
	"skci/notification/internal/store"
)

const testSecret = "campaign-test-secret"

// testDB opens TEST_PG_DSN (skips when unset) and applies every migration.
func testDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("TEST_PG_DSN")
	if dsn == "" {
		t.Skip("TEST_PG_DSN not set")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	driver, err := postgres.WithInstance(db, &postgres.Config{})
	if err != nil {
		t.Fatalf("migrate driver: %v", err)
	}
	m, err := migrate.NewWithDatabaseInstance("file://../../migrations", "postgres", driver)
	if err != nil {
		t.Fatalf("migrate init: %v", err)
	}
	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		t.Fatalf("migrate up: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func uuid(t *testing.T, db *sql.DB) string {
	t.Helper()
	var id string
	if err := db.QueryRow(`SELECT gen_random_uuid()`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

type fakeSource struct{ cands []Candidate }

func (f fakeSource) Resolve(context.Context, string, Segment, string) ([]Candidate, error) {
	return f.cands, nil
}

func tok(t *testing.T, user, school, role string) string {
	t.Helper()
	s, err := jwt.NewWithClaims(jwt.SigningMethodHS256, common.Claims{
		UserID: user, SchoolID: school, ActiveRole: role, Roles: []string{role},
		RegisteredClaims: jwt.RegisteredClaims{ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour))},
	}).SignedString([]byte(testSecret))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func newServer(svc *Service) http.Handler {
	r := chi.NewRouter()
	h := NewHandler(svc)
	r.Group(func(pr chi.Router) {
		pr.Use(common.JWTMiddleware(testSecret))
		h.Mount(pr)
	})
	r.Get("/public/unsubscribe", h.PublicUnsubscribe)
	r.Post("/webhooks/whatsapp", h.WhatsAppWebhook)
	return r
}

func call(router http.Handler, method, path, bearer, body string) *httptest.ResponseRecorder {
	var req *http.Request
	if body != "" {
		req = httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
	} else {
		req = httptest.NewRequest(method, path, nil)
	}
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func dataID(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var env struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil || env.Data.ID == "" {
		t.Fatalf("no id in %s", rec.Body.String())
	}
	return env.Data.ID
}

func baseService(db *sql.DB, src SegmentSource) *Service {
	loc, _ := time.LoadLocation("Asia/Jakarta")
	return &Service{
		Store: NewStore(db), InApp: store.New(db), Source: src, Email: provider.NoopProvider{},
		UnsubscribeSecret: "unsub", PublicURL: "http://notif.test", Loc: loc,
	}
}

// AC-147-01: 500 parents, 20 opted out → 480 sent rows, 20 excluded/opted_out.
func TestAC14701SendNowExcludesOptedOut(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	school, humas := uuid(t, db), uuid(t, db)
	cands := make([]Candidate, 500)
	st := NewStore(db)
	for i := range cands {
		uid := uuid(t, db)
		cands[i] = Candidate{SubjectKey: UserSubject(uid), UserID: uid, Name: fmt.Sprintf("Wali %d", i), Contact: fmt.Sprintf("wali%d@mail.test", i)}
		status := ConsentOptedIn
		if i < 20 {
			status = ConsentOptedOut
		}
		if err := st.SetConsent(ctx, school, cands[i].SubjectKey, uid, ChannelEmail, status, SourcePreferences, uid); err != nil {
			t.Fatal(err)
		}
	}
	router := newServer(baseService(db, fakeSource{cands}))
	bearer := tok(t, humas, school, "humas")

	rec := call(router, http.MethodPost, "/campaign-segments", bearer, `{"name":"Ortu kelas 9","kind":"parents","filter":{"grade_levels":["9"]}}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("segment: %d %s", rec.Code, rec.Body.String())
	}
	segID := dataID(t, rec)

	rec = call(router, http.MethodGet, "/campaign-segments/"+segID+"/preview?channel=email", bearer, "")
	if !strings.Contains(rec.Body.String(), `"eligible":480`) || !strings.Contains(rec.Body.String(), `"opted_out":20`) {
		t.Fatalf("preview: %s", rec.Body.String())
	}

	rec = call(router, http.MethodPost, "/campaigns", bearer, `{"name":"Open house","segment_id":"`+segID+`","channel":"email","subject":"Open house","body":"Datang ya"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("campaign: %d %s", rec.Code, rec.Body.String())
	}
	campID := dataID(t, rec)

	rec = call(router, http.MethodPost, "/campaigns/"+campID+"/send-now", bearer, "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"status":"sent"`) {
		t.Fatalf("send-now: %d %s", rec.Code, rec.Body.String())
	}
	var sent, excluded int
	_ = db.QueryRow(`SELECT count(*) FILTER (WHERE status='sent'), count(*) FILTER (WHERE status='excluded' AND exclude_reason='opted_out')
		FROM campaign_recipients WHERE campaign_id=$1`, campID).Scan(&sent, &excluded)
	if sent != 480 || excluded != 20 {
		t.Fatalf("ledger sent=%d excluded=%d, want 480/20", sent, excluded)
	}
	var raw int
	_ = db.QueryRow(`SELECT count(*) FROM campaign_recipients WHERE campaign_id=$1 AND contact_masked LIKE '%@mail.test' AND contact_masked NOT LIKE '%***%'`, campID).Scan(&raw)
	if raw != 0 {
		t.Fatalf("%d recipients stored an unmasked contact", raw)
	}

	rec = call(router, http.MethodGet, "/campaigns/"+campID+"/stats", bearer, "")
	if !strings.Contains(rec.Body.String(), `"sent":480`) || !strings.Contains(rec.Body.String(), `"excluded":20`) {
		t.Fatalf("stats: %s", rec.Body.String())
	}

	// Re-dispatching a finished campaign is refused; the ledger is unchanged.
	if err := baseService(db, fakeSource{cands}).Dispatch(ctx, campID); !errors.Is(err, ErrNotDispatchable) {
		t.Fatalf("second dispatch err = %v", err)
	}
}

func TestWhatsAppDisabledReturns409AndWebhook404(t *testing.T) {
	db := testDB(t)
	school, humas := uuid(t, db), uuid(t, db)
	router := newServer(baseService(db, fakeSource{}))
	bearer := tok(t, humas, school, "humas")
	segID := dataID(t, call(router, http.MethodPost, "/campaign-segments", bearer, `{"name":"S","kind":"parents"}`))
	rec := call(router, http.MethodPost, "/campaigns", bearer, `{"name":"WA","segment_id":"`+segID+`","channel":"whatsapp","template_id":"x"}`)
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "Kanal WhatsApp belum dikonfigurasi") {
		t.Fatalf("whatsapp disabled: %d %s", rec.Code, rec.Body.String())
	}
	if rec := call(router, http.MethodPost, "/webhooks/whatsapp", "", `{}`); rec.Code != http.StatusNotFound {
		t.Fatalf("webhook disabled: %d", rec.Code)
	}
}

func TestWhatsAppNoopTemplateFlowAndStopKeyword(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	school, humas, parent := uuid(t, db), uuid(t, db), uuid(t, db)
	svc := baseService(db, fakeSource{[]Candidate{{SubjectKey: UserSubject(parent), UserID: parent, Name: "Ortu", Contact: "081234567890"}}})
	wa, _ := provider.NewWhatsApp("noop", "hook", []string{"STOP"})
	svc.WhatsApp = wa
	router := newServer(svc)
	bearer := tok(t, humas, school, "humas")
	if err := svc.Store.SetConsent(ctx, school, UserSubject(parent), parent, ChannelWhatsApp, ConsentOptedIn, SourcePreferences, parent); err != nil {
		t.Fatal(err)
	}
	segID := dataID(t, call(router, http.MethodPost, "/campaign-segments", bearer, `{"name":"S","kind":"parents"}`))
	tplID := dataID(t, call(router, http.MethodPost, "/wa-templates", bearer, `{"name":"PPDB","provider_template_id":"ppdb_v1","language":"id","body_preview":"Halo {{nama}}","variables":["nama"],"approval_status":"pending"}`))
	body := `{"name":"WA","segment_id":"` + segID + `","channel":"whatsapp","template_id":"` + tplID + `","template_vars":{"nama":"Bapak/Ibu"}}`
	if rec := call(router, http.MethodPost, "/campaigns", bearer, body); rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("pending template must be rejected: %d %s", rec.Code, rec.Body.String())
	}
	call(router, http.MethodPut, "/wa-templates/"+tplID, bearer, `{"name":"PPDB","provider_template_id":"ppdb_v1","language":"id","body_preview":"Halo {{nama}}","variables":["nama"],"approval_status":"approved"}`)
	campID := dataID(t, call(router, http.MethodPost, "/campaigns", bearer, body))
	if rec := call(router, http.MethodPost, "/campaigns/"+campID+"/send-now", bearer, ""); rec.Code != http.StatusOK {
		t.Fatalf("send-now: %d %s", rec.Code, rec.Body.String())
	}
	var msgID string
	if err := db.QueryRow(`SELECT provider_message_id FROM campaign_recipients WHERE campaign_id=$1 AND status='sent'`, campID).Scan(&msgID); err != nil {
		t.Fatalf("sent row: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/webhooks/whatsapp", strings.NewReader(`{"events":[{"provider_message_id":"`+msgID+`","status":"replied","text":"stop"}]}`))
	req.Header.Set("X-Webhook-Secret", "hook")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("webhook: %d %s", rec.Code, rec.Body.String())
	}
	var status, consent string
	_ = db.QueryRow(`SELECT status FROM campaign_recipients WHERE provider_message_id=$1`, msgID).Scan(&status)
	_ = db.QueryRow(`SELECT status||'/'||source FROM marketing_consents WHERE school_id=$1 AND subject_key=$2 AND channel='whatsapp'`, school, UserSubject(parent)).Scan(&consent)
	if status != RecipientReplied || consent != "opted_out/wa_keyword" {
		t.Fatalf("after reply: status=%s consent=%s", status, consent)
	}
}

func TestUnsubscribeLinkOptsOut(t *testing.T) {
	db := testDB(t)
	school, user := uuid(t, db), uuid(t, db)
	svc := baseService(db, fakeSource{})
	router := newServer(svc)
	_ = svc.Store.SetConsent(context.Background(), school, UserSubject(user), user, ChannelEmail, ConsentOptedIn, SourcePreferences, user)
	link := svc.UnsubscribeURL(school, UserSubject(user), ChannelEmail)
	rec := call(router, http.MethodGet, strings.TrimPrefix(link, "http://notif.test"), "", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("unsubscribe: %d %s", rec.Code, rec.Body.String())
	}
	var st string
	_ = db.QueryRow(`SELECT status FROM marketing_consents WHERE school_id=$1 AND subject_key=$2 AND channel='email'`, school, UserSubject(user)).Scan(&st)
	if st != ConsentOptedOut {
		t.Fatalf("consent = %s", st)
	}
	if rec := call(router, http.MethodGet, "/public/unsubscribe?token=forged.x", "", ""); rec.Code != http.StatusBadRequest {
		t.Fatalf("forged token: %d", rec.Code)
	}
}

func TestRoleGuardAndConsentDefault(t *testing.T) {
	db := testDB(t)
	school, user := uuid(t, db), uuid(t, db)
	router := newServer(baseService(db, fakeSource{}))
	if rec := call(router, http.MethodGet, "/campaigns", tok(t, user, school, "orang_tua"), ""); rec.Code != http.StatusForbidden {
		t.Fatalf("orang_tua on campaigns: %d", rec.Code)
	}
	if rec := call(router, http.MethodPut, "/campaign-settings", tok(t, user, school, "humas"), `{"monthly_wa_quota":10}`); rec.Code != http.StatusForbidden {
		t.Fatalf("humas on settings: %d", rec.Code)
	}
	parent := tok(t, user, school, "orang_tua")
	rec := call(router, http.MethodGet, "/marketing-consents", parent, "")
	if strings.Contains(rec.Body.String(), `"opted_in":true`) {
		t.Fatalf("default must be opted out (OQ-43): %s", rec.Body.String())
	}
	rec = call(router, http.MethodPut, "/marketing-consents", parent, `{"consents":[{"channel":"email","opt_in":true}]}`)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"channel":"email","opted_in":true`) {
		t.Fatalf("put consent: %d %s", rec.Code, rec.Body.String())
	}
}
