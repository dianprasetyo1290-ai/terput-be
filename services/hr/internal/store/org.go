package store

import (
	"context"
	"fmt"
	"strings"
	"time"

	"skci/hr/internal/model"
)

// --- departments ---

func ListDepartments(ctx context.Context, q Q, schoolID string) ([]model.Department, error) {
	rows, err := q.QueryContext(ctx, `SELECT id, school_id, code, name, parent_id::text, active, created_at, updated_at
		FROM departments WHERE school_id=$1 AND deleted_at IS NULL ORDER BY code`, schoolID)
	return collect(rows, err, func(r *sqlRows) (model.Department, error) {
		var d model.Department
		return d, r.Scan(&d.ID, &d.SchoolID, &d.Code, &d.Name, &d.ParentID, &d.Active, &d.CreatedAt, &d.UpdatedAt)
	})
}

func SaveDepartment(ctx context.Context, q Q, d *model.Department) error {
	if d.ID == "" {
		return q.QueryRowContext(ctx, `INSERT INTO departments (school_id, code, name, parent_id, active)
			VALUES ($1,$2,$3,$4,$5) RETURNING id, created_at, updated_at`,
			d.SchoolID, d.Code, d.Name, d.ParentID, d.Active).Scan(&d.ID, &d.CreatedAt, &d.UpdatedAt)
	}
	return one(q.QueryRowContext(ctx, `UPDATE departments SET code=$3, name=$4, parent_id=$5, active=$6, updated_at=now()
		WHERE school_id=$1 AND id=$2 AND deleted_at IS NULL RETURNING created_at, updated_at`,
		d.SchoolID, d.ID, d.Code, d.Name, d.ParentID, d.Active).Scan(&d.CreatedAt, &d.UpdatedAt))
}

func CreateDepartment(ctx context.Context, q Q, d *model.Department) error {
	return SaveDepartment(ctx, q, d)
}

func UpdateDepartment(ctx context.Context, q Q, d *model.Department) error {
	return SaveDepartment(ctx, q, d)
}

func DeleteDepartment(ctx context.Context, q Q, schoolID, id string) error {
	return affected(q.ExecContext(ctx, `UPDATE departments SET deleted_at=now() WHERE school_id=$1 AND id=$2 AND deleted_at IS NULL`, schoolID, id))
}

// --- positions ---

const positionSelect = `SELECT p.id, p.school_id, p.code, p.name, p.level, p.job_description, p.department_id::text,
	d.name, p.salary_min_idr, p.salary_max_idr, p.created_at, p.updated_at
	FROM positions p LEFT JOIN departments d ON d.id = p.department_id
	WHERE p.school_id=$1 AND p.deleted_at IS NULL`

func scanPosition(r interface{ Scan(...any) error }) (model.Position, error) {
	var p model.Position
	err := r.Scan(&p.ID, &p.SchoolID, &p.Code, &p.Name, &p.Level, &p.JobDescription, &p.DepartmentID,
		&p.DepartmentName, &p.SalaryMinIDR, &p.SalaryMaxIDR, &p.CreatedAt, &p.UpdatedAt)
	return p, err
}

func ListPositions(ctx context.Context, q Q, schoolID string, deptID ...string) ([]model.Position, error) {
	sqlText := positionSelect
	args := []any{schoolID}
	if len(deptID) > 0 && strings.TrimSpace(deptID[0]) != "" {
		sqlText += " AND p.department_id = $2::uuid"
		args = append(args, strings.TrimSpace(deptID[0]))
	}
	sqlText += ` ORDER BY p.level, p.code`
	rows, err := q.QueryContext(ctx, sqlText, args...)
	return collect(rows, err, func(r *sqlRows) (model.Position, error) { return scanPosition(r) })
}

func GetPosition(ctx context.Context, q Q, schoolID, id string) (model.Position, error) {
	p, err := scanPosition(q.QueryRowContext(ctx, positionSelect+` AND p.id=$2::uuid`, schoolID, id))
	return p, one(err)
}

