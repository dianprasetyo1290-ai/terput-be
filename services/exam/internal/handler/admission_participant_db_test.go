package handler

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"skci/exam/internal/cache"
	"skci/exam/internal/common"
	"skci/exam/internal/model"
	"skci/exam/internal/store"
)

// admissionHandlerTestDB follows the convention store/exams_test.go already
// uses: EXAM_TEST_DATABASE_URL, else the docker-compose dev Postgres, skipping
// (never failing) when neither is reachable.
func admissionHandlerTestDB(t *testing.T) *store.Store {
	t.Helper()
	dsn := os.Getenv("EXAM_TEST_DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://sekoci:change-me@localhost:5480/skci_exam?sslmode=disable"
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Skipf("skipping: could not open %s: %v", dsn, err)
	}
	if err := db.Ping(); err != nil {
		db.Close()
		t.Skipf("skipping: dev postgres not reachable at %s: %v", dsn, err)
	}
	t.Cleanup(func() { db.Close() })
	return store.New(db)
}

func testUUID(t *testing.T, ctx context.Context, db *sql.DB) string {
	t.Helper()
	var id string
	if err := db.QueryRowContext(ctx, "SELECT gen_random_uuid()").Scan(&id); err != nil {
		t.Fatalf("gen_random_uuid: %v", err)
	}
	return id
}

// TestParticipantTokenAcceptedOnlyWhenIssuedHere is the end-to-end version of
// the token check: a correctly signed, correctly scoped, unexpired applicant
// token is still refused until THIS service has recorded its hash for that
// exam+applicant, and is accepted (reaching the store) once it has. It also
// re-checks expiry and cross-exam scoping through the real router.
func TestParticipantTokenAcceptedOnlyWhenIssuedHere(t *testing.T) {
	s := admissionHandlerTestDB(t)
	db, ctx := s.DB, context.Background()
	h := New(s, nil, testParticipantSecret, 15*time.Minute)
	router := NewRouter(h, testJWTSecret, testInternalKey, []string{"*"}, cache.New("", 0))

	schoolID := testUUID(t, ctx, db)
	now := time.Now()
	exam, err := s.UpsertAdmissionExam(ctx, store.AdmissionExamInput{
		SchoolID: schoolID, ExternalRef: "REG-HANDLER-" + testUUID(t, ctx, db)[:8],
		Title: "Seleksi (handler test)", StartAt: now.Add(-time.Hour), EndAt: now.Add(time.Hour),
		CreatedBy: schoolID, AdmissionContext: map[string]interface{}{},
	})
	if err != nil {
		t.Fatalf("UpsertAdmissionExam: %v", err)
	}
	t.Cleanup(func() { _, _ = db.ExecContext(context.Background(), `DELETE FROM exams WHERE id=$1`, exam.ID) })

	applicantID := testUUID(t, ctx, db)
	if _, err := s.ProvisionApplicant(ctx, schoolID, exam.ID, applicantID, "", nil); err != nil {
		t.Fatalf("ProvisionApplicant: %v", err)
	}

	newToken := func(examID string, exp time.Time) string {
		t.Helper()
		claims := common.ParticipantClaims{
			SchoolID: schoolID, ExamID: examID,
			ParticipantType: model.ParticipantApplicant, Scope: common.ParticipantScopeExamAttempt,
			RegisteredClaims: jwt.RegisteredClaims{
				Subject: applicantID, IssuedAt: jwt.NewNumericDate(now),
				ExpiresAt: jwt.NewNumericDate(exp),
			},
		}
		tok, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(testParticipantSecret))
		if err != nil {
			t.Fatalf("sign participant token: %v", err)
		}
		return tok
	}

	get := func(examID, token string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/participant/exams/"+examID+"/attempt", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		return rec
	}

	// 1. Well-formed but never issued -> 401, even though the signature and
	//    claims are perfect.
	unissued := newToken(exam.ID, now.Add(time.Hour))
	if rec := get(exam.ID, unissued); rec.Code != http.StatusUnauthorized {
		t.Fatalf("unissued token: status = %d, want 401 (body %s)", rec.Code, rec.Body.String())
	}

	// 2. Persist its hash exactly the way InternalIssueParticipantAccess does.
	if err := s.SaveParticipantToken(ctx, common.TokenHash(unissued), schoolID, exam.ID, applicantID, now.Add(time.Hour)); err != nil {
		t.Fatalf("SaveParticipantToken: %v", err)
	}
	//    The same token now authenticates and reaches the store: this applicant
	//    has not started, so the answer is 404 (not 401/403).
	if rec := get(exam.ID, unissued); rec.Code != http.StatusNotFound {
		t.Fatalf("issued token: status = %d, want 404 (reached the store) (body %s)", rec.Code, rec.Body.String())
	}

	// 3. An expired token for the same exam stays refused.
	expired := newToken(exam.ID, now.Add(-time.Minute))
	if rec := get(exam.ID, expired); rec.Code != http.StatusUnauthorized {
		t.Fatalf("expired token: status = %d, want 401 (body %s)", rec.Code, rec.Body.String())
	}

	// 4. A live token is still scoped to ITS exam: another exam of the same
	//    school gets 403.
	other, err := s.UpsertAdmissionExam(ctx, store.AdmissionExamInput{
		SchoolID: schoolID, ExternalRef: "REG-HANDLER2-" + testUUID(t, ctx, db)[:8],
		Title: "Seleksi lain", StartAt: now.Add(-time.Hour), EndAt: now.Add(time.Hour),
		CreatedBy: schoolID, AdmissionContext: map[string]interface{}{},
	})
	if err != nil {
		t.Fatalf("UpsertAdmissionExam (other): %v", err)
	}
	t.Cleanup(func() { _, _ = db.ExecContext(context.Background(), `DELETE FROM exams WHERE id=$1`, other.ID) })
	if rec := get(other.ID, unissued); rec.Code != http.StatusForbidden {
		t.Fatalf("token used on another exam: status = %d, want 403 (body %s)", rec.Code, rec.Body.String())
	}
}
