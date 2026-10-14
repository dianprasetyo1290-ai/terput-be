package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"skci/core/internal/jobs"
	"skci/core/internal/model"
)

// Helper to seed standard refund flow:
// Step 1: seq 1, role finance_manager
// Step 2: seq 2, role kepala_sekolah, min_amount 5.000.000
func seedRefundFlow(t *testing.T, fs *fakeStore) {
	t.Helper()
	_, err := fs.CreateFlow(context.Background(), testSchoolA, testUserAdmin, model.ApprovalFlow{
		RequestType: model.ReqTypeRefund,
		Name:        "Alur Pengembalian Dana (Refund)",
		Active:      true,
		Steps: []model.FlowStep{
			{
				Seq:          1,
				ApproverKind: model.ApproverKindRole,
				ApproverRole: ptr("finance_manager"),
				SLAHours:     ptr(24),
			},
			{
				Seq:          2,
				ApproverKind: model.ApproverKindRole,
				ApproverRole: ptr("kepala_sekolah"),
				MinAmountIDR: ptr(int64(5_000_000)),
				SLAHours:     ptr(24),
			},
		},
	})
	if err != nil {
		t.Fatalf("seed flow: %v", err)
	}
}

// AC-79-01: Pengaju tidak dapat menyetujui sendiri (maker-checker)
func TestAC79_01_MakerChecker(t *testing.T) {
	fs, router, _ := newTestServer(t)
	seedRefundFlow(t, fs)

	// Finance Manager submits a refund of 1.000.000 via internal endpoint
	createBody := `{
		"school_id": "` + testSchoolA + `",
		"request_type": "refund",
		"source_service": "billing",
		"source_id": "ref-100",
		"title": "Refund SPP Siswa A",
		"summary": {"amount": 1000000, "student": "Siswa A"},
		"amount_idr": 1000000,
		"maker_user_id": "` + testUserFinance + `",
		"callback_url": "http://localhost/callback",
		"correlation_id": "corr-1",
		"idempotency_key": "refund:ref-100"
	}`

	rec := do(router, http.MethodPost, "/internal/approvals", "", map[string]string{
		"X-Internal-Key": testInternalKey,
	}, createBody)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create approval: want 201, got %d (body %s)", rec.Code, rec.Body.String())
	}

	var resp struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	reqID := resp.Data.ID

	// 1. Finance Manager views inbox: can_decide must be false
	tokFinance := token(t, testUserFinance, testSchoolA, "finance_manager")
	recInbox := do(router, http.MethodGet, "/approvals/inbox", tokFinance, nil, "")
	if recInbox.Code != http.StatusOK {
		t.Fatalf("get inbox: want 200, got %d", recInbox.Code)
	}

	var inboxResp struct {
		Data []model.ApprovalRequest `json:"data"`
	}
	_ = json.Unmarshal(recInbox.Body.Bytes(), &inboxResp)
	if len(inboxResp.Data) == 0 {
		t.Fatal("inbox should contain the request")
	}
	if inboxResp.Data[0].CanDecide {
		t.Error("AC-79-01 violated: maker should have can_decide = false in inbox")
	}

	// 2. Finance Manager tries to approve: must be rejected with 403 Forbidden
	recApprove := do(router, http.MethodPost, "/approvals/"+reqID+"/approve", tokFinance, nil, `{"comment":"menyetujui sendiri"}`)
	if recApprove.Code != http.StatusForbidden {
		t.Errorf("AC-79-01 violated: maker approving own request returned status %d, want 403 Forbidden", recApprove.Code)
	}

	// 3. Another user with role finance_manager CAN approve
	tokOtherFinance := token(t, "77777777-7777-7777-7777-777777777777", testSchoolA, "finance_manager")
	recApprove2 := do(router, http.MethodPost, "/approvals/"+reqID+"/approve", tokOtherFinance, nil, `{"comment":"disetujui penyetuju lain"}`)
	if recApprove2.Code != http.StatusOK {
		t.Errorf("other approver should succeed, got %d (body %s)", recApprove2.Code, recApprove2.Body.String())
	}
}

