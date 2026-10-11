package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"skci/hr/internal/model"
)

func parseDays(s string) []int16 {
	s = strings.Trim(s, "{}")
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	var res []int16
	for _, p := range parts {
		var v int16
		if _, err := fmt.Sscanf(strings.TrimSpace(p), "%d", &v); err == nil {
			res = append(res, v)
		}
	}
	return res
}

func formatDays(days []int16) string {
	var parts []string
	for _, d := range days {
		parts = append(parts, fmt.Sprintf("%d", d))
	}
	return "{" + strings.Join(parts, ",") + "}"
}

// --- Work Schedules ---

func ListWorkSchedules(ctx context.Context, q Q, schoolID string) ([]model.WorkSchedule, error) {
	rows, err := q.QueryContext(ctx, `SELECT id, school_id, name, position_id::text, department_id::text,
		days::text, start_time::text, end_time::text, late_tolerance_minutes,
		effective_from::text, effective_to::text, created_at, updated_at
		FROM work_schedules WHERE school_id = $1 ORDER BY name`, schoolID)
	return collect(rows, err, func(r *sqlRows) (model.WorkSchedule, error) {
		var ws model.WorkSchedule
		var daysStr string
		err := r.Scan(&ws.ID, &ws.SchoolID, &ws.Name, &ws.PositionID, &ws.DepartmentID,
			&daysStr, &ws.StartTime, &ws.EndTime, &ws.LateToleranceMinutes,
			&ws.EffectiveFrom, &ws.EffectiveTo, &ws.CreatedAt, &ws.UpdatedAt)
		ws.Days = parseDays(daysStr)
		return ws, err
	})
}

func CreateWorkSchedule(ctx context.Context, q Q, ws *model.WorkSchedule) error {
	daysStr := formatDays(ws.Days)
	if ws.EffectiveFrom == "" {
		ws.EffectiveFrom = time.Now().Format("2006-01-02")
	}
	return q.QueryRowContext(ctx, `INSERT INTO work_schedules (school_id, name, position_id, department_id,
		days, start_time, end_time, late_tolerance_minutes, effective_from, effective_to)
		VALUES ($1,$2,NULLIF($3,'')::uuid,NULLIF($4,'')::uuid,$5::smallint[],$6::time,$7::time,$8,$9::date,NULLIF($10,'')::date)
		RETURNING id, created_at, updated_at`,
		ws.SchoolID, ws.Name, ws.PositionID, ws.DepartmentID, daysStr, ws.StartTime, ws.EndTime, ws.LateToleranceMinutes,
		ws.EffectiveFrom, ws.EffectiveTo).Scan(&ws.ID, &ws.CreatedAt, &ws.UpdatedAt)
}

func UpdateWorkSchedule(ctx context.Context, q Q, ws *model.WorkSchedule) error {
	daysStr := formatDays(ws.Days)
	return affected(q.ExecContext(ctx, `UPDATE work_schedules SET name=$3, position_id=NULLIF($4,'')::uuid,
		department_id=NULLIF($5,'')::uuid, days=$6::smallint[], start_time=$7::time, end_time=$8::time,
		late_tolerance_minutes=$9, effective_from=$10::date, effective_to=NULLIF($11,'')::date, updated_at=now()
		WHERE school_id=$1 AND id=$2`,
		ws.SchoolID, ws.ID, ws.Name, ws.PositionID, ws.DepartmentID, daysStr, ws.StartTime, ws.EndTime,
		ws.LateToleranceMinutes, ws.EffectiveFrom, ws.EffectiveTo))
}

func DeleteWorkSchedule(ctx context.Context, q Q, schoolID, id string) error {
	return affected(q.ExecContext(ctx, `DELETE FROM work_schedules WHERE school_id=$1 AND id=$2`, schoolID, id))
}

// --- Holidays ---

func ListHolidays(ctx context.Context, q Q, schoolID string, year int) ([]model.Holiday, error) {
	sqlText := `SELECT id, school_id, date::text, name, created_at FROM holidays WHERE school_id=$1`
	args := []any{schoolID}
	if year > 0 {
		sqlText += ` AND EXTRACT(YEAR FROM date) = $2`
		args = append(args, year)
	}
	sqlText += ` ORDER BY date`
	rows, err := q.QueryContext(ctx, sqlText, args...)
	return collect(rows, err, func(r *sqlRows) (model.Holiday, error) {
		var h model.Holiday
		err := r.Scan(&h.ID, &h.SchoolID, &h.Date, &h.Name, &h.CreatedAt)
		return h, err
	})
}

func CreateHoliday(ctx context.Context, q Q, h *model.Holiday) error {
	return q.QueryRowContext(ctx, `INSERT INTO holidays (school_id, date, name)
		VALUES ($1,$2::date,$3) RETURNING id, created_at`,
		h.SchoolID, h.Date, h.Name).Scan(&h.ID, &h.CreatedAt)
}

