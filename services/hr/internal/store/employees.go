package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"skci/hr/internal/model"
)

// activeOn is the half-open [effective_from, effective_to) test used for
// every effective-dated table (matches the gist EXCLUDE on assignments).
func activeOn(alias, param string) string {
	return fmt.Sprintf("%[1]s.effective_from <= %[2]s AND (%[1]s.effective_to IS NULL OR %[1]s.effective_to > %[2]s)", alias, param)
}

// employeeSelect joins the assignment active on $2 (BR-HR-02: current
// assignment is computed, never a view).
var employeeSelect = `SELECT e.id, e.school_id, e.user_id::text, e.employee_no, e.full_name, e.email,
	e.phone_enc, e.nik_enc, e.nik_bidx, e.npwp_enc, e.npwp_bidx, e.ptkp_status,
	e.bank_code, e.bank_account_enc, e.bank_account_name, e.birth_date::text, e.gender,
	e.address_enc, e.hire_date::text, e.exit_date::text, e.exit_reason, e.status, e.created_at, e.updated_at,
	cur.position_id::text, cur.position_name, cur.department_id::text, cur.department_name,
	cur.employment_type, cur.contract_end::text, cur.base_salary_enc,
	cur.supervisor_employee_id::text, cur.supervisor_name
	FROM employees e
	LEFT JOIN LATERAL (
		SELECT ea.position_id, p.name AS position_name, ea.department_id, d.name AS department_name,
		       ea.employment_type, ea.contract_end, ea.base_salary_enc,
		       ea.supervisor_employee_id, sup.full_name AS supervisor_name
		FROM employment_assignments ea
		JOIN positions p ON p.id = ea.position_id
		LEFT JOIN departments d ON d.id = ea.department_id
		LEFT JOIN employees sup ON sup.id = ea.supervisor_employee_id
		WHERE ea.employee_id = e.id AND ` + activeOn("ea", "$2::date") + `
		ORDER BY ea.effective_from DESC LIMIT 1
	) cur ON true
	WHERE e.school_id = $1 AND e.deleted_at IS NULL`

func scanEmployee(r interface{ Scan(...any) error }) (model.Employee, error) {
	var e model.Employee
	err := r.Scan(&e.ID, &e.SchoolID, &e.UserID, &e.EmployeeNo, &e.FullName, &e.Email,
		&e.PhoneEnc, &e.NIKEnc, &e.NIKBidx, &e.NPWPEnc, &e.NPWPBidx, &e.PTKPStatus,
		&e.BankCode, &e.BankAccountEnc, &e.BankAccountName, &e.BirthDate, &e.Gender,
		&e.AddressEnc, &e.HireDate, &e.ExitDate, &e.ExitReason, &e.Status, &e.CreatedAt, &e.UpdatedAt,
		&e.CurrentPositionID, &e.CurrentPositionName, &e.CurrentDepartmentID, &e.CurrentDepartmentName,
		&e.CurrentEmploymentType, &e.CurrentContractEnd, &e.BaseSalaryEnc,
		&e.CurrentSupervisorID, &e.CurrentSupervisorName)
	return e, err
}

// EmployeeFilter narrows ListEmployees.
type EmployeeFilter struct {
	Search, Q                                     string
	DepartmentID, PositionID, Status, EmploymentType string
	Page, PerPage                                 int
}

