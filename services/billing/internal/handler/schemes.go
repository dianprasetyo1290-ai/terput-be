package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"skci/billing/internal/client"
	"skci/billing/internal/common"
	"skci/billing/internal/model"
)

// ---------------------------------------------------------------------------
// Scheme configuration (admin CRUD, menu 18)
//
// Tariff rules (QA BUG-051/124/127/171/172/086):
//   - component codes are unique per scheme and a Rp0 line must be flagged
//     free (model.ValidateComponents);
//   - a scheme has an optional validity window (effective_from/to) and two
//     live schemes may not bill the same component to the same academic
//     year / grade / classroom in overlapping windows (model.SchemeConflict);
//   - changing the tariff of an active scheme goes through the Approval
//     Center (request_type fee_change) and is applied by the callback;
//   - every mutation is audited with actor and before/after values, locally
//     and in core's central Audit Trail.
// ---------------------------------------------------------------------------

const reqTypeFeeChange = "fee_change"

type componentInput struct {
	Code      string `json:"code"`
	Name      string `json:"name"`
	Kind      string `json:"kind"` // monthly | one_time
	AmountIDR int64  `json:"amount_idr"`
	SortOrder int    `json:"sort_order"`
	IsFree    bool   `json:"is_free"`
}

type schemeInput struct {
	AcademicYearID string           `json:"academic_year_id"`
	GradeLevel     string           `json:"grade_level"`
	ClassroomID    string           `json:"classroom_id"`
	Name           string           `json:"name"`
	Installment    bool             `json:"installment"`
	DueDay         int              `json:"due_day"`
	Status         string           `json:"status"`
	Notes          string           `json:"notes"`
	EffectiveFrom  string           `json:"effective_from"` // YYYY-MM-DD, optional
	EffectiveTo    string           `json:"effective_to"`   // YYYY-MM-DD, optional
	Components     []componentInput `json:"components"`
}

func (in schemeInput) modelComponents() []model.SchemeComponent {
	out := make([]model.SchemeComponent, 0, len(in.Components))
	for _, c := range in.Components {
		out = append(out, model.SchemeComponent{Code: c.Code, Name: c.Name, Kind: c.Kind, AmountIDR: c.AmountIDR, IsFree: c.IsFree})
	}
	return out
}

func parseOptionalDate(s, field string) (*time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return nil, &httpError{http.StatusUnprocessableEntity, field + " harus berformat YYYY-MM-DD"}
	}
	return &t, nil
}

func (in schemeInput) window() (*time.Time, *time.Time, error) {
	from, err := parseOptionalDate(in.EffectiveFrom, "effective_from")
	if err != nil {
		return nil, nil, err
	}
	to, err := parseOptionalDate(in.EffectiveTo, "effective_to")
	if err != nil {
		return nil, nil, err
	}
	if from != nil && to != nil && to.Before(*from) {
		return nil, nil, &httpError{http.StatusUnprocessableEntity, "effective_to tidak boleh sebelum effective_from"}
	}
	return from, to, nil
}

func (in schemeInput) validate(requireComponents bool) error {
	if strings.TrimSpace(in.AcademicYearID) == "" {
		return &httpError{http.StatusUnprocessableEntity, "academic_year_id is required"}
	}
	if strings.TrimSpace(in.Name) == "" {
		return &httpError{http.StatusUnprocessableEntity, "name is required"}
	}
	if in.Status != "" && in.Status != "draft" && in.Status != "active" && in.Status != "archived" {
		return &httpError{http.StatusUnprocessableEntity, "invalid status"}
	}
	if requireComponents && len(in.Components) == 0 {
		return &httpError{http.StatusUnprocessableEntity, "at least one bill component is required"}
	}
	if err := model.ValidateComponents(in.modelComponents()); err != nil {
		return &httpError{http.StatusUnprocessableEntity, err.Error()}
	}
	_, _, err := in.window()
	return err
}

func schemeStatus(in schemeInput) string {
	if in.Status == "" {
		return "active"
	}
	return in.Status
}

func schemeDueDay(in schemeInput) int {
	if in.DueDay < 1 || in.DueDay > 31 {
		return 10
	}
	return in.DueDay
}

func componentKind(c componentInput) string {
	if c.Kind == "" {
		return "one_time"
	}
	return c.Kind
}

