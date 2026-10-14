package handler

import (
	"bytes"
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/go-chi/chi/v5"
	_ "github.com/jackc/pgx/v5/stdlib"

	"skci/billing/internal/client"
	"skci/billing/internal/common"
	"skci/billing/internal/config"
)

func newUUID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

type mockTransferRoster struct {
	gradeLevels  map[string]string
	schoolGrades []string
}

func (m mockTransferRoster) Configured() bool { return true }
func (m mockTransferRoster) TeacherStudentIDs(_ context.Context, _, _ string) (map[string]bool, error) {
	return nil, nil
}
func (m mockTransferRoster) StudentGradeLevel(_ context.Context, _, studentUserID string) (string, error) {
	return m.gradeLevels[studentUserID], nil
}
func (m mockTransferRoster) SchoolGradeLevels(_ context.Context, _ string) ([]string, error) {
	return m.schoolGrades, nil
}

func setupBillingTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("TEST_PG_DSN")
	if dsn == "" {
		dsn = "postgres://localhost/skci_t7a_billing?sslmode=disable"
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Skipf("skipping DB test: cannot open connection: %v", err)
	}
	if err := db.Ping(); err != nil {
		t.Skipf("skipping DB test: database not reachable at %s: %v", dsn, err)
	}
	return db
}

func newBillingTestRouter(app *App) http.Handler {
	r := chi.NewRouter()

	// Authenticated routes
	r.Group(func(auth chi.Router) {
		auth.Use(common.JWTMiddleware(testSecret))
		auth.Get("/me/bills", app.ListMyBills)
		auth.Get("/me/bills/visibility", app.GetMyBillVisibility)
		auth.Get("/admin/student-bill-visibility", app.GetStudentBillVisibility)
		auth.Put("/admin/student-bill-visibility", app.UpdateStudentBillVisibility)
	})

	// Internal routes
	r.Group(func(in chi.Router) {
		in.Get("/internal/students/{studentUserId}/financial-summary", app.InternalStudentFinancialSummary)
		in.Post("/internal/students/{studentUserId}/exit", app.InternalStudentExit)
		in.Post("/internal/students/{studentUserId}/refund", app.InternalStudentRefund)
	})

	return r
}

// AC-82-01: Siswa pada jenjang dengan visibility dimatikan (mis. SD) mendapat 403 Forbidden.
func TestStudentBillVisibility_AC82_01_SD_Forbidden(t *testing.T) {
	db := setupBillingTestDB(t)
	defer db.Close()

	schoolID := newUUID()
	studentSD := newUUID()

	// Seed visibility SD = false
	_, err := db.Exec(`
		INSERT INTO student_bill_visibility (school_id, grade_level, enabled)
		VALUES ($1, 'SD', false)
		ON CONFLICT (school_id, grade_level) DO UPDATE SET enabled = false
	`, schoolID)
	if err != nil {
		t.Fatalf("seed visibility: %v", err)
	}

	app := New(db, config.Config{}, stubIdentity{}, nil)
	app.Roster = mockTransferRoster{
		gradeLevels: map[string]string{studentSD: "SD"},
	}

	srv := httptest.NewServer(newBillingTestRouter(app))
	defer srv.Close()

	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/me/bills", nil)
	req.Header.Set("Authorization", "Bearer "+signToken(t, studentSD, schoolID, common.RoleStudent))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden for SD student, got %d", resp.StatusCode)
	}

	// Also verify /me/bills/visibility returns enabled: false
	visReq, _ := http.NewRequest(http.MethodGet, srv.URL+"/me/bills/visibility", nil)
	visReq.Header.Set("Authorization", "Bearer "+signToken(t, studentSD, schoolID, common.RoleStudent))
	visResp, err := http.DefaultClient.Do(visReq)
	if err != nil {
		t.Fatalf("visibility request: %v", err)
	}
	defer visResp.Body.Close()

	var env struct {
		Data struct {
			Enabled    bool   `json:"enabled"`
			GradeLevel string `json:"grade_level"`
		} `json:"data"`
	}
	_ = json.NewDecoder(visResp.Body).Decode(&env)
	if env.Data.Enabled {
		t.Errorf("expected enabled=false for SD, got true")
	}
	if env.Data.GradeLevel != "SD" {
		t.Errorf("expected grade_level='SD', got %q", env.Data.GradeLevel)
	}
}

