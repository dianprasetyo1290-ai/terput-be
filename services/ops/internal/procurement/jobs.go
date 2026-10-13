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
)

type SyncJob struct {
	db            *sql.DB
	client        *client.Client
	accountingURL string
	internalKey   string
	httpClient    *http.Client
}

func NewSyncJob(db *sql.DB, c *client.Client, accountingURL, internalKey string) *SyncJob {
	return &SyncJob{
		db:            db,
		client:        c,
		accountingURL: accountingURL,
		internalKey:   internalKey,
		httpClient:    &http.Client{Timeout: 8 * time.Second},
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
		WHERE s.status = 'pending' AND s.attempts < 5
		LIMIT 50
	`)
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
			// Network error
			_, _ = j.db.ExecContext(ctx, `
				UPDATE vendor_accounting_sync
				SET attempts = attempts + 1, last_error = $1, updated_at = now()
				WHERE vendor_id = $2
			`, err.Error(), it.vendorID)
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
			// Other error
			errMsg := fmt.Sprintf("status %d: %s", resp.StatusCode, string(respBytes))
			newAttempts := it.attempts + 1
			newStatus := "pending"
			if newAttempts >= 5 {
				newStatus = "failed"
				// Report dead letter exception to core
				if j.client != nil {
					_ = j.client.PostException(ctx, client.Exception{
						SchoolID:       it.schoolID,
						SourceService:  "ops",
						Kind:           "dead_letter",
						Severity:       "warning",
						ErrorMessage:   "Gagal sinkronisasi vendor ke Accounting Engine: " + errMsg,
						CorrelationID:  it.vendorID,
						MaxAttempts:    5,
						IdempotencyKey: fmt.Sprintf("vendor_sync_ex:%s", it.vendorID),
					})
				}
			}
			_, _ = j.db.ExecContext(ctx, `
				UPDATE vendor_accounting_sync
				SET status = $1, attempts = $2, last_error = $3, updated_at = now()
				WHERE vendor_id = $4
			`, newStatus, newAttempts, errMsg, it.vendorID)
		}
	}
	return nil
}