// normalized returns the scheme as it is stored (defaults applied), so
// before/after snapshots compare like with like.
func (in schemeInput) normalized() schemeInput {
	out := in
	out.Status = schemeStatus(in)
	out.DueDay = schemeDueDay(in)
	out.Components = make([]componentInput, len(in.Components))
	for i, c := range in.Components {
		c.Kind = componentKind(c)
		if c.SortOrder == 0 {
			c.SortOrder = i + 1
		}
		out.Components[i] = c
	}
	return out
}

// tariffChanged reports whether moving from before to after changes what
// students are billed: component amounts/lines or the validity window.
func tariffChanged(before, after schemeInput) bool {
	return model.TariffChanged(before.modelComponents(), after.modelComponents()) ||
		strings.TrimSpace(before.EffectiveFrom) != strings.TrimSpace(after.EffectiveFrom) ||
		strings.TrimSpace(before.EffectiveTo) != strings.TrimSpace(after.EffectiveTo)
}

// loadScheme reads a scheme with its components as a schemeInput.
func loadScheme(ctx context.Context, q interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}, schoolID, id string, lock bool) (schemeInput, error) {
	var in schemeInput
	query := `SELECT academic_year_id::text,COALESCE(grade_level,''),COALESCE(classroom_id::text,''),name,status,installment,due_day,
	                 COALESCE(notes,''),COALESCE(to_char(effective_from,'YYYY-MM-DD'),''),COALESCE(to_char(effective_to,'YYYY-MM-DD'),'')
	            FROM billing_schemes WHERE id::text=$1 AND school_id=$2`
	if lock {
		query += ` FOR UPDATE`
	}
	err := q.QueryRowContext(ctx, query, id, schoolID).Scan(&in.AcademicYearID, &in.GradeLevel, &in.ClassroomID, &in.Name,
		&in.Status, &in.Installment, &in.DueDay, &in.Notes, &in.EffectiveFrom, &in.EffectiveTo)
	if err == sql.ErrNoRows {
		return in, &httpError{http.StatusNotFound, "scheme not found"}
	}
	if err != nil {
		return in, err
	}
	rows, err := q.QueryContext(ctx,
		`SELECT code,name,kind,amount_idr,sort_order,is_free FROM billing_scheme_components
		  WHERE scheme_id::text=$1 AND school_id=$2 ORDER BY sort_order,code`, id, schoolID)
	if err != nil {
		return in, err
	}
	defer rows.Close()
	in.Components = []componentInput{}
	for rows.Next() {
		var c componentInput
		if err := rows.Scan(&c.Code, &c.Name, &c.Kind, &c.AmountIDR, &c.SortOrder, &c.IsFree); err != nil {
			return in, err
		}
		in.Components = append(in.Components, c)
	}
	return in, rows.Err()
}

// checkSchemeConflicts refuses a double tariff (BUG-127). It serializes
// scheme writes per school so two concurrent saves cannot both pass.
func checkSchemeConflicts(ctx context.Context, tx *sql.Tx, schoolID, id string, in schemeInput) error {
	if schemeStatus(in) == "archived" {
		return nil
	}
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtext('billing_scheme:' || $1::text))`, schoolID); err != nil {
		return err
	}
	from, to, err := in.window()
	if err != nil {
		return err
	}
	candidate := model.SchemeWindow{ID: id, Name: in.Name, AcademicYearID: in.AcademicYearID, GradeLevel: in.GradeLevel,
		ClassroomID: in.ClassroomID, From: from, To: to}
	for _, c := range in.Components {
		candidate.Codes = append(candidate.Codes, c.Code)
	}
	rows, err := tx.QueryContext(ctx,
		`SELECT s.id::text,s.name,s.academic_year_id::text,COALESCE(s.grade_level,''),COALESCE(s.classroom_id::text,''),
		        s.effective_from,s.effective_to,COALESCE(string_agg(c.code, chr(31)),'')
		   FROM billing_schemes s LEFT JOIN billing_scheme_components c ON c.scheme_id=s.id
		  WHERE s.school_id=$1 AND s.status<>'archived' AND s.academic_year_id::text=$2 AND s.id::text<>$3
		  GROUP BY s.id`, schoolID, in.AcademicYearID, id)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var other model.SchemeWindow
		var codes string
		var f, t sql.NullTime
		if err := rows.Scan(&other.ID, &other.Name, &other.AcademicYearID, &other.GradeLevel, &other.ClassroomID, &f, &t, &codes); err != nil {
			return err
		}
		if f.Valid {
			other.From = &f.Time
		}
		if t.Valid {
			other.To = &t.Time
		}
		if codes != "" {
			other.Codes = strings.Split(codes, "\x1f")
		}
		if shared := model.SchemeConflict(candidate, other); len(shared) > 0 {
			return &httpError{http.StatusConflict, "tarif ganda: komponen " + strings.Join(shared, ", ") +
				" sudah ditagihkan skema \"" + other.Name + "\" untuk tahun ajaran, tingkat/kelas dan periode berlaku yang sama"}
		}
	}
	return rows.Err()
}

func insertComponents(ctx context.Context, tx *sql.Tx, schoolID, schemeID string, components []componentInput) error {
	for i, c := range components {
		order := c.SortOrder
		if order == 0 {
			order = i + 1
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO billing_scheme_components(school_id,scheme_id,code,name,kind,amount_idr,sort_order,is_free)
			 VALUES($1,$2,$3,$4,$5,$6,$7,$8)`,
			schoolID, schemeID, strings.TrimSpace(c.Code), strings.TrimSpace(c.Name), componentKind(c), c.AmountIDR, order, c.IsFree); err != nil {
			return err
		}
	}
	return nil
}