func SavePosition(ctx context.Context, q Q, p *model.Position) error {
	if p.ID == "" {
		return q.QueryRowContext(ctx, `INSERT INTO positions (school_id, code, name, level, job_description, department_id, salary_min_idr, salary_max_idr)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8) RETURNING id, created_at, updated_at`,
			p.SchoolID, p.Code, p.Name, p.Level, p.JobDescription, p.DepartmentID, p.SalaryMinIDR, p.SalaryMaxIDR).Scan(&p.ID, &p.CreatedAt, &p.UpdatedAt)
	}
	return one(q.QueryRowContext(ctx, `UPDATE positions SET code=$3, name=$4, level=$5, job_description=$6, department_id=$7,
		salary_min_idr=$8, salary_max_idr=$9, updated_at=now() WHERE school_id=$1 AND id=$2 AND deleted_at IS NULL
		RETURNING created_at, updated_at`,
		p.SchoolID, p.ID, p.Code, p.Name, p.Level, p.JobDescription, p.DepartmentID, p.SalaryMinIDR, p.SalaryMaxIDR).Scan(&p.CreatedAt, &p.UpdatedAt))
}

func CreatePosition(ctx context.Context, q Q, p *model.Position) error {
	return SavePosition(ctx, q, p)
}

func UpdatePosition(ctx context.Context, q Q, p *model.Position) error {
	return SavePosition(ctx, q, p)
}

func DeletePosition(ctx context.Context, q Q, schoolID, id string) error {
	return affected(q.ExecContext(ctx, `UPDATE positions SET deleted_at=now() WHERE school_id=$1 AND id=$2 AND deleted_at IS NULL`, schoolID, id))
}

// --- headcount plans (FR-84-006) ---

func ListHeadcountPlans(ctx context.Context, q Q, schoolID string, year ...int) ([]model.HeadcountPlan, error) {
	sqlText := `SELECT hp.id, hp.school_id, hp.position_id, p.name, p.code, hp.department_id::text, d.name,
		hp.planned, hp.effective_from::text, hp.effective_to::text, hp.created_at, hp.updated_at
		FROM headcount_plans hp JOIN positions p ON p.id = hp.position_id LEFT JOIN departments d ON d.id = hp.department_id
		WHERE hp.school_id=$1`
	args := []any{schoolID}
	if len(year) > 0 && year[0] > 0 {
		sqlText += fmt.Sprintf(" AND EXTRACT(YEAR FROM hp.effective_from) = $%d", len(args)+1)
		args = append(args, year[0])
	}
	sqlText += ` ORDER BY p.code, hp.effective_from DESC`
	rows, err := q.QueryContext(ctx, sqlText, args...)
	return collect(rows, err, func(r *sqlRows) (model.HeadcountPlan, error) {
		var h model.HeadcountPlan
		err := r.Scan(&h.ID, &h.SchoolID, &h.PositionID, &h.PositionName, &h.PositionCode, &h.DepartmentID, &h.DepartmentName,
			&h.Planned, &h.EffectiveFrom, &h.EffectiveTo, &h.CreatedAt, &h.UpdatedAt)
		h.PlannedCount = h.Planned
		return h, err
	})
}

func SaveHeadcountPlan(ctx context.Context, q Q, h *model.HeadcountPlan) error {
	if h.ID == "" {
		return q.QueryRowContext(ctx, `INSERT INTO headcount_plans (school_id, position_id, department_id, planned, effective_from, effective_to)
			VALUES ($1,$2,$3,$4,$5,$6) RETURNING id, created_at, updated_at`,
			h.SchoolID, h.PositionID, h.DepartmentID, h.Planned, h.EffectiveFrom, h.EffectiveTo).Scan(&h.ID, &h.CreatedAt, &h.UpdatedAt)
	}
	return one(q.QueryRowContext(ctx, `UPDATE headcount_plans SET position_id=$3, department_id=$4, planned=$5, effective_from=$6,
		effective_to=$7, updated_at=now() WHERE school_id=$1 AND id=$2 RETURNING created_at, updated_at`,
		h.SchoolID, h.ID, h.PositionID, h.DepartmentID, h.Planned, h.EffectiveFrom, h.EffectiveTo).Scan(&h.CreatedAt, &h.UpdatedAt))
}

func CreateHeadcountPlan(ctx context.Context, q Q, h *model.HeadcountPlan) error {
	if h.Planned == 0 && h.PlannedCount > 0 {
		h.Planned = h.PlannedCount
	}
	if h.EffectiveFrom == "" {
		if h.Year > 0 {
			h.EffectiveFrom = fmt.Sprintf("%04d-01-01", h.Year)
		} else {
			h.EffectiveFrom = time.Now().Format("2006-01-02")
		}
	}
	return SaveHeadcountPlan(ctx, q, h)
}

func DeleteHeadcountPlan(ctx context.Context, q Q, schoolID, id string) error {
	return affected(q.ExecContext(ctx, `DELETE FROM headcount_plans WHERE school_id=$1 AND id=$2`, schoolID, id))
}