// AC-79-02: Tahap tambahan berdasarkan nominal (refund 6jt needs Kepala Unit after Finance Manager)
func TestAC79_02_NominalBasedSteps(t *testing.T) {
	fs, router, _ := newTestServer(t)
	seedRefundFlow(t, fs)

	// Refund 6.000.000 (exceeds 5.000.000 threshold)
	createBody := `{
		"school_id": "` + testSchoolA + `",
		"request_type": "refund",
		"source_service": "billing",
		"source_id": "ref-600",
		"title": "Refund SPP Besar",
		"summary": {"amount": 6000000},
		"amount_idr": 6000000,
		"maker_user_id": "` + testUserMaker + `",
		"callback_url": "http://localhost/callback",
		"correlation_id": "corr-600",
		"idempotency_key": "refund:ref-600"
	}`

	rec := do(router, http.MethodPost, "/internal/approvals", "", map[string]string{
		"X-Internal-Key": testInternalKey,
	}, createBody)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create approval: want 201, got %d", rec.Code)
	}

	var resp struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	reqID := resp.Data.ID

	// Step 1 approved by Finance Manager
	tokFinance := token(t, testUserFinance, testSchoolA, "finance_manager")
	recApprove1 := do(router, http.MethodPost, "/approvals/"+reqID+"/approve", tokFinance, nil, `{"comment":"acc finance"}`)
	if recApprove1.Code != http.StatusOK {
		t.Fatalf("step 1 approve: want 200, got %d", recApprove1.Code)
	}

	// Verify request is STILL pending and current_seq is 2
	recGet := do(router, http.MethodGet, "/approvals/"+reqID, tokFinance, nil, "")
	var getResp struct {
		Data model.ApprovalRequest `json:"data"`
	}
	_ = json.Unmarshal(recGet.Body.Bytes(), &getResp)
	if getResp.Data.Status != model.RequestStatusPending {
		t.Errorf("AC-79-02 violated: status should remain pending after step 1, got %s", getResp.Data.Status)
	}
	if getResp.Data.CurrentSeq == nil || *getResp.Data.CurrentSeq != 2 {
		t.Errorf("AC-79-02 violated: current_seq should advance to 2, got %v", getResp.Data.CurrentSeq)
	}

	// Step 2 approved by Kepala Sekolah -> finalized
	tokKepala := token(t, testUserKepala, testSchoolA, "kepala_sekolah")
	recApprove2 := do(router, http.MethodPost, "/approvals/"+reqID+"/approve", tokKepala, nil, `{"comment":"acc kepala unit"}`)
	if recApprove2.Code != http.StatusOK {
		t.Fatalf("step 2 approve: want 200, got %d", recApprove2.Code)
	}

	_ = json.Unmarshal(recApprove2.Body.Bytes(), &getResp)
	if getResp.Data.Status != model.RequestStatusApproved {
		t.Errorf("request should be approved after final step, got %s", getResp.Data.Status)
	}
}

