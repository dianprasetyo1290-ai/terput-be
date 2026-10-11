package store

import (
	"context"
	"os"
	"testing"
	"time"

	"skci/core/internal/model"
)

func ptr[T any](v T) *T { return &v }

func TestPGStore_Integration(t *testing.T) {
	dsn := os.Getenv("TEST_PG_DSN")
	if dsn == "" {
		t.Skip("TEST_PG_DSN not set, skipping postgres store integration test")
	}

	db, err := Open(dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	st := NewPGStore(db)
	ctx := context.Background()
	schoolID := "11111111-1111-1111-1111-111111111111"
	userID := "22222222-2222-2222-2222-222222222222"

	// Clean up any leftovers from previous test runs
	_, _ = db.ExecContext(ctx, "DELETE FROM approval_flows WHERE school_id = $1", schoolID)
	_, _ = db.ExecContext(ctx, "DELETE FROM approval_requests WHERE school_id = $1", schoolID)
	_, _ = db.ExecContext(ctx, "DELETE FROM integration_exceptions WHERE school_id = $1", schoolID)
	_, _ = db.ExecContext(ctx, "DELETE FROM audit_events WHERE school_id = $1", schoolID)

	// 1. Flow CRUD
	flow, err := st.CreateFlow(ctx, schoolID, userID, model.ApprovalFlow{
		RequestType: model.ReqTypeRefund,
		Name:        "Refund Siswa Test",
		Active:      true,
		Steps: []model.FlowStep{
			{
				Seq:          1,
				ApproverKind: model.ApproverKindRole,
				ApproverRole: ptr("finance_manager"),
				SLAHours:     ptr(24),
			},
		},
	})
	if err != nil {
		t.Fatalf("create flow: %v", err)
	}
	if flow.ID == "" {
		t.Fatal("flow ID should not be empty")
	}

	loadedFlow, err := st.GetFlow(ctx, schoolID, flow.ID)
	if err != nil {
		t.Fatalf("get flow: %v", err)
	}
	if loadedFlow.Name != "Refund Siswa Test" || len(loadedFlow.Steps) != 1 {
		t.Errorf("loaded flow mismatch: %+v", loadedFlow)
	}

	// 2. Request & Steps
	idemKey := "refund:test-" + time.Now().Format("150405.000")
	req, err := st.CreateRequest(ctx, model.ApprovalRequest{
		SchoolID:       schoolID,
		RequestType:    model.ReqTypeRefund,
		SourceService:  "billing",
		SourceID:       "src-1",
		Title:          "Refund Rp500.000",
		Summary:        map[string]any{"amount": 500000},
		MakerUserID:    userID,
		CallbackURL:    "http://localhost:8090/callback",
		CorrelationID:  "corr-test",
		IdempotencyKey: idemKey,
		FlowSnapshot:   flow.Steps,
	}, []model.ApprovalStep{
		{
			Seq:          1,
			ApproverKind: model.ApproverKindRole,
			ApproverRole: ptr("finance_manager"),
			Status:       model.StepStatusPending,
		},
	})
	if err != nil {
		t.Fatalf("create request: %v", err)
	}

	// Test idempotency
	_, err = st.CreateRequest(ctx, model.ApprovalRequest{
		SchoolID:       schoolID,
		IdempotencyKey: idemKey,
	}, nil)
	if err != ErrIdempotentHit {
		t.Errorf("expected ErrIdempotentHit on replay, got %v", err)
	}

	// 3. Exception upsert
	excKey := "exc:test-" + time.Now().Format("150405.000")
	_, isNew, err := st.UpsertException(ctx, model.IntegrationException{
		SchoolID:       schoolID,
		SourceService:  "billing",
		Kind:           model.ExceptionKindWebhook,
		Severity:       model.SeverityNormal,
		CorrelationID:  "corr-exc",
		ErrorMessage:   "first error",
		PayloadSummary: map[string]any{"nik": "1234567890123456"},
		IdempotencyKey: excKey,
	})
	if err != nil || !isNew {
		t.Fatalf("upsert exception 1 failed: %v, isNew=%v", err, isNew)
	}

	exc2, isNew2, err := st.UpsertException(ctx, model.IntegrationException{
		SchoolID:       schoolID,
		SourceService:  "billing",
		Kind:           model.ExceptionKindWebhook,
		Severity:       model.SeverityNormal,
		CorrelationID:  "corr-exc",
		ErrorMessage:   "second error",
		PayloadSummary: map[string]any{"nik": "1234567890123456"},
		IdempotencyKey: excKey,
	})
	if err != nil || isNew2 || exc2.Attempts != 2 {
		t.Fatalf("upsert exception 2 failed: %v, isNew=%v, attempts=%d", err, isNew2, exc2.Attempts)
	}

	// 4. Audit insert & query
	err = st.InsertAudit(ctx, model.AuditEvent{
		SchoolID:   &schoolID,
		Service:    "billing",
		Module:     "finance",
		EntityType: "refund",
		EntityID:   req.ID,
		Action:     "refund_created",
	})
	if err != nil {
		t.Fatalf("insert audit: %v", err)
	}

	audits, total, err := st.ListAudit(ctx, AuditFilter{SchoolID: &schoolID, Module: "finance"}, 1, 10)
	if err != nil || total == 0 || len(audits) == 0 {
		t.Fatalf("list audit: total=%d len=%d err=%v", total, len(audits), err)
	}
}
