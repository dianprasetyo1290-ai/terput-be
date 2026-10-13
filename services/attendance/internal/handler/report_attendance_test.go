package handler

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/xuri/excelize/v2"

	"skci/attendance/internal/client"
	"skci/attendance/internal/common"
	"skci/attendance/internal/model"
	"skci/attendance/internal/store"
)

func TestParseReportPeriod(t *testing.T) {
	now := time.Date(2026, 10, 13, 9, 0, 0, 0, time.UTC)
	cases := []struct {
		q              string
		kind, from, to string
		label          string
		wantErr        bool
	}{
		{q: "", kind: "monthly", from: "2026-10-01", to: "2026-10-31", label: "Bulanan Oktober 2026"},
		{q: "period=monthly&month=2026-02", kind: "monthly", from: "2026-02-01", to: "2026-02-28"},
		{q: "period=daily", kind: "daily", from: "2026-10-13", to: "2026-10-13", label: "Harian 13 Oktober 2026"},
		{q: "period=daily&date=2026-10-07", kind: "daily", from: "2026-10-07", to: "2026-10-07"},
		{q: "period=semester&term_id=t1", kind: "semester"},
		{q: "period=semester", wantErr: true},
		{q: "period=range&from=2026-08-01&to=2026-09-30", kind: "range", from: "2026-08-01", to: "2026-09-30"},
		{q: "period=range&from=2026-09-30&to=2026-08-01", wantErr: true},
		{q: "period=range&from=2025-01-01&to=2026-09-30", wantErr: true},
		{q: "period=monthly&month=10-2026", wantErr: true},
		{q: "period=yearly", wantErr: true},
	}
	for _, tc := range cases {
		q, _ := url.ParseQuery(tc.q)
		p, err := parseReportPeriod(q, now)
		if tc.wantErr {
			if err == nil {
				t.Errorf("%q: expected error", tc.q)
			}
			continue
		}
		if err != nil {
			t.Errorf("%q: %v", tc.q, err)
			continue
		}
		if p.Kind != tc.kind || p.From != tc.from || p.To != tc.to {
			t.Errorf("%q: got %+v", tc.q, p)
		}
		if tc.label != "" && p.Label != tc.label {
			t.Errorf("%q: label %q, want %q", tc.q, p.Label, tc.label)
		}
	}
}

func reportReq(target string, claims *common.Claims) *http.Request {
	r := httptest.NewRequest(http.MethodGet, target, nil)
	return r.WithContext(context.WithValue(r.Context(), common.ClaimsCtxKey, claims))
}

func TestAttendanceReportRejectsGuruAndBadParams(t *testing.T) {
	h := &Handler{}
	rec := httptest.NewRecorder()
	h.AttendanceReportRecap(rec, reportReq("/admin/attendance-report/recap", parentClaims("guru", "g", "s")))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("guru: status %d, want 403", rec.Code)
	}
	for _, target := range []string{
		"/admin/attendance-report/recap?period=weird",
		"/admin/attendance-report/recap?scope=galaxy",
		"/admin/attendance-report/trend?granularity=hour",
		"/admin/attendance-report/subjects?group=teacher",
		"/admin/attendance-report/export?format=pdf",
		"/admin/attendance-report/export?report=nope",
	} {
		rec := httptest.NewRecorder()
		r := reportReq(target, parentClaims("admin", "a", "s"))
		switch {
		case strings.Contains(target, "/trend"):
			h.AttendanceReportTrend(rec, r)
		case strings.Contains(target, "/subjects"):
			h.AttendanceReportSubjects(rec, r)
		case strings.Contains(target, "/export"):
			h.AttendanceReportExport(rec, r)
		default:
			h.AttendanceReportRecap(rec, r)
		}
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status %d, want 400", target, rec.Code)
		}
	}
}