// AC-79-03: Penolakan wajib komentar
func TestAC79_03_RejectMandatoryComment(t *testing.T) {
	fs, router, _ := newTestServer(t)
	seedRefundFlow(t, fs)

	createBody := `{
		"school_id": "` + testSchoolA + `",
		"request_type": "refund",
		"source_service": "billing",
		"source_id": "ref-300",
		"title": "Refund Uji Tolak",
		"summary": {"amount": 1000000},
		"amount_idr": 1000000,
		"maker_user_id": "` + testUserMaker + `",
		"callback_url": "http://localhost/callback",
		"correlation_id": "corr-300",
		"idempotency_key": "refund:ref-300"
	}`

	rec := do(router, http.MethodPost, "/internal/approvals", "", map[string]string{
		"X-Internal-Key": testInternalKey,
	}, createBody)
	var resp struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	reqID := resp.Data.ID

	tokFinance := token(t, testUserFinance, testSchoolA, "finance_manager")

	// Try rejecting with empty comment -> 422
	recRejectEmpty := do(router, http.MethodPost, "/approvals/"+reqID+"/reject", tokFinance, nil, `{"comment":""}`)
	if recRejectEmpty.Code != http.StatusUnprocessableEntity {
		t.Errorf("AC-79-03 violated: reject with empty comment should return 422, got %d", recRejectEmpty.Code)
	}

	recRejectWhitespace := do(router, http.MethodPost, "/approvals/"+reqID+"/reject", tokFinance, nil, `{"comment":"   "}`)
	if recRejectWhitespace.Code != http.StatusUnprocessableEntity {
		t.Errorf("AC-79-03 violated: reject with whitespace comment should return 422, got %d", recRejectWhitespace.Code)
	}

	// Reject with valid comment -> 200
	recRejectValid := do(router, http.MethodPost, "/approvals/"+reqID+"/reject", tokFinance, nil, `{"comment":"berkas tidak lengkap"}`)
	if recRejectValid.Code != http.StatusOK {
		t.Errorf("reject with valid comment should succeed, got %d", recRejectValid.Code)
	}
}

// AC-79-04: Eskalasi saat melewati batas waktu (SLA)
func TestAC79_04_Escalation(t *testing.T) {
	fs, _, cl := newTestServer(t)

	// Seed flow with escalation
	_, err := fs.CreateFlow(context.Background(), testSchoolA, testUserAdmin, model.ApprovalFlow{
		RequestType: model.ReqTypeLeave,
		Name:        "Alur Cuti",
		Active:      true,
		Steps: []model.FlowStep{
			{
				Seq:            1,
				ApproverKind:   model.ApproverKindRole,
				ApproverRole:   ptr("guru"),
				SLAHours:       ptr(24),
				EscalateToRole: ptr("kepala_sekolah"),
			},
		},
	})
	if err != nil {
		t.Fatalf("seed flow: %v", err)
	}

	// Create request with step due_at in the past
	past := time.Now().Add(-25 * time.Hour)
	req, err := fs.CreateRequest(context.Background(), model.ApprovalRequest{
		SchoolID:       testSchoolA,
		RequestType:    model.ReqTypeLeave,
		SourceService:  "attendance",
		SourceID:       "leave-1",
		Title:          "Cuti Tahunan",
		Summary:        map[string]any{"days": 2},
		MakerUserID:    testUserMaker,
		CallbackURL:    "http://localhost/callback",
		CorrelationID:  "corr-leave",
		IdempotencyKey: "leave:leave-1",
		FlowSnapshot: []model.FlowStep{
			{
				Seq:            1,
				ApproverKind:   model.ApproverKindRole,
				ApproverRole:   ptr("guru"),
				SLAHours:       ptr(24),
				EscalateToRole: ptr("kepala_sekolah"),
			},
		},
	}, []model.ApprovalStep{
		{
			Seq:            1,
			ApproverKind:   model.ApproverKindRole,
			ApproverRole:   ptr("guru"),
			EscalateToRole: ptr("kepala_sekolah"),
			Status:         model.StepStatusPending,
			DueAt:          &past,
		},
	})
	if err != nil {
		t.Fatalf("create request: %v", err)
	}

	runner := jobs.NewRunner(nil, fs, cl, time.Minute, 5)
	runner.RunOnce(context.Background())

	// Verify step 1 became escalated and step 2 was created for kepala_sekolah
	updated, err := fs.GetRequestByID(context.Background(), testSchoolA, req.ID)
	if err != nil {
		t.Fatalf("reload request: %v", err)
	}

	if len(updated.Steps) < 2 {
		t.Fatalf("AC-79-04 violated: expected escalated replacement step to be created, total steps = %d", len(updated.Steps))
	}
	if updated.Steps[0].Status != model.StepStatusEscalated {
		t.Errorf("step 1 status want escalated, got %s", updated.Steps[0].Status)
	}
	if updated.Steps[1].ApproverRole == nil || *updated.Steps[1].ApproverRole != "kepala_sekolah" {
		t.Errorf("step 2 approver role want kepala_sekolah, got %v", updated.Steps[1].ApproverRole)
	}
}

