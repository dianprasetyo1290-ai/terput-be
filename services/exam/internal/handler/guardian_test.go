package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"skci/exam/internal/cache"
	"skci/exam/internal/common"
	"skci/exam/internal/httpclient"
	"skci/exam/internal/store"
)

// signWaliJWT mints a normal user JWT the way identity-service does: the
// caller's subject, school and active_role are what the guardian check reads.
func signWaliJWT(t *testing.T, role, userID, schoolID string) string {
	t.Helper()
	tok, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": userID, "school_id": schoolID, "active_role": role,
		"roles": []string{role}, "exp": time.Now().Add(time.Hour).Unix(),
	}).SignedString([]byte(testJWTSecret))
	if err != nil {
		t.Fatalf("sign user jwt: %v", err)
	}
	return tok
}

// newWaliFakeIdentity stands in for identity-service's two internal reads the
// parent routes make: the guardian<->student access check and the best-effort
// classroom resolution. `linked` is read live (it is a map the test can mutate
// between cases), so one server covers "not linked" then "linked".
func newWaliFakeIdentity(t *testing.T, linked map[string]bool, classrooms map[string]string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Internal-Key") != testInternalKey {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
		switch {
		case len(parts) == 6 && parts[0] == "internal" && parts[1] == "guardians" &&
			parts[3] == "students" && parts[5] == "access":
			if linked[parts[2]+"|"+parts[4]] {
				common.WriteJSON(w, http.StatusOK, map[string]bool{"linked": true}, nil)
				return
			}
			common.WriteError(w, http.StatusNotFound, "not linked")
		case len(parts) == 3 && parts[0] == "students" && parts[2] == "classroom":
			common.WriteJSON(w, http.StatusOK, map[string]string{"classroom_id": classrooms[parts[1]]}, nil)
		default:
			common.WriteError(w, http.StatusNotFound, "not found")
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// waliChildRow mirrors the sanitized parent-portal JSON for decoding in tests.
type waliChildRow struct {
	Exam struct {
		ID       string `json:"id"`
		Audience string `json:"audience"`
	} `json:"exam"`
	Status  string `json:"status"`
	Attempt *struct {
		Status string   `json:"status"`
		Score  *float64 `json:"score"`
	} `json:"attempt"`
}

type waliEnvelope struct {
	Data    json.RawMessage `json:"data"`
	Message string          `json:"message"`
}

// TestParentRoutes_RequireWaliRole: a non-guardian JWT and an unauthenticated
// request never reach the guardian check -- the parent surface is orang_tua
// only, and it is read-only.
func TestParentRoutes_RequireWaliRole(t *testing.T) {
	h := New(nil, nil, testParticipantSecret, time.Minute)
	router := NewRouter(h, testJWTSecret, testInternalKey, []string{"*"}, cache.New("", 0))

	schoolID, studentID := "33333333-3333-3333-3333-333333333333", "44444444-4444-4444-4444-444444444444"
	listPath := "/parent/children/" + studentID + "/exams"
	detailPath := listPath + "/" + testExamA

	do := func(method, path, auth string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, nil)
		if auth != "" {
			req.Header.Set("Authorization", "Bearer "+auth)
		}
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		return rec
	}

	for _, role := range []string{"siswa", "guru", "admin", "superadmin"} {
		if rec := do(http.MethodGet, listPath, signWaliJWT(t, role, studentID, schoolID)); rec.Code != http.StatusForbidden {
			t.Errorf("role %s GET list: status = %d, want 403 (body %s)", role, rec.Code, rec.Body.String())
		}
	}
	if rec := do(http.MethodGet, detailPath, ""); rec.Code != http.StatusUnauthorized {
		t.Errorf("no token: status = %d, want 401", rec.Code)
	}
	// The guardian role must never be able to write: every method but GET is
	// rejected by the router before any handler runs.
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete} {
		if rec := do(method, listPath, signWaliJWT(t, roleOrangTua, studentID, schoolID)); rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s list: status = %d, want 405", method, rec.Code)
		}
	}
}

