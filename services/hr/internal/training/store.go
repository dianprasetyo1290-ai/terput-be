package training

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"skci/hr/internal/training/calc"
)

var (
	ErrNotFound = errors.New("not found")
	ErrConflict = errors.New("conflict")
)

type Store struct {
	DB *sql.DB
}

func NewStore(db *sql.DB) *Store {
	return &Store{DB: db}
}

// -----------------------------------------------------------------------------
// Categories & Competencies
// -----------------------------------------------------------------------------

func (s *Store) ListCategories(ctx context.Context, schoolID string) ([]Category, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT id, school_id, name, created_at, updated_at
		FROM training_categories
		WHERE school_id = $1 AND deleted_at IS NULL
		ORDER BY name ASC`, schoolID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []Category
	for rows.Next() {
		var c Category
		if err := rows.Scan(&c.ID, &c.SchoolID, &c.Name, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, err
		}
		list = append(list, c)
	}
	return list, rows.Err()
}

func (s *Store) CreateCategory(ctx context.Context, schoolID, name string) (*Category, error) {
	var c Category
	err := s.DB.QueryRowContext(ctx, `
		INSERT INTO training_categories (school_id, name, created_at, updated_at)
		VALUES ($1, $2, now(), now())
		RETURNING id, school_id, name, created_at, updated_at`, schoolID, name).Scan(
		&c.ID, &c.SchoolID, &c.Name, &c.CreatedAt, &c.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (s *Store) ListCompetencies(ctx context.Context, schoolID string) ([]Competency, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT id, school_id, code, name, created_at, updated_at
		FROM competencies
		WHERE school_id = $1 AND deleted_at IS NULL
		ORDER BY code ASC`, schoolID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []Competency
	for rows.Next() {
		var c Competency
		if err := rows.Scan(&c.ID, &c.SchoolID, &c.Code, &c.Name, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, err
		}
		list = append(list, c)
	}
	return list, rows.Err()
}