// AC-80-01: Retry posting berhasil
func TestAC80_01_RetrySuccess(t *testing.T) {
	fs, router, _ := newTestServer(t)

	// Mock server to receive retry
	retryCalled := false
	mockDownstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		retryCalled = true
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer mockDownstream.Close()

	// Seed exception
	exc, _, err := fs.UpsertException(context.Background(), model.IntegrationException{
		SchoolID:       testSchoolA,
		SourceService:  "billing",
		Kind:           model.ExceptionKindJournalPosting,
		Severity:       model.SeverityNormal,
		CorrelationID:  "corr-post-1",
		ErrorMessage:   "service unavailable",
		PayloadSummary: map[string]any{"journal_id": "j-1"},
		RetryURL:       &mockDownstream.URL,
		IdempotencyKey: "retry:j-1",
	})
	if err != nil {
		t.Fatalf("upsert exception: %v", err)
	}

	tokFinance := token(t, testUserFinance, testSchoolA, "finance_manager")
	rec := do(router, http.MethodPost, "/exceptions/"+exc.ID+"/retry", tokFinance, nil, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("AC-80-01: retry want 200, got %d (body %s)", rec.Code, rec.Body.String())
	}
	if !retryCalled {
		t.Error("AC-80-01: downstream retry URL was not invoked")
	}

	// Verify item is now resolved
	reloaded, _ := fs.GetExceptionByID(context.Background(), testSchoolA, exc.ID)
	if reloaded.Status != model.ExceptionStatusResolved {
		t.Errorf("exception status want resolved, got %s", reloaded.Status)
	}
}

// AC-80-02: Selesai manual wajib alasan
func TestAC80_02_ManualResolveMandatoryReason(t *testing.T) {
	fs, router, _ := newTestServer(t)

	exc, _, _ := fs.UpsertException(context.Background(), model.IntegrationException{
		SchoolID:       testSchoolA,
		SourceService:  "billing",
		Kind:           model.ExceptionKindWebhook,
		Severity:       model.SeverityNormal,
		CorrelationID:  "corr-wh-1",
		ErrorMessage:   "webhook timeout",
		PayloadSummary: map[string]any{"event": "payment"},
		IdempotencyKey: "wh:1",
	})

	tokAdmin := token(t, testUserAdmin, testSchoolA, "admin")

	// Try resolving with empty reason -> 422
	recEmpty := do(router, http.MethodPost, "/exceptions/"+exc.ID+"/resolve", tokAdmin, nil, `{"reason":""}`)
	if recEmpty.Code != http.StatusUnprocessableEntity {
		t.Errorf("AC-80-02 violated: resolve without reason want 422, got %d", recEmpty.Code)
	}

	// Resolve with valid reason -> 200
	recValid := do(router, http.MethodPost, "/exceptions/"+exc.ID+"/resolve", tokAdmin, nil, `{"reason":"Sudah diverifikasi manual via mutasi bank"}`)
	if recValid.Code != http.StatusOK {
		t.Errorf("resolve with reason want 200, got %d", recValid.Code)
	}

	reloaded, _ := fs.GetExceptionByID(context.Background(), testSchoolA, exc.ID)
	if reloaded.Status != model.ExceptionStatusResolved {
		t.Errorf("status want resolved, got %s", reloaded.Status)
	}
}