func DeleteHoliday(ctx context.Context, q Q, schoolID, id string) error {
	return affected(q.ExecContext(ctx, `DELETE FROM holidays WHERE school_id=$1 AND id=$2`, schoolID, id))
}

// --- Attendance ---

func ListAttendance(ctx context.Context, q Q, schoolID, month, empID string, page, perPage int) ([]model.EmployeeAttendance, int, error) {
	where := " WHERE a.school_id = $1"
	args := []any{schoolID}

	if month != "" {
		args = append(args, month)
		where += fmt.Sprintf(" AND to_char(a.work_date, 'YYYY-MM') = $%d", len(args))
	}
	if empID != "" {
		args = append(args, empID)
		where += fmt.Sprintf(" AND a.employee_id = $%d::uuid", len(args))
	}

	var total int
	countQuery := `SELECT count(*) FROM employee_attendance a` + where
	if err := q.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	sqlText := `SELECT a.id, a.school_id, a.employee_id, e.full_name, e.employee_no, a.work_date::text,
		a.status, a.check_in_at, a.check_out_at, a.lat, a.lng, a.source, a.import_batch_id::text, a.note,
		a.created_at, a.updated_at
		FROM employee_attendance a
		JOIN employees e ON e.id = a.employee_id` + where + ` ORDER BY a.work_date DESC, e.full_name`

	if perPage > 0 {
		sqlText += fmt.Sprintf(" LIMIT %d OFFSET %d", perPage, (page-1)*perPage)
	}

	rows, err := q.QueryContext(ctx, sqlText, args...)
	out, err := collect(rows, err, func(r *sqlRows) (model.EmployeeAttendance, error) {
		var a model.EmployeeAttendance
		err := r.Scan(&a.ID, &a.SchoolID, &a.EmployeeID, &a.EmployeeName, &a.EmployeeNo, &a.WorkDate,
			&a.Status, &a.CheckInAt, &a.CheckOutAt, &a.Lat, &a.Lng, &a.Source, &a.ImportBatchID, &a.Note,
			&a.CreatedAt, &a.UpdatedAt)
		return a, err
	})
	return out, total, err
}

