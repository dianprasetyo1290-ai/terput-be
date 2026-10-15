package store

import (
	"context"
	"time"
)

// HRDashboard is the HR admin / HR manager landing page (it used to be a
// hardcoded mock in the SPA). Every number is a live count for one school.
type HRDashboard struct {
	ActiveEmployees       int       `json:"active_employees"`
	ContractsEndingSoon   int       `json:"contracts_ending_30d"`
	DocumentsExpiringSoon int       `json:"documents_expiring_30d"`
	AttendanceRecorded    int       `json:"attendance_today_recorded"`
	AttendancePresent     int       `json:"attendance_today_present"`
	AttendanceLate        int       `json:"attendance_today_late"`
	AttendanceAbsent      int       `json:"attendance_today_absent"`
	OnLeaveToday          int       `json:"on_leave_today"`
	PendingLeave          int       `json:"pending_leave_requests"`
	PendingOvertime       int       `json:"pending_overtime_requests"`
	PendingProfileChanges int       `json:"pending_profile_changes"`
	OpenDocumentRequests  int       `json:"open_document_requests"`
	OpenRequisitions      int       `json:"open_requisitions"`
	OpenPostings          int       `json:"open_postings"`
	ActiveApplications    int       `json:"active_applications"`
	TrainingOverdue       int       `json:"training_overdue"`
	TrainingInProgress    int       `json:"training_in_progress"`
	GeneratedAt           time.Time `json:"generated_at"`
}

func GetHRDashboard(ctx context.Context, q Q, schoolID string, today time.Time) (*HRDashboard, error) {
	d := &HRDashboard{GeneratedAt: time.Now().UTC()}
	day := today.Format("2006-01-02")
	err := q.QueryRowContext(ctx, `
		SELECT
		  (SELECT COUNT(*) FROM employees WHERE school_id = $1 AND status = 'active' AND deleted_at IS NULL),
		  (SELECT COUNT(DISTINCT ea.employee_id) FROM employment_assignments ea
		     JOIN employees e ON e.id = ea.employee_id AND e.status = 'active' AND e.deleted_at IS NULL
		    WHERE ea.school_id = $1 AND ea.effective_to IS NULL
		      AND ea.contract_end BETWEEN $2::date AND $2::date + 30),
		  (SELECT COUNT(*) FROM employee_documents d
		     JOIN employees e ON e.id = d.employee_id AND e.status = 'active' AND e.deleted_at IS NULL
		    WHERE d.school_id = $1 AND d.superseded_at IS NULL
		      AND d.expires_on BETWEEN $2::date AND $2::date + 30),
		  (SELECT COUNT(*) FROM employee_attendance WHERE school_id = $1 AND work_date = $2::date),
		  (SELECT COUNT(*) FROM employee_attendance WHERE school_id = $1 AND work_date = $2::date AND status IN ('hadir', 'terlambat', 'dinas_luar')),
		  (SELECT COUNT(*) FROM employee_attendance WHERE school_id = $1 AND work_date = $2::date AND status = 'terlambat'),
		  (SELECT COUNT(*) FROM employee_attendance WHERE school_id = $1 AND work_date = $2::date AND status = 'alpa'),
		  (SELECT COUNT(DISTINCT employee_id) FROM leave_requests
		    WHERE school_id = $1 AND status = 'approved' AND $2::date BETWEEN start_date AND end_date),
		  (SELECT COUNT(*) FROM leave_requests WHERE school_id = $1 AND status = 'pending'),
		  (SELECT COUNT(*) FROM overtime_requests WHERE school_id = $1 AND status = 'pending'),
		  (SELECT COUNT(*) FROM profile_change_requests WHERE school_id = $1 AND status = 'pending'),
		  (SELECT COUNT(*) FROM document_requests WHERE school_id = $1 AND status = 'open'),
		  (SELECT COUNT(*) FROM requisitions WHERE school_id = $1 AND status IN ('pending_approval', 'approved')),
		  (SELECT COUNT(*) FROM job_postings WHERE school_id = $1 AND status = 'open'),
		  (SELECT COUNT(*) FROM applications WHERE school_id = $1 AND status = 'active'),
		  (SELECT COUNT(*) FROM training_assignments
		    WHERE school_id = $1 AND status IN ('not_started', 'in_progress') AND due_date < $2::date),
		  (SELECT COUNT(*) FROM training_assignments WHERE school_id = $1 AND status = 'in_progress')
	`, schoolID, day).Scan(
		&d.ActiveEmployees, &d.ContractsEndingSoon, &d.DocumentsExpiringSoon,
		&d.AttendanceRecorded, &d.AttendancePresent, &d.AttendanceLate, &d.AttendanceAbsent,
		&d.OnLeaveToday, &d.PendingLeave, &d.PendingOvertime, &d.PendingProfileChanges,
		&d.OpenDocumentRequests, &d.OpenRequisitions, &d.OpenPostings, &d.ActiveApplications,
		&d.TrainingOverdue, &d.TrainingInProgress,
	)
	if err != nil {
		return nil, err
	}
	return d, nil
}