// AC-80-03: Data sensitif disembunyikan (nomor telepon tersamar)
func TestAC80_03_SensitiveDataMasking(t *testing.T) {
	_, router, _ := newTestServer(t)

	// Ingest exception via internal endpoint with sensitive phone number
	body := `{
		"school_id": "` + testSchoolA + `",
		"source_service": "notification",
		"kind": "notification",
		"severity": "normal",
		"correlation_id": "corr-notif-1",
		"error_message": "sms provider unreachable",
		"payload_summary": {
			"recipient_name": "Orang Tua Siswa",
			"phone": "081234567890",
			"email": "orangtua@example.com",
			"nik": "3275012345678901"
		},
		"idempotency_key": "notif:1"
	}`

	recInternal := do(router, http.MethodPost, "/internal/exceptions", "", map[string]string{
		"X-Internal-Key": testInternalKey,
	}, body)
	if recInternal.Code != http.StatusCreated {
		t.Fatalf("ingest exception: want 201, got %d", recInternal.Code)
	}

	// Read via public endpoint
	tokAdmin := token(t, testUserAdmin, testSchoolA, "admin")
	recList := do(router, http.MethodGet, "/exceptions", tokAdmin, nil, "")
	if recList.Code != http.StatusOK {
		t.Fatalf("get exceptions: want 200, got %d", recList.Code)
	}

	var listResp struct {
		Data []model.IntegrationException `json:"data"`
	}
	_ = json.Unmarshal(recList.Body.Bytes(), &listResp)
	if len(listResp.Data) == 0 {
		t.Fatal("expected at least 1 exception")
	}

	payload := listResp.Data[0].PayloadSummary
	if payload["phone"] != "••••7890" {
		t.Errorf("AC-80-03 violated: phone want ••••7890, got %v", payload["phone"])
	}
	if payload["nik"] != "••••8901" {
		t.Errorf("AC-80-03 violated: nik want ••••8901, got %v", payload["nik"])
	}
	if payload["email"] != "o•••@example.com" {
		t.Errorf("AC-80-03 violated: email want o•••@example.com, got %v", payload["email"])
	}
}

// AC-06-01: Perubahan tarif tercatat di log audit
func TestAC06_01_PriceChangeAuditLogged(t *testing.T) {
	fs, router, _ := newTestServer(t)
	grantAuditor(t, fs, testUserAuditor) // AC-RBAC-10: auditor reads only inside an access window

	// Price change event sent to /internal/audit
	body := `{
		"school_id": "` + testSchoolA + `",
		"service": "billing",
		"module": "finance",
		"entity_type": "fee_scheme",
		"entity_id": "scheme-smk",
		"action": "fee_updated",
		"actor_user_id": "` + testUserFinance + `",
		"actor_role": "finance_manager",
		"before": {"fee_idr": 500000},
		"after": {"fee_idr": 550000},
		"reason": "Penyesuaian tahun ajaran baru",
		"correlation_id": "corr-fee-smk"
	}`

	recInternal := do(router, http.MethodPost, "/internal/audit", "", map[string]string{
		"X-Internal-Key": testInternalKey,
	}, body)
	if recInternal.Code != http.StatusAccepted {
		t.Fatalf("post audit: want 202, got %d", recInternal.Code)
	}

	// Auditor reads audit trail
	tokAuditor := token(t, testUserAuditor, testSchoolA, "auditor")
	recAudit := do(router, http.MethodGet, "/audit?module=finance", tokAuditor, nil, "")
	if recAudit.Code != http.StatusOK {
		t.Fatalf("get audit: want 200, got %d", recAudit.Code)
	}

	var auditResp struct {
		Data []model.AuditEvent `json:"data"`
	}
	_ = json.Unmarshal(recAudit.Body.Bytes(), &auditResp)
	if len(auditResp.Data) == 0 {
		t.Fatal("AC-06-01: audit event not found")
	}

	ev := auditResp.Data[0]
	if ev.Module != "finance" || ev.Action != "fee_updated" {
		t.Errorf("unexpected event: %+v", ev)
	}
	if ev.ActorUserID == nil || *ev.ActorUserID != testUserFinance {
		t.Errorf("actor_user_id mismatch: %v", ev.ActorUserID)
	}

	beforeMap, ok := ev.BeforeData.(map[string]any)
	if !ok || beforeMap["fee_idr"] != float64(500000) {
		t.Errorf("before_data fee_idr mismatch: %v", ev.BeforeData)
	}
	afterMap, ok := ev.AfterData.(map[string]any)
	if !ok || afterMap["fee_idr"] != float64(550000) {
		t.Errorf("after_data fee_idr mismatch: %v", ev.AfterData)
	}
}

