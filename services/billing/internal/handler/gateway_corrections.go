package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"time"

	"skci/billing/internal/client"
	"skci/billing/internal/common"
)

type PaymentCorrectionInput struct {
	Source              string  `json:"source"` // 'gateway', 'cashier', 'manual'
	SourceID            string  `json:"source_id"`
	Kind                string  `json:"kind"` // 'reassign', 'cancel'
	TargetStudentUserID *string `json:"target_student_user_id,omitempty"`
	Reason              string  `json:"reason"`
}

// AdminCreateCorrection submits a payment correction for approval (BR-FIN-05, AC-29-04).
func (a *App) AdminCreateCorrection(w http.ResponseWriter, r *http.Request) {
	claims := common.RequireFinance(w, r)
	if claims == nil {
		return
	}
	schoolID := common.EffectiveSchoolID(r)
	if schoolID == "" {
		common.WriteError(w, http.StatusBadRequest, "school_id required")
		return
	}

	var req PaymentCorrectionInput
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		common.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.SourceID == "" || req.Reason == "" {
		common.WriteError(w, http.StatusBadRequest, "source_id and reason are required")
		return
	}
	if req.Source != "gateway" && req.Source != "cashier" && req.Source != "manual" {
		req.Source = "cashier"
	}
	if req.Kind != "reassign" && req.Kind != "cancel" {
		req.Kind = "cancel"
	}

	// The correction row and its core approval are created together: when
	// core refuses (422, no flow) nothing is stored and the message is shown.
	tx, err := a.DB.BeginTx(r.Context(), nil)
	if err != nil {
		writeErr(w, err)
		return
	}
	defer tx.Rollback()
	var corrID string
	err = tx.QueryRowContext(r.Context(),
		`INSERT INTO payment_corrections (
			school_id, source, source_id, kind, target_student_user_id, reason, status, requested_by, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, 'pending', $7, now(), now())
		RETURNING id`,
		schoolID, req.Source, req.SourceID, req.Kind, req.TargetStudentUserID, req.Reason, claims.UserID).Scan(&corrID)
	if err != nil {
		writeErr(w, err)
		return
	}

	// Submit approval request to Core if available (T1 §3). Core reaches the
	// callback over BILLING_INTERNAL_URL (docker service name), never localhost.
	if a.Core != nil {
		resp, appErr := a.Core.RequestApproval(r.Context(), client.ApprovalRequest{
			SchoolID:       schoolID,
			RequestType:    "payment_correction",
			SourceService:  "billing",
			SourceID:       corrID,
			Title:          fmt.Sprintf("Koreksi Pembayaran (%s %s)", req.Source, req.Kind),
			Summary:        map[string]any{"source": req.Source, "source_id": req.SourceID, "reason": req.Reason},
			MakerUserID:    claims.UserID,
			CallbackURL:    a.Config.Gateway.BillingInternalURL + "/internal/corrections/" + corrID + "/callback",
			CorrelationID:  corrID,
			IdempotencyKey: fmt.Sprintf("corr-appr:%s", corrID),
		})
		if appErr != nil {
			log.Printf("payment correction %s: request approval: %v", corrID, appErr)
			var he *client.HTTPError
			if errors.As(appErr, &he) && he.Status >= 400 && he.Status < 500 {
				common.WriteError(w, http.StatusUnprocessableEntity, "Approval Center menolak pengajuan: "+he.Body)
				return
			}
			common.WriteError(w, http.StatusBadGateway, "Approval Center tidak dapat dihubungi, pengajuan belum tersimpan")
			return
		}
		if _, err := tx.ExecContext(r.Context(),
			`UPDATE payment_corrections SET approval_id = $1 WHERE id = $2`, resp.ID, corrID); err != nil {
			writeErr(w, err)
			return
		}
	}
	if err := tx.Commit(); err != nil {
		writeErr(w, err)
		return
	}
	if a.Core == nil && (claims.ActiveRole == common.RoleFinanceManager || claims.ActiveRole == common.RoleAdmin || claims.ActiveRole == common.RoleSuperadmin) {
		// If finance_manager submits directly without Core, apply correction (AC-29-04)
		_ = a.applyPaymentCorrection(r.Context(), schoolID, corrID, claims.UserID)
	}

	common.WriteSuccess(w, map[string]string{
		"id":      corrID,
		"status":  "pending",
		"message": "Pengajuan koreksi berhasil dibuat dan menunggu approval",
	}, nil)
}

type correctionCallbackPayload struct {
	ApprovalID  string `json:"approval_id"`
	SourceID    string `json:"source_id"`
	RequestType string `json:"request_type"`
	Decision    string `json:"decision"` // 'approved' | 'rejected'
	DecidedBy   string `json:"decided_by"`
	Comment     string `json:"comment"`
}

