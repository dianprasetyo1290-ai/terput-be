package handler

import (
	"context"
	"database/sql"
	"encoding/csv"
	"net/http"
	"strconv"
	"strings"
	"time"

	"skci/billing/internal/common"
	"skci/billing/internal/model"
)

// ---------------------------------------------------------------------------
// Scheme configuration (admin CRUD)
// ---------------------------------------------------------------------------

type componentInput struct {
	Code      string `json:"code"`
	Name      string `json:"name"`
	Kind      string `json:"kind"` // monthly | one_time
	AmountIDR int64  `json:"amount_idr"`
	SortOrder int    `json:"sort_order"`
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
	Components     []componentInput `json:"components"`
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
	for _, c := range in.Components {
		if strings.TrimSpace(c.Code) == "" || strings.TrimSpace(c.Name) == "" {
			return &httpError{http.StatusUnprocessableEntity, "every component needs a code and a name"}
		}
		if c.AmountIDR < 0 {
			return &httpError{http.StatusUnprocessableEntity, "component amount must not be negative"}
		}
		if c.Kind != "" && c.Kind != "monthly" && c.Kind != "one_time" {
			return &httpError{http.StatusUnprocessableEntity, "component kind must be monthly or one_time"}
		}
	}
	return nil
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
	tx, err := a.DB.BeginTx(r.Context(), nil)
	if err != nil {
		writeErr(w, err)
		return
	}
	defer func() { _ = tx.Rollback() }()

	var id string
	err = tx.QueryRowContext(r.Context(),
		`INSERT INTO billing_schemes(school_id,academic_year_id,grade_level,classroom_id,name,status,installment,due_day,notes)
		 VALUES($1,$2,NULLIF($3,''),NULLIF($4,'')::uuid,$5,$6,$7,$8,NULLIF($9,''))
		 RETURNING id`,
		schoolID, in.AcademicYearID, in.GradeLevel, in.ClassroomID, in.Name, schemeStatus(in),
		in.Installment, schemeDueDay(in), in.Notes).Scan(&id)
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
	a.audit(r.Context(), schoolID, "scheme", id, "scheme_created", "admin", "", "", "", nil,
		map[string]any{"name": in.Name, "components": len(in.Components)})
	common.WriteJSON(w, http.StatusCreated, map[string]any{"id": id}, nil)
}

// UpdateScheme is PUT /admin/schemes/{id}. Components are replaced when the
// payload carries any.
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

	res, err := tx.ExecContext(r.Context(),
		`UPDATE billing_schemes SET academic_year_id=$1,grade_level=NULLIF($2,''),classroom_id=NULLIF($3,'')::uuid,
		        name=$4,status=$5,installment=$6,due_day=$7,notes=NULLIF($8,''),updated_at=now()
		 WHERE id::text=$9 AND school_id=$10`,
		in.AcademicYearID, in.GradeLevel, in.ClassroomID, in.Name, schemeStatus(in),
		in.Installment, schemeDueDay(in), in.Notes, id, schoolID)
	if err != nil {
		writeErr(w, err)
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		common.WriteError(w, http.StatusNotFound, "scheme not found")
		return
	}
	if len(in.Components) > 0 {
		if _, err = tx.ExecContext(r.Context(),
			`DELETE FROM billing_scheme_components WHERE scheme_id::text=$1 AND school_id=$2`, id, schoolID); err != nil {
			writeErr(w, err)
			return
		}
		if err = insertComponents(r.Context(), tx, schoolID, id, in.Components); err != nil {
			writeErr(w, err)
			return
		}
	}
	if err = tx.Commit(); err != nil {
		writeErr(w, err)
		return
	}
	a.audit(r.Context(), schoolID, "scheme", id, "scheme_updated", "admin", "", "", "", nil,
		map[string]any{"name": in.Name, "components": len(in.Components)})
	common.WriteJSON(w, http.StatusOK, map[string]any{"id": id}, nil)
}

