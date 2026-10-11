package handler

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"

	"skci/academic/internal/client"
	"skci/academic/internal/common"
	"skci/academic/internal/model"
	"skci/academic/internal/store"
)

func getAcademicTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("TEST_PG_DSN")
	if dsn == "" {
		dsn = os.Getenv("ACADEMIC_TEST_DATABASE_URL")
	}
	if dsn == "" {
		dsn = "postgres://localhost/skci_t7a_academic?sslmode=disable"
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Skipf("cannot open test db: %v", err)
	}
	if err := db.Ping(); err != nil {
		t.Skipf("test db not reachable: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// TestAC78_01_WithdrawalWithArrears tests AC-78-01:
// Given student Rina with arrears 800.000
// When staff initiates a withdrawal
// Then the request records snapshot with total outstanding 800.000 and waits for financial approval
// And Rina's enrollment status does not change before approval.
func TestAC78_01_WithdrawalWithArrears(t *testing.T) {
	db := getAcademicTestDB(t)
	st := store.New(db)
	ctx := context.Background()

	// Seed test data
	schoolID := "11111111-1111-1111-1111-111111111111"
	academicYearID := "22222222-2222-2222-2222-222222222222"
	classID := "33333333-3333-3333-3333-333333333333"
	rinaUserID := "44444444-4444-4444-4444-444444444444"
	staffUserID := "55555555-5555-5555-5555-555555555555"

	// Insert classroom and active enrollment for Rina
	_, _ = db.ExecContext(ctx, `DELETE FROM class_enrollments WHERE school_id = $1`, schoolID)
	_, _ = db.ExecContext(ctx, `DELETE FROM classrooms WHERE id = $1`, classID)
	_, err := db.ExecContext(ctx, `
		INSERT INTO classrooms (id, school_id, academic_year_id, name, grade_level)
		VALUES ($1, $2, $3, 'X IPA 1', '10')
		ON CONFLICT (id) DO NOTHING
	`, classID, schoolID, academicYearID)
	if err != nil {
		t.Fatalf("failed to insert classroom: %v", err)
	}

	_, err = db.ExecContext(ctx, `
		INSERT INTO class_enrollments (school_id, academic_year_id, classroom_id, student_user_id, status)
		VALUES ($1, $2, $3, $4, 'active')
	`, schoolID, academicYearID, classID, rinaUserID)
	if err != nil {
		t.Fatalf("failed to insert enrollment: %v", err)
	}

	// Fake billing service returning 800.000 arrears for Rina
	var fakeBillingCalls int
	fakeBilling := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fakeBillingCalls++
		if r.URL.Path == "/internal/students/"+rinaUserID+"/financial-summary" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{
					"student_user_id":       rinaUserID,
					"school_id":             schoolID,
					"total_outstanding_idr": 800000,
					"open_bills_idr":        800000,
					"unmatured_bills_idr":   0,
					"deposit_idr":           0,
					"refundable_idr":        0,
				},
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer fakeBilling.Close()

	// Fake core approval service
	var coreApprovalReq client.CoreApprovalRequest
	fakeCore := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/internal/approvals" {
			_ = json.NewDecoder(r.Body).Decode(&coreApprovalReq)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{
					"id":          "appr-12345",
					"status":      "pending",
					"current_seq": 1,
				},
			})
			return
		}
		if r.URL.Path == "/internal/audit" {
			w.WriteHeader(http.StatusAccepted)
			return
		}
		http.NotFound(w, r)
	}))
	defer fakeCore.Close()

	h := &Handler{
		Store:    st,
		Identity: client.NewIdentityClient(),
		Billing: &client.BillingClient{
			BaseURL:     fakeBilling.URL,
			InternalKey: "change-me",
			HTTPClient:  fakeBilling.Client(),
		},
		Core: &client.CoreClient{
			BaseURL:     fakeCore.URL,
			InternalKey: "change-me",
			HTTPClient:  fakeCore.Client(),
		},
	}

	// Staff submits withdrawal
	reqBody, _ := json.Marshal(createStatusRequestInput{
		StudentUserID: rinaUserID,
		Kind:          model.KindWithdrawal,
		EffectiveDate: "2026-08-01",
		Reason:        "Pindah sekolah ke luar kota",
	})
	httpReq := httptest.NewRequest(http.MethodPost, "/student-status-requests", bytes.NewReader(reqBody))
	claims := &common.Claims{
		UserID:     staffUserID,
		ActiveRole: "admin",
		SchoolID:   schoolID,
	}
	httpReq = httpReq.WithContext(common.WithClaims(httpReq.Context(), claims))
	rec := httptest.NewRecorder()

	h.CreateStudentStatusRequest(rec, httpReq)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected status 201 Created, got %d: %s", rec.Code, rec.Body.String())
	}

	var res struct {
		Data model.StudentStatusRequest `json:"data"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&res); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// Verification of AC-78-01:
	// 1. Status is pending_approval
	if res.Data.Status != model.StatusPendingApproval {
		t.Errorf("expected status 'pending_approval', got %q", res.Data.Status)
	}
	// 2. Snapshot captured 800.000 arrears
	var snap model.FinanceSnapshot
	if err := json.Unmarshal(res.Data.FinanceSnapshot, &snap); err != nil {
		t.Fatalf("failed to unmarshal snapshot: %v", err)
	}
	if snap.TotalOutstandingIDR != 800000 {
		t.Errorf("expected snapshot total outstanding 800000, got %d", snap.TotalOutstandingIDR)
	}
	// 3. Sent to core approval with amount 800000 and flag 'has_outstanding'
	if coreApprovalReq.AmountIDR == nil || *coreApprovalReq.AmountIDR != 800000 {
		t.Errorf("expected core approval amount 800000, got %v", coreApprovalReq.AmountIDR)
	}
	if len(coreApprovalReq.Flags) == 0 || coreApprovalReq.Flags[0] != "has_outstanding" {
		t.Errorf("expected flag 'has_outstanding', got %v", coreApprovalReq.Flags)
	}
	// 4. Rina's status is UNCHANGED (still active)
	var activeCount int
	err = db.QueryRowContext(ctx, `SELECT count(*) FROM class_enrollments WHERE school_id=$1 AND student_user_id=$2 AND status='active'`,
		schoolID, rinaUserID).Scan(&activeCount)
	if err != nil || activeCount != 1 {
		t.Errorf("expected Rina to still have 1 active enrollment before approval, got count=%d, err=%v", activeCount, err)
	}
}

// TestAC78_02_UnitTransferWithoutHistoryLoss tests AC-78-02:
// Given student Bayu transferring from SMP to SMK with arrears 300.000 in SMP
// When transfer is approved and applied
// Then Bayu's old enrollment in SMP is marked 'transferred_out' (not deleted)
// And identity create-or-link is called for unit SMK
// And billing exit is called so new bills issue at SMK while old arrears stay at SMP.
func TestAC78_02_UnitTransferWithoutHistoryLoss(t *testing.T) {
	db := getAcademicTestDB(t)
	st := store.New(db)
	ctx := context.Background()

	smpSchoolID := "11111111-2222-3333-4444-555555555555"
	smkSchoolID := "66666666-7777-8888-9999-000000000000"
	yearID := "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"
	smpClassID := "12345678-1234-1234-1234-123456789012"
	bayuUserID := "87654321-4321-4321-4321-210987654321"

	// Seed SMP classroom and enrollment
	_, _ = db.ExecContext(ctx, `DELETE FROM class_enrollments WHERE school_id = $1`, smpSchoolID)
	_, _ = db.ExecContext(ctx, `DELETE FROM classrooms WHERE id = $1`, smpClassID)
	_, _ = db.ExecContext(ctx, `
		INSERT INTO classrooms (id, school_id, academic_year_id, name, grade_level)
		VALUES ($1, $2, $3, 'IX SMP', '9') ON CONFLICT (id) DO NOTHING
	`, smpClassID, smpSchoolID, yearID)
	_, _ = db.ExecContext(ctx, `
		INSERT INTO class_enrollments (school_id, academic_year_id, classroom_id, student_user_id, status)
		VALUES ($1, $2, $3, $4, 'active')
	`, smpSchoolID, yearID, smpClassID, bayuUserID)

	var billingExitCalled bool
	var identityLinkCalled bool
	var mu sync.Mutex

	fakeBilling := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if r.URL.Path == "/internal/students/"+bayuUserID+"/exit" {
			billingExitCalled = true
			w.WriteHeader(http.StatusOK)
			return
		}
		http.NotFound(w, r)
	}))
	defer fakeBilling.Close()

	fakeIdentity := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if r.URL.Path == "/internal/students/create-or-link" {
			identityLinkCalled = true
			w.WriteHeader(http.StatusOK)
			return
		}
		http.NotFound(w, r)
	}))
	defer fakeIdentity.Close()

	h := &Handler{
		Store: st,
		Identity: &client.IdentityClient{
			BaseURL:     fakeIdentity.URL,
			InternalKey: "change-me",
			HTTPClient:  fakeIdentity.Client(),
		},
		Billing: &client.BillingClient{
			BaseURL:     fakeBilling.URL,
			InternalKey: "change-me",
			HTTPClient:  fakeBilling.Client(),
		},
	}

	// Create request
	toSchool := smkSchoolID
	fromClass := smpClassID
	req := &model.StudentStatusRequest{
		SchoolID:        smpSchoolID,
		StudentUserID:   bayuUserID,
		Kind:            model.KindUnitTransfer,
		FromClassroomID: &fromClass,
		ToSchoolID:      &toSchool,
		EffectiveDate:   "2026-07-01",
		Reason:          "Lulus SMP lanjut ke SMK unit teratai",
		Status:          model.StatusApproved,
		RequestedBy:     "55555555-5555-5555-5555-555555555555",
	}
	created, err := st.CreateStudentStatusRequest(ctx, req)
	if err != nil {
		t.Fatalf("failed to create status request: %v", err)
	}

	// Apply request
	httpReq := httptest.NewRequest(http.MethodPost, "/internal/apply", nil)
	err = h.applyRequest(httpReq, created)
	if err != nil {
		t.Fatalf("applyRequest failed: %v", err)
	}

	// Verify:
	// 1. Bayu's SMP enrollment is 'transferred_out' (never deleted!)
	var status string
	err = db.QueryRowContext(ctx, `SELECT status FROM class_enrollments WHERE school_id=$1 AND student_user_id=$2`,
		smpSchoolID, bayuUserID).Scan(&status)
	if err != nil {
		t.Fatalf("failed to query enrollment: %v", err)
	}
	if status != "transferred_out" {
		t.Errorf("expected enrollment status 'transferred_out', got %q", status)
	}

	// 2. Identity called to link student to SMK
	mu.Lock()
	if !identityLinkCalled {
		t.Errorf("expected identity create-or-link to be called for SMK")
	}
	if !billingExitCalled {
		t.Errorf("expected billing exit to be called for SMP")
	}
	mu.Unlock()
}

// TestAC78_03_WithdrawalDepositRefund tests AC-78-03:
// Given student withdrawing with deposit 200.000
// When applied
// Then billing internal refund is called with 200.000 and idempotency 'withdrawal:<id>'
// And enrollment status becomes 'withdrawn'.
func TestAC78_03_WithdrawalDepositRefund(t *testing.T) {
	db := getAcademicTestDB(t)
	st := store.New(db)
	ctx := context.Background()

	schoolID := "77777777-1111-2222-3333-444444444444"
	yearID := "88888888-1111-2222-3333-444444444444"
	classID := "99999999-1111-2222-3333-444444444444"
	studentID := "aaaaaaaa-1111-2222-3333-444444444444"

	_, _ = db.ExecContext(ctx, `DELETE FROM class_enrollments WHERE school_id = $1`, schoolID)
	_, _ = db.ExecContext(ctx, `DELETE FROM classrooms WHERE id = $1`, classID)
	_, _ = db.ExecContext(ctx, `
		INSERT INTO classrooms (id, school_id, academic_year_id, name, grade_level)
		VALUES ($1, $2, $3, 'X IPS 1', '10') ON CONFLICT (id) DO NOTHING
	`, classID, schoolID, yearID)
	_, _ = db.ExecContext(ctx, `
		INSERT INTO class_enrollments (school_id, academic_year_id, classroom_id, student_user_id, status)
		VALUES ($1, $2, $3, $4, 'active')
	`, schoolID, yearID, classID, studentID)

	var refundCalled bool
	var refundAmount int64
	var idempotencySent string
	var mu sync.Mutex

	fakeBilling := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if r.URL.Path == "/internal/students/"+studentID+"/exit" {
			w.WriteHeader(http.StatusOK)
			return
		}
		if r.URL.Path == "/internal/students/"+studentID+"/refund" {
			refundCalled = true
			var body struct {
				AmountIDR      int64  `json:"amount_idr"`
				IdempotencyKey string `json:"idempotency_key"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			refundAmount = body.AmountIDR
			idempotencySent = body.IdempotencyKey
			w.WriteHeader(http.StatusOK)
			return
		}
		http.NotFound(w, r)
	}))
	defer fakeBilling.Close()

	fakeIdentity := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer fakeIdentity.Close()

	h := &Handler{
		Store: st,
		Identity: &client.IdentityClient{
			BaseURL:     fakeIdentity.URL,
			InternalKey: "change-me",
			HTTPClient:  fakeIdentity.Client(),
		},
		Billing: &client.BillingClient{
			BaseURL:     fakeBilling.URL,
			InternalKey: "change-me",
			HTTPClient:  fakeBilling.Client(),
		},
	}

	fromClass := classID
	refundAmt := int64(200000)
	req := &model.StudentStatusRequest{
		SchoolID:        schoolID,
		StudentUserID:   studentID,
		Kind:            model.KindWithdrawal,
		FromClassroomID: &fromClass,
		EffectiveDate:   "2026-07-01",
		Reason:          "Keluar dan klaim deposit",
		RefundAmountIDR: &refundAmt,
		Status:          model.StatusApproved,
		RequestedBy:     "55555555-5555-5555-5555-555555555555",
	}
	created, err := st.CreateStudentStatusRequest(ctx, req)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	httpReq := httptest.NewRequest(http.MethodPost, "/internal/apply", nil)
	err = h.applyRequest(httpReq, created)
	if err != nil {
		t.Fatalf("applyRequest failed: %v", err)
	}

	// 1. Enrollment status is 'withdrawn'
	var enrollmentStatus string
	_ = db.QueryRowContext(ctx, `SELECT status FROM class_enrollments WHERE school_id=$1 AND student_user_id=$2`,
		schoolID, studentID).Scan(&enrollmentStatus)
	if enrollmentStatus != "withdrawn" {
		t.Errorf("expected status 'withdrawn', got %q", enrollmentStatus)
	}

	// 2. Billing refund was called
	mu.Lock()
	if !refundCalled {
		t.Errorf("expected billing refund to be called")
	}
	if refundAmount != 200000 {
		t.Errorf("expected refund amount 200000, got %d", refundAmount)
	}
	expectedKey := "withdrawal:" + created.ID
	if idempotencySent != expectedKey {
		t.Errorf("expected idempotency key %q, got %q", expectedKey, idempotencySent)
	}
	mu.Unlock()
}

