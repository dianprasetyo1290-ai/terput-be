package handler

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/xuri/excelize/v2"

	"skci/attendance/internal/cache"
	"skci/attendance/internal/client"
	"skci/attendance/internal/common"
	"skci/attendance/internal/model"
	"skci/attendance/internal/store"
)

// ---- FR-41-004: alpa notification dedupe (fakes, no DB) -------------------

type fakeAlpaClaims struct {
	mu       sync.Mutex
	claimed  map[string]bool
	released int
}

func (f *fakeAlpaClaims) ClaimAlpaNotification(_ context.Context, _, studentID, date string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.claimed == nil {
		f.claimed = map[string]bool{}
	}
	k := studentID + "|" + date
	if f.claimed[k] {
		return false, nil
	}
	f.claimed[k] = true
	return true, nil
}

func (f *fakeAlpaClaims) ReleaseAlpaNotification(_ context.Context, studentID, date string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.claimed, studentID+"|"+date)
	f.released++
	return nil
}

type sentNotif struct{ user, key string }

type fakeGuardianNotifier struct {
	guardians map[string][]string
	failOnce  bool
	sent      []sentNotif
}

func (f *fakeGuardianNotifier) StudentGuardianIDs(_ context.Context, studentID, _ string) ([]string, error) {
	return f.guardians[studentID], nil
}

func (f *fakeGuardianNotifier) Notify(_ context.Context, _, userID, typ, _, _, key string) error {
	if f.failOnce {
		f.failOnce = false
		return errors.New("notification down")
	}
	if typ != "attendance.alpa" {
		return errors.New("unexpected type " + typ)
	}
	f.sent = append(f.sent, sentNotif{userID, key})
	return nil
}

func TestNotifyAlpaDedupePerStudentPerDay(t *testing.T) {
	claims := &fakeAlpaClaims{}
	notif := &fakeGuardianNotifier{guardians: map[string][]string{"dani": {"ibu-dani", "ayah-dani"}, "rina": {"ibu-rina"}}}
	h := &Handler{alpaClaims: claims, alpaNotify: notif}

	var c alpaCollector
	c.add("sch", "dani", "2026-10-06", "tanpa_keterangan")
	c.add("sch", "dani", "2026-10-06", "tanpa_keterangan") // same request twice
	c.add("sch", "rina", "2026-10-06", "hadir")            // not alpa -> ignored
	c.add("sch", "rina", "2026-10-06", "terlambat")        // not alpa -> ignored
	if len(c) != 2 {
		t.Fatalf("collector = %d events, want 2 (only alpa)", len(c))
	}
	if n := h.notifyAlpa(context.Background(), c); n != 1 {
		t.Fatalf("first write sent %d, want 1", n)
	}
	if len(notif.sent) != 2 {
		t.Fatalf("sent %d notifications, want one per guardian (2)", len(notif.sent))
	}
	// A later write path (e.g. import, correction) for the same day: no resend.
	if n := h.notifyAlpa(context.Background(), []alpaEvent{{"sch", "dani", "2026-10-06"}}); n != 0 || len(notif.sent) != 2 {
		t.Fatalf("second write sent %d (total %d), want deduped", n, len(notif.sent))
	}
	// A different day is a new notification.
	if n := h.notifyAlpa(context.Background(), []alpaEvent{{"sch", "dani", "2026-10-07"}}); n != 1 {
		t.Fatalf("next day sent %d, want 1", n)
	}
}

func TestNotifyAlpaReleasesClaimOnFailure(t *testing.T) {
	claims := &fakeAlpaClaims{}
	notif := &fakeGuardianNotifier{guardians: map[string][]string{"dani": {"ibu-dani"}}, failOnce: true}
	h := &Handler{alpaClaims: claims, alpaNotify: notif}
	ev := []alpaEvent{{"sch", "dani", "2026-10-06"}}
	if n := h.notifyAlpa(context.Background(), ev); n != 0 || claims.released != 1 {
		t.Fatalf("failed delivery: sent=%d released=%d, want 0/1", n, claims.released)
	}
	if n := h.notifyAlpa(context.Background(), ev); n != 1 || len(notif.sent) != 1 {
		t.Fatalf("retry: sent=%d total=%d, want 1/1", n, len(notif.sent))
	}
}

// ---- FR-41-003 lock enforcement + BR-ATT-01 callback (needs TEST_PG_DSN) ---

type p4aFixture struct {
	db                                    *sql.DB
	h                                     *Handler
	school, ay, cls, guru, student, admin string
	sessionID, date                       string
}