// AC-82-02: Siswa pada jenjang dengan visibility aktif (mis. SMK) mendapat 200 OK.
func TestStudentBillVisibility_AC82_02_SMK_Allowed(t *testing.T) {
	db := setupBillingTestDB(t)
	defer db.Close()

	schoolID := newUUID()
	studentSMK := newUUID()

	// Seed visibility SMK = true
	_, err := db.Exec(`
		INSERT INTO student_bill_visibility (school_id, grade_level, enabled)
		VALUES ($1, 'SMK', true)
		ON CONFLICT (school_id, grade_level) DO UPDATE SET enabled = true
	`, schoolID)
	if err != nil {
		t.Fatalf("seed visibility: %v", err)
	}

	app := New(db, config.Config{}, stubIdentity{}, nil)
	app.Roster = mockTransferRoster{
		gradeLevels: map[string]string{studentSMK: "SMK"},
	}

	srv := httptest.NewServer(newBillingTestRouter(app))
	defer srv.Close()

	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/me/bills", nil)
	req.Header.Set("Authorization", "Bearer "+signToken(t, studentSMK, schoolID, common.RoleStudent))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK for SMK student with enabled visibility, got %d", resp.StatusCode)
	}
}

// Admin visibility config endpoints: PUT and GET
func TestAdminStudentBillVisibility(t *testing.T) {
	db := setupBillingTestDB(t)
	defer db.Close()

	schoolID := newUUID()

	app := New(db, config.Config{}, stubIdentity{}, nil)
	app.Roster = mockTransferRoster{
		schoolGrades: []string{"SD", "SMP", "SMA", "SMK"},
	}

	srv := httptest.NewServer(newBillingTestRouter(app))
	defer srv.Close()

	// Update visibility
	putPayload := map[string]any{
		"items": []map[string]any{
			{"grade_level": "SD", "enabled": false},
			{"grade_level": "SMP", "enabled": false},
			{"grade_level": "SMA", "enabled": true},
			{"grade_level": "SMK", "enabled": true},
		},
	}
	bodyBytes, _ := json.Marshal(putPayload)

	putReq, _ := http.NewRequest(http.MethodPut, srv.URL+"/admin/student-bill-visibility", bytes.NewReader(bodyBytes))
	putReq.Header.Set("Authorization", "Bearer "+signToken(t, "admin-1", schoolID, common.RoleAdmin))
	putReq.Header.Set("Content-Type", "application/json")

	putResp, err := http.DefaultClient.Do(putReq)
	if err != nil {
		t.Fatalf("put request: %v", err)
	}
	defer putResp.Body.Close()

	if putResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK on PUT visibility, got %d", putResp.StatusCode)
	}

	// Fetch visibility
	getReq, _ := http.NewRequest(http.MethodGet, srv.URL+"/admin/student-bill-visibility", nil)
	getReq.Header.Set("Authorization", "Bearer "+signToken(t, "admin-1", schoolID, common.RoleAdmin))

	getResp, err := http.DefaultClient.Do(getReq)
	if err != nil {
		t.Fatalf("get request: %v", err)
	}
	defer getResp.Body.Close()

	if getResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK on GET visibility, got %d", getResp.StatusCode)
	}

	var res struct {
		Data []struct {
			GradeLevel string `json:"grade_level"`
			Enabled    bool   `json:"enabled"`
		} `json:"data"`
	}
	_ = json.NewDecoder(getResp.Body).Decode(&res)
	if len(res.Data) != 4 {
		t.Errorf("expected 4 grade levels, got %d", len(res.Data))
	}
}

