package handler

import (
	"database/sql"
	"fmt"
	"net/http"
	"strings"

	"skci/billing/internal/client"
	"skci/billing/internal/common"
	"skci/billing/internal/model"
)

// BUG-055: scholarship/discount programs are created in the app. A new
// program is stored pending_approval (inactive) and becomes active only when
// the Approval Center approves request_type discount_award (maker ≠
// approver). Generation applies a program only inside its validity period.
const reqTypeDiscountAward = "discount_award"

// CreateDiscount is POST /admin/discounts. 202: waiting for approval.
func (a *App) CreateDiscount(w http.ResponseWriter, r *http.Request) {
	c, schoolID := common.RequireFinanceStaff(w, r)
	if c == nil {
		return
	}
	var in struct {
		Name          string `json:"name"`
		Kind          string `json:"kind"`
		Value         int64  `json:"value"`
		CapIDR        int64  `json:"cap_idr"`
		Priority      *int   `json:"priority"`
		Scope         string `json:"scope"`
		SchemeID      string `json:"scheme_id"`
		GradeLevel    string `json:"grade_level"`
		StudentUserID string `json:"student_user_id"`
		Scholarship   bool   `json:"scholarship"`
		ValidFrom     string `json:"valid_from"`
		ValidTo       string `json:"valid_to"`
		Reason        string `json:"reason"`
	}
	if !common.Decode(w, r, &in) {
		return
	}
	from, err := parseOptionalDate(in.ValidFrom, "valid_from")
	if err != nil {
		writeErr(w, err)
		return
	}
	to, err := parseOptionalDate(in.ValidTo, "valid_to")
	if err != nil {
		writeErr(w, err)
		return
	}
	prog := model.DiscountProgramInput{
		Name: strings.TrimSpace(in.Name), Kind: model.DiscountKind(strings.TrimSpace(in.Kind)), Value: in.Value,
		CapIDR: in.CapIDR, Scope: strings.TrimSpace(in.Scope), SchemeID: strings.TrimSpace(in.SchemeID),
		GradeLevel: strings.TrimSpace(in.GradeLevel), StudentUserID: strings.TrimSpace(in.StudentUserID),
		ValidFrom: from, ValidTo: to, Reason: strings.TrimSpace(in.Reason),
	}
	if prog.Scope == "" {
		prog.Scope = "school"
	}
	if err := model.ValidateDiscountProgram(prog); err != nil {
		common.WriteError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	// Only the field matching the scope is stored.
	schemeID, grade, student := "", "", ""
	switch prog.Scope {
	case "scheme":
		schemeID = prog.SchemeID
	case "grade":
		grade = prog.GradeLevel
	case "student":
		student = prog.StudentUserID
	}
	priority := 100
	if in.Priority != nil {
		priority = *in.Priority
	}
	ctx := r.Context()
	if schemeID != "" {
		var ok bool
		if err := a.DB.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM billing_schemes WHERE id::text=$1 AND school_id=$2)`,
			schemeID, schoolID).Scan(&ok); err != nil {
			writeErr(w, err)
			return
		}
		if !ok {
			common.WriteError(w, http.StatusUnprocessableEntity, "skema tidak ditemukan di sekolah ini")
			return
		}
	}
	var capArg any
	if prog.CapIDR > 0 {
		capArg = prog.CapIDR
	}
	key := idempotencyKey(r)
	var id, status string
	err = a.DB.QueryRowContext(ctx,
		`INSERT INTO bill_discounts(school_id,name,kind,value,cap_idr,priority,scope,scheme_id,grade_level,student_user_id,
		                            scholarship,active,status,valid_from,valid_to,reason,requested_by,idempotency_key)
		 VALUES($1,$2,$3,$4,$5,$6,$7,NULLIF($8,'')::uuid,NULLIF($9,''),NULLIF($10,'')::uuid,$11,false,'pending_approval',$12,$13,$14,$15::uuid,$16)
		 ON CONFLICT (school_id,idempotency_key) WHERE idempotency_key IS NOT NULL DO NOTHING
		 RETURNING id::text,status`,
		schoolID, prog.Name, string(prog.Kind), prog.Value, capArg, priority, prog.Scope, schemeID, grade, student,
		in.Scholarship, from, to, prog.Reason, c.UserID, key).Scan(&id, &status)
	if err == sql.ErrNoRows {
		// Replay of the same Idempotency-Key: return the existing program.
		var approvalID string
		if err := a.DB.QueryRowContext(ctx,
			`SELECT id::text,status,COALESCE(approval_id,'') FROM bill_discounts WHERE school_id=$1 AND idempotency_key=$2`,
			schoolID, key).Scan(&id, &status, &approvalID); err != nil {
			writeErr(w, err)
			return
		}
		common.WriteJSON(w, http.StatusAccepted, map[string]any{"id": id, "status": status, "approval_id": approvalID}, nil)
		return
	}
	if err != nil {
		writeErr(w, err)
		return
	}
	valueText := formatRupiah(prog.Value)
	if prog.Kind == model.DiscountPercent {
		valueText = fmt.Sprintf("%d,%02d%%", prog.Value/100, prog.Value%100)
	}
	after := map[string]any{"name": prog.Name, "kind": prog.Kind, "value": prog.Value, "cap_idr": prog.CapIDR,
		"scope": prog.Scope, "scheme_id": schemeID, "grade_level": grade, "student_user_id": student,
		"scholarship": in.Scholarship, "valid_from": in.ValidFrom, "valid_to": in.ValidTo, "reason": prog.Reason}
	resp, err := a.submitApproval(ctx, client.ApprovalRequest{
		SchoolID: schoolID, RequestType: reqTypeDiscountAward, SourceID: id,
		Title:   fmt.Sprintf("Program potongan/beasiswa: %s (%s)", prog.Name, valueText),
		Summary: after, MakerUserID: c.UserID, SubjectUserID: student,
		CallbackURL:   a.Config.Gateway.BillingInternalURL + "/internal/discounts/" + id + "/callback",
		CorrelationID: id, IdempotencyKey: "discount-award:" + id,
	})
	if err != nil {
		_, _ = a.DB.ExecContext(ctx, `DELETE FROM bill_discounts WHERE id::text=$1 AND status='pending_approval'`, id)
		writeErr(w, err)
		return
	}
	if _, err = a.DB.ExecContext(ctx, `UPDATE bill_discounts SET approval_id=$1 WHERE id::text=$2`, resp.ID, id); err != nil {
		writeErr(w, err)
		return
	}
	a.auditBoth(ctx, schoolID, "billing", "bill_discount", id, "discount_requested", c.ActiveRole, c.UserID, prog.Reason,
		nil, map[string]any{"program": after, "status": model.DiscountPending, "approval_id": resp.ID})
	common.WriteJSON(w, http.StatusAccepted, map[string]any{"id": id, "status": model.DiscountPending, "approval_id": resp.ID}, nil)
}

// DiscountCallback is POST /internal/discounts/{id}/callback from the
// Approval Center. Replays are 200; a self-approval is refused.
func (a *App) DiscountCallback(w http.ResponseWriter, r *http.Request) {
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
	var schoolID, status, requestedBy, approvalID, name string
	err = tx.QueryRowContext(ctx,
		`SELECT school_id::text,status,COALESCE(requested_by::text,''),COALESCE(approval_id,''),name
		   FROM bill_discounts WHERE id::text=$1 FOR UPDATE`, id).Scan(&schoolID, &status, &requestedBy, &approvalID, &name)
	if err == sql.ErrNoRows {
		common.WriteError(w, http.StatusNotFound, "program potongan tidak ditemukan")
		return
	}
	if err != nil {
		writeErr(w, err)
		return
	}
	if approvalID != "" && cb.ApprovalID != "" && cb.ApprovalID != approvalID {
		common.WriteError(w, http.StatusConflict, "approval_id tidak cocok dengan pengajuan")
		return
	}
	approved, changed, derr := model.ApprovalOutcome(status, model.DiscountPending, cb.Decision, cb.DecidedBy, requestedBy)
	if derr != nil {
		common.WriteError(w, http.StatusUnprocessableEntity, derr.Error())
		return
	}
	if !changed {
		common.WriteJSON(w, http.StatusOK, map[string]any{"id": id, "status": status}, nil)
		return
	}
	next := model.DiscountRejected
	if approved {
		next = model.DiscountActive
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE bill_discounts SET status=$1,active=$2,decided_by=$3,decision_note=NULLIF($4,''),updated_at=now() WHERE id::text=$5`,
		next, approved, cb.DecidedBy, strings.TrimSpace(cb.Comment), id); err != nil {
		writeErr(w, err)
		return
	}
	if err := tx.Commit(); err != nil {
		writeErr(w, err)
		return
	}
	a.auditBoth(ctx, schoolID, "billing", "bill_discount", id, "discount_"+next, "approver", cb.DecidedBy,
		strings.TrimSpace(cb.Comment), map[string]any{"status": status, "name": name}, map[string]any{"status": next, "active": approved})
	common.WriteJSON(w, http.StatusOK, map[string]any{"id": id, "status": next}, nil)
}

