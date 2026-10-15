package procurement

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"skci/ops/internal/client"
	"skci/ops/internal/common"
)

type SyncJob struct {
	db            *sql.DB
	client        *client.Client
	accountingURL string
	internalKey   string
	httpClient    *http.Client
	// Reporter queues vendors that exhausted their retries in core's
	// exception queue (kind dead_letter). Nil -> the failure is only logged.
	Reporter *common.ExceptionReporter
}

// vendorSyncMaxAttempts: deliveries before a vendor sync is dead-lettered.
const vendorSyncMaxAttempts = 5

// vendorSyncTopic names the queue in dead-letter reports and their keys
// (dead_letter:ops:vendor_accounting_sync:<vendor_id>).
const vendorSyncTopic = "vendor_accounting_sync"

func NewSyncJob(db *sql.DB, c *client.Client, accountingURL, internalKey string) *SyncJob {
	return &SyncJob{
		db:            db,
		client:        c,
		accountingURL: accountingURL,
		internalKey:   internalKey,
		httpClient:    &http.Client{Timeout: 8 * time.Second, Transport: common.CorrelationTransport{}},
	}
}

// RunSyncVendors processes pending vendor_accounting_sync records.
// FR-106-005 & IF-OPS-01: Synchronizes vendor profile to Accounting Engine.
func (j *SyncJob) RunSyncVendors(ctx context.Context) error {
	rows, err := j.db.QueryContext(ctx, `
		SELECT s.vendor_id, s.school_id, s.payload_hash, s.attempts,
		       v.code, v.name, v.status, v.npwp_enc
		FROM vendor_accounting_sync s
		JOIN vendors v ON v.id = s.vendor_id
		WHERE s.status = 'pending' AND s.attempts < $1
		LIMIT 50
	`, vendorSyncMaxAttempts)
	if err != nil {
		return err
	}
	defer rows.Close()

	type item struct {
		vendorID    string
		schoolID    string
		payloadHash string
		attempts    int
		code        string
		name        string
		status      string
		npwpEnc     []byte
	}

	var items []item
	for rows.Next() {
		var it item
		if err := rows.Scan(&it.vendorID, &it.schoolID, &it.payloadHash, &it.attempts,
			&it.code, &it.name, &it.status, &it.npwpEnc); err != nil {
			return err
		}
		items = append(items, it)
	}

	for _, it := range items {
		// Check if school has 'accounting' feature enabled
		if j.client != nil {
			on, err := j.client.FeatureEnabled(ctx, it.schoolID, "accounting")
			if err == nil && !on {
				// School doesn't have accounting enabled, skip sync
				continue
			}
		}

		bodyMap := map[string]any{
			"school_id": it.schoolID,
			"code":      it.code,
			"name":      it.name,
			"status":    it.status,
		}

		jsonBody, _ := json.Marshal(bodyMap)
		url := fmt.Sprintf("%s/api/internal/vendors/%s", j.accountingURL, it.vendorID)
		req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, bytes.NewReader(jsonBody))
		if err != nil {
			continue
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Internal-Key", j.internalKey)
		req.Header.Set("Idempotency-Key", fmt.Sprintf("vendor:%s:%s", it.vendorID, it.payloadHash))

		resp, err := j.httpClient.Do(req)
		if err != nil {
			// A network error counts as a failed delivery too, so an
			// unreachable accounting service ends in the dead-letter queue
			// instead of leaving the row pending forever.
			j.recordFailure(ctx, it.vendorID, it.schoolID, it.attempts, "jaringan: "+err.Error())
			continue
		}
		respBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		resp.Body.Close()

		if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusCreated || resp.StatusCode == http.StatusNoContent {
			_, _ = j.db.ExecContext(ctx, `
				UPDATE vendor_accounting_sync
				SET status = 'synced', attempts = attempts + 1, synced_at = now(), last_error = NULL, updated_at = now()
				WHERE vendor_id = $1
			`, it.vendorID)
		} else if resp.StatusCode == http.StatusNotFound {
			// FR-106-005: 404 until T17 is merged -> keep pending, log info, no exception
			log.Printf("vendor sync %s: accounting endpoint not found (404), keeping pending", it.vendorID)
			_, _ = j.db.ExecContext(ctx, `
				UPDATE vendor_accounting_sync
				SET attempts = attempts + 1, last_error = 'endpoint 404 (accounting not ready)', updated_at = now()
				WHERE vendor_id = $1
			`, it.vendorID)
		} else {
			j.recordFailure(ctx, it.vendorID, it.schoolID, it.attempts, fmt.Sprintf("status %d: %s", resp.StatusCode, string(respBytes)))
		}
	}
	return nil
}

// recordFailure counts one failed delivery; at vendorSyncMaxAttempts the row
// is marked failed and reported to core as a dead letter.
func (j *SyncJob) recordFailure(ctx context.Context, vendorID, schoolID string, attempts int, errMsg string) {
	newAttempts := attempts + 1
	newStatus := "pending"
	if newAttempts >= vendorSyncMaxAttempts {
		newStatus = "failed"
		j.reportDeadLetter(ctx, vendorID, schoolID, newAttempts, errMsg)
	}
	if _, err := j.db.ExecContext(ctx, `
		UPDATE vendor_accounting_sync
		SET status = $1, attempts = $2, last_error = $3, updated_at = now()
		WHERE vendor_id = $4
	`, newStatus, newAttempts, errMsg, vendorID); err != nil {
		log.Printf("vendor sync %s: record failure: %v", vendorID, err)
	}
}

func (j *SyncJob) reportDeadLetter(ctx context.Context, vendorID, schoolID string, attempts int, errMsg string) {
	if j.Reporter == nil {
		log.Printf("vendor sync %s: dead letter not reported (no reporter): %s", vendorID, errMsg)
		return
	}
	if err := j.Reporter.ReportDeadLetter(ctx, schoolID, vendorSyncTopic, vendorID,
		"Gagal sinkronisasi vendor ke Accounting Engine: "+errMsg, attempts,
		map[string]any{"vendor_id": vendorID}); err != nil {
		log.Printf("vendor sync %s: report dead letter: %v", vendorID, err)
	}
}