func insertComponents(ctx context.Context, tx *sql.Tx, schoolID, schemeID string, components []componentInput) error {
	for i, c := range components {
		order := c.SortOrder
		if order == 0 {
			order = i + 1
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO billing_scheme_components(school_id,scheme_id,code,name,kind,amount_idr,sort_order)
			 VALUES($1,$2,$3,$4,$5,$6,$7)`,
			schoolID, schemeID, c.Code, c.Name, componentKind(c), c.AmountIDR, order); err != nil {
			return err
		}
	}
	return nil
}

// ArchiveScheme is DELETE /admin/schemes/{id} — a soft archive, never a hard
// delete, so already-generated bills keep their scheme reference.
func (a *App) ArchiveScheme(w http.ResponseWriter, r *http.Request) {
	c, schoolID := common.RequireFinanceStaff(w, r)
	if c == nil {
		return
	}
	id := param(r, "id")
	res, err := a.DB.ExecContext(r.Context(),
		`UPDATE billing_schemes SET status='archived',updated_at=now() WHERE id::text=$1 AND school_id=$2`, id, schoolID)
	if err != nil {
		writeErr(w, err)
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		common.WriteError(w, http.StatusNotFound, "scheme not found")
		return
	}
	a.audit(r.Context(), schoolID, "scheme", id, "scheme_archived", "admin", "", "", "", nil, nil)
	common.WriteJSON(w, http.StatusOK, map[string]any{"id": id, "status": "archived"}, nil)
}

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
		`SELECT id,academic_year_id::text,COALESCE(grade_level,''),COALESCE(classroom_id::text,''),name,status,installment,due_day,COALESCE(notes,''),created_at,updated_at
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
		components, err := a.schemeComponents(r.Context(), schoolID, m["id"].(string))
		if err != nil {
			writeErr(w, err)
			return
		}
		m["components"] = components
		items = append(items, m)
	}
	if err := rows.Err(); err != nil {
		writeErr(w, err)
		return
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
		`SELECT id,academic_year_id::text,COALESCE(grade_level,''),COALESCE(classroom_id::text,''),name,status,installment,due_day,COALESCE(notes,''),created_at,updated_at
		 FROM billing_schemes WHERE id::text=$1 AND school_id=$2`, param(r, "id"), schoolID)
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
		id, year, grade, classroom, name, status, notes string
		installment                                     bool
		dueDay                                          int
		createdAt, updatedAt                            time.Time
	)
	if err := row.Scan(&id, &year, &grade, &classroom, &name, &status, &installment, &dueDay, &notes, &createdAt, &updatedAt); err != nil {
		return nil, err
	}
	return map[string]any{
		"id": id, "academic_year_id": year, "grade_level": grade, "classroom_id": classroom,
		"name": name, "status": status, "installment": installment, "due_day": dueDay,
		"notes": notes, "created_at": createdAt, "updated_at": updatedAt,
	}, nil
}