// applyScheme writes in over scheme id (components replaced).
func applyScheme(ctx context.Context, tx *sql.Tx, schoolID, id string, in schemeInput) error {
	res, err := tx.ExecContext(ctx,
		`UPDATE billing_schemes SET academic_year_id=$1,grade_level=NULLIF($2,''),classroom_id=NULLIF($3,'')::uuid,
		        name=$4,status=$5,installment=$6,due_day=$7,notes=NULLIF($8,''),
		        effective_from=NULLIF($9,'')::date,effective_to=NULLIF($10,'')::date,updated_at=now()
		 WHERE id::text=$11 AND school_id=$12`,
		in.AcademicYearID, in.GradeLevel, in.ClassroomID, in.Name, schemeStatus(in),
		in.Installment, schemeDueDay(in), in.Notes, in.EffectiveFrom, in.EffectiveTo, id, schoolID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return &httpError{http.StatusNotFound, "scheme not found"}
	}
	if _, err = tx.ExecContext(ctx,
		`DELETE FROM billing_scheme_components WHERE scheme_id::text=$1 AND school_id=$2`, id, schoolID); err != nil {
		return err
	}
	return insertComponents(ctx, tx, schoolID, id, in.Components)
}

// CreateScheme is POST /admin/schemes.
func (a *App) CreateScheme(w http.ResponseWriter, r *http.Request) {
	c, schoolID := common.RequireFinanceStaff(w, r)
	if c == nil {
		return
	}
	var in schemeInput
	if !common.Decode(w, r, &in) {
		return
	}
	if err := in.validate(true); err != nil {
		writeErr(w, err)
		return
	}
	in = in.normalized()
	tx, err := a.DB.BeginTx(r.Context(), nil)
	if err != nil {
		writeErr(w, err)
		return
	}
	defer func() { _ = tx.Rollback() }()
	if err := checkSchemeConflicts(r.Context(), tx, schoolID, "", in); err != nil {
		writeErr(w, err)
		return
	}

	var id string
	err = tx.QueryRowContext(r.Context(),
		`INSERT INTO billing_schemes(school_id,academic_year_id,grade_level,classroom_id,name,status,installment,due_day,notes,effective_from,effective_to)
		 VALUES($1,$2,NULLIF($3,''),NULLIF($4,'')::uuid,$5,$6,$7,$8,NULLIF($9,''),NULLIF($10,'')::date,NULLIF($11,'')::date)
		 RETURNING id`,
		schoolID, in.AcademicYearID, in.GradeLevel, in.ClassroomID, in.Name, in.Status,
		in.Installment, in.DueDay, in.Notes, in.EffectiveFrom, in.EffectiveTo).Scan(&id)
	if err != nil {
		writeErr(w, err)
		return
	}
	if err = insertComponents(r.Context(), tx, schoolID, id, in.Components); err != nil {
		writeErr(w, err)
		return
	}
	if err = tx.Commit(); err != nil {
		writeErr(w, err)
		return
	}
	a.auditBoth(r.Context(), schoolID, "billing", "scheme", id, "scheme_created", c.ActiveRole, c.UserID, "", nil, in)
	common.WriteJSON(w, http.StatusCreated, map[string]any{"id": id}, nil)
}