func TestWriteReportXLSX(t *testing.T) {
	c := model.AttendanceCounts{Hadir: 17, Terlambat: 1, Sakit: 1, TanpaKeterangan: 1}
	row := model.AttendanceRecapRow{Scope: "student", StudentName: "Dani", NISN: "123", ClassroomName: "X-A", Counts: c}
	store.FinalizeCounts(&row)
	sum := store.SumCounts([]model.AttendanceRecapRow{row}, "summary")
	tbl := recapTable("student", reportPeriod{Label: "Bulanan Oktober 2026"}, []model.AttendanceRecapRow{row}, sum)
	var buf bytes.Buffer
	if err := writeReportXLSX(&buf, tbl); err != nil {
		t.Fatal(err)
	}
	f, err := excelize.OpenReader(&buf)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := f.GetRows("Laporan")
	if err != nil {
		t.Fatal(err)
	}
	// title, subtitle, blank, header, Dani, TOTAL
	if len(rows) != 6 {
		t.Fatalf("rows = %d: %v", len(rows), rows)
	}
	if rows[3][2] != "Nama Siswa" || rows[3][5] != "Terlambat" {
		t.Fatalf("header wrong: %v", rows[3])
	}
	dani := rows[4]
	if dani[2] != "Dani" || dani[4] != "17" || dani[5] != "1" || dani[6] != "1" || dani[8] != "1" || dani[9] != "20" {
		t.Fatalf("Dani row wrong: %v", dani)
	}
	if rows[5][0] != "TOTAL" {
		t.Fatalf("total row wrong: %v", rows[5])
	}
}