// TestParentRoutes_FailClosedWhenIdentityNotConfigured pins the security
// property: an absent/blank identity client (or one that errors) must deny
// with 503, never fall through to a read. No database is needed because the
// denial happens before any store call.
func TestParentRoutes_FailClosedWhenIdentityNotConfigured(t *testing.T) {
	schoolID, studentID := "33333333-3333-3333-3333-333333333333", "44444444-4444-4444-4444-444444444444"
	token := signWaliJWT(t, roleOrangTua, "55555555-5555-5555-5555-555555555555", schoolID)
	path := "/parent/children/" + studentID + "/exams"

	brokenIdentity := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	t.Cleanup(brokenIdentity.Close)

	cases := []struct {
		name string
		ic   *httpclient.Client
	}{
		{"nil client", nil},
		{"no identity url", httpclient.New("", "", testInternalKey)},
		{"no internal key", httpclient.New("http://identity.local", "", testInternalKey)},
		{"identity errors", httpclient.New(brokenIdentity.URL, brokenIdentity.URL, testInternalKey)},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h := New(nil, c.ic, testParticipantSecret, time.Minute)
			router := NewRouter(h, testJWTSecret, testInternalKey, []string{"*"}, cache.New("", 0))
			req := httptest.NewRequest(http.MethodGet, path, nil)
			req.Header.Set("Authorization", "Bearer "+token)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			if rec.Code != http.StatusServiceUnavailable {
				t.Fatalf("status = %d, want 503 (fail closed) (body %s)", rec.Code, rec.Body.String())
			}
		})
	}
}