// CorrectionCallback receives approval decisions from Core (T1 §3).
func (a *App) CorrectionCallback(w http.ResponseWriter, r *http.Request) {
	corrID := param(r, "id")
	var req correctionCallbackPayload
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || (req.Decision != "approved" && req.Decision != "rejected") {
		common.WriteError(w, http.StatusBadRequest, "invalid callback payload")
		return
	}

	var schoolID, status string
	err := a.DB.QueryRowContext(r.Context(),
		`SELECT school_id, status FROM payment_corrections WHERE id = $1`, corrID).Scan(&schoolID, &status)
	if errors.Is(err, sql.ErrNoRows) {
		common.WriteError(w, http.StatusNotFound, "correction not found")
		return
	}
	if err != nil {
		writeErr(w, err)
		return
	}
	// Idempotent: core may redeliver (Idempotency-Key approval:<id>).
	if status != "pending" {
		common.WriteSuccess(w, map[string]string{"status": status}, nil)
		return
	}

	if req.Decision == "approved" {
		err = a.applyPaymentCorrection(r.Context(), schoolID, corrID, req.DecidedBy)
	} else {
		_, err = a.DB.ExecContext(r.Context(),
			`UPDATE payment_corrections SET status = 'rejected', updated_at = now() WHERE id = $1 AND status = 'pending'`, corrID)
	}
	if err != nil {
		// 5xx makes core retry and, when exhausted, queue an exception.
		writeErr(w, err)
		return
	}

	common.WriteSuccess(w, map[string]string{"status": req.Decision}, nil)
}

// applyPaymentCorrection applies reversal and marks source as 'corrected' (AC-29-04).
func (a *App) applyPaymentCorrection(ctx context.Context, schoolID, corrID, actorID string) error {
	var source, sourceID, reason string
	err := a.DB.QueryRowContext(ctx,
		`SELECT source, source_id, reason FROM payment_corrections WHERE id = $1`, corrID).Scan(&source, &sourceID, &reason)
	if err != nil {
		return err
	}

	tx, err := a.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	now := time.Now()

	// 1. Mark origin source as 'corrected'
	if source == "cashier" {
		_, _ = tx.ExecContext(ctx,
			`UPDATE cashier_payments SET status = 'corrected', corrected_by_id = $1, updated_at = $2 WHERE id = $3`,
			actorID, now, sourceID)
	} else if source == "gateway" {
		_, _ = tx.ExecContext(ctx,
			`UPDATE gateway_transactions SET status = 'cancelled', updated_at = $1 WHERE id = $2`,
			now, sourceID)
	}

	// 2. Find all payments created from this source and reverse them
	refPrefix := ""
	if source == "cashier" {
		refPrefix = "KASIR-" + sourceID
	} else {
		// find tx_no
		_ = tx.QueryRowContext(ctx,
			`SELECT tx_no FROM gateway_transactions WHERE id = $1`, sourceID).Scan(&refPrefix)
	}

	rows, err := tx.QueryContext(ctx,
		`SELECT id, student_bill_id, amount_idr
		 FROM bill_payments
		 WHERE school_id = $1 AND (reference = $2 OR reference = 'KASIR-' || $2)`,
		schoolID, refPrefix)
	if err == nil {
		defer rows.Close()
		type pRev struct {
			pID    string
			billID string
			amount int64
		}
		var revs []pRev
		for rows.Next() {
			var r pRev
			if err := rows.Scan(&r.pID, &r.billID, &r.amount); err == nil {
				revs = append(revs, r)
			}
		}
		rows.Close()

		for _, r := range revs {
			idemKey := fmt.Sprintf("corr-rev:%s:%s", corrID, r.pID)
			_, err = tx.ExecContext(ctx,
				`INSERT INTO bill_payment_reversals (
					school_id, payment_id, amount_idr, reason, actor_id, idempotency_key, created_at
				) VALUES ($1, $2, $3, $4, $5, $6, $7)`,
				schoolID, r.pID, r.amount, reason, actorID, idemKey, now)
			if err != nil && !isUniqueViolation(err) {
				return err
			}
			if err := recomputeBillStatusInTx(ctx, tx, r.billID); err != nil {
				return err
			}
		}
	}

	// 3. Update correction record to approved
	_, err = tx.ExecContext(ctx,
		`UPDATE payment_corrections SET status = 'approved', updated_at = $1 WHERE id = $2`, now, corrID)
	if err != nil {
		return err
	}

	return tx.Commit()
}