// ListEmployees pages the school's employees with their assignment on `on`.
// PerPage 0 returns every row (export).
func ListEmployees(ctx context.Context, q Q, schoolID string, f EmployeeFilter, on ...string) ([]model.Employee, int, error) {
	onDate := time.Now().Format("2006-01-02")
	if len(on) > 0 && on[0] != "" {
		onDate = on[0]
	}

	where := ""
	args := []any{schoolID, onDate}
	add := func(cond string, v any) {
		args = append(args, v)
		where += fmt.Sprintf(" AND "+cond, len(args))
	}

	searchTerm := f.Search
	if searchTerm == "" && f.Q != "" {
		searchTerm = f.Q
	}
	if s := strings.TrimSpace(searchTerm); s != "" {
		add("(e.full_name ILIKE $%[1]d OR e.employee_no ILIKE $%[1]d OR e.email ILIKE $%[1]d)", "%"+s+"%")
	}
	if f.Status != "" {
		add("e.status = $%d", f.Status)
	}
	if f.DepartmentID != "" {
		add("cur.department_id = $%d::uuid", f.DepartmentID)
	}
	if f.PositionID != "" {
		add("cur.position_id = $%d::uuid", f.PositionID)
	}
	if f.EmploymentType != "" {
		add("cur.employment_type = $%d", f.EmploymentType)
	}

	var total int
	if err := q.QueryRowContext(ctx, `SELECT count(*) FROM (`+employeeSelect+where+`) t`, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	sqlText := employeeSelect + where + " ORDER BY e.full_name"
	if f.PerPage > 0 {
		sqlText += fmt.Sprintf(" LIMIT %d OFFSET %d", f.PerPage, (f.Page-1)*f.PerPage)
	}
	rows, err := q.QueryContext(ctx, sqlText, args...)
	out, err := collect(rows, err, func(r *sqlRows) (model.Employee, error) { return scanEmployee(r) })
	return out, total, err
}

// GetEmployee loads one employee with the assignment active on `on`.
func GetEmployee(ctx context.Context, q Q, schoolID, id, on string) (model.Employee, error) {
	e, err := scanEmployee(q.QueryRowContext(ctx, employeeSelect+` AND e.id = $3::uuid`, schoolID, on, id))
	return e, one(err)
}

// GetEmployeeByID returns a pointer to the employee active on today or given date.
func GetEmployeeByID(ctx context.Context, q Q, schoolID, id string, on ...string) (*model.Employee, error) {
	onDate := time.Now().Format("2006-01-02")
	if len(on) > 0 && on[0] != "" {
		onDate = on[0]
	}
	e, err := GetEmployee(ctx, q, schoolID, id, onDate)
	if err != nil {
		return nil, err
	}
	return &e, nil
}

// EmployeeByUser loads the employee linked to an identity account.
func EmployeeByUser(ctx context.Context, q Q, schoolID, userID, on string) (model.Employee, error) {
	e, err := scanEmployee(q.QueryRowContext(ctx, employeeSelect+` AND e.user_id = $3::uuid`, schoolID, on, userID))
	return e, one(err)
}

// GetEmployeeByUserID returns a pointer to the employee linked to an identity account.
func GetEmployeeByUserID(ctx context.Context, q Q, schoolID, userID string, on ...string) (*model.Employee, error) {
	onDate := time.Now().Format("2006-01-02")
	if len(on) > 0 && on[0] != "" {
		onDate = on[0]
	}
	e, err := EmployeeByUser(ctx, q, schoolID, userID, onDate)
	if err != nil {
		return nil, err
	}
	return &e, nil
}

// FindDuplicates returns existing employees matching the NIK blind index or email.
func FindDuplicates(ctx context.Context, q Q, schoolID, nikBidx, email string, excludeID ...string) ([]model.DuplicateMatch, error) {
	ex := ""
	if len(excludeID) > 0 {
		ex = excludeID[0]
	}
	rows, err := q.QueryContext(ctx, `SELECT id, employee_no, full_name, email, nik_bidx FROM employees
		WHERE school_id = $1 AND deleted_at IS NULL AND ($4 = '' OR id <> $4::uuid)
		  AND ((nik_bidx = $2 AND $2 <> '') OR (lower(email) = lower($3) AND $3 <> ''))`, schoolID, nikBidx, email, ex)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var matches []model.DuplicateMatch
	for rows.Next() {
		var (
			id, empNo, name string
			em, nb          sql.NullString
		)
		if err := rows.Scan(&id, &empNo, &name, &em, &nb); err != nil {
			return nil, err
		}
		reason := "Data duplikat terdeteksi"
		if nb.Valid && nb.String == nikBidx && nikBidx != "" {
			reason = "NIK sudah terdaftar"
		} else if em.Valid && strings.EqualFold(em.String, email) && email != "" {
			reason = "Email sudah terdaftar"
		}
		matches = append(matches, model.DuplicateMatch{
			ID:          id,
			EmployeeNo:  empNo,
			FullName:    name,
			Email:       em.String,
			MatchReason: reason,
		})
	}
	return matches, rows.Err()
}

// InsertEmployee creates the employees row.
func InsertEmployee(ctx context.Context, q Q, e *model.Employee) error {
	return q.QueryRowContext(ctx, `INSERT INTO employees (school_id, user_id, employee_no, full_name, email, phone_enc,
		nik_enc, nik_bidx, npwp_enc, npwp_bidx, ptkp_status, bank_code, bank_account_enc, bank_account_name,
		birth_date, gender, address_enc, hire_date, status)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19) RETURNING id, created_at, updated_at`,
		e.SchoolID, e.UserID, e.EmployeeNo, e.FullName, e.Email, e.PhoneEnc,
		e.NIKEnc, e.NIKBidx, e.NPWPEnc, e.NPWPBidx, e.PTKPStatus, e.BankCode, e.BankAccountEnc, e.BankAccountName,
		e.BirthDate, e.Gender, e.AddressEnc, e.HireDate, e.Status).Scan(&e.ID, &e.CreatedAt, &e.UpdatedAt)
}

// CreateEmployee creates the employee and initial assignment inside a transaction.
func CreateEmployee(ctx context.Context, db *sql.DB, e *model.Employee, a *model.EmploymentAssignment) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if e.EmployeeNo == "" {
		var nextNum int
		err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(SUBSTRING(employee_no FROM '[0-9]+')::int), 0) + 1 FROM employees WHERE school_id = $1`, e.SchoolID).Scan(&nextNum)
		if err != nil || nextNum == 0 {
			nextNum = 1
		}
		e.EmployeeNo = fmt.Sprintf("EMP-%04d", nextNum)
	}

	if e.Status == "" {
		e.Status = "active"
	}

	if err := InsertEmployee(ctx, tx, e); err != nil {
		return err
	}

	if a != nil && a.PositionID != "" {
		a.EmployeeID = e.ID
		a.SchoolID = e.SchoolID
		if a.EffectiveFrom == "" {
			a.EffectiveFrom = e.HireDate
		}
		if a.EmploymentType == "" {
			a.EmploymentType = "kontrak"
		}
		if err := InsertAssignment(ctx, tx, a); err != nil {
			return err
		}
	}

	return tx.Commit()
}

// UpdateEmployee rewrites the profile columns (assignments are separate).
func UpdateEmployee(ctx context.Context, q Q, e *model.Employee) error {
	return affected(q.ExecContext(ctx, `UPDATE employees SET employee_no=$3, full_name=$4, email=$5, phone_enc=$6,
		nik_enc=$7, nik_bidx=$8, npwp_enc=$9, npwp_bidx=$10, ptkp_status=$11, bank_code=$12, bank_account_enc=$13,
		bank_account_name=$14, birth_date=$15, gender=$16, address_enc=$17, hire_date=$18, user_id=$19, updated_at=now()
		WHERE school_id=$1 AND id=$2 AND deleted_at IS NULL`,
		e.SchoolID, e.ID, e.EmployeeNo, e.FullName, e.Email, e.PhoneEnc,
		e.NIKEnc, e.NIKBidx, e.NPWPEnc, e.NPWPBidx, e.PTKPStatus, e.BankCode, e.BankAccountEnc,
		e.BankAccountName, e.BirthDate, e.Gender, e.AddressEnc, e.HireDate, e.UserID))
}

// SetEmployeeStatus records an exit (offboarding) or activation.
func SetEmployeeStatus(ctx context.Context, q Q, schoolID, id, status string, exitDate, exitReason *string) error {
	return affected(q.ExecContext(ctx, `UPDATE employees SET status=$3, exit_date=$4, exit_reason=$5, updated_at=now()
		WHERE school_id=$1 AND id=$2 AND deleted_at IS NULL`, schoolID, id, status, exitDate, exitReason))
}

// OffboardEmployee sets employee status to exited and records exit date and reason.
func OffboardEmployee(ctx context.Context, q Q, schoolID, id, exitDate, reason string) error {
	return SetEmployeeStatus(ctx, q, schoolID, id, "exited", &exitDate, &reason)
}

// DueExits returns active employees whose exit_date has arrived.
func DueExits(ctx context.Context, q Q, schoolID, today string) ([]string, error) {
	rows, err := q.QueryContext(ctx, `SELECT id FROM employees WHERE school_id=$1 AND status='active'
		AND exit_date IS NOT NULL AND exit_date <= $2::date AND deleted_at IS NULL`, schoolID, today)
	return collect(rows, err, func(r *sqlRows) (string, error) {
		var id string
		return id, r.Scan(&id)
	})
}

const assignmentSelect = `SELECT ea.id, ea.school_id, ea.employee_id, ea.position_id, p.name,
	ea.department_id::text, d.name, ea.supervisor_employee_id::text, sup.full_name,
	ea.employment_type, ea.grade, ea.base_salary_enc, ea.contract_end::text,
	ea.effective_from::text, ea.effective_to::text, ea.created_at, ea.updated_at
	FROM employment_assignments ea
	JOIN positions p ON p.id = ea.position_id
	LEFT JOIN departments d ON d.id = ea.department_id
	LEFT JOIN employees sup ON sup.id = ea.supervisor_employee_id
	WHERE ea.school_id = $1 AND ea.employee_id = $2`

func scanAssignment(r interface{ Scan(...any) error }) (model.EmploymentAssignment, error) {
	var a model.EmploymentAssignment
	err := r.Scan(&a.ID, &a.SchoolID, &a.EmployeeID, &a.PositionID, &a.PositionName,
		&a.DepartmentID, &a.DepartmentName, &a.SupervisorEmployeeID, &a.SupervisorName,
		&a.EmploymentType, &a.Grade, &a.BaseSalaryEnc, &a.ContractEnd,
		&a.EffectiveFrom, &a.EffectiveTo, &a.CreatedAt, &a.UpdatedAt)
	return a, err
}

// CurrentAssignment is the assignment active on `on` (ErrNotFound if none).
func CurrentAssignment(ctx context.Context, q Q, schoolID, employeeID, on string) (model.EmploymentAssignment, error) {
	a, err := scanAssignment(q.QueryRowContext(ctx, assignmentSelect+` AND `+activeOn("ea", "$3::date")+` ORDER BY ea.effective_from DESC LIMIT 1`, schoolID, employeeID, on))
	return a, one(err)
}

// Assignments is the full history, newest first (FR-83-002).
func Assignments(ctx context.Context, q Q, schoolID, employeeID string) ([]model.EmploymentAssignment, error) {
	rows, err := q.QueryContext(ctx, assignmentSelect+` ORDER BY ea.effective_from DESC`, schoolID, employeeID)
	return collect(rows, err, func(r *sqlRows) (model.EmploymentAssignment, error) { return scanAssignment(r) })
}

// GetEmployeeTimeline returns the employee assignment history.
func GetEmployeeTimeline(ctx context.Context, q Q, schoolID, employeeID string) ([]model.EmploymentAssignment, error) {
	return Assignments(ctx, q, schoolID, employeeID)
}

// HasAssignmentFrom reports whether a row starts on/after `from`, which a
// new effective-dated change may not jump in front of.
func HasAssignmentFrom(ctx context.Context, q Q, employeeID, from string) (bool, error) {
	var ok bool
	err := q.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM employment_assignments WHERE employee_id=$1 AND effective_from >= $2::date)`, employeeID, from).Scan(&ok)
	return ok, err
}