// seedWaliAcademicExam creates a published ACADEMIC exam targeted at one
// classroom (the whole class, as in reality) and returns its id.
func seedWaliAcademicExam(t *testing.T, s *store.Store, schoolID, classroomID string) string {
	t.Helper()
	ctx := context.Background()
	db := s.DB
	academicYearID, termID, subjectID, teacherID := testUUID(t, ctx, db), testUUID(t, ctx, db), testUUID(t, ctx, db), testUUID(t, ctx, db)

	var examTypeID string
	if err := db.QueryRowContext(ctx, `
		INSERT INTO exam_types (school_id, academic_year_id, name, mapping_rapor, created_by)
		VALUES ($1,$2,'wali test','sumatif',$3) RETURNING id`, schoolID, academicYearID, teacherID).Scan(&examTypeID); err != nil {
		t.Fatalf("insert exam_type: %v", err)
	}
	var examID string
	if err := db.QueryRowContext(ctx, `
		INSERT INTO exams (school_id, academic_year_id, term_id, exam_type_id, title, classroom_id, subject_id,
			teacher_id, start_at, end_at, is_published, published_at, created_by)
		VALUES ($1,$2,$3,$4,'UAS Wali',$5,$6,$7, now()-interval '1 day', now()-interval '1 hour', true, now(), $7)
		RETURNING id`, schoolID, academicYearID, termID, examTypeID, classroomID, subjectID, teacherID).Scan(&examID); err != nil {
		t.Fatalf("insert academic exam: %v", err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO exam_classrooms (exam_id, classroom_id) VALUES ($1,$2)`, examID, classroomID); err != nil {
		t.Fatalf("insert exam_classrooms: %v", err)
	}
	t.Cleanup(func() {
		db.ExecContext(context.Background(), `DELETE FROM exams WHERE id=$1`, examID)
		db.ExecContext(context.Background(), `DELETE FROM exam_types WHERE id=$1`, examTypeID)
	})
	return examID
}

// seedWaliStudentAttempt records one student's own attempt on an existing exam.
func seedWaliStudentAttempt(t *testing.T, s *store.Store, schoolID, classroomID, examID, studentID, status string, score float64) {
	t.Helper()
	ctx := context.Background()
	if _, err := s.DB.ExecContext(ctx, `
		INSERT INTO exam_attempts (school_id, exam_id, participant_type, participant_id, user_id,
			classroom_id, status, score, submitted_at)
		VALUES ($1,$2,'student',$3,$3,$4,$5,$6, now())`, schoolID, examID, studentID, classroomID, status, score); err != nil {
		t.Fatalf("insert student attempt: %v", err)
	}
}

// seedWaliAdmissionExam creates an admission (PPDB) exam + applicant
// participant + applicant attempt in the same school, to prove it can never
// reach the parent portal.
func seedWaliAdmissionExam(t *testing.T, s *store.Store, schoolID, applicantID string, score float64) string {
	t.Helper()
	ctx := context.Background()
	db := s.DB
	now := time.Now()
	exam, err := s.UpsertAdmissionExam(ctx, store.AdmissionExamInput{
		SchoolID: schoolID, ExternalRef: "WALI-" + testUUID(t, ctx, db)[:8],
		Title: "Seleksi PPDB (wali test)", StartAt: now.Add(-time.Hour), EndAt: now.Add(time.Hour),
		CreatedBy: schoolID, AdmissionContext: map[string]interface{}{},
	})
	if err != nil {
		t.Fatalf("UpsertAdmissionExam: %v", err)
	}
	if _, err := s.ProvisionApplicant(ctx, schoolID, exam.ID, applicantID, "", nil); err != nil {
		t.Fatalf("ProvisionApplicant: %v", err)
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO exam_attempts (school_id, exam_id, participant_type, participant_id, status, score, submitted_at)
		VALUES ($1,$2,'applicant',$3,'selesai',$4, now())`, schoolID, exam.ID, applicantID, score); err != nil {
		t.Fatalf("insert applicant attempt: %v", err)
	}
	t.Cleanup(func() { db.ExecContext(context.Background(), `DELETE FROM exams WHERE id=$1`, exam.ID) })
	return exam.ID
}

// TestParentExams_PortalOrangTua is the end-to-end check for the parent portal:
// reject without a link, allow with one, keep PPDB/applicant data out, and
// return only the linked student's own attempt.
func TestParentExams_PortalOrangTua(t *testing.T) {
	s := admissionHandlerTestDB(t)
	ctx := context.Background()

	schoolID := testUUID(t, ctx, s.DB)
	classroomID := testUUID(t, ctx, s.DB)
	studentA := testUUID(t, ctx, s.DB)
	studentB := testUUID(t, ctx, s.DB)
	applicantID := testUUID(t, ctx, s.DB)
	guardianID := testUUID(t, ctx, s.DB)

	// One class exam both students sit; a guardian must only ever see their own
	// child's attempt (score 88), never the classmate's (score 42).
	academicExamID := seedWaliAcademicExam(t, s, schoolID, classroomID)
	seedWaliStudentAttempt(t, s, schoolID, classroomID, academicExamID, studentA, "selesai", 88)
	seedWaliStudentAttempt(t, s, schoolID, classroomID, academicExamID, studentB, "selesai", 42)
	// A second class exam studentA has NOT entered: it must still appear (it is
	// the class schedule) with a derived "belum_mulai" status.
	untakenExamID := seedWaliAcademicExam(t, s, schoolID, classroomID)
	admissionExamID := seedWaliAdmissionExam(t, s, schoolID, applicantID, 99)

	linked := map[string]bool{}
	classrooms := map[string]string{studentA: classroomID, studentB: classroomID}
	srv := newWaliFakeIdentity(t, linked, classrooms)
	ic := httpclient.New(srv.URL, srv.URL, testInternalKey)
	h := New(s, ic, testParticipantSecret, time.Minute)
	router := NewRouter(h, testJWTSecret, testInternalKey, []string{"*"}, cache.New("", 0))

	token := signWaliJWT(t, roleOrangTua, guardianID, schoolID)
	do := func(method, path string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		return rec
	}
	listA := "/parent/children/" + studentA + "/exams"
	detailA := listA + "/" + academicExamID

	// 1. No link yet -> rejected (403), including the applicant's own id.
	if rec := do(http.MethodGet, listA); rec.Code != http.StatusForbidden {
		t.Fatalf("unlinked list: status = %d, want 403 (body %s)", rec.Code, rec.Body.String())
	}
	if rec := do(http.MethodGet, detailA); rec.Code != http.StatusForbidden {
		t.Fatalf("unlinked detail: status = %d, want 403 (body %s)", rec.Code, rec.Body.String())
	}
	if rec := do(http.MethodGet, "/parent/children/"+applicantID+"/exams"); rec.Code != http.StatusForbidden {
		t.Fatalf("applicant id as a child: status = %d, want 403 (body %s)", rec.Code, rec.Body.String())
	}

	// 2. Linked to studentA only.
	linked[guardianID+"|"+studentA] = true

	rec := do(http.MethodGet, listA)
	if rec.Code != http.StatusOK {
		t.Fatalf("linked list: status = %d, want 200 (body %s)", rec.Code, rec.Body.String())
	}
	raw := rec.Body.String()
	if strings.Contains(raw, "token_id") || strings.Contains(raw, "question_order") {
		t.Errorf("list response leaks attempt internals: %s", raw)
	}
	var listEnv waliEnvelope
	if err := json.Unmarshal([]byte(raw), &listEnv); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	var rows []waliChildRow
	if err := json.Unmarshal(listEnv.Data, &rows); err != nil {
		t.Fatalf("decode rows: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("list returned %d rows, want 2 (both academic exams of the class): %s", len(rows), raw)
	}
	byExam := map[string]waliChildRow{}
	for _, r := range rows {
		byExam[r.Exam.ID] = r
	}
	got, ok := byExam[academicExamID]
	if !ok || got.Exam.Audience != "academic" {
		t.Fatalf("academic exam missing from list: %+v", rows)
	}
	if got.Attempt == nil || got.Attempt.Score == nil || *got.Attempt.Score != 88 || got.Status != "selesai" {
		t.Errorf("studentA should see their OWN attempt (88/selesai), got attempt=%+v status=%q", got.Attempt, got.Status)
	}
	untaken, ok := byExam[untakenExamID]
	if !ok || untaken.Attempt != nil || untaken.Status != "belum_mulai" {
		t.Errorf("untaken exam should list with status belum_mulai and no attempt, got %+v", untaken)
	}
	if strings.Contains(raw, admissionExamID) {
		t.Errorf("admission exam leaked into the parent list: %s", raw)
	}

	// 3. Detail of studentA's own academic exam -> 200, sanitized.
	rec = do(http.MethodGet, detailA)
	if rec.Code != http.StatusOK {
		t.Fatalf("detail: status = %d, want 200 (body %s)", rec.Code, rec.Body.String())
	}
	raw = rec.Body.String()
	for _, forbidden := range []string{"token_id", "question_order", "selected_option_id", "essay_answer", "is_correct", "answer_options"} {
		if strings.Contains(raw, forbidden) {
			t.Errorf("detail response leaks %q: %s", forbidden, raw)
		}
	}
	var detailEnv waliEnvelope
	if err := json.Unmarshal([]byte(raw), &detailEnv); err != nil {
		t.Fatalf("decode detail: %v", err)
	}
	var detail waliChildRow
	if err := json.Unmarshal(detailEnv.Data, &detail); err != nil {
		t.Fatalf("decode detail row: %v", err)
	}
	if detail.Exam.ID != academicExamID {
		t.Errorf("detail exam = %s, want %s", detail.Exam.ID, academicExamID)
	}
	if detail.Attempt == nil || detail.Attempt.Status != "selesai" || detail.Attempt.Score == nil || *detail.Attempt.Score != 88 || detail.Status != "selesai" {
		t.Errorf("studentA detail should carry their own attempt (88/selesai), got %+v", detail.Attempt)
	}

	// 4. The admission/PPDB exam is invisible even by direct id.
	if rec := do(http.MethodGet, listA+"/"+admissionExamID); rec.Code != http.StatusNotFound {
		t.Fatalf("admission detail: status = %d, want 404 (body %s)", rec.Code, rec.Body.String())
	}

	// 5. Student scoping: the same guardian may not read studentB.
	if rec := do(http.MethodGet, "/parent/children/"+studentB+"/exams"); rec.Code != http.StatusForbidden {
		t.Fatalf("other child list: status = %d, want 403 (body %s)", rec.Code, rec.Body.String())
	}
	if rec := do(http.MethodGet, "/parent/children/"+studentB+"/exams/"+academicExamID); rec.Code != http.StatusForbidden {
		t.Fatalf("other child detail: status = %d, want 403 (body %s)", rec.Code, rec.Body.String())
	}
}
