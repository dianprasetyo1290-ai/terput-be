package handler

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"skci/billing/internal/client"
	"skci/billing/internal/common"
	"skci/billing/internal/model"
)

// Reconciliation exceptions (menu 31, BUG-056). Resolving one is
// maker-checker: the finance user proposes a resolution (note plus an
// optional balanced adjustment journal), core's Approval Center decides
// (request type 'recon_adjustment'), and ReconExceptionCallback applies the
// decision. An approved adjustment is queued in the journal outbox.

const reqTypeReconAdjustment = "recon_adjustment"

// AdminListExceptions is GET /admin/reconciliation/exceptions
// ?status=&kind=&provider=&page=&per_page=.
func (a *App) AdminListExceptions(w http.ResponseWriter, r *http.Request) {
	claims := common.RequireFinance(w, r)
	if claims == nil {
		return
	}
	schoolID := common.EffectiveSchoolID(r)
	if schoolID == "" {
		common.WriteError(w, http.StatusBadRequest, "school_id required")
		return
	}
	q := r.URL.Query()
	page, perPage := common.Page(r)
	const where = ` FROM reconciliation_exceptions
	  WHERE school_id = $1 AND ($2 = '' OR status = $2) AND ($3 = '' OR kind = $3) AND ($4 = '' OR provider = $4)`
	args := []any{schoolID, q.Get("status"), q.Get("kind"), q.Get("provider")}
	var total int
	if err := a.DB.QueryRowContext(r.Context(), `SELECT count(*)`+where, args...).Scan(&total); err != nil {
		writeErr(w, err)
		return
	}
	rows, err := a.DB.QueryContext(r.Context(),
		`SELECT id, provider, kind, ref, detail, status, COALESCE(resolution_note, ''), COALESCE(adjustment_lines, 'null'),
		        COALESCE(adjustment_approval_id, ''), COALESCE(requested_by, ''), requested_at, COALESCE(resolved_by, ''), resolved_at,
		        COALESCE(decision_note, ''), created_at, updated_at`+where+`
		  ORDER BY created_at DESC, id LIMIT $5 OFFSET $6`, append(args, perPage, common.Offset(page, perPage))...)
	if err != nil {
		writeErr(w, err)
		return
	}
	defer rows.Close()
	type exRow struct {
		ID              string          `json:"id"`
		Provider        string          `json:"provider"`
		Kind            string          `json:"kind"`
		Ref             string          `json:"ref"`
		Detail          json.RawMessage `json:"detail"`
		Status          string          `json:"status"`
		ResolutionNote  string          `json:"resolution_note"`
		AdjustmentLines json.RawMessage `json:"adjustment_lines"`
		ApprovalID      string          `json:"approval_id"`
		RequestedBy     string          `json:"requested_by"`
		RequestedAt     *time.Time      `json:"requested_at"`
		ResolvedBy      string          `json:"resolved_by"`
		ResolvedAt      *time.Time      `json:"resolved_at"`
		DecisionNote    string          `json:"decision_note"`
		CreatedAt       time.Time       `json:"created_at"`
		UpdatedAt       time.Time       `json:"updated_at"`
	}
	items := []exRow{}
	for rows.Next() {
		var it exRow
		var det, lines []byte
		var reqAt, resAt sql.NullTime
		if err := rows.Scan(&it.ID, &it.Provider, &it.Kind, &it.Ref, &det, &it.Status, &it.ResolutionNote, &lines,
			&it.ApprovalID, &it.RequestedBy, &reqAt, &it.ResolvedBy, &resAt, &it.DecisionNote, &it.CreatedAt, &it.UpdatedAt); err != nil {
			writeErr(w, err)
			return
		}
		it.Detail, it.AdjustmentLines = det, lines
		if reqAt.Valid {
			it.RequestedAt = &reqAt.Time
		}
		if resAt.Valid {
			it.ResolvedAt = &resAt.Time
		}
		items = append(items, it)
	}
	if err := rows.Err(); err != nil {
		writeErr(w, err)
		return
	}
	common.WriteJSON(w, http.StatusOK, items, common.Meta{Page: page, PerPage: perPage, Total: total})
}