// CloseAssignment ends the row active on `at` at that date (BR-HR-02: the
// only mutation an assignment ever gets).
func CloseAssignment(ctx context.Context, q Q, schoolID, employeeID, at string) error {
	_, err := q.ExecContext(ctx, `UPDATE employment_assignments ea SET effective_to = $3::date, updated_at = now()
		WHERE ea.school_id = $1 AND ea.employee_id = $2 AND ea.effective_from < $3::date
		  AND (ea.effective_to IS NULL OR ea.effective_to > $3::date)`, schoolID, employeeID, at)
	return err
}

// InsertAssignment adds a new effective-dated row.
func InsertAssignment(ctx context.Context, q Q, a *model.EmploymentAssignment) error {
	return q.QueryRowContext(ctx, `INSERT INTO employment_assignments (school_id, employee_id, position_id, department_id,
		supervisor_employee_id, employment_type, grade, base_salary_enc, contract_end, effective_from, effective_to)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11) RETURNING id, created_at, updated_at`,
		a.SchoolID, a.EmployeeID, a.PositionID, a.DepartmentID, a.SupervisorEmployeeID, a.EmploymentType,
		a.Grade, a.BaseSalaryEnc, a.ContractEnd, a.EffectiveFrom, a.EffectiveTo).Scan(&a.ID, &a.CreatedAt, &a.UpdatedAt)
}

