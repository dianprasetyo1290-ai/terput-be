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
	// approvalsSeen records every POST /internal/approvals body sent to the
	// fake core, keyed by request_type.
	approvalsSeen = map[string]map[string]any{}
	// coreRejects makes the fake core answer 422 like a missing approval flow.
	coreRejects  = false
	testCallback = "http://hr.internal:8092/internal/approval-callbacks"
)

const fakeApprovalID = "30000000-0000-0000-0000-000000000099"

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
		payroll_runs, requisitions,
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
		if r.URL.Path == "/internal/approvals" && coreRejects {
			w.WriteHeader(http.StatusUnprocessableEntity)
			w.Write([]byte(`{"message":"alur approval overtime belum dikonfigurasi"}`))
			return
		}
		if r.URL.Path == "/internal/approvals" {
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			rt, _ := body["request_type"].(string)
			approvalsSeen[rt] = body
			w.WriteHeader(http.StatusCreated)
			w.Write([]byte(`{"data":{"id":"` + fakeApprovalID + `","status":"pending","current_seq":1}}`))
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"data":{}}`))
	}))
	t.Cleanup(mockServer.Close)

	cli := client.NewClient(mockServer.URL, mockServer.URL, mockServer.URL, testIntKey)
	cli.CallbackURL = testCallback
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

		// Approval opened in core with the T1 contract: subject = requester
		// (core resolves the direct supervisor), callback to hr's own URL.
		sent := approvalsSeen["leave"]
		if sent == nil {
			t.Fatalf("expected leave approval sent to core, got %v", approvalsSeen)
		}
		pegawaiID := "20000000-0000-0000-0000-000000000001"
		if sent["subject_user_id"] != pegawaiID || sent["maker_user_id"] != pegawaiID {
			t.Fatalf("subject/maker must be the requester, got subject=%v maker=%v", sent["subject_user_id"], sent["maker_user_id"])
		}
		if sent["callback_url"] != testCallback || sent["source_service"] != "hr" || sent["source_id"] != leaveReqID {
			t.Fatalf("unexpected approval contract: %v", sent)
		}

		// Approval callback exactly as core sends it (core jobs.CallbackPayload),
		// delivered twice: the retry must not double-count the balance.
		callbackBody := fmt.Sprintf(`{
			"approval_id": "%s",
			"source_id": "%s",
			"request_type": "leave",
			"decision": "approved",
			"decided_by": "40000000-0000-0000-0000-000000000001",
			"comment": "",
			"decided_at": "2026-10-12T08:00:00+07:00"
		}`, fakeApprovalID, leaveReqID)
		for i := 0; i < 2; i++ {
			req = httptest.NewRequest("POST", "/internal/approval-callbacks", bytes.NewReader([]byte(callbackBody)))
			req.Header.Set("X-Internal-Key", testIntKey)
			req.Header.Set("Idempotency-Key", "approval:"+fakeApprovalID)
			req.Header.Set("Content-Type", "application/json")
			rr = httptest.NewRecorder()
			router.ServeHTTP(rr, req)
			if rr.Code != http.StatusOK {
				t.Fatalf("approval callback #%d failed (%d): %s", i+1, rr.Code, rr.Body.String())
			}
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

		if approvalsSeen["profile_change"] == nil {
			t.Fatalf("expected profile_change approval sent to core")
		}
		// Callback approval (core payload)
		callbackBody := fmt.Sprintf(`{
			"approval_id": "%s",
			"source_id": "%s",
			"request_type": "profile_change",
			"decision": "approved",
			"decided_by": "40000000-0000-0000-0000-000000000001",
			"comment": "",
			"decided_at": "2026-10-12T08:00:00+07:00"
		}`, fakeApprovalID, pcrID)
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

	// 9b. Overtime: core refusal is surfaced (nothing stored), a rejected
	// decision is applied, and malformed/legacy callbacks are refused.
	{
		otBody := `{"work_date":"2026-11-04","start_at":"2026-11-04 17:00","end_at":"2026-11-04 19:00","minutes":120,"reason":"Rapat","idempotency_key":"ot-e2e"}`
		post := func(path, body, token string) *httptest.ResponseRecorder {
			req := httptest.NewRequest("POST", path, bytes.NewReader([]byte(body)))
			if token != "" {
				req.Header.Set("Authorization", "Bearer "+token)
			} else {
				req.Header.Set("X-Internal-Key", testIntKey)
			}
			req.Header.Set("Content-Type", "application/json")
			rr := httptest.NewRecorder()
			router.ServeHTTP(rr, req)
			return rr
		}

		coreRejects = true
		rr := post("/me/overtime-requests", otBody, pegawaiToken)
		coreRejects = false
		if rr.Code != http.StatusUnprocessableEntity || !strings.Contains(rr.Body.String(), "belum dikonfigurasi") {
			t.Fatalf("expected 422 with core message, got %d: %s", rr.Code, rr.Body.String())
		}
		var n int
		_ = testDB.QueryRow(`SELECT COUNT(*) FROM overtime_requests WHERE idempotency_key = 'ot-e2e'`).Scan(&n)
		if n != 0 {
			t.Fatalf("overtime must not be stored when core refuses, found %d", n)
		}

		rr = post("/me/overtime-requests", otBody, pegawaiToken)
		if rr.Code != http.StatusCreated {
			t.Fatalf("POST /me/overtime-requests failed (%d): %s", rr.Code, rr.Body.String())
		}
		var res map[string]any
		json.Unmarshal(rr.Body.Bytes(), &res)
		otID := res["data"].(map[string]any)["id"].(string)

		reject := fmt.Sprintf(`{"approval_id":"%s","source_id":"%s","request_type":"overtime","decision":"rejected","decided_by":"40000000-0000-0000-0000-000000000001","comment":"Tidak mendesak","decided_at":"2026-10-12T08:00:00+07:00"}`, fakeApprovalID, otID)
		if rr = post("/internal/approval-callbacks", reject, ""); rr.Code != http.StatusOK {
			t.Fatalf("overtime reject callback failed (%d): %s", rr.Code, rr.Body.String())
		}
		// A late "approved" for an already-final request is a no-op.
		approve := strings.Replace(reject, `"rejected"`, `"approved"`, 1)
		if rr = post("/internal/approval-callbacks", approve, ""); rr.Code != http.StatusOK {
			t.Fatalf("repeated callback must be 200, got %d: %s", rr.Code, rr.Body.String())
		}
		var status string
		_ = testDB.QueryRow(`SELECT status FROM overtime_requests WHERE id = $1`, otID).Scan(&status)
		if status != "rejected" {
			t.Fatalf("expected overtime rejected, got %s", status)
		}

		legacy := fmt.Sprintf(`{"approval_id":"x","resource_type":"overtime_request","resource_id":"%s","decision":"approved"}`, otID)
		if rr = post("/internal/approval-callbacks", legacy, ""); rr.Code != http.StatusBadRequest {
			t.Fatalf("legacy payload must be 400, got %d", rr.Code)
		}
		unknown := strings.Replace(approve, `"overtime"`, `"refund"`, 1)
		if rr = post("/internal/approval-callbacks", unknown, ""); rr.Code != http.StatusUnprocessableEntity {
			t.Fatalf("unknown request_type must be 422, got %d", rr.Code)
		}
		missing := strings.Replace(approve, otID, "30000000-0000-0000-0000-000000000000", 1)
		if rr = post("/internal/approval-callbacks", missing, ""); rr.Code != http.StatusNotFound {
			t.Fatalf("unknown source_id must be 404, got %d", rr.Code)
		}
	}

	// 9c. T3/T4 approvals go through core: payroll_run (amount = total net)
	// and requisition, each applied by the shared callback.
	{
		call := func(method, path, body string, internal bool) *httptest.ResponseRecorder {
			req := httptest.NewRequest(method, path, bytes.NewReader([]byte(body)))
			if internal {
				req.Header.Set("X-Internal-Key", testIntKey)
			} else {
				req.Header.Set("Authorization", "Bearer "+adminToken)
			}
			req.Header.Set("Content-Type", "application/json")
			rr := httptest.NewRecorder()
			router.ServeHTTP(rr, req)
			return rr
		}
		callback := func(rt, id string) string {
			return fmt.Sprintf(`{"approval_id":"%s","source_id":"%s","request_type":"%s","decision":"approved","decided_by":"40000000-0000-0000-0000-000000000001","comment":"","decided_at":"2026-10-12T08:00:00+07:00"}`, fakeApprovalID, id, rt)
		}

		var runID string
		if err := testDB.QueryRow(`INSERT INTO payroll_runs (school_id, period, kind, status, calculated_at, idempotency_key)
			VALUES ($1, '2026-09', 'regular', 'calculated', now(), 'run-e2e') RETURNING id`, schoolID).Scan(&runID); err != nil {
			t.Fatalf("seed payroll run: %v", err)
		}
		if rr := call("POST", "/payroll/runs/"+runID+"/submit", "", false); rr.Code != http.StatusOK {
			t.Fatalf("submit payroll run failed (%d): %s", rr.Code, rr.Body.String())
		}
		if sent := approvalsSeen["payroll_run"]; sent == nil || sent["source_id"] != runID || sent["callback_url"] != testCallback {
			t.Fatalf("payroll_run approval not sent with contract: %v", sent)
		}
		if rr := call("POST", "/payroll/runs/"+runID+"/finalize", "", false); rr.Code != http.StatusConflict {
			t.Fatalf("finalize before approval must be 409, got %d: %s", rr.Code, rr.Body.String())
		}
		for i := 0; i < 2; i++ {
			if rr := call("POST", "/internal/approval-callbacks", callback("payroll_run", runID), true); rr.Code != http.StatusOK {
				t.Fatalf("payroll_run callback failed (%d): %s", rr.Code, rr.Body.String())
			}
		}
		var status string
		_ = testDB.QueryRow(`SELECT status FROM payroll_runs WHERE id = $1`, runID).Scan(&status)
		if status != "approved" {
			t.Fatalf("expected payroll run approved, got %s", status)
		}

		reqBody := fmt.Sprintf(`{"position_id":"%s","quantity":1,"reason":"addition","idempotency_key":"rq-e2e"}`, posID)
		rr := call("POST", "/ats/requisitions", reqBody, false)
		if rr.Code != http.StatusCreated {
			t.Fatalf("create requisition failed (%d): %s", rr.Code, rr.Body.String())
		}
		var res map[string]any
		json.Unmarshal(rr.Body.Bytes(), &res)
		reqID := res["data"].(map[string]any)["id"].(string)
		if rr = call("POST", "/ats/requisitions/"+reqID+"/submit", "", false); rr.Code != http.StatusOK {
			t.Fatalf("submit requisition failed (%d): %s", rr.Code, rr.Body.String())
		}
		if sent := approvalsSeen["requisition"]; sent == nil || sent["source_id"] != reqID {
			t.Fatalf("requisition approval not sent: %v", sent)
		}
		if rr = call("POST", "/internal/approval-callbacks", callback("requisition", reqID), true); rr.Code != http.StatusOK {
			t.Fatalf("requisition callback failed (%d): %s", rr.Code, rr.Body.String())
		}
		_ = testDB.QueryRow(`SELECT status FROM requisitions WHERE id = $1`, reqID).Scan(&status)
		if status != "approved" {
			t.Fatalf("expected requisition approved, got %s", status)
		}
	}

	// 11. T5 Training Self-Enrollment & Paid Training Approval (training_paid)
	{
		call := func(method, path, body string, token string) *httptest.ResponseRecorder {
			req := httptest.NewRequest(method, path, bytes.NewReader([]byte(body)))
			if token == "internal" {
				req.Header.Set("X-Internal-Key", testIntKey)
			} else if token != "" {
				req.Header.Set("Authorization", "Bearer "+token)
			}
			req.Header.Set("Content-Type", "application/json")
			rr := httptest.NewRecorder()
			router.ServeHTTP(rr, req)
			return rr
		}

		// Seed a free course and a paid course
		var freeCourseID, paidCourseID string
		if err := testDB.QueryRow(`
			INSERT INTO courses (school_id, title, objective, duration_minutes, provider, kind, cost_idr, quota, status)
			VALUES ($1, 'Kursus Gratis', 'Tujuan', 60, 'Internal', 'online', 0, 50, 'published')
			RETURNING id
		`, schoolID).Scan(&freeCourseID); err != nil {
			t.Fatalf("seed free course: %v", err)
		}
		if err := testDB.QueryRow(`
			INSERT INTO courses (school_id, title, objective, duration_minutes, provider, kind, cost_idr, quota, status)
			VALUES ($1, 'Sertifikasi Berbayar', 'Tujuan', 120, 'Vendor Luar', 'online', 750000, 20, 'published')
			RETURNING id
		`, schoolID).Scan(&paidCourseID); err != nil {
			t.Fatalf("seed paid course: %v", err)
		}

		// 11a. Free course enrolls immediately as not_started
		enrollFree := fmt.Sprintf(`{"course_id":"%s"}`, freeCourseID)
		rr := call("POST", "/me/training/enroll", enrollFree, pegawaiToken)
		if rr.Code != http.StatusCreated {
			t.Fatalf("enroll free course failed (%d): %s", rr.Code, rr.Body.String())
		}
		var freeRes map[string]any
		json.Unmarshal(rr.Body.Bytes(), &freeRes)
		freeAssign := freeRes["data"].(map[string]any)
		if freeAssign["status"] != "not_started" {
			t.Fatalf("expected free course status not_started, got %v", freeAssign["status"])
		}

		// 11b. Paid course creates pending_approval and opens Core approval (training_paid)
		enrollPaid := fmt.Sprintf(`{"course_id":"%s"}`, paidCourseID)
		rr = call("POST", "/me/training/enroll", enrollPaid, pegawaiToken)
		if rr.Code != http.StatusCreated {
			t.Fatalf("enroll paid course failed (%d): %s", rr.Code, rr.Body.String())
		}
		var paidRes map[string]any
		json.Unmarshal(rr.Body.Bytes(), &paidRes)
		paidAssign := paidRes["data"].(map[string]any)
		if paidAssign["status"] != "pending_approval" {
			t.Fatalf("expected paid course status pending_approval, got %v", paidAssign["status"])
		}
		paidAssignID := paidAssign["id"].(string)

		sent := approvalsSeen["training_paid"]
		if sent == nil || sent["source_id"] != paidAssignID {
			t.Fatalf("training_paid approval not sent with contract: %v", sent)
		}
		if int64(sent["amount_idr"].(float64)) != 750000 {
			t.Fatalf("expected amount_idr 750000, got %v", sent["amount_idr"])
		}

		// 11c. Core callback approved sets status to not_started
		cbPayload := fmt.Sprintf(`{"approval_id":"%s","source_id":"%s","request_type":"training_paid","decision":"approved","decided_by":"40000000-0000-0000-0000-000000000001","comment":"","decided_at":"2026-10-12T08:00:00+07:00"}`, fakeApprovalID, paidAssignID)
		if rr = call("POST", "/internal/approval-callbacks", cbPayload, "internal"); rr.Code != http.StatusOK {
			t.Fatalf("training_paid callback failed (%d): %s", rr.Code, rr.Body.String())
		}
		var finalStatus string
		_ = testDB.QueryRow(`SELECT status FROM training_assignments WHERE id = $1`, paidAssignID).Scan(&finalStatus)
		if finalStatus != "not_started" {
			t.Fatalf("expected assignment status not_started after approve, got %s", finalStatus)
		}
	}

	// 12. T4 ATS Offer Workflow & Core Approval (offer, salary_out_of_range)
	{
		call := func(method, path, body string, internal bool) *httptest.ResponseRecorder {
			req := httptest.NewRequest(method, path, bytes.NewReader([]byte(body)))
			if internal {
				req.Header.Set("X-Internal-Key", testIntKey)
			} else {
				req.Header.Set("Authorization", "Bearer "+adminToken)
			}
			req.Header.Set("Content-Type", "application/json")
			rr := httptest.NewRecorder()
			router.ServeHTTP(rr, req)
			return rr
		}

		// Seed candidate, application
		var candID, appID string
		candEmail := fmt.Sprintf("kandidat-%d@example.com", time.Now().UnixNano())
		if err := testDB.QueryRow(`
			INSERT INTO candidates (school_id, full_name, email, email_norm, consent_processing_at)
			VALUES ($1, 'Kandidat Uji', $2, $2, now())
			RETURNING id
		`, schoolID, candEmail).Scan(&candID); err != nil {
			t.Fatalf("seed candidate: %v", err)
		}

		var postID string
		var reqID string
		reqKey := fmt.Sprintf("req-offer-%d", time.Now().UnixNano())
		if err := testDB.QueryRow(`
			INSERT INTO requisitions (school_id, position_id, quantity, reason, status, exceeds_headcount, requested_by, idempotency_key)
			VALUES ($1, $2, 1, 'addition', 'approved', false, '40000000-0000-0000-0000-000000000001', $3)
			RETURNING id
		`, schoolID, posID, reqKey).Scan(&reqID); err != nil {
			t.Fatalf("seed req for offer: %v", err)
		}
		postSlug := fmt.Sprintf("lowongan-offer-%d", time.Now().UnixNano())
		if err := testDB.QueryRow(`
			INSERT INTO job_postings (school_id, requisition_id, slug, title, description, qualifications, employment_type, status)
			VALUES ($1, $2, $3, 'Guru Fisika', 'Desc', 'Qual', 'tetap', 'open')
			RETURNING id
		`, schoolID, reqID, postSlug).Scan(&postID); err != nil {
			t.Fatalf("seed posting for offer: %v", err)
		}
		var stageID string
		stageCode := fmt.Sprintf("stg-%d", time.Now().UnixNano()%100000)
		if err := testDB.QueryRow(`
			INSERT INTO pipeline_stages (school_id, code, name, sort, kind)
			VALUES ($1, $2, 'Lamaran Masuk', 1, 'normal')
			RETURNING id
		`, schoolID, stageCode).Scan(&stageID); err != nil {
			t.Fatalf("seed stage for offer: %v", err)
		}
		if err := testDB.QueryRow(`
			INSERT INTO applications (school_id, posting_id, candidate_id, stage_id, status, access_token_hash)
			VALUES ($1, $2, $3, $4, 'active', 'tok-hash-test')
			RETURNING id
		`, schoolID, postID, candID, stageID).Scan(&appID); err != nil {
			t.Fatalf("seed app for offer: %v", err)
		}
		// Update position with salary range [3m, 5m]
		if _, err := testDB.Exec(`UPDATE positions SET salary_min_idr = 3000000, salary_max_idr = 5000000 WHERE id = $1`, posID); err != nil {
			t.Fatalf("update pos salary range: %v", err)
		}

		// Create offer with salary 6.000.000 (exceeds position salary max 5.000.000 -> salary_out_of_range)
		offerBody := fmt.Sprintf(`{
			"application_id": "%s",
			"position_id": "%s",
			"start_date": "2026-11-01",
			"employment_type": "tetap",
			"base_salary_idr": 6000000
		}`, appID, posID)
		rr := call("POST", "/ats/offers", offerBody, false)
		if rr.Code != http.StatusCreated {
			t.Fatalf("create offer failed (%d): %s", rr.Code, rr.Body.String())
		}
		var oRes map[string]any
		json.Unmarshal(rr.Body.Bytes(), &oRes)
		offerID := oRes["data"].(map[string]any)["id"].(string)

		// 12a. Send offer before approved must fail with 409
		if rr = call("POST", "/ats/offers/"+offerID+"/send", "", false); rr.Code != http.StatusConflict {
			t.Fatalf("send before approved must be 409, got %d", rr.Code)
		}

		// 12b. Submit offer opens Core approval with salary_out_of_range flag
		if rr = call("POST", "/ats/offers/"+offerID+"/submit", "", false); rr.Code != http.StatusOK {
			t.Fatalf("submit offer failed (%d): %s", rr.Code, rr.Body.String())
		}
		sent := approvalsSeen["offer"]
		if sent == nil || sent["source_id"] != offerID {
			t.Fatalf("offer approval not sent to Core: %v", sent)
		}
		flags, _ := sent["flags"].([]any)
		hasFlag := false
		for _, f := range flags {
			if f == "salary_out_of_range" {
				hasFlag = true
			}
		}
		if !hasFlag {
			t.Fatalf("expected flag salary_out_of_range in offer approval, got: %v", flags)
		}

		// 12c. Callback approved
		cbPayload := fmt.Sprintf(`{"approval_id":"%s","source_id":"%s","request_type":"offer","decision":"approved","decided_by":"40000000-0000-0000-0000-000000000001","comment":"","decided_at":"2026-10-12T08:00:00+07:00"}`, fakeApprovalID, offerID)
		if rr = call("POST", "/internal/approval-callbacks", cbPayload, true); rr.Code != http.StatusOK {
			t.Fatalf("offer callback failed (%d): %s", rr.Code, rr.Body.String())
		}
		var oStatus string
		_ = testDB.QueryRow(`SELECT status FROM offers WHERE id = $1`, offerID).Scan(&oStatus)
		if oStatus != "approved" {
			t.Fatalf("expected offer status approved, got %s", oStatus)
		}

		// 12d. Send offer after approved succeeds
		if rr = call("POST", "/ats/offers/"+offerID+"/send", "", false); rr.Code != http.StatusOK {
			t.Fatalf("send offer failed (%d): %s", rr.Code, rr.Body.String())
		}
		_ = testDB.QueryRow(`SELECT status FROM offers WHERE id = $1`, offerID).Scan(&oStatus)
		if oStatus != "sent" {
			t.Fatalf("expected offer status sent, got %s", oStatus)
		}
	}

	// 13. T3 Payroll Component Change & Input Change after Cutoff
	{
		call := func(method, path, body string, internal bool) *httptest.ResponseRecorder {
			req := httptest.NewRequest(method, path, bytes.NewReader([]byte(body)))
			if internal {
				req.Header.Set("X-Internal-Key", testIntKey)
			} else {
				req.Header.Set("Authorization", "Bearer "+adminToken)
			}
			req.Header.Set("Content-Type", "application/json")
			rr := httptest.NewRecorder()
			router.ServeHTTP(rr, req)
			return rr
		}

		// 13a. Component approval submission
		compCode := fmt.Sprintf("TJ_%d", time.Now().UnixNano()%100000)
		compBody := fmt.Sprintf(`{"code":"%s","name":"Tunjangan Uji","kind":"earning","nature":"fixed","taxable":true,"bpjs_base":false,"calc":"fixed_amount","sort":10,"acc_code":"5103"}`, compCode)
		rr := call("POST", "/payroll/components", compBody, false)
		if rr.Code != http.StatusCreated {
			t.Fatalf("create component failed (%d): %s", rr.Code, rr.Body.String())
		}
		var compRes map[string]any
		json.Unmarshal(rr.Body.Bytes(), &compRes)
		compID := compRes["data"].(map[string]any)["id"].(string)

		if rr = call("POST", "/payroll/components/"+compID+"/submit-approval", "", false); rr.Code != http.StatusOK {
			t.Fatalf("submit component approval failed (%d): %s", rr.Code, rr.Body.String())
		}
		sent := approvalsSeen["payroll_component_change"]
		if sent == nil || sent["source_id"] != compID {
			t.Fatalf("payroll_component_change approval not sent: %v", sent)
		}
		cbPayload := fmt.Sprintf(`{"approval_id":"%s","source_id":"%s","request_type":"payroll_component_change","decision":"approved","decided_by":"40000000-0000-0000-0000-000000000001","comment":"","decided_at":"2026-10-12T08:00:00+07:00"}`, fakeApprovalID, compID)
		if rr = call("POST", "/internal/approval-callbacks", cbPayload, true); rr.Code != http.StatusOK {
			t.Fatalf("component approval callback failed (%d): %s", rr.Code, rr.Body.String())
		}

		// 13b. Payroll input change after cutoff date
		yesterday := time.Now().AddDate(0, 0, -2).Format("2006-01-02")
		payDate := time.Now().AddDate(0, 0, 5).Format("2006-01-02")
		_, _ = testDB.Exec(`
			INSERT INTO payroll_calendars (school_id, period, cutoff_date, pay_date)
			VALUES ($1, '2026-10', $2, $3)
			ON CONFLICT (school_id, period) DO UPDATE SET cutoff_date = $2
		`, schoolID, yesterday, payDate)

		inputBody := fmt.Sprintf(`{
			"period": "2026-10",
			"employee_id": "%s",
			"component_id": "%s",
			"amount_idr": 450000,
			"reason": "Tambahan tugas khusus setelah cutoff"
		}`, empID, compID)

		rr = call("POST", "/payroll/inputs", inputBody, false)
		if rr.Code != http.StatusAccepted {
			t.Fatalf("input past cutoff must be 202 Accepted, got %d: %s", rr.Code, rr.Body.String())
		}
		var inpRes map[string]any
		json.Unmarshal(rr.Body.Bytes(), &inpRes)
		changeID := inpRes["data"].(map[string]any)["change_id"].(string)

		sentInput := approvalsSeen["payroll_input_change"]
		if sentInput == nil || sentInput["source_id"] != changeID {
			t.Fatalf("payroll_input_change approval not sent: %v", sentInput)
		}

		// Callback approved
		cbInputPayload := fmt.Sprintf(`{"approval_id":"%s","source_id":"%s","request_type":"payroll_input_change","decision":"approved","decided_by":"40000000-0000-0000-0000-000000000001","comment":"","decided_at":"2026-10-12T08:00:00+07:00"}`, fakeApprovalID, changeID)
		if rr = call("POST", "/internal/approval-callbacks", cbInputPayload, true); rr.Code != http.StatusOK {
			t.Fatalf("input change callback failed (%d): %s", rr.Code, rr.Body.String())
		}

		var appliedAmount int64
		err := testDB.QueryRow(`SELECT amount_idr FROM payroll_inputs WHERE school_id = $1 AND period = '2026-10' AND employee_id = $2 AND component_id = $3`, schoolID, empID, compID).Scan(&appliedAmount)
		if err != nil || appliedAmount != 450000 {
			t.Fatalf("expected applied amount 450000, got %d (err: %v)", appliedAmount, err)
		}
	}

	// 14. Role Gate Authorization: Pegawai cannot access /employees (Forbidden 403)
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