// AdminResolveException is POST /admin/reconciliation/exceptions/{id}/resolve
// {resolution_note, adjustment_lines?}. It submits the resolution to the
// Approval Center and answers 202; the exception is resolved only by an
// approval from someone else.
func (a *App) AdminResolveException(w http.ResponseWriter, r *http.Request) {
	claims := common.RequireFinance(w, r)
	if claims == nil {
		return
	}
	schoolID := common.EffectiveSchoolID(r)
	id := param(r, "id")
	var req struct {
		ResolutionNote  string              `json:"resolution_note"`
		AdjustmentLines []model.JournalLine `json:"adjustment_lines"`
	}
	if !common.Decode(w, r, &req) {
		return
	}
	req.ResolutionNote = strings.TrimSpace(req.ResolutionNote)
	total, verr := model.ValidateReconResolution(req.ResolutionNote, req.AdjustmentLines)
	if verr != nil {
		common.WriteError(w, http.StatusUnprocessableEntity, verr.Error())
		return
	}
	ctx := r.Context()
	tx, err := a.DB.BeginTx(ctx, nil)
	if err != nil {
		writeErr(w, err)
		return
	}
	defer func() { _ = tx.Rollback() }()
	var status, kind, ref, provider string
	var detail []byte
	err = tx.QueryRowContext(ctx,
		`SELECT status, kind, ref, provider, detail FROM reconciliation_exceptions WHERE id::text = $1 AND school_id = $2 FOR UPDATE`,
		id, schoolID).Scan(&status, &kind, &ref, &provider, &detail)
	if err == sql.ErrNoRows {
		common.WriteError(w, http.StatusNotFound, "pengecualian rekonsiliasi tidak ditemukan")
		return
	}
	if err != nil {
		writeErr(w, err)
		return
	}
	switch status {
	case model.ReconPendingApproval:
		common.WriteError(w, http.StatusConflict, "penyelesaian pengecualian ini sedang menunggu approval")
		return
	case model.ReconResolved:
		common.WriteError(w, http.StatusUnprocessableEntity, "pengecualian sudah diselesaikan")
		return
	}
	var lines any
	if len(req.AdjustmentLines) > 0 {
		lines = jsonRaw(req.AdjustmentLines)
	}
	if _, err = tx.ExecContext(ctx,
		`UPDATE reconciliation_exceptions SET status = 'pending_approval', resolution_note = $1, adjustment_lines = $2,
		        requested_by = $3, requested_at = now(), decision_note = NULL, adjustment_approval_id = NULL, updated_at = now()
		  WHERE id::text = $4`, req.ResolutionNote, lines, claims.UserID, id); err != nil {
		writeErr(w, err)
		return
	}
	if err = tx.Commit(); err != nil {
		writeErr(w, err)
		return
	}
	var amount *int64
	if total > 0 {
		amount = &total
	}
	resp, err := a.submitApproval(ctx, client.ApprovalRequest{
		SchoolID: schoolID, RequestType: reqTypeReconAdjustment, SourceID: id,
		Title: "Penyelesaian selisih rekonsiliasi " + ref,
		Summary: map[string]any{
			"kind": kind, "ref": ref, "provider": provider, "detail": json.RawMessage(detail),
			"resolution_note": req.ResolutionNote, "adjustment_lines": req.AdjustmentLines, "adjustment_total_idr": total,
		},
		AmountIDR: amount, MakerUserID: claims.UserID,
		CallbackURL:   a.Config.Gateway.BillingInternalURL + "/internal/reconciliation/exceptions/" + id + "/callback",
		CorrelationID: id, IdempotencyKey: "recon-adjustment:" + id + ":" + randomKey(),
	})
	if err != nil {
		_, _ = a.DB.ExecContext(ctx,
			`UPDATE reconciliation_exceptions SET status = 'open', adjustment_lines = NULL, requested_by = NULL, requested_at = NULL, updated_at = now()
			  WHERE id::text = $1 AND status = 'pending_approval'`, id)
		writeErr(w, err)
		return
	}
	if _, err = a.DB.ExecContext(ctx, `UPDATE reconciliation_exceptions SET adjustment_approval_id = $1 WHERE id::text = $2`, resp.ID, id); err != nil {
		writeErr(w, err)
		return
	}
	a.auditBoth(ctx, schoolID, "billing", "reconciliation_exception", id, "recon_resolution_requested", claims.ActiveRole, claims.UserID,
		req.ResolutionNote, map[string]any{"status": status}, map[string]any{"status": model.ReconPendingApproval,
			"adjustment_lines": req.AdjustmentLines, "approval_id": resp.ID})
	common.WriteJSON(w, http.StatusAccepted, map[string]any{"id": id, "status": model.ReconPendingApproval, "approval_id": resp.ID}, nil)
}