// CreateAssignment closes active assignment and inserts new one inside a transaction.
func CreateAssignment(ctx context.Context, db *sql.DB, a *model.EmploymentAssignment) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if err := CloseAssignment(ctx, tx, a.SchoolID, a.EmployeeID, a.EffectiveFrom); err != nil {
		return err
	}

	if err := InsertAssignment(ctx, tx, a); err != nil {
		return err
	}

	return tx.Commit()
}

// ReportsTo reports whether `ancestor` is somewhere above `employeeID` in the
// supervisor chain on `on` (cycle guard for new assignments).
func ReportsTo(ctx context.Context, q Q, employeeID, ancestor, on string) (bool, error) {
	var ok bool
	err := q.QueryRowContext(ctx, `WITH RECURSIVE chain(id, depth) AS (
			SELECT $1::uuid, 0
			UNION ALL
			SELECT ea.supervisor_employee_id, c.depth + 1 FROM chain c
			JOIN employment_assignments ea ON ea.employee_id = c.id AND `+activeOn("ea", "$3::date")+`
			WHERE ea.supervisor_employee_id IS NOT NULL AND c.depth < 50)
		SELECT EXISTS (SELECT 1 FROM chain WHERE id = $2::uuid AND depth > 0)`, employeeID, ancestor, on).Scan(&ok)
	return ok, err
}

