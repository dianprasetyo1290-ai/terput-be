package store

import (
	"context"
	"errors"
	"os"
	"slices"
	"sync"
	"testing"
	"time"

	"skci/core/internal/model"
)

func openTestStore(t *testing.T) *PGStore {
	t.Helper()
	dsn := os.Getenv("TEST_PG_DSN")
	if dsn == "" {
		t.Skip("TEST_PG_DSN not set")
	}
	db, err := Open(dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return NewPGStore(db)
}

// BUG-087: grant windows, expiry and revoke against the real schema.
func TestPGStore_AuditGrants(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	school, user, by := "11111111-1111-1111-1111-111111111111", "55555555-5555-5555-5555-555555555599", "66666666-6666-6666-6666-666666666666"
	if ok, err := st.HasActiveAuditGrant(ctx, school, user); err != nil || ok {
		t.Fatalf("before grant: ok=%v err=%v", ok, err)
	}
	if _, err := st.CreateAuditGrant(ctx, model.AuditAccessGrant{SchoolID: school, UserID: user, ValidFrom: time.Now().Add(-2 * time.Hour),
		ValidTo: time.Now().Add(-time.Hour), Reason: "lama", GrantedBy: by}); err != nil {
		t.Fatal(err)
	}
	if ok, _ := st.HasActiveAuditGrant(ctx, school, user); ok {
		t.Fatal("expired grant must not count")
	}
	g, err := st.CreateAuditGrant(ctx, model.AuditAccessGrant{SchoolID: school, UserID: user, ValidFrom: time.Now().Add(-time.Minute),
		ValidTo: time.Now().Add(time.Hour), Reason: "audit", GrantedBy: by})
	if err != nil {
		t.Fatal(err)
	}
	if ok, _ := st.HasActiveAuditGrant(ctx, school, user); !ok {
		t.Fatal("active grant must count")
	}
	if _, err := st.RevokeAuditGrant(ctx, school, g.ID, by); err != nil {
		t.Fatal(err)
	}
	if _, err := st.RevokeAuditGrant(ctx, school, g.ID, by); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second revoke: %v", err)
	}
	if ok, _ := st.HasActiveAuditGrant(ctx, school, user); ok {
		t.Fatal("revoked grant must not count")
	}
	if _, err := st.CreateAuditGrant(ctx, model.AuditAccessGrant{SchoolID: school, UserID: user, ValidFrom: time.Now(),
		ValidTo: time.Now().Add(-time.Hour), Reason: "x", GrantedBy: by}); err == nil {
		t.Fatal("CHECK valid_to > valid_from must reject")
	}
}

// BUG-167: concurrent claims on the same item, exactly one wins.
func TestPGStore_ClaimExceptionForRetry(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	school := "11111111-1111-1111-1111-111111111111"
	exc, _, err := st.UpsertException(ctx, model.IntegrationException{SchoolID: school, SourceService: "billing",
		Kind: model.ExceptionKindWebhook, Severity: model.SeverityNormal, CorrelationID: "c", ErrorMessage: "x",
		PayloadSummary: map[string]any{}, IdempotencyKey: "qa-claim-" + time.Now().Format(time.RFC3339Nano), MaxAttempts: 3})
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	results := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); results <- st.ClaimExceptionForRetry(ctx, school, exc.ID, 5*time.Minute) }()
	}
	wg.Wait()
	close(results)
	wins := 0
	for err := range results {
		switch {
		case err == nil:
			wins++
		case !errors.Is(err, ErrRetryInProgress):
			t.Fatalf("unexpected: %v", err)
		}
	}
	if wins != 1 {
		t.Fatalf("wins = %d, want 1", wins)
	}
	// a stale lock (crashed worker) can be taken over
	if err := st.ClaimExceptionForRetry(ctx, school, exc.ID, 0); err != nil {
		t.Fatalf("stale takeover: %v", err)
	}
}

// BUG-152/117: audit filter by correlation id and a date window.
func TestPGStore_AuditFilters(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	school := "11111111-1111-1111-1111-111111111111"
	corr := "qa-corr-" + time.Now().Format("150405.000000")
	for _, svc := range []string{"billing", "accounting"} {
		if err := st.InsertAudit(ctx, model.AuditEvent{SchoolID: &school, Service: svc, Module: "finance", EntityType: "bill",
			EntityID: "b1", Action: "posted", CorrelationID: &corr}); err != nil {
			t.Fatal(err)
		}
	}
	from := time.Now().Add(-time.Hour)
	evs, total, err := st.ListAudit(ctx, AuditFilter{SchoolID: &school, CorrelationID: corr, From: &from}, 1, 50)
	if err != nil || total != 2 || len(evs) != 2 {
		t.Fatalf("by correlation: total=%d err=%v", total, err)
	}
	_, total, _ = st.ListAudit(ctx, AuditFilter{SchoolID: &school, CorrelationID: corr, Service: "billing"}, 1, 50)
	if total != 1 {
		t.Fatalf("by correlation+service: %d", total)
	}
	_, total, err = st.ListAudit(ctx, AuditFilter{SchoolID: &school, CorrelationID: corr, Actions: []string{"posted", "data_exported"}}, 1, 50)
	if err != nil || total != 2 {
		t.Fatalf("by action list: total=%d err=%v", total, err)
	}
	facets, err := st.AuditFacets(ctx, &school, time.Now().Add(-time.Hour))
	if err != nil || !slices.Contains(facets.Services, "accounting") || !slices.Contains(facets.Actions, "posted") {
		t.Fatalf("facets = %+v err=%v", facets, err)
	}
	if _, err := st.AuditFacets(ctx, nil, time.Now().Add(-time.Hour)); err != nil {
		t.Fatalf("facets all tenants: %v", err)
	}
}