// UpdateScheme is PUT /admin/schemes/{id}. Components are replaced when the
// payload carries any. A tariff change on an active scheme is not applied:
// it is queued for Finance Manager approval and answered with 202.
func (a *App) UpdateScheme(w http.ResponseWriter, r *http.Request) {
	c, schoolID := common.RequireFinanceStaff(w, r)
	if c == nil {
		return
	}
	var in schemeInput
	if !common.Decode(w, r, &in) {
		return
	}
	if err := in.validate(false); err != nil {
		writeErr(w, err)
		return
	}
	id := param(r, "id")

	tx, err := a.DB.BeginTx(r.Context(), nil)
	if err != nil {
		writeErr(w, err)
		return
	}
	defer func() { _ = tx.Rollback() }()

	before, err := loadScheme(r.Context(), tx, schoolID, id, true)
	if err != nil {
		writeErr(w, err)
		return
	}
	if len(in.Components) == 0 {
		in.Components = before.Components
	}
	in = in.normalized()
	if err := checkSchemeConflicts(r.Context(), tx, schoolID, id, in); err != nil {
		writeErr(w, err)
		return
	}
	if before.Status == "active" && tariffChanged(before, in) {
		_ = tx.Rollback()
		a.requestSchemeChange(w, r, c, schoolID, id, before, in)
		return
	}
	if err := applyScheme(r.Context(), tx, schoolID, id, in); err != nil {
		writeErr(w, err)
		return
	}
	if err = tx.Commit(); err != nil {
		writeErr(w, err)
		return
	}
	a.auditBoth(r.Context(), schoolID, "billing", "scheme", id, "scheme_updated", c.ActiveRole, c.UserID, "", before, in)
	common.WriteJSON(w, http.StatusOK, map[string]any{"id": id, "status": "applied"}, nil)
}

func schemeTotal(in schemeInput) int64 {
	var total int64
	for _, c := range in.Components {
		total += c.AmountIDR
	}
	return total
}

// requestSchemeChange stores the requested tariff and asks the Approval
// Center (fee_change). The row is removed again when core does not accept
// the request, so nothing waits on an approval that does not exist.
func (a *App) requestSchemeChange(w http.ResponseWriter, r *http.Request, c *common.Claims, schoolID, schemeID string, before, after schemeInput) {
	ctx := r.Context()
	var changeID string
	err := a.DB.QueryRowContext(ctx,
		`INSERT INTO billing_scheme_changes (school_id, scheme_id, payload, before_data, requested_by, requested_role, idempotency_key)
		 VALUES ($1, $2::uuid, $3, $4, $5::uuid, $6, $7) RETURNING id::text`,
		schoolID, schemeID, jsonRaw(after), jsonRaw(before), c.UserID, c.ActiveRole, idempotencyKey(r)).Scan(&changeID)
	if isUniqueViolation(err) {
		common.WriteError(w, http.StatusConflict, "masih ada perubahan tarif skema ini yang menunggu approval")
		return
	}
	if err != nil {
		writeErr(w, err)
		return
	}
	total := schemeTotal(after)
	resp, err := a.submitApproval(ctx, client.ApprovalRequest{
		SchoolID: schoolID, RequestType: reqTypeFeeChange, SourceID: changeID,
		Title: "Perubahan tarif " + after.Name,
		Summary: map[string]any{
			"scheme_id": schemeID, "scheme": after.Name,
			"before": before.Components, "after": after.Components,
			"before_total_idr": schemeTotal(before), "after_total_idr": total,
			"effective_from": after.EffectiveFrom, "effective_to": after.EffectiveTo,
		},
		AmountIDR: &total, MakerUserID: c.UserID,
		CallbackURL:   a.Config.Gateway.BillingInternalURL + "/internal/schemes/changes/" + changeID + "/callback",
		CorrelationID: changeID, IdempotencyKey: "fee-change:" + changeID,
	})
	if err != nil {
		_, _ = a.DB.ExecContext(ctx, `DELETE FROM billing_scheme_changes WHERE id::text=$1 AND status='pending_approval'`, changeID)
		writeErr(w, err)
		return
	}
	if _, err := a.DB.ExecContext(ctx, `UPDATE billing_scheme_changes SET approval_id=$1 WHERE id::text=$2`, resp.ID, changeID); err != nil {
		writeErr(w, err)
		return
	}
	a.auditBoth(ctx, schoolID, "billing", "scheme", schemeID, "scheme_change_requested", c.ActiveRole, c.UserID, "", before, after)
	common.WriteJSON(w, http.StatusAccepted, map[string]any{
		"id": schemeID, "change_id": changeID, "status": "pending_approval", "approval_id": resp.ID,
	}, nil)
}