func newP4AFixture(t *testing.T, academicURL string) *p4aFixture {
	t.Helper()
	dsn := os.Getenv("TEST_PG_DSN")
	if dsn == "" {
		t.Skip("TEST_PG_DSN not set")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	f := &p4aFixture{db: db, date: "2026-10-06"}
	for _, p := range []*string{&f.school, &f.ay, &f.cls, &f.guru, &f.student, &f.admin} {
		if err := db.QueryRow(`SELECT gen_random_uuid()::text`).Scan(p); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.QueryRow(`INSERT INTO class_sessions (school_id, academic_year_id, schedule_id, classroom_id, guru_id, session_date)
		VALUES ($1,$2,gen_random_uuid(),$3,$4,$5) RETURNING id`, f.school, f.ay, f.cls, f.guru, f.date).Scan(&f.sessionID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO student_attendances (class_session_id, school_id, academic_year_id, classroom_id, student_id, status, session_date)
		VALUES ($1,$2,$3,$4,$5,'tanpa_keterangan',$6)`, f.sessionID, f.school, f.ay, f.cls, f.student, f.date); err != nil {
		t.Fatal(err)
	}
	f.h = New(store.New(db), client.NewWithBaseURLs("", academicURL, "ik"))
	f.h.alpaNotify = nil // no outbound notifications from these tests
	// "now" = two days after the session date, 09:00 WIB.
	f.h.now = func() time.Time { return time.Date(2026, 10, 8, 9, 0, 0, 0, wibLocation) }
	return f
}

func (f *p4aFixture) lock(t *testing.T, enabled bool) {
	t.Helper()
	if _, err := f.h.Store.UpsertLockSettings(context.Background(), model.LockSettings{
		SchoolID: f.school, Enabled: enabled, CutoffDays: 1, CutoffTime: "08:00"}, f.admin, "admin"); err != nil {
		t.Fatal(err)
	}
}

func (f *p4aFixture) status(t *testing.T) string {
	t.Helper()
	var s string
	if err := f.db.QueryRow(`SELECT status FROM student_attendances WHERE classroom_id=$1 AND session_date=$2 AND student_id=$3`,
		f.cls, f.date, f.student).Scan(&s); err != nil {
		t.Fatal(err)
	}
	return s
}

func p4aRequest(method, target string, body []byte, claims *common.Claims, params map[string]string) *http.Request {
	r := httptest.NewRequest(method, target, bytes.NewReader(body))
	rctx := chi.NewRouteContext()
	for k, v := range params {
		rctx.URLParams.Add(k, v)
	}
	ctx := context.WithValue(r.Context(), chi.RouteCtxKey, rctx)
	ctx = context.WithValue(ctx, common.ClaimsCtxKey, claims)
	return r.WithContext(ctx)
}

func TestLockEnforcementOnWritePaths(t *testing.T) {
	// Fake academic: empty roster for the import path.
	academic := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		common.WriteJSON(w, http.StatusOK, []any{}, nil)
	}))
	defer academic.Close()
	f := newP4AFixture(t, academic.URL)
	guru := &common.Claims{UserID: f.guru, SchoolID: f.school, ActiveRole: "guru"}
	admin := &common.Claims{UserID: f.admin, SchoolID: f.school, ActiveRole: "admin"}

	markOne := func() *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		f.h.MarkOneStudent(rec, p4aRequest(http.MethodPut, "/", []byte(`{"status":"hadir"}`), guru,
			map[string]string{"id": f.sessionID, "student_id": f.student}))
		return rec
	}

	// Lock configured but disabled: write goes through.
	f.lock(t, false)
	if rec := markOne(); rec.Code != http.StatusOK {
		t.Fatalf("unlocked mark: %d %s", rec.Code, rec.Body)
	}
	if s := f.status(t); s != "hadir" {
		t.Fatalf("status = %s, want hadir", s)
	}

	// Enabled: 2026-10-06 locked from 2026-10-07 08:00 WIB; now is 10-08.
	f.lock(t, true)
	if rec := markOne(); rec.Code != http.StatusConflict {
		t.Fatalf("locked mark one: %d %s", rec.Code, rec.Body)
	}
	rec := httptest.NewRecorder()
	f.h.BulkMarkStudents(rec, p4aRequest(http.MethodPost, "/", []byte(`{"student_ids":["`+f.student+`"],"status":"tanpa_keterangan"}`), guru,
		map[string]string{"id": f.sessionID}))
	if rec.Code != http.StatusConflict {
		t.Fatalf("locked bulk: %d %s", rec.Code, rec.Body)
	}
	rec = httptest.NewRecorder()
	f.h.SaveDailyAttendance(rec, p4aRequest(http.MethodPost, "/", []byte(`{"classroom_id":"`+f.cls+`","date":"`+f.date+
		`","entries":[{"student_id":"`+f.student+`","status":"sakit"}],"submit":true}`), admin, nil))
	if rec.Code != http.StatusConflict {
		t.Fatalf("locked daily input: %d %s", rec.Code, rec.Body)
	}

	// Admin import touching the locked day is refused as a whole.
	x := excelize.NewFile()
	x.SetCellValue("Sheet1", "A1", "Nama Siswa")
	x.SetCellValue("Sheet1", "G1", "6")
	x.SetCellValue("Sheet1", "A2", "Dani")
	x.SetCellValue("Sheet1", "G2", "S")
	var xbuf bytes.Buffer
	_ = x.Write(&xbuf)
	var form bytes.Buffer
	mw := multipart.NewWriter(&form)
	_ = mw.WriteField("classroom_id", f.cls)
	_ = mw.WriteField("academic_year_id", f.ay)
	_ = mw.WriteField("month", "2026-10")
	fw, _ := mw.CreateFormFile("file", "presensi.xlsx")
	_, _ = fw.Write(xbuf.Bytes())
	_ = mw.Close()
	req := p4aRequest(http.MethodPost, "/", form.Bytes(), admin, nil)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	rec = httptest.NewRecorder()
	f.h.AdminAttendanceImport(rec, req)
	if rec.Code != http.StatusConflict {
		t.Fatalf("locked import: %d %s", rec.Code, rec.Body)
	}

	if s := f.status(t); s != "hadir" {
		t.Fatalf("locked writes changed status to %s", s)
	}

	// Correction on an unlocked day is refused (must be edited directly).
	rec = httptest.NewRecorder()
	f.h.CreateCorrection(rec, p4aRequest(http.MethodPost, "/", []byte(`{"classroom_id":"`+f.cls+`","student_id":"`+f.student+
		`","session_date":"2026-10-08","status":"sakit","reason":"x"}`), admin, nil))
	if rec.Code != http.StatusConflict {
		t.Fatalf("correction on unlocked day: %d %s", rec.Code, rec.Body)
	}
}

func TestCorrectionCallbackAppliesAndIsIdempotent(t *testing.T) {
	f := newP4AFixture(t, "")
	f.lock(t, true)
	ctx := context.Background()
	from := "tanpa_keterangan"
	c, err := f.h.Store.CreateCorrection(ctx, store.CreateCorrectionInput{SchoolID: f.school, AcademicYearID: f.ay,
		ClassroomID: f.cls, StudentID: f.student, SessionDate: f.date, FromStatus: &from, ToStatus: "sakit",
		Reason: "orang tua lupa lapor", RequestedBy: f.admin, RequestedRole: "walikelas"})
	if err != nil {
		t.Fatal(err)
	}
	router := NewRouter(f.h, RouterConfig{JWTSecret: "s", InternalKey: "ik", Cache: cache.New("", 0)})
	post := func(key, decision string) *httptest.ResponseRecorder {
		raw, _ := json.Marshal(map[string]any{"approval_id": "appr-1", "source_id": c.ID, "request_type": "attendance_correction",
			"decision": decision, "decided_by": f.admin, "comment": "ok"})
		req := httptest.NewRequest(http.MethodPost, "/internal/approval-callbacks", bytes.NewReader(raw))
		req.Header.Set("X-Internal-Key", key)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		return rec
	}
	if rec := post("wrong", "approved"); rec.Code != http.StatusUnauthorized {
		t.Fatalf("wrong key: %d", rec.Code)
	}
	if rec := post("ik", "approved"); rec.Code != http.StatusOK {
		t.Fatalf("callback: %d %s", rec.Code, rec.Body)
	}
	if s := f.status(t); s != "sakit" {
		t.Fatalf("status after approved correction = %s, want sakit (applied despite lock)", s)
	}
	// core retries the callback (or sends a conflicting late one): no change.
	if rec := post("ik", "rejected"); rec.Code != http.StatusOK {
		t.Fatalf("retry: %d %s", rec.Code, rec.Body)
	}
	got, _ := f.h.Store.GetCorrection(ctx, c.ID)
	if got.Status != "approved" || f.status(t) != "sakit" {
		t.Fatalf("retry changed state: %s / %s", got.Status, f.status(t))
	}
	var n int
	_ = f.db.QueryRow(`SELECT count(*) FROM audit_outbox WHERE correlation_id = $1`, "attendance_correction:"+c.ID).Scan(&n)
	if n != 2 {
		t.Fatalf("audit rows = %d, want 2 (requested + approved)", n)
	}
}