func (a *App) schemeComponents(ctx context.Context, schoolID, schemeID string) ([]map[string]any, error) {
	rows, err := a.DB.QueryContext(ctx,
		`SELECT id,code,name,kind,amount_idr,sort_order FROM billing_scheme_components
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
		)
		if err := rows.Scan(&id, &code, &name, &kind, &amount, &order); err != nil {
			return nil, err
		}
		out = append(out, map[string]any{
			"id": id, "code": code, "name": name, "kind": kind,
			"amount_idr": amount, "sort_order": order,
		})
	}
	return out, rows.Err()
}

// ---------------------------------------------------------------------------
// Bill generation, listing and arrears
// ---------------------------------------------------------------------------

// GenerateBills is POST /admin/bills/generate. It is idempotent per
// (school, scheme, student, period).
func (a *App) GenerateBills(w http.ResponseWriter, r *http.Request) {
	claims, schoolID := common.RequireFinanceStaff(w, r)
	if claims == nil {
		return
	}
	var in generateRequest
	if !common.Decode(w, r, &in) {
		return
	}
	result, err := a.generateBills(r.Context(), schoolID, in)
	if err != nil {
		writeErr(w, err)
		return
	}
	common.WriteJSON(w, http.StatusOK, result, nil)
}

// ListBills is GET /admin/bills.
func (a *App) ListBills(w http.ResponseWriter, r *http.Request) {
	c, schoolID := common.RequireBillReader(w, r)
	if c == nil {
		return
	}
	if schoolID == "" {
		common.WriteError(w, http.StatusBadRequest, "school_id is required")
		return
	}
	studentID := strings.TrimSpace(r.URL.Query().Get("student_user_id"))
	if !a.teacherMaySee(w, r, c, schoolID, studentID) {
		return
	}
	status := strings.TrimSpace(r.URL.Query().Get("status"))
	schemeID := strings.TrimSpace(r.URL.Query().Get("scheme_id"))
	year := common.IntParam(r, "period_year")
	month := common.IntParam(r, "period_month")
	page, perPage := common.Page(r)

	rows, err := a.DB.QueryContext(r.Context(),
		`SELECT b.id,b.student_user_id::text,b.scheme_id::text,COALESCE(s.name,''),b.status,b.period_year,b.period_month,
		        b.base_idr,b.discount_idr,b.net_idr,b.due_at,b.created_at,
		        COALESCE((SELECT sum(al.amount_idr) FROM bill_payment_allocations al JOIN bill_payments p ON p.id=al.payment_id
		                  WHERE p.student_bill_id=b.id AND p.school_id=b.school_id AND p.status='verified'),0),
		        COALESCE((SELECT sum(rv.amount_idr) FROM bill_payment_reversals rv JOIN bill_payments p ON p.id=rv.payment_id
		                  WHERE p.student_bill_id=b.id AND p.school_id=b.school_id),0)
		 FROM student_bills b LEFT JOIN billing_schemes s ON s.id=b.scheme_id
		 WHERE b.school_id=$1
		   AND ($2='' OR b.student_user_id::text=$2)
		   AND ($3='' OR b.status=$3)
		   AND ($4='' OR b.scheme_id::text=$4)
		   AND ($5=0 OR b.period_year=$5)
		   AND ($6=0 OR b.period_month=$6)
		 ORDER BY b.period_year DESC,b.period_month DESC,b.created_at DESC
		 LIMIT $7 OFFSET $8`,
		schoolID, studentID, status, schemeID, year, month, perPage, common.Offset(page, perPage))
	if err != nil {
		writeErr(w, err)
		return
	}
	defer rows.Close()

	items := []map[string]any{}
	for rows.Next() {
		var (
			id, student, scheme, schemeName, billStatus string
			y, m                                        int
			base, discount, net, verified, reversed     int64
			dueAt                                       sql.NullTime
			createdAt                                   time.Time
		)
		if err := rows.Scan(&id, &student, &scheme, &schemeName, &billStatus, &y, &m,
			&base, &discount, &net, &dueAt, &createdAt, &verified, &reversed); err != nil {
			writeErr(w, err)
			return
		}
		outstanding, err := model.Outstanding(net, []int64{verified}, []int64{reversed})
		if err != nil {
			writeErr(w, err)
			return
		}
		item := billSummary(id, scheme, schemeName, billStatus, y, m, base, discount, net, verified, reversed, outstanding, dueAt, createdAt)
		item["student_user_id"] = student
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		writeErr(w, err)
		return
	}
	common.WriteJSON(w, http.StatusOK, items, nil)
}

// ListArrears is GET /admin/arrears — bills that still owe money.
func (a *App) ListArrears(w http.ResponseWriter, r *http.Request) {
	c, schoolID := common.RequireFinanceViewer(w, r)
	if c == nil {
		return
	}
	year := common.IntParam(r, "period_year")
	month := common.IntParam(r, "period_month")
	schemeID := strings.TrimSpace(r.URL.Query().Get("scheme_id"))
	page, perPage := common.Page(r)

	rows, err := a.DB.QueryContext(r.Context(),
		`SELECT * FROM (
		   SELECT b.id,b.student_user_id::text,b.scheme_id::text,COALESCE(s.name,''),b.status,b.period_year,b.period_month,
		          b.base_idr,b.discount_idr,b.net_idr,b.due_at,b.created_at,
		          COALESCE((SELECT sum(al.amount_idr) FROM bill_payment_allocations al JOIN bill_payments p ON p.id=al.payment_id
		                    WHERE p.student_bill_id=b.id AND p.school_id=b.school_id AND p.status='verified'),0) AS verified,
		          COALESCE((SELECT sum(rv.amount_idr) FROM bill_payment_reversals rv JOIN bill_payments p ON p.id=rv.payment_id
		                    WHERE p.student_bill_id=b.id AND p.school_id=b.school_id),0) AS reversed
		   FROM student_bills b LEFT JOIN billing_schemes s ON s.id=b.scheme_id
		   WHERE b.school_id=$1 AND b.status IN('unpaid','partial')
		     AND ($2=0 OR b.period_year=$2) AND ($3=0 OR b.period_month=$3) AND ($4='' OR b.scheme_id::text=$4)
		 ) t
		 WHERE (t.net_idr - t.verified + t.reversed) > 0
		 ORDER BY t.due_at ASC NULLS LAST, t.period_year, t.period_month
		 LIMIT $5 OFFSET $6`,
		schoolID, year, month, schemeID, perPage, common.Offset(page, perPage))
	if err != nil {
		writeErr(w, err)
		return
	}
	defer rows.Close()

	items := []map[string]any{}
	for rows.Next() {
		var (
			id, student, scheme, schemeName, billStatus string
			y, m                                        int
			base, discount, net, verified, reversed     int64
			dueAt                                       sql.NullTime
			createdAt                                   time.Time
		)
		if err := rows.Scan(&id, &student, &scheme, &schemeName, &billStatus, &y, &m,
			&base, &discount, &net, &dueAt, &createdAt, &verified, &reversed); err != nil {
			writeErr(w, err)
			return
		}
		outstanding, err := model.Outstanding(net, []int64{verified}, []int64{reversed})
		if err != nil {
			writeErr(w, err)
			return
		}
		item := billSummary(id, scheme, schemeName, billStatus, y, m, base, discount, net, verified, reversed, outstanding, dueAt, createdAt)
		item["student_user_id"] = student
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		writeErr(w, err)
		return
	}
	common.WriteJSON(w, http.StatusOK, items, nil)
}

// ---------------------------------------------------------------------------
// Reconciliation export
// ---------------------------------------------------------------------------

// ExportReconciliation is GET /admin/reconciliation.csv.
func (a *App) ExportReconciliation(w http.ResponseWriter, r *http.Request) {
	c, schoolID := common.RequireFinanceStaff(w, r)
	if c == nil {
		return
	}
	year := common.IntParam(r, "period_year")
	month := common.IntParam(r, "period_month")
	status := strings.TrimSpace(r.URL.Query().Get("status"))

	rows, err := a.DB.QueryContext(r.Context(),
		`SELECT p.id,p.student_bill_id::text,b.student_user_id::text,b.period_year,b.period_month,
		        p.amount_idr,p.status,COALESCE(p.bank_name,''),COALESCE(p.account_name,''),COALESCE(p.reference,''),
		        p.transfer_at,p.verified_at,p.created_at,
		        COALESCE((SELECT sum(al.amount_idr) FROM bill_payment_allocations al WHERE al.payment_id=p.id),0),
		        COALESCE((SELECT sum(rv.amount_idr) FROM bill_payment_reversals rv WHERE rv.payment_id=p.id),0),
		        b.net_idr
		 FROM bill_payments p JOIN student_bills b ON b.id=p.student_bill_id
		 WHERE p.school_id=$1 AND ($2=0 OR b.period_year=$2) AND ($3=0 OR b.period_month=$3) AND ($4='' OR p.status=$4)
		 ORDER BY p.created_at`,
		schoolID, year, month, status)
	if err != nil {
		writeErr(w, err)
		return
	}
	defer rows.Close()

	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="rekonsiliasi-pembayaran.csv"`)
	cw := csv.NewWriter(w)
	defer cw.Flush()
	_ = cw.Write([]string{
		"payment_id", "bill_id", "student_user_id", "period_year", "period_month",
		"amount_idr", "status", "bank_name", "account_name", "reference",
		"transfer_at", "verified_at", "created_at", "allocated_idr", "reversed_idr",
		"net_idr", "outstanding_idr",
	})
	for rows.Next() {
		var (
			id, billID, studentID, payStatus string
			bank, account, reference         string
			year, month                      int
			amount, allocated, reversed, net int64
			transferAt, verifiedAt           sql.NullTime
			createdAt                        time.Time
		)
		if err := rows.Scan(&id, &billID, &studentID, &year, &month, &amount, &payStatus,
			&bank, &account, &reference, &transferAt, &verifiedAt, &createdAt,
			&allocated, &reversed, &net); err != nil {
			writeErr(w, err)
			return
		}
		outstanding, err := model.Outstanding(net, []int64{allocated}, []int64{reversed})
		if err != nil {
			writeErr(w, err)
			return
		}
		_ = cw.Write([]string{
			id, billID, studentID, strconv.Itoa(year), strconv.Itoa(month),
			strconv.FormatInt(amount, 10), payStatus, bank, account, reference,
			csvTime(transferAt), csvTime(verifiedAt), createdAt.Format(time.RFC3339),
			strconv.FormatInt(allocated, 10), strconv.FormatInt(reversed, 10),
			strconv.FormatInt(net, 10), strconv.FormatInt(outstanding, 10),
		})
	}
	if err := rows.Err(); err != nil {
		common.CaptureError(err)
	}
}

func csvTime(t sql.NullTime) string {
	if !t.Valid {
		return ""
	}
	return t.Time.Format(time.RFC3339)
}

// teacherMaySee enforces that a guru/walikelas only reads bills of one
// student at a time, and only a student of a class they teach or are wali
// kelas of (resolved via academic-service). Admins pass straight through.
// Fails closed: a roster lookup error denies.
func (a *App) teacherMaySee(w http.ResponseWriter, r *http.Request, c *common.Claims, schoolID, studentID string) bool {
	if c.ActiveRole != common.RoleGuru && c.ActiveRole != common.RoleWalikelas {
		return true
	}
	if studentID == "" {
		common.WriteError(w, http.StatusForbidden, "student_user_id is required for teachers")
		return false
	}
	if a.Roster == nil {
		common.WriteError(w, http.StatusForbidden, "class roster unavailable")
		return false
	}
	allowed, err := a.Roster.TeacherStudentIDs(r.Context(), schoolID, c.UserID)
	if err != nil {
		common.CaptureError(err)
		common.WriteError(w, http.StatusForbidden, "class roster unavailable")
		return false
	}
	if !allowed[studentID] {
		common.WriteError(w, http.StatusForbidden, "student is not in your classes")
		return false
	}
	return true
}