// SchemeChangeCallback is POST /internal/schemes/changes/{id}/callback
// (core). Idempotent: a change that is no longer pending is acknowledged.
func (a *App) SchemeChangeCallback(w http.ResponseWriter, r *http.Request) {
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
	var schoolID, schemeID, status string
	var payload []byte
	err = tx.QueryRowContext(ctx,
		`SELECT school_id::text, scheme_id::text, status, payload FROM billing_scheme_changes WHERE id::text=$1 FOR UPDATE`, id).
		Scan(&schoolID, &schemeID, &status, &payload)
	if err == sql.ErrNoRows {
		common.WriteError(w, http.StatusNotFound, "perubahan tarif tidak ditemukan")
		return
	}
	if err != nil {
		writeErr(w, err)
		return
	}
	if status != "pending_approval" {
		common.WriteJSON(w, http.StatusOK, map[string]any{"id": id, "status": status}, nil)
		return
	}
	finish := func(next, failure string) error {
		_, err := tx.ExecContext(ctx,
			`UPDATE billing_scheme_changes SET status=$1, decided_by=NULLIF($2,''), decision_note=NULLIF($3,''),
			        failure_reason=NULLIF($4,''), decided_at=now() WHERE id::text=$5`,
			next, cb.DecidedBy, cb.Comment, failure, id)
		return err
	}
	if cb.Decision != "approved" {
		if err := finish("rejected", ""); err != nil {
			writeErr(w, err)
			return
		}
		if err := tx.Commit(); err != nil {
			writeErr(w, err)
			return
		}
		a.auditBoth(ctx, schoolID, "billing", "scheme", schemeID, "scheme_change_rejected", "approver", cb.DecidedBy, cb.Comment, nil, map[string]any{"change_id": id})
		common.WriteJSON(w, http.StatusOK, map[string]any{"id": id, "status": "rejected"}, nil)
		return
	}

	var after schemeInput
	if err := json.Unmarshal(payload, &after); err != nil {
		writeErr(w, err)
		return
	}
	before, err := loadScheme(ctx, tx, schoolID, schemeID, true)
	if err != nil {
		writeErr(w, err)
		return
	}
	// The world may have moved since the request: re-check before applying.
	if err := checkSchemeConflicts(ctx, tx, schoolID, schemeID, after); err != nil {
		var he *httpError
		if !errors.As(err, &he) {
			writeErr(w, err)
			return
		}
		if err := finish("failed", he.msg); err != nil {
			writeErr(w, err)
			return
		}
		if err := tx.Commit(); err != nil {
			writeErr(w, err)
			return
		}
		a.auditBoth(ctx, schoolID, "billing", "scheme", schemeID, "scheme_change_failed", "approver", cb.DecidedBy, he.msg, before, after)
		common.WriteJSON(w, http.StatusOK, map[string]any{"id": id, "status": "failed", "reason": he.msg}, nil)
		return
	}
	if err := applyScheme(ctx, tx, schoolID, schemeID, after); err != nil {
		writeErr(w, err)
		return
	}
	if err := finish("applied", ""); err != nil {
		writeErr(w, err)
		return
	}
	if err := tx.Commit(); err != nil {
		writeErr(w, err)
		return
	}
	a.auditBoth(ctx, schoolID, "billing", "scheme", schemeID, "scheme_change_applied", "approver", cb.DecidedBy, cb.Comment, before, after)
	common.WriteJSON(w, http.StatusOK, map[string]any{"id": id, "status": "applied"}, nil)
}