// Idempotency: POST /internal/approvals and POST /internal/exceptions
func TestIdempotency(t *testing.T) {
	fs, router, _ := newTestServer(t)
	seedRefundFlow(t, fs)

	approvalBody := `{
		"school_id": "` + testSchoolA + `",
		"request_type": "refund",
		"source_service": "billing",
		"source_id": "ref-idem-1",
		"title": "Refund Uji Idempotensi",
		"summary": {"amount": 1000000},
		"amount_idr": 1000000,
		"maker_user_id": "` + testUserMaker + `",
		"callback_url": "http://localhost/callback",
		"correlation_id": "corr-idem",
		"idempotency_key": "refund:idem-1"
	}`

	// Call 1 -> 201
	rec1 := do(router, http.MethodPost, "/internal/approvals", "", map[string]string{
		"X-Internal-Key": testInternalKey,
	}, approvalBody)
	if rec1.Code != http.StatusCreated {
		t.Fatalf("first approval call: want 201, got %d", rec1.Code)
	}

	// Call 2 (replay) -> 200 with same ID
	rec2 := do(router, http.MethodPost, "/internal/approvals", "", map[string]string{
		"X-Internal-Key": testInternalKey,
	}, approvalBody)
	if rec2.Code != http.StatusOK {
		t.Fatalf("replay approval call: want 200, got %d", rec2.Code)
	}

	var d1, d2 struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	_ = json.Unmarshal(rec1.Body.Bytes(), &d1)
	_ = json.Unmarshal(rec2.Body.Bytes(), &d2)
	if d1.Data.ID != d2.Data.ID {
		t.Errorf("idempotency violated: got different IDs %s != %s", d1.Data.ID, d2.Data.ID)
	}

	// Exception idempotency: second call increments attempts without creating new row
	excBody := `{
		"school_id": "` + testSchoolA + `",
		"source_service": "billing",
		"kind": "webhook",
		"severity": "normal",
		"correlation_id": "corr-idem-exc",
		"error_message": "temporary failure 1",
		"payload_summary": {"attempt": 1},
		"idempotency_key": "webhook:idem-1"
	}`
	recExc1 := do(router, http.MethodPost, "/internal/exceptions", "", map[string]string{
		"X-Internal-Key": testInternalKey,
	}, excBody)
	if recExc1.Code != http.StatusCreated {
		t.Fatalf("first exception call: want 201, got %d", recExc1.Code)
	}

	excBody2 := `{
		"school_id": "` + testSchoolA + `",
		"source_service": "billing",
		"kind": "webhook",
		"severity": "normal",
		"correlation_id": "corr-idem-exc",
		"error_message": "temporary failure 2",
		"payload_summary": {"attempt": 2},
		"idempotency_key": "webhook:idem-1"
	}`
	recExc2 := do(router, http.MethodPost, "/internal/exceptions", "", map[string]string{
		"X-Internal-Key": testInternalKey,
	}, excBody2)
	if recExc2.Code != http.StatusOK {
		t.Fatalf("replayed exception call: want 200, got %d", recExc2.Code)
	}

	var e2 struct {
		Data model.IntegrationException `json:"data"`
	}
	_ = json.Unmarshal(recExc2.Body.Bytes(), &e2)
	if e2.Data.Attempts != 2 {
		t.Errorf("exception attempts want 2, got %d", e2.Data.Attempts)
	}
	if e2.Data.ErrorMessage != "temporary failure 2" {
		t.Errorf("exception error message want latest, got %s", e2.Data.ErrorMessage)
	}
}
