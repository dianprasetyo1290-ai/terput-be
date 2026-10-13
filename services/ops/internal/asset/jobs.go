package asset

import (
	"context"
	"log"
	"time"

	opsclient "skci/ops/internal/client"
)

type JobRunner struct {
	store      *Store
	ac         AccountingClient
	coreClient *opsclient.Client
	loc        *time.Location
}

func NewJobRunner(store *Store, ac AccountingClient, coreClient *opsclient.Client, loc *time.Location) *JobRunner {
	return &JobRunner{
		store:      store,
		ac:         ac,
		coreClient: coreClient,
		loc:        loc,
	}
}

// DispatchOutbox processes pending accounting events (acquisition, transfer, disposal)
func (r *JobRunner) DispatchOutbox(ctx context.Context) error {
	if r.ac == nil {
		return nil
	}
	return r.store.DispatchPendingEvents(ctx, r.ac, r.coreClient)
}

// BackfillReceipts finds any goods receipt lines with tracking=asset not yet handed off
func (r *JobRunner) BackfillReceipts(ctx context.Context) error {
	count, err := r.store.BackfillFromReceipts(ctx)
	if err != nil {
		log.Printf("[asset-backfill] error: %v", err)
		return err
	}
	if count > 0 {
		log.Printf("[asset-backfill] created %d candidate assets from goods receipts", count)
	}
	return nil
}

// RunMonthlyDepreciation runs monthly depreciation for schools that have asset feature enabled
func (r *JobRunner) RunMonthlyDepreciation(ctx context.Context, schoolIDs []string) error {
	if r.ac == nil {
		return nil
	}

	now := time.Now().In(r.loc)
	// Depreciation period is previous month: YYYY-MM
	prevMonth := now.AddDate(0, -1, 0)
	period := prevMonth.Format("2006-01")

	for _, sid := range schoolIDs {
		// check if feature is enabled
		enabled, err := r.coreClient.FeatureEnabled(ctx, sid, "asset")
		if err != nil || !enabled {
			continue
		}

		accEnabled, err := r.coreClient.FeatureEnabled(ctx, sid, "accounting")
		if err != nil || !accEnabled {
			continue
		}

		_, err = r.store.RunMonthlyDepreciation(ctx, sid, period, r.ac)
		if err != nil {
			log.Printf("[asset-depreciation] school %s period %s error: %v", sid, period, err)
		}
	}
	return nil
}