func (s *Store) CreateCompetency(ctx context.Context, schoolID, code, name string) (*Competency, error) {
	var c Competency
	err := s.DB.QueryRowContext(ctx, `
		INSERT INTO competencies (school_id, code, name, created_at, updated_at)
		VALUES ($1, $2, $3, now(), now())
		RETURNING id, school_id, code, name, created_at, updated_at`, schoolID, code, name).Scan(
		&c.ID, &c.SchoolID, &c.Code, &c.Name, &c.CreatedAt, &c.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// -----------------------------------------------------------------------------
// Courses
// -----------------------------------------------------------------------------

func (s *Store) ListCourses(ctx context.Context, schoolID string, statusFilter string) ([]Course, error) {
	query := `
		SELECT c.id, c.school_id, c.title, c.objective, c.duration_minutes, c.provider, c.kind,
		       c.category_id, coalesce(cat.name, ''), c.cost_idr, c.quota, c.passing_score, c.status,
		       c.created_at, c.updated_at,
		       (SELECT count(*) FROM course_materials m WHERE m.course_id = c.id) as mat_count,
		       (SELECT count(*) FROM course_sessions ses WHERE ses.course_id = c.id) as ses_count
		FROM courses c
		LEFT JOIN training_categories cat ON cat.id = c.category_id
		WHERE c.school_id = $1 AND c.deleted_at IS NULL`
	args := []interface{}{schoolID}
	if statusFilter != "" {
		query += " AND c.status = $2"
		args = append(args, statusFilter)
	}
	query += " ORDER BY c.created_at DESC"

	rows, err := s.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []Course
	for rows.Next() {
		var cr Course
		if err := rows.Scan(
			&cr.ID, &cr.SchoolID, &cr.Title, &cr.Objective, &cr.DurationMinutes, &cr.Provider, &cr.Kind,
			&cr.CategoryID, &cr.CategoryName, &cr.CostIDR, &cr.Quota, &cr.PassingScore, &cr.Status,
			&cr.CreatedAt, &cr.UpdatedAt, &cr.MaterialsCount, &cr.SessionsCount,
		); err != nil {
			return nil, err
		}
		list = append(list, cr)
	}
	return list, rows.Err()
}

func (s *Store) GetCourse(ctx context.Context, schoolID, id string) (*Course, error) {
	query := `
		SELECT c.id, c.school_id, c.title, c.objective, c.duration_minutes, c.provider, c.kind,
		       c.category_id, coalesce(cat.name, ''), c.cost_idr, c.quota, c.passing_score, c.status,
		       c.created_at, c.updated_at
		FROM courses c
		LEFT JOIN training_categories cat ON cat.id = c.category_id
		WHERE c.school_id = $1 AND c.id = $2 AND c.deleted_at IS NULL`

	var cr Course
	err := s.DB.QueryRowContext(ctx, query, schoolID, id).Scan(
		&cr.ID, &cr.SchoolID, &cr.Title, &cr.Objective, &cr.DurationMinutes, &cr.Provider, &cr.Kind,
		&cr.CategoryID, &cr.CategoryName, &cr.CostIDR, &cr.Quota, &cr.PassingScore, &cr.Status,
		&cr.CreatedAt, &cr.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	} else if err != nil {
		return nil, err
	}
	return &cr, nil
}

func (s *Store) CreateCourse(ctx context.Context, cr Course) (*Course, error) {
	query := `
		INSERT INTO courses (
			school_id, title, objective, duration_minutes, provider, kind,
			category_id, cost_idr, quota, passing_score, status, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, now(), now())
		RETURNING id, created_at, updated_at`

	err := s.DB.QueryRowContext(ctx, query,
		cr.SchoolID, cr.Title, cr.Objective, cr.DurationMinutes, cr.Provider, cr.Kind,
		cr.CategoryID, cr.CostIDR, cr.Quota, cr.PassingScore, cr.Status,
	).Scan(&cr.ID, &cr.CreatedAt, &cr.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &cr, nil
}

func (s *Store) UpdateCourse(ctx context.Context, cr Course) (*Course, error) {
	query := `
		UPDATE courses
		SET title = $1, objective = $2, duration_minutes = $3, provider = $4, kind = $5,
		    category_id = $6, cost_idr = $7, quota = $8, passing_score = $9, status = $10, updated_at = now()
		WHERE school_id = $11 AND id = $12 AND deleted_at IS NULL
		RETURNING updated_at`

	err := s.DB.QueryRowContext(ctx, query,
		cr.Title, cr.Objective, cr.DurationMinutes, cr.Provider, cr.Kind,
		cr.CategoryID, cr.CostIDR, cr.Quota, cr.PassingScore, cr.Status,
		cr.SchoolID, cr.ID,
	).Scan(&cr.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	} else if err != nil {
		return nil, err
	}
	return &cr, nil
}

// -----------------------------------------------------------------------------
// Course Materials, Sessions, Questions
// -----------------------------------------------------------------------------

func (s *Store) ListMaterials(ctx context.Context, courseID string) ([]CourseMaterial, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT id, school_id, course_id, sort, kind, title, object_key, url, created_at
		FROM course_materials
		WHERE course_id = $1
		ORDER BY sort ASC, created_at ASC`, courseID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []CourseMaterial
	for rows.Next() {
		var m CourseMaterial
		if err := rows.Scan(&m.ID, &m.SchoolID, &m.CourseID, &m.Sort, &m.Kind, &m.Title, &m.ObjectKey, &m.URL, &m.CreatedAt); err != nil {
			return nil, err
		}
		list = append(list, m)
	}
	return list, rows.Err()
}

func (s *Store) CreateMaterial(ctx context.Context, m CourseMaterial) (*CourseMaterial, error) {
	err := s.DB.QueryRowContext(ctx, `
		INSERT INTO course_materials (school_id, course_id, sort, kind, title, object_key, url, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, now())
		RETURNING id, created_at`,
		m.SchoolID, m.CourseID, m.Sort, m.Kind, m.Title, m.ObjectKey, m.URL,
	).Scan(&m.ID, &m.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &m, nil
}

func (s *Store) ListSessions(ctx context.Context, courseID string) ([]CourseSession, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT s.id, s.school_id, s.course_id, s.starts_at, s.ends_at, s.location, s.quota,
		       (SELECT count(*) FROM training_assignments a WHERE a.session_id = s.id AND a.status NOT IN ('cancelled', 'rejected')) as enrolled,
		       s.created_at
		FROM course_sessions s
		WHERE s.course_id = $1
		ORDER BY s.starts_at ASC`, courseID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []CourseSession
	for rows.Next() {
		var ses CourseSession
		if err := rows.Scan(&ses.ID, &ses.SchoolID, &ses.CourseID, &ses.StartsAt, &ses.EndsAt, &ses.Location, &ses.Quota, &ses.Enrolled, &ses.CreatedAt); err != nil {
			return nil, err
		}
		list = append(list, ses)
	}
	return list, rows.Err()
}

func (s *Store) CreateSession(ctx context.Context, ses CourseSession) (*CourseSession, error) {
	err := s.DB.QueryRowContext(ctx, `
		INSERT INTO course_sessions (school_id, course_id, starts_at, ends_at, location, quota, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, now())
		RETURNING id, created_at`,
		ses.SchoolID, ses.CourseID, ses.StartsAt, ses.EndsAt, ses.Location, ses.Quota,
	).Scan(&ses.ID, &ses.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &ses, nil
}

func (s *Store) ListQuestions(ctx context.Context, courseID string, includeAnswer bool) ([]QuizQuestion, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT id, school_id, course_id, sort, prompt, options, correct_index, created_at
		FROM quiz_questions
		WHERE course_id = $1
		ORDER BY sort ASC, created_at ASC`, courseID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []QuizQuestion
	for rows.Next() {
		var q QuizQuestion
		if err := rows.Scan(&q.ID, &q.SchoolID, &q.CourseID, &q.Sort, &q.Prompt, &q.Options, &q.CorrectIndex, &q.CreatedAt); err != nil {
			return nil, err
		}
		if !includeAnswer {
			q.CorrectIndex = -1 // Strip key for ESS security
		}
		list = append(list, q)
	}
	return list, rows.Err()
}

func (s *Store) CreateQuestion(ctx context.Context, q QuizQuestion) (*QuizQuestion, error) {
	err := s.DB.QueryRowContext(ctx, `
		INSERT INTO quiz_questions (school_id, course_id, sort, prompt, options, correct_index, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, now())
		RETURNING id, created_at`,
		q.SchoolID, q.CourseID, q.Sort, q.Prompt, q.Options, q.CorrectIndex,
	).Scan(&q.ID, &q.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &q, nil
}

// -----------------------------------------------------------------------------
// Mandatory Trainings & Assignments
// -----------------------------------------------------------------------------

func (s *Store) ListMandatory(ctx context.Context, schoolID string) ([]MandatoryTraining, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT m.id, m.school_id, m.position_id, coalesce(pos.title, ''), m.course_id, c.title, m.due_days, m.created_at
		FROM mandatory_trainings m
		JOIN courses c ON c.id = m.course_id
		LEFT JOIN positions pos ON pos.id = m.position_id
		WHERE m.school_id = $1
		ORDER BY pos.title ASC, c.title ASC`, schoolID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []MandatoryTraining
	for rows.Next() {
		var mt MandatoryTraining
		if err := rows.Scan(&mt.ID, &mt.SchoolID, &mt.PositionID, &mt.PositionName, &mt.CourseID, &mt.CourseTitle, &mt.DueDays, &mt.CreatedAt); err != nil {
			return nil, err
		}
		list = append(list, mt)
	}
	return list, rows.Err()
}

func (s *Store) CreateMandatory(ctx context.Context, schoolID, positionID, courseID string, dueDays int) (*MandatoryTraining, error) {
	var mt MandatoryTraining
	err := s.DB.QueryRowContext(ctx, `
		INSERT INTO mandatory_trainings (school_id, position_id, course_id, due_days, created_at)
		VALUES ($1, $2, $3, $4, now())
		RETURNING id, school_id, position_id, course_id, due_days, created_at`,
		schoolID, positionID, courseID, dueDays,
	).Scan(&mt.ID, &mt.SchoolID, &mt.PositionID, &mt.CourseID, &mt.DueDays, &mt.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &mt, nil
}

func (s *Store) CreateAssignment(ctx context.Context, a Assignment) (*Assignment, error) {
	status := a.Status
	if status == "" {
		status = "not_started"
	}
	err := s.DB.QueryRowContext(ctx, `
		INSERT INTO training_assignments (
			school_id, employee_id, course_id, session_id, source, due_date, status, approval_id, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, now(), now())
		RETURNING id, created_at, updated_at`,
		a.SchoolID, a.EmployeeID, a.CourseID, a.SessionID, a.Source, a.DueDate, status, a.ApprovalID,
	).Scan(&a.ID, &a.CreatedAt, &a.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &a, nil
}

func (s *Store) ListAssignments(ctx context.Context, schoolID string, employeeID string) ([]Assignment, error) {
	query := `
		SELECT a.id, a.school_id, a.employee_id, coalesce(e.full_name, ''),
		       a.course_id, c.title, a.session_id, a.source, a.due_date, a.status,
		       a.approval_id, a.started_at, a.completed_at, a.score, a.certificate_doc_id,
		       a.created_at, a.updated_at
		FROM training_assignments a
		JOIN courses c ON c.id = a.course_id
		LEFT JOIN employees e ON e.id = a.employee_id
		WHERE a.school_id = $1`
	args := []interface{}{schoolID}
	if employeeID != "" {
		query += " AND a.employee_id = $2"
		args = append(args, employeeID)
	}
	query += " ORDER BY a.created_at DESC"

	rows, err := s.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []Assignment
	today := time.Now()
	for rows.Next() {
		var as Assignment
		if err := rows.Scan(
			&as.ID, &as.SchoolID, &as.EmployeeID, &as.EmployeeName,
			&as.CourseID, &as.CourseTitle, &as.SessionID, &as.Source, &as.DueDate, &as.Status,
			&as.ApprovalID, &as.StartedAt, &as.CompletedAt, &as.Score, &as.CertificateDocID,
			&as.CreatedAt, &as.UpdatedAt,
		); err != nil {
			return nil, err
		}
		as.DisplayStatus = calc.DisplayStatus(as.Status, as.DueDate, today)
		list = append(list, as)
	}
	return list, rows.Err()
}

func (s *Store) MarkMaterialDone(ctx context.Context, assignmentID, materialID string) error {
	_, err := s.DB.ExecContext(ctx, `
		INSERT INTO material_progress (assignment_id, material_id, done_at)
		VALUES ($1, $2, now())
		ON CONFLICT (assignment_id, material_id) DO NOTHING`,
		assignmentID, materialID,
	)
	if err != nil {
		return err
	}
	// Update assignment status to in_progress if not_started
	_, _ = s.DB.ExecContext(ctx, `
		UPDATE training_assignments
		SET status = 'in_progress', started_at = coalesce(started_at, now()), updated_at = now()
		WHERE id = $1 AND status = 'not_started'`, assignmentID)
	return nil
}

func (s *Store) SubmitQuiz(ctx context.Context, schoolID, assignmentID string, answers []int) (*QuizAttempt, error) {
	// Fetch questions with correct answers
	var courseID string
	var passingScore *int
	err := s.DB.QueryRowContext(ctx, `
		SELECT a.course_id, c.passing_score
		FROM training_assignments a
		JOIN courses c ON c.id = a.course_id
		WHERE a.id = $1`, assignmentID).Scan(&courseID, &passingScore)
	if err != nil {
		return nil, err
	}

	questions, err := s.ListQuestions(ctx, courseID, true)
	if err != nil {
		return nil, err
	}

	var calcQuestions []calc.Question
	for _, q := range questions {
		calcQuestions = append(calcQuestions, calc.Question{
			ID:           q.ID,
			CorrectIndex: q.CorrectIndex,
		})
	}

	score := calc.QuizScore(calcQuestions, answers)
	threshold := 70
	if passingScore != nil {
		threshold = *passingScore
	}
	passed := calc.Passed(score, threshold)

	answersJSON, _ := json.Marshal(answers)
	var attempt QuizAttempt
	err = s.DB.QueryRowContext(ctx, `
		INSERT INTO quiz_attempts (school_id, assignment_id, answers, score, passed, created_at)
		VALUES ($1, $2, $3, $4, $5, now())
		RETURNING id, school_id, assignment_id, answers, score, passed, created_at`,
		schoolID, assignmentID, answersJSON, score, passed,
	).Scan(&attempt.ID, &attempt.SchoolID, &attempt.AssignmentID, &attempt.Answers, &attempt.Score, &attempt.Passed, &attempt.CreatedAt)
	if err != nil {
		return nil, err
	}

	if passed {
		_, _ = s.DB.ExecContext(ctx, `
			UPDATE training_assignments
			SET status = 'completed', completed_at = now(), score = $1, updated_at = now()
			WHERE id = $2`, score, assignmentID)
	}

	return &attempt, nil
}

// -----------------------------------------------------------------------------
// Onboarding
// -----------------------------------------------------------------------------

func (s *Store) ListOnboardingTemplates(ctx context.Context, schoolID string) ([]OnboardingTemplate, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT id, school_id, name, position_id, department_id, created_at, updated_at
		FROM onboarding_templates
		WHERE school_id = $1 AND deleted_at IS NULL
		ORDER BY name ASC`, schoolID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []OnboardingTemplate
	for rows.Next() {
		var t OnboardingTemplate
		if err := rows.Scan(&t.ID, &t.SchoolID, &t.Name, &t.PositionID, &t.DepartmentID, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, err
		}
		list = append(list, t)
	}
	return list, rows.Err()
}

func (s *Store) CreateOnboardingTemplate(ctx context.Context, t OnboardingTemplate) (*OnboardingTemplate, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	err = tx.QueryRowContext(ctx, `
		INSERT INTO onboarding_templates (school_id, name, position_id, department_id, created_at, updated_at)
		VALUES ($1, $2, $3, $4, now(), now())
		RETURNING id, created_at, updated_at`,
		t.SchoolID, t.Name, t.PositionID, t.DepartmentID,
	).Scan(&t.ID, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		return nil, err
	}

	for _, task := range t.Tasks {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO onboarding_template_tasks (school_id, template_id, sort, title, category, owner_kind, owner_role, due_offset_days, created_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, now())`,
			t.SchoolID, t.ID, task.Sort, task.Title, task.Category, task.OwnerKind, task.OwnerRole, task.DueOffsetDays,
		)
		if err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &t, nil
}

func (s *Store) GetOnboardingPlan(ctx context.Context, schoolID, employeeID string) (*OnboardingPlan, error) {
	var plan OnboardingPlan
	err := s.DB.QueryRowContext(ctx, `
		SELECT p.id, p.school_id, p.employee_id, coalesce(e.full_name, ''), p.template_id, t.name, p.start_date, p.status, p.created_at, p.updated_at
		FROM onboarding_plans p
		JOIN onboarding_templates t ON t.id = p.template_id
		LEFT JOIN employees e ON e.id = p.employee_id
		WHERE p.school_id = $1 AND p.employee_id = $2`, schoolID, employeeID).Scan(
		&plan.ID, &plan.SchoolID, &plan.EmployeeID, &plan.EmployeeName, &plan.TemplateID, &plan.TemplateName, &plan.StartDate, &plan.Status, &plan.CreatedAt, &plan.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	} else if err != nil {
		return nil, err
	}

	rows, err := s.DB.QueryContext(ctx, `
		SELECT id, school_id, plan_id, title, category, owner_user_id, owner_role, due_date, done_at, done_by, created_at, updated_at
		FROM onboarding_tasks
		WHERE plan_id = $1
		ORDER BY due_date ASC, created_at ASC`, plan.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	now := time.Now()
	for rows.Next() {
		var task OnboardingTask
		if err := rows.Scan(
			&task.ID, &task.SchoolID, &task.PlanID, &task.Title, &task.Category,
			&task.OwnerUserID, &task.OwnerRole, &task.DueDate, &task.DoneAt, &task.DoneBy,
			&task.CreatedAt, &task.UpdatedAt,
		); err != nil {
			return nil, err
		}
		task.IsOverdue = task.DoneAt == nil && now.After(task.DueDate)
		plan.Tasks = append(plan.Tasks, task)
	}

	return &plan, rows.Err()
}

func (s *Store) MarkTaskDone(ctx context.Context, schoolID, taskID, doneBy string) error {
	res, err := s.DB.ExecContext(ctx, `
		UPDATE onboarding_tasks
		SET done_at = now(), done_by = $1, updated_at = now()
		WHERE school_id = $2 AND id = $3 AND done_at IS NULL`,
		doneBy, schoolID, taskID,
	)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// StartForEmployee initializes onboarding plan for newly created employee
func (s *Store) StartForEmployee(ctx context.Context, schoolID, employeeID, positionID string, startDate time.Time) error {
	// Check if already has plan
	var exists bool
	_ = s.DB.QueryRowContext(ctx, `SELECT true FROM onboarding_plans WHERE employee_id = $1`, employeeID).Scan(&exists)
	if exists {
		return nil
	}

	// Pick template
	var templateID string
	err := s.DB.QueryRowContext(ctx, `
		SELECT id FROM onboarding_templates
		WHERE school_id = $1 AND deleted_at IS NULL AND (position_id = $2 OR position_id IS NULL)
		ORDER BY position_id NULLS LAST
		LIMIT 1`, schoolID, positionID).Scan(&templateID)
	if err == sql.ErrNoRows {
		return nil // No template found -> skip safely
	} else if err != nil {
		return err
	}

	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var planID string
	err = tx.QueryRowContext(ctx, `
		INSERT INTO onboarding_plans (school_id, employee_id, template_id, start_date, status, created_at, updated_at)
		VALUES ($1, $2, $3, $4, 'active', now(), now())
		RETURNING id`, schoolID, employeeID, templateID, startDate).Scan(&planID)
	if err != nil {
		return err
	}

	// Fetch template tasks
	rows, err := tx.QueryContext(ctx, `
		SELECT title, category, owner_kind, owner_role, due_offset_days
		FROM onboarding_template_tasks
		WHERE template_id = $1
		ORDER BY sort ASC`, templateID)
	if err != nil {
		return err
	}
	defer rows.Close()

	var templateTasks []calc.TemplateTask
	for rows.Next() {
		var tt calc.TemplateTask
		var ownerRole sql.NullString
		if err := rows.Scan(&tt.Title, &tt.Category, &tt.OwnerKind, &ownerRole, &tt.DueOffsetDays); err != nil {
			return err
		}
		if ownerRole.Valid {
			tt.OwnerRole = ownerRole.String
		}
		templateTasks = append(templateTasks, tt)
	}

	builtTasks := calc.BuildOnboarding(templateTasks, startDate, "")
	for _, bt := range builtTasks {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO onboarding_tasks (school_id, plan_id, title, category, owner_user_id, owner_role, due_date, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, now(), now())`,
			schoolID, planID, bt.Title, bt.Category, bt.OwnerUserID, bt.OwnerRole, bt.DueDate,
		)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}