// ListSchemeChanges is GET /admin/schemes/{id}/changes.
func (a *App) ListSchemeChanges(w http.ResponseWriter, r *http.Request) {
	c, schoolID := common.RequireFinanceStaff(w, r)
	if c == nil {
		return
	}
	id := param(r, "id")
	page, perPage := common.Page(r)
	var total int
	if err := a.DB.QueryRowContext(r.Context(),
		`SELECT count(*) FROM billing_scheme_changes WHERE school_id=$1 AND scheme_id::text=$2`, schoolID, id).Scan(&total); err != nil {
		writeErr(w, err)
		return
	}
	rows, err := a.DB.QueryContext(r.Context(),
		`SELECT id::text, status, payload, before_data, COALESCE(approval_id,''), requested_by::text, COALESCE(decided_by,''),
		        COALESCE(decision_note,''), COALESCE(failure_reason,''), created_at, decided_at
		   FROM billing_scheme_changes WHERE school_id=$1 AND scheme_id::text=$2
		  ORDER BY created_at DESC LIMIT $3 OFFSET $4`, schoolID, id, perPage, common.Offset(page, perPage))
	if err != nil {
		writeErr(w, err)
		return
	}
	defer rows.Close()
	type change struct {
		ID            string          `json:"id"`
		Status        string          `json:"status"`
		After         json.RawMessage `json:"after"`
		Before        json.RawMessage `json:"before"`
		ApprovalID    string          `json:"approval_id"`
		RequestedBy   string          `json:"requested_by"`
		DecidedBy     string          `json:"decided_by"`
		DecisionNote  string          `json:"decision_note"`
		FailureReason string          `json:"failure_reason"`
		CreatedAt     time.Time       `json:"created_at"`
		DecidedAt     *time.Time      `json:"decided_at"`
	}
	out := []change{}
	for rows.Next() {
		var ch change
		if err := rows.Scan(&ch.ID, &ch.Status, &ch.After, &ch.Before, &ch.ApprovalID, &ch.RequestedBy, &ch.DecidedBy,
			&ch.DecisionNote, &ch.FailureReason, &ch.CreatedAt, &ch.DecidedAt); err != nil {
			writeErr(w, err)
			return
		}
		out = append(out, ch)
	}
	if err := rows.Err(); err != nil {
		writeErr(w, err)
		return
	}
	common.WriteJSON(w, http.StatusOK, out, map[string]any{"page": page, "per_page": perPage, "total": total})
}

// ArchiveScheme is DELETE /admin/schemes/{id} — a soft archive, never a hard
// delete, so already-generated bills keep their scheme reference.
func (a *App) ArchiveScheme(w http.ResponseWriter, r *http.Request) {
	c, schoolID := common.RequireFinanceStaff(w, r)
	if c == nil {
		return
	}
	id := param(r, "id")
	var before string
	err := a.DB.QueryRowContext(r.Context(),
		`UPDATE billing_schemes s SET status='archived',updated_at=now()
		   FROM (SELECT id, status FROM billing_schemes WHERE id::text=$1 AND school_id=$2 FOR UPDATE) old
		  WHERE s.id=old.id RETURNING old.status`, id, schoolID).Scan(&before)
	if err == sql.ErrNoRows {
		common.WriteError(w, http.StatusNotFound, "scheme not found")
		return
	}
	if err != nil {
		writeErr(w, err)
		return
	}
	a.auditBoth(r.Context(), schoolID, "billing", "scheme", id, "scheme_archived", c.ActiveRole, c.UserID, "",
		map[string]any{"status": before}, map[string]any{"status": "archived"})
	common.WriteJSON(w, http.StatusOK, map[string]any{"id": id, "status": "archived"}, nil)
}

const schemeColumns = `id,academic_year_id::text,COALESCE(grade_level,''),COALESCE(classroom_id::text,''),name,status,installment,due_day,
	COALESCE(notes,''),COALESCE(to_char(effective_from,'YYYY-MM-DD'),''),COALESCE(to_char(effective_to,'YYYY-MM-DD'),''),created_at,updated_at,
	(SELECT ch.id::text FROM billing_scheme_changes ch WHERE ch.scheme_id=billing_schemes.id AND ch.status='pending_approval')`