// Student exit with policy "void" voids future unpaid bills (FR-78-005)
func TestInternalStudentExit_VoidPolicy(t *testing.T) {
	db := setupBillingTestDB(t)
	defer db.Close()

	schoolID := newUUID()
	studentID := newUUID()
	ayID := newUUID()

	// Create scheme first to satisfy FK
	var schemeID string
	err := db.QueryRow(`
		INSERT INTO billing_schemes (school_id, academic_year_id, name, status, installment, due_day)
		VALUES ($1, $2, 'SPP', 'active', false, 10)
		RETURNING id::text
	`, schoolID, ayID).Scan(&schemeID)
	if err != nil {
		t.Fatalf("insert scheme: %v", err)
	}

	// Insert future unpaid bill (due in 2026-11-10)
	var futureBillID string
	err = db.QueryRow(`
		INSERT INTO student_bills (school_id, scheme_id, student_user_id, period_year, period_month, base_idr, discount_idr, net_idr, status, due_at, discount_snapshot)
		VALUES ($1, $2, $3, 2026, 11, 500000, 0, 500000, 'unpaid', '2026-11-10', '[]'::jsonb)
		RETURNING id::text
	`, schoolID, schemeID, studentID).Scan(&futureBillID)
	if err != nil {
		t.Fatalf("insert future bill: %v", err)
	}

	// Insert past unpaid bill (due in 2026-09-10)
	var pastBillID string
	err = db.QueryRow(`
		INSERT INTO student_bills (school_id, scheme_id, student_user_id, period_year, period_month, base_idr, discount_idr, net_idr, status, due_at, discount_snapshot)
		VALUES ($1, $2, $3, 2026, 9, 500000, 0, 500000, 'unpaid', '2026-09-10', '[]'::jsonb)
		RETURNING id::text
	`, schoolID, schemeID, studentID).Scan(&pastBillID)
	if err != nil {
		t.Fatalf("insert past bill: %v", err)
	}

	app := New(db, config.Config{}, stubIdentity{}, nil)
	srv := httptest.NewServer(newBillingTestRouter(app))
	defer srv.Close()

	// Call POST /internal/students/{id}/exit with effective_date = 2026-10-01 and policy = void
	exitPayload := map[string]any{
		"school_id":      schoolID,
		"effective_date": "2026-10-01",
		"policy":         "void",
	}
	body, _ := json.Marshal(exitPayload)

	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/internal/students/"+studentID+"/exit", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("exit request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK for exit, got %d", resp.StatusCode)
	}

	// Verify future bill was voided
	var futureStatus string
	_ = db.QueryRow(`SELECT status FROM student_bills WHERE id = $1`, futureBillID).Scan(&futureStatus)
	if futureStatus != "void" {
		t.Errorf("expected future bill to be voided, got %q", futureStatus)
	}

	// Verify past bill remained unpaid
	var pastStatus string
	_ = db.QueryRow(`SELECT status FROM student_bills WHERE id = $1`, pastBillID).Scan(&pastStatus)
	if pastStatus != "unpaid" {
		t.Errorf("expected past bill to remain unpaid, got %q", pastStatus)
	}
}

