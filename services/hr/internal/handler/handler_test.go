package handler_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"skci/hr/internal/client"
	"skci/hr/internal/common"
	"skci/hr/internal/config"
	"skci/hr/internal/crypto"
	"skci/hr/internal/handler"
	"skci/hr/internal/storage"
	"skci/hr/internal/store"
)

var (
	testApp     *handler.App
	testDB      *sql.DB
	testKey32   = "01234567890123456789012345678901"
	testBidxKey = "01234567890123456789012345678901"
	testJWT     = "test-jwt-secret-1234567890123456"
	testIntKey  = "test-internal-key-hr"
	schoolID    = "10000000-0000-0000-0000-000000000001"
)

func setupTestApp(t *testing.T) *handler.App {
	dsn := os.Getenv("TEST_PG_DSN")
	if dsn == "" {
		t.Skip("TEST_PG_DSN not set, skipping DB handler test")
	}

	var err error
	testDB, err = store.Open(dsn)
	if err != nil {
		t.Fatalf("failed to connect to DB: %v", err)
	}

	// Truncate tables for clean test slate
	ctx := context.Background()
	_, _ = testDB.ExecContext(ctx, `
		TRUNCATE TABLE audit_events, profile_change_requests, overtime_requests,
		leave_requests, leave_balances, leave_types, employee_attendance,
		attendance_periods, holidays, work_schedules, employee_documents,
		required_documents, document_requests, document_types, headcount_plans,
		employment_assignments, employees, positions, departments, hr_settings CASCADE;
	`)

	cip, err := crypto.New([]byte(testKey32), testBidxKey)
	if err != nil {
		t.Fatalf("cipher init failed: %v", err)
	}

	cfg := &config.Config{
		JWTSecret:   testJWT,
		InternalKey: testIntKey,
	}

	// Mock server for Identity and Core
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/features") || r.URL.Path == "/internal/features/hris" {
			// HRIS feature always enabled for test school
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"data":[{"feature_key":"hris","enabled":true}]}`))
			return
		}
		if r.URL.Path == "/internal/employees/create-or-link" {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"data":{"user_id":"20000000-0000-0000-0000-000000000001"}}`))
			return
		}
		if r.URL.Path == "/internal/approvals" {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"data":{"approval_id":"app-999"}}`))
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"data":{}}`))
	}))
	t.Cleanup(mockServer.Close)

	cli := client.NewClient(mockServer.URL, mockServer.URL, mockServer.URL, testIntKey)
	st := storage.NewLocalStorage(t.TempDir())

	testApp = handler.NewApp(testDB, cfg, cip, st, cli)
	return testApp
}