// ReconExceptionCallback is POST /internal/reconciliation/exceptions/{id}/callback
// (core). Approval resolves the exception and queues the adjustment journal;
// rejection reopens it with the checker's note. Replays are acknowledged.
func (a *App) ReconExceptionCallback(w http.ResponseWriter, r *http.Request) {
	var cb client.ApprovalCallback
	if !common.Decode(w, r, &cb) {
		return
	}
	id := param(r, "id")
	ctx := r.Context()
	tx, err := a.DB.BeginTx(ctx, nil)
	if err != nil {
		writeErr(w, err)
		return
	}
	defer func() { _ = tx.Rollback() }()
	var schoolID, status, requestedBy, approvalID, ref, note string
	var lines []byte
	err = tx.QueryRowContext(ctx,
		`SELECT school_id::text, status, COALESCE(requested_by, ''), COALESCE(adjustment_approval_id, ''), ref,
		        COALESCE(resolution_note, ''), COALESCE(adjustment_lines, 'null')
		   FROM reconciliation_exceptions WHERE id::text = $1 FOR UPDATE`, id).
		Scan(&schoolID, &status, &requestedBy, &approvalID, &ref, &note, &lines)
	if err == sql.ErrNoRows {
		common.WriteError(w, http.StatusNotFound, "pengecualian rekonsiliasi tidak ditemukan")
		return
	}
	if err != nil {
		writeErr(w, err)
		return
	}
	if approvalID != "" && cb.ApprovalID != "" && cb.ApprovalID != approvalID {
		common.WriteError(w, http.StatusConflict, "approval_id tidak cocok dengan pengajuan penyelesaian")
		return
	}
	next, changed, derr := model.ReconDecision(status, cb.Decision, cb.DecidedBy, requestedBy)
	if derr != nil {
		common.WriteError(w, http.StatusUnprocessableEntity, derr.Error())
		return
	}
	if !changed {
		common.WriteJSON(w, http.StatusOK, map[string]any{"id": id, "status": next}, nil)
		return
	}
	comment := strings.TrimSpace(cb.Comment)
	journalRef := ""
	if next == model.ReconResolved {
		if _, err = tx.ExecContext(ctx,
			`UPDATE reconciliation_exceptions SET status = 'resolved', resolved_by = $1, resolved_at = now(),
			        decision_note = NULLIF($2, ''), updated_at = now() WHERE id::text = $3`, cb.DecidedBy, comment, id); err != nil {
			writeErr(w, err)
			return
		}
		var adj []model.JournalLine
		if err = json.Unmarshal(lines, &adj); err != nil {
			writeErr(w, err)
			return
		}
		if len(adj) > 0 {
			journalRef = "bil:recon-adj:" + id
			day := a.now().In(a.location()).Format("2006-01-02")
			if _, err = tx.ExecContext(ctx,
				`INSERT INTO gateway_journal_postings (school_id, reference, trans_date, description, lines, kind, source, source_refs, exception_id)
				 VALUES ($1, $2, $3, $4, $5, 'recon_adjustment', $6, $7, $8) ON CONFLICT (reference) DO NOTHING`,
				schoolID, journalRef, day, "Penyesuaian rekonsiliasi "+ref+": "+note, jsonRaw(adj), sourceBilling,
				jsonRaw(map[string][]string{"reconciliation_exceptions": {id}}), id); err != nil {
				writeErr(w, err)
				return
			}
		}
	} else {
		if _, err = tx.ExecContext(ctx,
			`UPDATE reconciliation_exceptions SET status = 'open', decision_note = NULLIF($1, ''), requested_by = NULL,
			        requested_at = NULL, updated_at = now() WHERE id::text = $2`, comment, id); err != nil {
			writeErr(w, err)
			return
		}
	}
	if err = tx.Commit(); err != nil {
		writeErr(w, err)
		return
	}
	a.auditBoth(ctx, schoolID, "billing", "reconciliation_exception", id, "recon_resolution_"+cb.Decision, "approver", cb.DecidedBy,
		comment, map[string]any{"status": status}, map[string]any{"status": next, "journal_reference": journalRef})
	common.WriteJSON(w, http.StatusOK, map[string]any{"id": id, "status": next, "journal_reference": journalRef}, nil)
}