// Student refund endpoint with idempotency (FR-78-006, AC-78-03)
func TestInternalStudentRefund_Idempotency(t *testing.T) {
	db := setupBillingTestDB(t)
	defer db.Close()

	schoolID := newUUID()
	studentID := newUUID()
	idempotencyKey := fmt.Sprintf("refund-key-%s", newUUID())
	makerID := newUUID()

	core := &fakeCore{}
	coreSrv := httptest.NewServer(core.handler())
	defer coreSrv.Close()
	app := New(db, config.Config{}, stubIdentity{}, nil)
	app.Core = client.New(coreSrv.URL, "k")
	app.Config.Gateway.BillingInternalURL = "http://billing:8080"
	srv := httptest.NewServer(newBillingTestRouter(app))
	defer srv.Close()

	refundPayload := map[string]any{
		"school_id":       schoolID,
		"amount_idr":      200000,
		"reason":          "Kelebihan bayar mutasi keluar",
		"idempotency_key": idempotencyKey,
		"requested_by":    makerID,
	}

	// Core refuses (no approval flow): nothing is stored, the caller sees 422.
	core.refuse = true
	refusedBody, _ := json.Marshal(refundPayload)
	reqR, _ := http.NewRequest(http.MethodPost, srv.URL+"/internal/students/"+studentID+"/refund", bytes.NewReader(refusedBody))
	reqR.Header.Set("Content-Type", "application/json")
	respR, err := http.DefaultClient.Do(reqR)
	if err != nil {
		t.Fatalf("refused refund request: %v", err)
	}
	respR.Body.Close()
	if respR.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 when core refuses, got %d", respR.StatusCode)
	}
	var leftover int
	_ = db.QueryRow(`SELECT count(*) FROM bill_refunds WHERE school_id=$1`, schoolID).Scan(&leftover)
	if leftover != 0 {
		t.Fatalf("refused refund must not remain, found %d rows", leftover)
	}
	core.refuse = false
	body, _ := json.Marshal(refundPayload)

	// First call -> 201 Created
	req1, _ := http.NewRequest(http.MethodPost, srv.URL+"/internal/students/"+studentID+"/refund", bytes.NewReader(body))
	req1.Header.Set("Content-Type", "application/json")
	resp1, err := http.DefaultClient.Do(req1)
	if err != nil {
		t.Fatalf("first refund request: %v", err)
	}
	defer resp1.Body.Close()

	if resp1.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201 Created on first refund call, got %d", resp1.StatusCode)
	}

	var res1 struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	_ = json.NewDecoder(resp1.Body).Decode(&res1)
	if res1.Data.ID == "" {
		t.Fatalf("expected refund id in response")
	}
	appr := core.lastApproval(t)
	if appr.RequestType != reqTypeRefund || appr.SourceID != res1.Data.ID || appr.MakerUserID != makerID ||
		appr.AmountIDR == nil || *appr.AmountIDR != 200000 ||
		appr.CallbackURL != "http://billing:8080/internal/refunds/"+res1.Data.ID+"/callback" {
		t.Fatalf("approval request mismatch: %+v", appr)
	}
	var approvalID, requestedBy, status string
	_ = db.QueryRow(`SELECT COALESCE(approval_id,''),requested_by::text,status FROM bill_refunds WHERE id=$1`, res1.Data.ID).
		Scan(&approvalID, &requestedBy, &status)
	if approvalID == "" || requestedBy != makerID || status != "requested" {
		t.Fatalf("refund row: approval_id=%q requested_by=%q status=%q", approvalID, requestedBy, status)
	}

	// Second call with same idempotency key -> 200 OK with same ID
	body2, _ := json.Marshal(refundPayload)
	req2, _ := http.NewRequest(http.MethodPost, srv.URL+"/internal/students/"+studentID+"/refund", bytes.NewReader(body2))
	req2.Header.Set("Content-Type", "application/json")
	resp2, err := http.DefaultClient.Do(req2)
	if err != nil {
		t.Fatalf("second refund request: %v", err)
	}
	defer resp2.Body.Close()

	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK on idempotent retry, got %d", resp2.StatusCode)
	}

	var res2 struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	_ = json.NewDecoder(resp2.Body).Decode(&res2)
	if res2.Data.ID != res1.Data.ID {
		t.Errorf("expected idempotent refund ID %q, got %q", res1.Data.ID, res2.Data.ID)
	}
}

// Menu 78 financial summary reports the BR-FIN-04 deposit balance instead of
// a constant 0 (FR-78-002, AC-78-03).
func TestInternalStudentFinancialSummary_Deposit(t *testing.T) {
	db := setupBillingTestDB(t)
	defer db.Close()

	schoolID, studentID, otherID := newUUID(), newUUID(), newUUID()
	if _, err := db.Exec(`INSERT INTO student_deposits (school_id, student_user_id, balance_idr) VALUES ($1, $2, 200000)`,
		schoolID, studentID); err != nil {
		t.Fatalf("seed deposit: %v", err)
	}
	app := New(db, config.Config{}, stubIdentity{}, nil)
	srv := httptest.NewServer(newBillingTestRouter(app))
	defer srv.Close()

	get := func(id string) int64 {
		resp, err := http.Get(srv.URL + "/internal/students/" + id + "/financial-summary?school_id=" + schoolID)
		if err != nil {
			t.Fatalf("summary: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("summary status %d", resp.StatusCode)
		}
		var env struct {
			Data struct {
				DepositIDR int64 `json:"deposit_idr"`
			} `json:"data"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&env)
		return env.Data.DepositIDR
	}
	if got := get(studentID); got != 200000 {
		t.Fatalf("deposit_idr = %d, want 200000", got)
	}
	if got := get(otherID); got != 0 {
		t.Fatalf("deposit_idr without deposit row = %d, want 0", got)
	}
}