func GetAttendanceRecord(ctx context.Context, q Q, schoolID, id string) (*model.EmployeeAttendance, error) {
	var a model.EmployeeAttendance
	err := q.QueryRowContext(ctx, `SELECT a.id, a.school_id, a.employee_id, e.full_name, e.employee_no, a.work_date::text,
		a.status, a.check_in_at, a.check_out_at, a.lat, a.lng, a.source, a.import_batch_id::text, a.note,
		a.created_at, a.updated_at
		FROM employee_attendance a
		JOIN employees e ON e.id = a.employee_id
		WHERE a.school_id = $1 AND a.id = $2`, schoolID, id).
		Scan(&a.ID, &a.SchoolID, &a.EmployeeID, &a.EmployeeName, &a.EmployeeNo, &a.WorkDate,
			&a.Status, &a.CheckInAt, &a.CheckOutAt, &a.Lat, &a.Lng, &a.Source, &a.ImportBatchID, &a.Note,
			&a.CreatedAt, &a.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &a, nil
}

func GetAttendanceByEmployeeAndDate(ctx context.Context, q Q, schoolID, empID, date string) (*model.EmployeeAttendance, error) {
	var a model.EmployeeAttendance
	err := q.QueryRowContext(ctx, `SELECT a.id, a.school_id, a.employee_id, e.full_name, e.employee_no, a.work_date::text,
		a.status, a.check_in_at, a.check_out_at, a.lat, a.lng, a.source, a.import_batch_id::text, a.note,
		a.created_at, a.updated_at
		FROM employee_attendance a
		JOIN employees e ON e.id = a.employee_id
		WHERE a.school_id = $1 AND a.employee_id = $2 AND a.work_date = $3::date`, schoolID, empID, date).
		Scan(&a.ID, &a.SchoolID, &a.EmployeeID, &a.EmployeeName, &a.EmployeeNo, &a.WorkDate,
			&a.Status, &a.CheckInAt, &a.CheckOutAt, &a.Lat, &a.Lng, &a.Source, &a.ImportBatchID, &a.Note,
			&a.CreatedAt, &a.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &a, nil
}

func RecordAttendance(ctx context.Context, q Q, a *model.EmployeeAttendance) error {
	return q.QueryRowContext(ctx, `INSERT INTO employee_attendance (school_id, employee_id, work_date, status,
		check_in_at, check_out_at, lat, lng, source, import_batch_id, note)
		VALUES ($1,$2,$3::date,$4,$5,$6,$7,$8,$9,NULLIF($10,'')::uuid,$11)
		ON CONFLICT (employee_id, work_date) DO UPDATE SET
			status = EXCLUDED.status,
			check_in_at = COALESCE(EXCLUDED.check_in_at, employee_attendance.check_in_at),
			check_out_at = COALESCE(EXCLUDED.check_out_at, employee_attendance.check_out_at),
			lat = COALESCE(EXCLUDED.lat, employee_attendance.lat),
			lng = COALESCE(EXCLUDED.lng, employee_attendance.lng),
			source = EXCLUDED.source,
			note = COALESCE(EXCLUDED.note, employee_attendance.note),
			updated_at = now()
		RETURNING id, created_at, updated_at`,
		a.SchoolID, a.EmployeeID, a.WorkDate, a.Status, a.CheckInAt, a.CheckOutAt, a.Lat, a.Lng,
		a.Source, a.ImportBatchID, a.Note).Scan(&a.ID, &a.CreatedAt, &a.UpdatedAt)
}

func IsPeriodLocked(ctx context.Context, q Q, schoolID, month string) (bool, error) {
	if len(month) > 7 {
		month = month[:7]
	}
	var status string
	err := q.QueryRowContext(ctx, `SELECT status FROM attendance_periods WHERE school_id = $1 AND period = $2`, schoolID, month).Scan(&status)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return status == "locked" || status == "sent", nil
}

func SendAttendancePeriod(ctx context.Context, q Q, schoolID, period, actorID string) error {
	if len(period) > 7 {
		period = period[:7]
	}
	var actor any
	if actorID != "" {
		actor = actorID
	}
	_, err := q.ExecContext(ctx, `INSERT INTO attendance_periods (school_id, period, status, sent_at, sent_by)
		VALUES ($1, $2, 'locked', now(), NULLIF($3,'')::uuid)
		ON CONFLICT (school_id, period) DO UPDATE SET status = 'locked', sent_at = now(), sent_by = NULLIF(EXCLUDED.sent_by::text,'')::uuid, updated_at = now()`,
		schoolID, period, actor)
	return err
}

func GetAttendanceRecap(ctx context.Context, q Q, schoolID, month string) ([]model.AttendanceRecapRow, error) {
	if len(month) > 7 {
		month = month[:7]
	}
	onDate := month + "-01"

	rows, err := q.QueryContext(ctx, `
		SELECT e.employee_no, e.full_name, COALESCE(d.name, '-'), COALESCE(p.name, '-'),
			COUNT(CASE WHEN a.status = 'hadir' THEN 1 END) AS hadir,
			COUNT(CASE WHEN a.status = 'terlambat' THEN 1 END) AS terlambat,
			COUNT(CASE WHEN a.status = 'izin' THEN 1 END) AS izin,
			COUNT(CASE WHEN a.status = 'sakit' THEN 1 END) AS sakit,
			COUNT(CASE WHEN a.status = 'cuti' THEN 1 END) AS cuti,
			COUNT(CASE WHEN a.status = 'dinas_luar' THEN 1 END) AS dinas_luar,
			COUNT(CASE WHEN a.status = 'alpa' THEN 1 END) AS alpa,
			COALESCE(SUM(ot.minutes), 0) AS overtime_minutes
		FROM employees e
		LEFT JOIN LATERAL (
			SELECT ea.department_id, ea.position_id FROM employment_assignments ea
			WHERE ea.employee_id = e.id AND `+activeOn("ea", "$2::date")+`
			ORDER BY ea.effective_from DESC LIMIT 1
		) cur ON true
		LEFT JOIN departments d ON d.id = cur.department_id
		LEFT JOIN positions p ON p.id = cur.position_id
		LEFT JOIN employee_attendance a ON a.employee_id = e.id AND to_char(a.work_date, 'YYYY-MM') = $3
		LEFT JOIN (
			SELECT employee_id, SUM(minutes) AS minutes
			FROM overtime_requests
			WHERE school_id = $1 AND to_char(work_date, 'YYYY-MM') = $3 AND status = 'approved'
			GROUP BY employee_id
		) ot ON ot.employee_id = e.id
		WHERE e.school_id = $1 AND e.status = 'active' AND e.deleted_at IS NULL
		GROUP BY e.employee_no, e.full_name, d.name, p.name
		ORDER BY e.full_name
	`, schoolID, onDate, month)
	return collect(rows, err, func(r *sqlRows) (model.AttendanceRecapRow, error) {
		var row model.AttendanceRecapRow
		err := r.Scan(&row.EmployeeNo, &row.FullName, &row.Department, &row.Position,
			&row.Hadir, &row.Terlambat, &row.Izin, &row.Sakit, &row.Cuti, &row.DinasLuar, &row.Alpa,
			&row.OvertimeMinutes)
		return row, err
	})
}