// SupervisorUserID resolves the direct supervisor of an account (AC-84-01).
// found=false: no active assignment or no supervisor; userID nil: the
// supervisor has no identity account yet.
func SupervisorUserID(ctx context.Context, q Q, schoolID, subjectUserID, on string) (userID *string, found bool, err error) {
	err = q.QueryRowContext(ctx, `SELECT sup.user_id::text FROM employees sub
		JOIN employment_assignments ea ON ea.employee_id = sub.id AND `+activeOn("ea", "$3::date")+`
		JOIN employees sup ON sup.id = ea.supervisor_employee_id
		WHERE sub.school_id = $1 AND sub.user_id = $2::uuid AND sub.deleted_at IS NULL LIMIT 1`, schoolID, subjectUserID, on).Scan(&userID)
	if err = one(err); err == ErrNotFound {
		return nil, false, nil
	}
	return userID, err == nil, err
}

// LogOrgChange appends to org_change_log (assignment, department, position).
func LogOrgChange(ctx context.Context, q Q, schoolID, entityType, entityID string, before, after map[string]any, effectiveDate string, actor string) error {
	b, err := jsonOrNil(before)
	if err != nil {
		return err
	}
	a, err := jsonOrNil(after)
	if err != nil {
		return err
	}
	_, err = q.ExecContext(ctx, `INSERT INTO org_change_log (school_id, entity_type, entity_id, before, after, effective_date, actor_user_id)
		VALUES ($1,$2,$3,$4,$5,$6,NULLIF($7,'')::uuid)`, schoolID, entityType, entityID, b, a, effectiveDate, actor)
	return err
}

// OrgChanges lists the change log for one entity, newest first.
func OrgChanges(ctx context.Context, q Q, schoolID, entityType, entityID string) ([]model.OrgChangeLog, error) {
	rows, err := q.QueryContext(ctx, `SELECT id, school_id, entity_type, entity_id, before, after, effective_date::text,
		actor_user_id::text, created_at FROM org_change_log WHERE school_id=$1 AND entity_type=$2 AND entity_id=$3
		ORDER BY effective_date DESC, created_at DESC`, schoolID, entityType, entityID)
	return collect(rows, err, func(r *sqlRows) (model.OrgChangeLog, error) {
		var l model.OrgChangeLog
		var before, after []byte
		err := r.Scan(&l.ID, &l.SchoolID, &l.EntityType, &l.EntityID, &before, &after, &l.EffectiveDate, &l.ActorUserID, &l.CreatedAt)
		l.Before, l.After = json.RawMessage(before), json.RawMessage(after)
		return l, err
	})
}