func createToken(userID, role string) string {
	claims := common.Claims{
		UserID:     userID,
		SchoolID:   schoolID,
		ActiveRole: role,
		Role:       role,
		Email:      userID + "@example.com",
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID,
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenStr, _ := tok.SignedString([]byte(testJWT))
	return tokenStr
}

func TestHandlerE2E(t *testing.T) {
	app := setupTestApp(t)
	router := app.Router()

	adminToken := createToken("10000000-0000-0000-0000-000000000001", common.RoleHRAdmin)
	pegawaiToken := createToken("20000000-0000-0000-0000-000000000001", common.RolePegawai)

	// 1. AC-84-01: Create Department and Position
	var deptID, posID string
	{
		deptBody := `{"code":"IT","name":"Teknologi Informasi"}`
		req := httptest.NewRequest("POST", "/departments", bytes.NewReader([]byte(deptBody)))
		req.Header.Set("Authorization", "Bearer "+adminToken)
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)
		if rr.Code != http.StatusCreated {
			t.Fatalf("POST /departments failed (%d): %s", rr.Code, rr.Body.String())
		}
		var res map[string]any
		json.Unmarshal(rr.Body.Bytes(), &res)
		data := res["data"].(map[string]any)
		deptID = data["id"].(string)

		posBody := fmt.Sprintf(`{"code":"DEV","name":"Software Engineer","department_id":"%s"}`, deptID)
		req = httptest.NewRequest("POST", "/positions", bytes.NewReader([]byte(posBody)))
		req.Header.Set("Authorization", "Bearer "+adminToken)
		req.Header.Set("Content-Type", "application/json")
		rr = httptest.NewRecorder()
		router.ServeHTTP(rr, req)
		if rr.Code != http.StatusCreated {
			t.Fatalf("POST /positions failed (%d): %s", rr.Code, rr.Body.String())
		}
		json.Unmarshal(rr.Body.Bytes(), &res)
		data = res["data"].(map[string]any)
		posID = data["id"].(string)
	}

	// 2. AC-83-01: Create Employee with NIK, NPWP, Bank Account (Encrypted)
	var empID string
	{
		empBody := fmt.Sprintf(`{
			"full_name": "Budi Santoso",
			"email": "budi@teratai.sch.id",
			"phone": "081234567890",
			"nik": "3201123456789001",
			"npwp": "091234567890000",
			"ptkp_status": "TK/0",
			"hire_date": "2026-01-01",
			"position_id": "%s",
			"department_id": "%s",
			"employment_type": "kontrak",
			"base_salary_idr": 8500000
		}`, posID, deptID)
		req := httptest.NewRequest("POST", "/employees", bytes.NewReader([]byte(empBody)))
		req.Header.Set("Authorization", "Bearer "+adminToken)
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)
		if rr.Code != http.StatusCreated {
			t.Fatalf("POST /employees failed (%d): %s", rr.Code, rr.Body.String())
		}
		var res map[string]any
		json.Unmarshal(rr.Body.Bytes(), &res)
		data := res["data"].(map[string]any)
		empID = data["id"].(string)

		// Link user_id to pegawai UUID for ESS tests
		_, err := testDB.ExecContext(context.Background(), `UPDATE employees SET user_id = '20000000-0000-0000-0000-000000000001' WHERE id = $1`, empID)
		if err != nil {
			t.Fatalf("failed to link user_id: %v", err)
		}
	}

	// 3. AC-83-02: Duplicate Check Rejection (Conflict 409)
	{
		dupBody := fmt.Sprintf(`{
			"full_name": "Budi Palsu",
			"email": "budi2@teratai.sch.id",
			"nik": "3201123456789001",
			"hire_date": "2026-02-01",
			"position_id": "%s"
		}`, posID)
		req := httptest.NewRequest("POST", "/employees", bytes.NewReader([]byte(dupBody)))
		req.Header.Set("Authorization", "Bearer "+adminToken)
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)
		if rr.Code != http.StatusConflict {
			t.Fatalf("expected 409 Conflict for duplicate NIK, got %d: %s", rr.Code, rr.Body.String())
		}
	}

	// 4. AC-83-03: Create Assignment / Career Transition (Closes active assignment)
	{
		assignBody := fmt.Sprintf(`{
			"position_id": "%s",
			"department_id": "%s",
			"employment_type": "tetap",
			"effective_from": "2026-07-01"
		}`, posID, deptID)
		req := httptest.NewRequest("POST", fmt.Sprintf("/employees/%s/assignments", empID), bytes.NewReader([]byte(assignBody)))
		req.Header.Set("Authorization", "Bearer "+adminToken)
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)
		if rr.Code != http.StatusCreated {
			t.Fatalf("POST /assignments failed (%d): %s", rr.Code, rr.Body.String())
		}

		// Verify timeline has 2 records
		req = httptest.NewRequest("GET", fmt.Sprintf("/employees/%s/timeline", empID), nil)
		req.Header.Set("Authorization", "Bearer "+adminToken)
		rr = httptest.NewRecorder()
		router.ServeHTTP(rr, req)
		var res map[string]any
		json.Unmarshal(rr.Body.Bytes(), &res)
		list := res["data"].([]any)
		if len(list) != 2 {
			t.Fatalf("expected 2 timeline rows, got %d", len(list))
		}
	}

	// 5. AC-85-01: Document Type, Upload and Completeness
	var docTypeID string
	{
		dtBody := `{"code":"KTP","name":"Kartu Tanda Penduduk","has_expiry":false}`
		req := httptest.NewRequest("POST", "/document-types", bytes.NewReader([]byte(dtBody)))
		req.Header.Set("Authorization", "Bearer "+adminToken)
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)
		var res map[string]any
		json.Unmarshal(rr.Body.Bytes(), &res)
		docTypeID = res["data"].(map[string]any)["id"].(string)

		// Upload document multipart
		var buf bytes.Buffer
		w := multipart.NewWriter(&buf)
		w.WriteField("document_type_id", docTypeID)
		part, _ := w.CreateFormFile("file", "ktp.pdf")
		part.Write([]byte("%PDF-1.4 dummy content"))
		w.Close()

		req = httptest.NewRequest("POST", fmt.Sprintf("/employees/%s/documents", empID), &buf)
		req.Header.Set("Authorization", "Bearer "+adminToken)
		req.Header.Set("Content-Type", w.FormDataContentType())
		rr = httptest.NewRecorder()
		router.ServeHTTP(rr, req)
		if rr.Code != http.StatusCreated {
			t.Fatalf("POST /documents failed (%d): %s", rr.Code, rr.Body.String())
		}

		// Download document
		json.Unmarshal(rr.Body.Bytes(), &res)
		docID := res["data"].(map[string]any)["id"].(string)
		req = httptest.NewRequest("GET", fmt.Sprintf("/documents/%s/download", docID), nil)
		req.Header.Set("Authorization", "Bearer "+pegawaiToken)
		rr = httptest.NewRecorder()
		router.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("GET /documents/{id}/download failed (%d): %s", rr.Code, rr.Body.String())
		}
	}

	// 6. AC-86-01: Geofence Check-in via ESS
	{
		// Configure school geofence (-6.200000, 106.816666, 100m)
		settingsBody := `{"school_lat":-6.200000,"school_lng":106.816666,"geofence_radius_m":100,"late_tolerance_minutes":15}`
		req := httptest.NewRequest("PUT", "/settings/hr", bytes.NewReader([]byte(settingsBody)))
		req.Header.Set("Authorization", "Bearer "+adminToken)
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)

		// Check-in far away -> should be rejected (>100m)
		farCheckIn := `{"lat":-6.300000,"lng":106.900000}`
		req = httptest.NewRequest("POST", "/me/attendance/check-in", bytes.NewReader([]byte(farCheckIn)))
		req.Header.Set("Authorization", "Bearer "+pegawaiToken)
		req.Header.Set("Content-Type", "application/json")
		rr = httptest.NewRecorder()
		router.ServeHTTP(rr, req)
		if rr.Code != http.StatusBadRequest {
			t.Fatalf("expected check-in to be rejected outside geofence, got %d", rr.Code)
		}

		// Check-in on site -> should succeed
		nearCheckIn := `{"lat":-6.200010,"lng":106.816670}`
		req = httptest.NewRequest("POST", "/me/attendance/check-in", bytes.NewReader([]byte(nearCheckIn)))
		req.Header.Set("Authorization", "Bearer "+pegawaiToken)
		req.Header.Set("Content-Type", "application/json")
		rr = httptest.NewRecorder()
		router.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("check-in on site failed (%d): %s", rr.Code, rr.Body.String())
		}

		// Check-out
		req = httptest.NewRequest("POST", "/me/attendance/check-out", nil)
		req.Header.Set("Authorization", "Bearer "+pegawaiToken)
		rr = httptest.NewRecorder()
		router.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("check-out failed (%d): %s", rr.Code, rr.Body.String())
		}
	}

	// 7. AC-86-02: Leave Request, Approval, Cancellation & Revert
	{
		// Create leave type
		ltBody := `{"code":"CT","name":"Cuti Tahunan","has_quota":true,"annual_quota_days":12,"accrual":"none","carry_over_max_days":5}`
		req := httptest.NewRequest("POST", "/leave-types", bytes.NewReader([]byte(ltBody)))
		req.Header.Set("Authorization", "Bearer "+adminToken)
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)
		var res map[string]any
		json.Unmarshal(rr.Body.Bytes(), &res)
		leaveTypeID := res["data"].(map[string]any)["id"].(string)

		// Seed balance for 2026: 12 days
		_ = store.EnsureLeaveBalance(context.Background(), testDB, schoolID, empID, leaveTypeID, 2026, 12.0)

		// Submit leave request for 2 days (Monday 2026-11-02 to Tuesday 2026-11-03)
		leaveReqBody := fmt.Sprintf(`{
			"leave_type_id": "%s",
			"start_date": "2026-11-02",
			"end_date": "2026-11-03",
			"reason": "Liburan keluarga",
			"idempotency_key": "lr-test-e2e"
		}`, leaveTypeID)
		req = httptest.NewRequest("POST", "/me/leave-requests", bytes.NewReader([]byte(leaveReqBody)))
		req.Header.Set("Authorization", "Bearer "+pegawaiToken)
		req.Header.Set("Content-Type", "application/json")
		rr = httptest.NewRecorder()
		router.ServeHTTP(rr, req)
		if rr.Code != http.StatusCreated {
			t.Fatalf("POST /me/leave-requests failed (%d): %s", rr.Code, rr.Body.String())
		}
		json.Unmarshal(rr.Body.Bytes(), &res)
		leaveReqID := res["data"].(map[string]any)["id"].(string)

		// Approval callback from Core
		callbackBody := fmt.Sprintf(`{
			"approval_id": "app-999",
			"resource_type": "leave_request",
			"resource_id": "%s",
			"decision": "approved"
		}`, leaveReqID)
		req = httptest.NewRequest("POST", "/internal/approval-callbacks", bytes.NewReader([]byte(callbackBody)))
		req.Header.Set("X-Internal-Key", testIntKey)
		req.Header.Set("Content-Type", "application/json")
		rr = httptest.NewRecorder()
		router.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("approval callback failed (%d): %s", rr.Code, rr.Body.String())
		}

		// Verify balance used is 2
		bal, _ := store.GetLeaveBalance(context.Background(), testDB, schoolID, empID, leaveTypeID, 2026)
		if bal.Used != 2.0 {
			t.Fatalf("expected used=2.0 after approval, got %f", bal.Used)
		}

		// Cancel leave request via ESS -> balance used reverts to 0
		req = httptest.NewRequest("POST", fmt.Sprintf("/me/leave-requests/%s/cancel", leaveReqID), nil)
		req.Header.Set("Authorization", "Bearer "+pegawaiToken)
		rr = httptest.NewRecorder()
		router.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("cancel leave request failed (%d): %s", rr.Code, rr.Body.String())
		}

		bal, _ = store.GetLeaveBalance(context.Background(), testDB, schoolID, empID, leaveTypeID, 2026)
		if bal.Used != 0 {
			t.Fatalf("expected used=0 after cancel, got %f", bal.Used)
		}
	}

	// 8. AC-86-04: Lock Attendance Period
	{
		period := "2026-08"
		req := httptest.NewRequest("POST", fmt.Sprintf("/attendance-periods/%s/send", period), nil)
		req.Header.Set("Authorization", "Bearer "+adminToken)
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("POST /attendance-periods/send failed (%d): %s", rr.Code, rr.Body.String())
		}

		// Check locked in DB
		locked, _ := store.IsPeriodLocked(context.Background(), testDB, schoolID, period)
		if !locked {
			t.Fatalf("expected period %s to be locked", period)
		}
	}

	// 9. AC-87-01: Profile Change Request via ESS & Approval Callback
	{
		pcrBody := `{"field":"ptkp_status","new_value":"K/2"}`
		req := httptest.NewRequest("POST", "/me/profile-change-requests", bytes.NewReader([]byte(pcrBody)))
		req.Header.Set("Authorization", "Bearer "+pegawaiToken)
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)
		if rr.Code != http.StatusCreated {
			t.Fatalf("POST /me/profile-change-requests failed (%d): %s", rr.Code, rr.Body.String())
		}
		var res map[string]any
		json.Unmarshal(rr.Body.Bytes(), &res)
		pcrID := res["data"].(map[string]any)["id"].(string)

		// Callback approval
		callbackBody := fmt.Sprintf(`{
			"approval_id": "app-888",
			"resource_type": "profile_change_request",
			"resource_id": "%s",
			"decision": "approved"
		}`, pcrID)
		req = httptest.NewRequest("POST", "/internal/approval-callbacks", bytes.NewReader([]byte(callbackBody)))
		req.Header.Set("X-Internal-Key", testIntKey)
		req.Header.Set("Content-Type", "application/json")
		rr = httptest.NewRecorder()
		router.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("profile change approval callback failed: %s", rr.Body.String())
		}

		// Verify employee PTKP status updated to K/2
		emp, _ := store.GetEmployeeByID(context.Background(), testDB, schoolID, empID)
		if emp.PTKPStatus != "K/2" {
			t.Fatalf("expected updated PTKP status 'K/2', got '%s'", emp.PTKPStatus)
		}
	}

	// 10. Role Gate Authorization: Pegawai cannot access /employees (Forbidden 403)
	{
		req := httptest.NewRequest("GET", "/employees", nil)
		req.Header.Set("Authorization", "Bearer "+pegawaiToken)
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)
		if rr.Code != http.StatusForbidden {
			t.Fatalf("expected 403 Forbidden for pegawai accessing /employees, got %d", rr.Code)
		}
	}
}