// Headcount is planned / filled / vacant per position on `on` (AC-84-02).
// positionID "" returns every position.
func Headcount(ctx context.Context, q Q, schoolID, on, positionID string) ([]model.HeadcountSummary, error) {
	rows, err := q.QueryContext(ctx, `WITH plan AS (
			SELECT position_id, sum(planned) AS planned FROM headcount_plans hp
			WHERE school_id=$1 AND `+activeOn("hp", "$2::date")+` GROUP BY position_id),
		filled AS (
			SELECT ea.position_id, count(DISTINCT ea.employee_id) AS filled
			FROM employment_assignments ea JOIN employees e ON e.id = ea.employee_id
			WHERE ea.school_id=$1 AND e.status='active' AND e.deleted_at IS NULL AND `+activeOn("ea", "$2::date")+`
			GROUP BY ea.position_id)
		SELECT p.id, p.code, p.name, COALESCE(plan.planned,0), COALESCE(filled.filled,0)
		FROM positions p LEFT JOIN plan ON plan.position_id = p.id LEFT JOIN filled ON filled.position_id = p.id
		WHERE p.school_id=$1 AND p.deleted_at IS NULL AND ($3 = '' OR p.id::text = $3)
		ORDER BY p.level, p.code`, schoolID, on, positionID)
	return collect(rows, err, func(r *sqlRows) (model.HeadcountSummary, error) {
		var s model.HeadcountSummary
		err := r.Scan(&s.PositionID, &s.PositionCode, &s.PositionName, &s.Planned, &s.Filled)
		s.Vacant = max(s.Planned-s.Filled, 0)
		s.IsFull = s.Vacant == 0
		return s, err
	})
}

func GetHeadcountOverview(ctx context.Context, q Q, schoolID string, year ...int) ([]model.HeadcountSummary, error) {
	on := time.Now().Format("2006-01-02")
	if len(year) > 0 && year[0] > 0 {
		on = fmt.Sprintf("%04d-12-31", year[0])
	}
	return Headcount(ctx, q, schoolID, on, "")
}

func GetPositionHeadcount(ctx context.Context, q Q, schoolID, positionID string) (planned, actual, available int, err error) {
	on := time.Now().Format("2006-01-02")
	summaries, err := Headcount(ctx, q, schoolID, on, positionID)
	if err != nil {
		return 0, 0, 0, err
	}
	if len(summaries) == 0 {
		return 0, 0, 0, nil
	}
	return summaries[0].Planned, summaries[0].Filled, summaries[0].Vacant, nil
}

// OrgChart builds the reporting tree from active assignments (FR-84-005).
func OrgChart(ctx context.Context, q Q, schoolID, on, departmentID string) ([]*model.OrgNode, error) {
	rows, err := q.QueryContext(ctx, `SELECT e.id, e.employee_no, e.full_name, ea.position_id, p.name,
		ea.department_id::text, d.name, ea.supervisor_employee_id::text
		FROM employees e
		JOIN employment_assignments ea ON ea.employee_id = e.id AND `+activeOn("ea", "$2::date")+`
		JOIN positions p ON p.id = ea.position_id
		LEFT JOIN departments d ON d.id = ea.department_id
		WHERE e.school_id=$1 AND e.status='active' AND e.deleted_at IS NULL
		  AND ($3 = '' OR ea.department_id::text = $3)
		ORDER BY p.level, e.full_name`, schoolID, on, departmentID)
	nodes, err := collect(rows, err, func(r *sqlRows) (*model.OrgNode, error) {
		n := &model.OrgNode{}
		return n, r.Scan(&n.ID, &n.EmployeeNo, &n.FullName, &n.PositionID, &n.PositionName, &n.DepartmentID, &n.DepartmentName, &n.SupervisorID)
	})
	if err != nil {
		return nil, err
	}
	byID := map[string]*model.OrgNode{}
	for _, n := range nodes {
		byID[n.ID] = n
	}
	roots := []*model.OrgNode{}
	for _, n := range nodes {
		if n.SupervisorID != nil && byID[*n.SupervisorID] != nil {
			p := byID[*n.SupervisorID]
			p.Children = append(p.Children, n)
			continue
		}
		roots = append(roots, n)
	}
	return roots, nil
}

func GetOrgChart(ctx context.Context, q Q, schoolID string) ([]*model.OrgNode, error) {
	return OrgChart(ctx, q, schoolID, time.Now().Format("2006-01-02"), "")
}