// ListSchemes is GET /admin/schemes.
func (a *App) ListSchemes(w http.ResponseWriter, r *http.Request) {
	c, schoolID := common.RequireFinanceStaff(w, r)
	if c == nil {
		return
	}
	year := strings.TrimSpace(r.URL.Query().Get("academic_year_id"))
	status := strings.TrimSpace(r.URL.Query().Get("status"))
	grade := strings.TrimSpace(r.URL.Query().Get("grade_level"))
	page, perPage := common.Page(r)

	rows, err := a.DB.QueryContext(r.Context(),
		`SELECT `+schemeColumns+`
		 FROM billing_schemes
		 WHERE school_id=$1 AND ($2='' OR academic_year_id::text=$2) AND ($3='' OR status=$3) AND ($4='' OR grade_level=$4)
		 ORDER BY created_at DESC LIMIT $5 OFFSET $6`,
		schoolID, year, status, grade, perPage, common.Offset(page, perPage))
	if err != nil {
		writeErr(w, err)
		return
	}
	defer rows.Close()

	items := []map[string]any{}
	for rows.Next() {
		m, err := scanScheme(rows)
		if err != nil {
			writeErr(w, err)
			return
		}
		items = append(items, m)
	}
	if err := rows.Err(); err != nil {
		writeErr(w, err)
		return
	}
	rows.Close()
	for _, m := range items {
		components, err := a.schemeComponents(r.Context(), schoolID, m["id"].(string))
		if err != nil {
			writeErr(w, err)
			return
		}
		m["components"] = components
	}
	common.WriteJSON(w, http.StatusOK, items, nil)
}

// GetScheme is GET /admin/schemes/{id}.
func (a *App) GetScheme(w http.ResponseWriter, r *http.Request) {
	c, schoolID := common.RequireFinanceStaff(w, r)
	if c == nil {
		return
	}
	row := a.DB.QueryRowContext(r.Context(),
		`SELECT `+schemeColumns+` FROM billing_schemes WHERE id::text=$1 AND school_id=$2`, param(r, "id"), schoolID)
	m, err := scanScheme(row)
	if err == sql.ErrNoRows {
		common.WriteError(w, http.StatusNotFound, "scheme not found")
		return
	}
	if err != nil {
		writeErr(w, err)
		return
	}
	components, err := a.schemeComponents(r.Context(), schoolID, m["id"].(string))
	if err != nil {
		writeErr(w, err)
		return
	}
	m["components"] = components
	common.WriteJSON(w, http.StatusOK, m, nil)
}

type rowScanner interface{ Scan(dest ...any) error }

func scanScheme(row rowScanner) (map[string]any, error) {
	var (
		id, year, grade, classroom, name, status, notes, from, to string
		installment                                               bool
		dueDay                                                    int
		createdAt, updatedAt                                      time.Time
		pending                                                   sql.NullString
	)
	if err := row.Scan(&id, &year, &grade, &classroom, &name, &status, &installment, &dueDay, &notes, &from, &to,
		&createdAt, &updatedAt, &pending); err != nil {
		return nil, err
	}
	return map[string]any{
		"id": id, "academic_year_id": year, "grade_level": grade, "classroom_id": classroom,
		"name": name, "status": status, "installment": installment, "due_day": dueDay,
		"notes": notes, "effective_from": from, "effective_to": to,
		"pending_change_id": pending.String, "created_at": createdAt, "updated_at": updatedAt,
	}, nil
}

func (a *App) schemeComponents(ctx context.Context, schoolID, schemeID string) ([]map[string]any, error) {
	rows, err := a.DB.QueryContext(ctx,
		`SELECT id,code,name,kind,amount_idr,sort_order,is_free FROM billing_scheme_components
		 WHERE scheme_id::text=$1 AND school_id=$2 ORDER BY sort_order,code`, schemeID, schoolID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var (
			id, code, name, kind string
			amount               int64
			order                int
			free                 bool
		)
		if err := rows.Scan(&id, &code, &name, &kind, &amount, &order, &free); err != nil {
			return nil, err
		}
		out = append(out, map[string]any{
			"id": id, "code": code, "name": name, "kind": kind,
			"amount_idr": amount, "sort_order": order, "is_free": free,
		})
	}
	return out, rows.Err()
}