// fakeLookupServer impersonates identity-service + academic-service for the
// name-resolution path.
func fakeLookupServer(t *testing.T, names map[string]string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/users/search":
			var body struct {
				IDs string `json:"ids"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			var users []map[string]interface{}
			for _, id := range strings.Split(body.IDs, ",") {
				if n, ok := names[id]; ok {
					users = append(users, map[string]interface{}{"id": id, "name": n, "nisn": "NISN-" + n})
				}
			}
			common.WriteJSON(w, http.StatusOK, users, nil)
		case len(parts) == 2:
			if n, ok := names[parts[1]]; ok {
				common.WriteJSON(w, http.StatusOK, map[string]string{"id": parts[1], "name": n}, nil)
				return
			}
			http.NotFound(w, r)
		default:
			http.NotFound(w, r)
		}
	}))
}

// TestAttendanceReportEndpointsDB drives the handlers against a migrated DB
// (TEST_PG_DSN) with fake sibling services, asserting AC-43-01 and that
// names (not ids) come back.
func TestAttendanceReportEndpointsDB(t *testing.T) {
	dsn := os.Getenv("TEST_PG_DSN")
	if dsn == "" {
		t.Skip("TEST_PG_DSN not set")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`ALTER TABLE student_attendances DROP CONSTRAINT IF EXISTS student_attendances_status_check;
		ALTER TABLE student_attendances ADD CONSTRAINT student_attendances_status_check
		CHECK (status IN ('belum_presensi','hadir','terlambat','sakit','izin','tanpa_keterangan'))`); err != nil {
		t.Fatal(err)
	}
	uuid := func() string {
		var id string
		_ = db.QueryRow(`SELECT gen_random_uuid()::text`).Scan(&id)
		return id
	}
	school, ay, cls, cls2, guru, dani, eka, subj := uuid(), uuid(), uuid(), uuid(), uuid(), uuid(), uuid(), uuid()
	srv := fakeLookupServer(t, map[string]string{school: "SMA Teratai", cls: "X-A", cls2: "X-B", dani: "Dani", eka: "Eka", subj: "Matematika"})
	defer srv.Close()

	insert := func(classroom, student, status, date string, subject interface{}) {
		var sid string
		if err := db.QueryRow(`INSERT INTO class_sessions (school_id, academic_year_id, schedule_id, classroom_id, subject_id, guru_id, session_date)
			VALUES ($1,$2,gen_random_uuid(),$3,$4,$5,$6) RETURNING id`, school, ay, classroom, subject, guru, date).Scan(&sid); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`INSERT INTO student_attendances (class_session_id, school_id, academic_year_id, classroom_id, student_id, status, session_date)
			VALUES ($1,$2,$3,$4,$5,$6,$7)`, sid, school, ay, classroom, student, status, date); err != nil {
			t.Fatal(err)
		}
	}
	statuses := append(strings.Split(strings.Repeat("hadir,", 17), ",")[:17], "terlambat", "sakit", "tanpa_keterangan")
	for i, st := range statuses {
		var subject interface{}
		if i == 0 {
			subject = subj
		}
		insert(cls, dani, st, fmt.Sprintf("2026-10-%02d", i+1), subject)
		insert(cls2, eka, "hadir", fmt.Sprintf("2026-10-%02d", i+1), nil)
	}

	h := New(store.New(db), client.NewWithBaseURLs(srv.URL, srv.URL, "k"))
	admin := parentClaims("admin", "adm", school)
	get := func(fn http.HandlerFunc, target string) map[string]json.RawMessage {
		rec := httptest.NewRecorder()
		fn(rec, reportReq(target, admin))
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: %d %s", target, rec.Code, rec.Body.String())
		}
		var env struct{ Data map[string]json.RawMessage }
		if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
			t.Fatal(err)
		}
		return env.Data
	}

	data := get(h.AttendanceReportRecap, "/x?scope=student&period=monthly&month=2026-10")
	var rows []model.AttendanceRecapRow
	_ = json.Unmarshal(data["rows"], &rows)
	if len(rows) != 2 || rows[0].StudentName != "Dani" || rows[0].ClassroomName != "X-A" || rows[0].NISN != "NISN-Dani" {
		t.Fatalf("student recap names wrong: %+v", rows)
	}
	if c := rows[0].Counts; c.Hadir != 17 || c.Terlambat != 1 || c.Sakit != 1 || c.TanpaKeterangan != 1 {
		t.Fatalf("AC-43-01 counts wrong: %+v", c)
	}

	data = get(h.AttendanceReportRecap, "/x?scope=unit&period=monthly&month=2026-10")
	_ = json.Unmarshal(data["rows"], &rows)
	if len(rows) != 1 || rows[0].SchoolName != "SMA Teratai" || rows[0].Students != 2 || rows[0].Counts.Total != 40 {
		t.Fatalf("unit recap wrong: %+v", rows)
	}

	data = get(h.AttendanceReportClassComparison, "/x?period=monthly&month=2026-10")
	_ = json.Unmarshal(data["rows"], &rows)
	if len(rows) != 2 || rows[0].ClassroomName != "X-B" || rows[0].Rank != 1 {
		t.Fatalf("comparison wrong: %+v", rows)
	}

	data = get(h.AttendanceReportSubjects, "/x?group=class&period=monthly&month=2026-10")
	_ = json.Unmarshal(data["rows"], &rows)
	if len(rows) != 1 || rows[0].SubjectName != "Matematika" || rows[0].ClassroomName != "X-A" || rows[0].Sessions != 1 {
		t.Fatalf("subject report wrong: %+v", rows)
	}

	// pagination
	rec := httptest.NewRecorder()
	h.AttendanceReportRecap(rec, reportReq("/x?scope=student&month=2026-10&page=2&per_page=1", admin))
	var env struct {
		Data struct{ Rows []model.AttendanceRecapRow }
		Meta map[string]int
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &env)
	if len(env.Data.Rows) != 1 || env.Data.Rows[0].StudentName != "Eka" || env.Meta["total"] != 2 {
		t.Fatalf("pagination wrong: %+v", env)
	}

	// Excel export
	rec = httptest.NewRecorder()
	h.AttendanceReportExport(rec, reportReq("/x?report=recap&scope=class&month=2026-10", admin))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Header().Get("Content-Type"), "spreadsheetml") {
		t.Fatalf("export: %d %s", rec.Code, rec.Header().Get("Content-Type"))
	}
	body, _ := io.ReadAll(rec.Body)
	f, err := excelize.OpenReader(bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	xrows, _ := f.GetRows("Laporan")
	if len(xrows) != 7 || xrows[4][2] != "X-A" || xrows[6][0] != "TOTAL" {
		t.Fatalf("xlsx rows wrong: %v", xrows)
	}
}