// DeactivateDiscount is POST /admin/discounts/{id}/deactivate {reason}. It
// stops the program for future generation; issued bills keep their snapshot.
func (a *App) DeactivateDiscount(w http.ResponseWriter, r *http.Request) {
	c, schoolID := common.RequireFinanceStaff(w, r)
	if c == nil {
		return
	}
	var in struct {
		Reason string `json:"reason"`
	}
	if !common.Decode(w, r, &in) {
		return
	}
	in.Reason = strings.TrimSpace(in.Reason)
	if in.Reason == "" {
		common.WriteError(w, http.StatusUnprocessableEntity, "alasan penonaktifan wajib diisi")
		return
	}
	id := param(r, "id")
	before := model.DiscountActive
	var got string
	err := a.DB.QueryRowContext(r.Context(),
		`UPDATE bill_discounts SET status='inactive',active=false,updated_at=now()
		  WHERE id::text=$1 AND school_id=$2 AND status='active' RETURNING id::text`, id, schoolID).Scan(&got)
	if err == sql.ErrNoRows {
		var cur string
		qerr := a.DB.QueryRowContext(r.Context(), `SELECT status FROM bill_discounts WHERE id::text=$1 AND school_id=$2`, id, schoolID).Scan(&cur)
		if qerr == sql.ErrNoRows {
			common.WriteError(w, http.StatusNotFound, "program potongan tidak ditemukan")
			return
		}
		if qerr != nil {
			writeErr(w, qerr)
			return
		}
		common.WriteError(w, http.StatusConflict, "hanya program berstatus aktif yang dapat dinonaktifkan (status saat ini: "+cur+")")
		return
	}
	if err != nil {
		writeErr(w, err)
		return
	}
	a.auditBoth(r.Context(), schoolID, "billing", "bill_discount", id, "discount_deactivated", c.ActiveRole, c.UserID, in.Reason,
		map[string]any{"status": before}, map[string]any{"status": model.DiscountInactive})
	common.WriteJSON(w, http.StatusOK, map[string]any{"id": id, "status": model.DiscountInactive}, nil)
}