// TestIdempotentApply ensures running apply twice produces only one mutation.
func TestIdempotentApply(t *testing.T) {
	db := getAcademicTestDB(t)
	st := store.New(db)
	ctx := context.Background()

	schoolID := "bbbbbbbb-1111-2222-3333-444444444444"
	studentID := "cccccccc-1111-2222-3333-444444444444"
	classID := "dddddddd-1111-2222-3333-444444444444"
	yearID := "eeeeeeee-1111-2222-3333-444444444444"

	_, _ = db.ExecContext(ctx, `DELETE FROM class_enrollments WHERE school_id = $1`, schoolID)
	_, _ = db.ExecContext(ctx, `DELETE FROM classrooms WHERE id = $1`, classID)
	_, _ = db.ExecContext(ctx, `
		INSERT INTO classrooms (id, school_id, academic_year_id, name, grade_level)
		VALUES ($1, $2, $3, 'X IPA 1', '10') ON CONFLICT (id) DO NOTHING
	`, classID, schoolID, yearID)
	_, _ = db.ExecContext(ctx, `
		INSERT INTO class_enrollments (school_id, academic_year_id, classroom_id, student_user_id, status)
		VALUES ($1, $2, $3, $4, 'active')
	`, schoolID, yearID, classID, studentID)

	callCount := 0
	fakeBilling := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.WriteHeader(http.StatusOK)
	}))
	defer fakeBilling.Close()

	h := &Handler{
		Store: st,
		Billing: &client.BillingClient{
			BaseURL:     fakeBilling.URL,
			InternalKey: "change-me",
			HTTPClient:  fakeBilling.Client(),
		},
	}

	fromClass := classID
	req := &model.StudentStatusRequest{
		SchoolID:        schoolID,
		StudentUserID:   studentID,
		Kind:            model.KindWithdrawal,
		FromClassroomID: &fromClass,
		EffectiveDate:   "2026-07-01",
		Reason:          "Keluar",
		Status:          model.StatusApproved,
		RequestedBy:     "55555555-5555-5555-5555-555555555555",
	}
	created, err := st.CreateStudentStatusRequest(ctx, req)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	httpReq := httptest.NewRequest(http.MethodPost, "/apply", nil)

	// First apply
	if err := h.applyRequest(httpReq, created); err != nil {
		t.Fatalf("first apply failed: %v", err)
	}
	countAfterFirst := callCount

	// Second apply on the same request
	if err := h.applyRequest(httpReq, created); err != nil {
		t.Fatalf("second apply failed: %v", err)
	}
	if callCount != countAfterFirst {
		t.Errorf("second apply made additional calls: %d vs %d", callCount, countAfterFirst)
	}
}
