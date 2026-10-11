package ats

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"skci/hr/internal/ats/calc"
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
// Pipeline Stages Defaults
// -----------------------------------------------------------------------------

func (s *Store) EnsureDefaultStages(ctx context.Context, schoolID string) ([]PipelineStage, error) {
	stages, err := s.ListStages(ctx, schoolID)
	if err == nil && len(stages) > 0 {
		return stages, nil
	}

	defaults := []struct {
		code string
		name string
		sort int
		kind string
	}{
		{"screening", "Screening Berkas", 1, "normal"},
		{"interview", "Wawancara", 2, "normal"},
		{"microteaching", "Microteaching / Tes Teknis", 3, "normal"},
		{"offer", "Penawaran Kerja", 4, "offer"},
		{"hired", "Diterima (Hired)", 5, "hired"},
		{"rejected", "Tidak Lolos", 6, "rejected"},
	}

	for _, d := range defaults {
		_, _ = s.DB.ExecContext(ctx, `
			INSERT INTO pipeline_stages (school_id, code, name, sort, kind)
			VALUES ($1, $2, $3, $4, $5)
			ON CONFLICT DO NOTHING`,
			schoolID, d.code, d.name, d.sort, d.kind)
	}

	return s.ListStages(ctx, schoolID)
}

func (s *Store) ListStages(ctx context.Context, schoolID string) ([]PipelineStage, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT id, school_id, code, name, sort, kind, notify_template_key, created_at
		FROM pipeline_stages
		WHERE school_id = $1 AND deleted_at IS NULL
		ORDER BY sort ASC`, schoolID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []PipelineStage
	for rows.Next() {
		var st PipelineStage
		if err := rows.Scan(
			&st.ID, &st.SchoolID, &st.Code, &st.Name, &st.Sort, &st.Kind, &st.NotifyTemplateKey, &st.CreatedAt,
		); err != nil {
			return nil, err
		}
		list = append(list, st)
	}
	return list, nil
}

// -----------------------------------------------------------------------------
// Requisitions
// -----------------------------------------------------------------------------

func (s *Store) ListRequisitions(ctx context.Context, schoolID string) ([]Requisition, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT r.id, r.school_id, r.position_id, COALESCE(p.name, '') as position_name,
		       r.department_id, r.quantity, r.reason, r.replaces_employee_id, r.qualifications,
		       r.budget_idr, r.exceeds_headcount, r.status, r.filled_count, r.approval_id,
		       r.requested_by, r.idempotency_key, r.created_at, r.updated_at
		FROM requisitions r
		LEFT JOIN positions p ON r.position_id = p.id
		WHERE r.school_id = $1
		ORDER BY r.created_at DESC`, schoolID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []Requisition
	for rows.Next() {
		var r Requisition
		if err := rows.Scan(
			&r.ID, &r.SchoolID, &r.PositionID, &r.PositionName, &r.DepartmentID, &r.Quantity,
			&r.Reason, &r.ReplacesEmployeeID, &r.Qualifications, &r.BudgetIDR, &r.ExceedsHeadcount,
			&r.Status, &r.FilledCount, &r.ApprovalID, &r.RequestedBy, &r.IdempotencyKey,
			&r.CreatedAt, &r.UpdatedAt,
		); err != nil {
			return nil, err
		}
		list = append(list, r)
	}
	return list, nil
}

func (s *Store) CreateRequisition(ctx context.Context, req Requisition) (*Requisition, error) {
	if req.IdempotencyKey == "" {
		req.IdempotencyKey = fmt.Sprintf("req-%d", time.Now().UnixNano())
	}
	// Check headcount vacancy from headcount_plans and active assignments
	var targetCount int
	_ = s.DB.QueryRowContext(ctx, `
		SELECT COALESCE(planned, 0) FROM headcount_plans WHERE position_id = $1 AND school_id = $2 ORDER BY created_at DESC LIMIT 1
	`, req.PositionID, req.SchoolID).Scan(&targetCount)

	var filledCount int
	_ = s.DB.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM employment_assignments WHERE position_id = $1 AND school_id = $2 AND is_primary = true
	`, req.PositionID, req.SchoolID).Scan(&filledCount)

	vacant := targetCount - filledCount
	if vacant < 0 {
		vacant = 0
	}
	req.ExceedsHeadcount = calc.ExceedsHeadcount(vacant, req.Quantity)
	req.Status = "draft"

	var id string
	err := s.DB.QueryRowContext(ctx, `
		INSERT INTO requisitions (
			school_id, position_id, department_id, quantity, reason, replaces_employee_id,
			qualifications, budget_idr, exceeds_headcount, status, requested_by, idempotency_key
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		RETURNING id
	`, req.SchoolID, req.PositionID, req.DepartmentID, req.Quantity, req.Reason,
		req.ReplacesEmployeeID, req.Qualifications, req.BudgetIDR, req.ExceedsHeadcount,
		req.Status, req.RequestedBy, req.IdempotencyKey).Scan(&id)
	if err != nil {
		return nil, err
	}
	req.ID = id
	return &req, nil
}

func (s *Store) SubmitRequisition(ctx context.Context, schoolID, id string) error {
	res, err := s.DB.ExecContext(ctx, `
		UPDATE requisitions
		SET status = 'pending_approval', updated_at = NOW()
		WHERE id = $1 AND school_id = $2 AND status = 'draft'`, id, schoolID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// -----------------------------------------------------------------------------
// Job Postings
// -----------------------------------------------------------------------------

func (s *Store) ListPostings(ctx context.Context, schoolID string, status string) ([]JobPosting, error) {
	q := `
		SELECT id, school_id, requisition_id, slug, title, description, qualifications,
		       employment_type, closes_on, status, closed_reason, published_at, created_at, updated_at
		FROM job_postings
		WHERE school_id = $1`
	args := []interface{}{schoolID}

	if status != "" {
		q += " AND status = $2"
		args = append(args, status)
	}
	q += " ORDER BY created_at DESC"

	rows, err := s.DB.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []JobPosting
	for rows.Next() {
		var p JobPosting
		if err := rows.Scan(
			&p.ID, &p.SchoolID, &p.RequisitionID, &p.Slug, &p.Title, &p.Description,
			&p.Qualifications, &p.EmploymentType, &p.ClosesOn, &p.Status, &p.ClosedReason,
			&p.PublishedAt, &p.CreatedAt, &p.UpdatedAt,
		); err != nil {
			return nil, err
		}
		list = append(list, p)
	}
	return list, nil
}

func (s *Store) GetPosting(ctx context.Context, schoolID, id string) (*JobPosting, error) {
	var p JobPosting
	err := s.DB.QueryRowContext(ctx, `
		SELECT id, school_id, requisition_id, slug, title, description, qualifications,
		       employment_type, closes_on, status, closed_reason, published_at, created_at, updated_at
		FROM job_postings
		WHERE id = $1 AND school_id = $2`, id, schoolID).Scan(
		&p.ID, &p.SchoolID, &p.RequisitionID, &p.Slug, &p.Title, &p.Description,
		&p.Qualifications, &p.EmploymentType, &p.ClosesOn, &p.Status, &p.ClosedReason,
		&p.PublishedAt, &p.CreatedAt, &p.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &p, nil
}

func (s *Store) GetPostingBySlug(ctx context.Context, schoolID, slug string) (*JobPosting, error) {
	var p JobPosting
	err := s.DB.QueryRowContext(ctx, `
		SELECT id, school_id, requisition_id, slug, title, description, qualifications,
		       employment_type, closes_on, status, closed_reason, published_at, created_at, updated_at
		FROM job_postings
		WHERE slug = $1 AND school_id = $2`, slug, schoolID).Scan(
		&p.ID, &p.SchoolID, &p.RequisitionID, &p.Slug, &p.Title, &p.Description,
		&p.Qualifications, &p.EmploymentType, &p.ClosesOn, &p.Status, &p.ClosedReason,
		&p.PublishedAt, &p.CreatedAt, &p.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &p, nil
}

func (s *Store) CreatePosting(ctx context.Context, p JobPosting) (*JobPosting, error) {
	if p.Slug == "" {
		p.Slug = fmt.Sprintf("job-%d", time.Now().UnixNano())
	}
	p.Status = "draft"
	var id string
	err := s.DB.QueryRowContext(ctx, `
		INSERT INTO job_postings (
			school_id, requisition_id, slug, title, description, qualifications,
			employment_type, closes_on, status
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id
	`, p.SchoolID, p.RequisitionID, p.Slug, p.Title, p.Description, p.Qualifications,
		p.EmploymentType, p.ClosesOn, p.Status).Scan(&id)
	if err != nil {
		return nil, err
	}
	p.ID = id
	return &p, nil
}

func (s *Store) PublishPosting(ctx context.Context, schoolID, id string) error {
	now := time.Now()
	res, err := s.DB.ExecContext(ctx, `
		UPDATE job_postings
		SET status = 'open', published_at = $3, updated_at = $3
		WHERE id = $1 AND school_id = $2`, id, schoolID, now)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) ClosePosting(ctx context.Context, schoolID, id, reason string) error {
	res, err := s.DB.ExecContext(ctx, `
		UPDATE job_postings
		SET status = 'closed', closed_reason = $3, updated_at = NOW()
		WHERE id = $1 AND school_id = $2`, id, schoolID, reason)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// -----------------------------------------------------------------------------
// Public Careers & Application Submission
// -----------------------------------------------------------------------------

func hashToken(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}

func (s *Store) ApplyCandidate(ctx context.Context, schoolID, postingID, fullName, email, cvKey string, talentPoolConsent bool) (*Application, string, error) {
	normEmail := strings.ToLower(strings.TrimSpace(email))

	// Get or create candidate
	var candidateID string
	err := s.DB.QueryRowContext(ctx, `
		SELECT id FROM candidates WHERE school_id = $1 AND email_norm = $2
	`, schoolID, normEmail).Scan(&candidateID)

	if errors.Is(err, sql.ErrNoRows) {
		err = s.DB.QueryRowContext(ctx, `
			INSERT INTO candidates (school_id, full_name, email, email_norm, consent_processing_at, consent_talent_pool)
			VALUES ($1, $2, $3, $4, NOW(), $5)
			RETURNING id
		`, schoolID, fullName, email, normEmail, talentPoolConsent).Scan(&candidateID)
		if err != nil {
			return nil, "", err
		}
	} else if err != nil {
		return nil, "", err
	}

	// Get initial screening stage
	var stageID string
	err = s.DB.QueryRowContext(ctx, `
		SELECT id FROM pipeline_stages WHERE school_id = $1 AND code = 'screening'
	`, schoolID).Scan(&stageID)
	if err != nil {
		// Fallback to first stage
		err = s.DB.QueryRowContext(ctx, `
			SELECT id FROM pipeline_stages WHERE school_id = $1 ORDER BY sort ASC LIMIT 1
		`, schoolID).Scan(&stageID)
		if err != nil {
			return nil, "", errors.New("pipeline stages belum dikonfigurasi")
		}
	}

	rawToken := fmt.Sprintf("%x%x", time.Now().UnixNano(), candidateID)
	tokenHash := hashToken(rawToken)

	var app Application
	err = s.DB.QueryRowContext(ctx, `
		INSERT INTO applications (school_id, posting_id, candidate_id, stage_id, status, cv_object_key, access_token_hash)
		VALUES ($1, $2, $3, $4, 'active', $5, $6)
		RETURNING id, created_at, updated_at
	`, schoolID, postingID, candidateID, stageID, cvKey, tokenHash).Scan(&app.ID, &app.CreatedAt, &app.UpdatedAt)

	if err != nil {
		if strings.Contains(err.Error(), "unique") || strings.Contains(err.Error(), "uq_application_posting_candidate") {
			return nil, "", ErrConflict
		}
		return nil, "", err
	}

	app.SchoolID = schoolID
	app.PostingID = postingID
	app.CandidateID = candidateID
	app.StageID = stageID
	app.Status = "active"
	app.CVObjectKey = cvKey

	return &app, rawToken, nil
}

func (s *Store) GetApplicationByToken(ctx context.Context, token string) (*Application, error) {
	tokenHash := hashToken(token)
	var a Application
	err := s.DB.QueryRowContext(ctx, `
		SELECT a.id, a.school_id, a.posting_id, p.title as posting_title,
		       a.candidate_id, c.full_name as candidate_name, c.email as candidate_email,
		       a.stage_id, st.name as stage_name, a.status, a.cv_object_key, a.created_at, a.updated_at
		FROM applications a
		JOIN job_postings p ON a.posting_id = p.id
		JOIN candidates c ON a.candidate_id = c.id
		JOIN pipeline_stages st ON a.stage_id = st.id
		WHERE a.access_token_hash = $1`, tokenHash).Scan(
		&a.ID, &a.SchoolID, &a.PostingID, &a.PostingTitle,
		&a.CandidateID, &a.CandidateName, &a.CandidateEmail,
		&a.StageID, &a.StageName, &a.Status, &a.CVObjectKey, &a.CreatedAt, &a.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &a, nil
}

// -----------------------------------------------------------------------------
// Pipeline Board & Movement
// -----------------------------------------------------------------------------

func (s *Store) ListApplications(ctx context.Context, schoolID, postingID string) ([]Application, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT a.id, a.school_id, a.posting_id, p.title as posting_title,
		       a.candidate_id, c.full_name as candidate_name, c.email as candidate_email,
		       a.stage_id, st.name as stage_name, a.status, a.cv_object_key, a.created_at, a.updated_at
		FROM applications a
		JOIN job_postings p ON a.posting_id = p.id
		JOIN candidates c ON a.candidate_id = c.id
		JOIN pipeline_stages st ON a.stage_id = st.id
		WHERE a.school_id = $1 AND ($2 = '' OR a.posting_id = $2::uuid)
		ORDER BY a.created_at DESC`, schoolID, postingID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []Application
	for rows.Next() {
		var a Application
		if err := rows.Scan(
			&a.ID, &a.SchoolID, &a.PostingID, &a.PostingTitle,
			&a.CandidateID, &a.CandidateName, &a.CandidateEmail,
			&a.StageID, &a.StageName, &a.Status, &a.CVObjectKey, &a.CreatedAt, &a.UpdatedAt,
		); err != nil {
			return nil, err
		}
		list = append(list, a)
	}
	return list, nil
}

func (s *Store) MoveApplication(ctx context.Context, schoolID, appID, toStageID, note, actorID string) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var fromStageID string
	err = tx.QueryRowContext(ctx, `
		SELECT stage_id FROM applications WHERE id = $1 AND school_id = $2
	`, appID, schoolID).Scan(&fromStageID)
	if err != nil {
		return err
	}

	_, err = tx.ExecContext(ctx, `
		UPDATE applications SET stage_id = $1, updated_at = NOW() WHERE id = $2 AND school_id = $3
	`, toStageID, appID, schoolID)
	if err != nil {
		return err
	}

	_, err = tx.ExecContext(ctx, `
		INSERT INTO application_events (school_id, application_id, from_stage, to_stage, note, actor_user_id)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, schoolID, appID, fromStageID, toStageID, note, actorID)
	if err != nil {
		return err
	}

	return tx.Commit()
}

// -----------------------------------------------------------------------------
// Scorecards & Interviews
// -----------------------------------------------------------------------------

func (s *Store) ListScorecardTemplates(ctx context.Context, schoolID string) ([]ScorecardTemplate, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT id, school_id, position_id, name, created_at
		FROM scorecard_templates
		WHERE school_id = $1
		ORDER BY name ASC`, schoolID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []ScorecardTemplate
	for rows.Next() {
		var t ScorecardTemplate
		if err := rows.Scan(&t.ID, &t.SchoolID, &t.PositionID, &t.Name, &t.CreatedAt); err != nil {
			return nil, err
		}
		list = append(list, t)
	}
	return list, nil
}

func (s *Store) CreateScorecardTemplate(ctx context.Context, t ScorecardTemplate) (*ScorecardTemplate, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	var id string
	err = tx.QueryRowContext(ctx, `
		INSERT INTO scorecard_templates (school_id, position_id, name)
		VALUES ($1, $2, $3)
		RETURNING id
	`, t.SchoolID, t.PositionID, t.Name).Scan(&id)
	if err != nil {
		return nil, err
	}
	t.ID = id

	for _, cr := range t.Criteria {
		_, err = tx.ExecContext(ctx, `
			INSERT INTO scorecard_criteria (school_id, template_id, name, weight_bp)
			VALUES ($1, $2, $3, $4)
		`, t.SchoolID, id, cr.Name, cr.WeightBP)
		if err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &t, nil
}

func (s *Store) ScheduleInterview(ctx context.Context, inv Interview, panelUserIDs []string) (*Interview, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	var id string
	err = tx.QueryRowContext(ctx, `
		INSERT INTO interviews (
			school_id, application_id, kind, starts_at, ends_at, location_or_link, template_id, status
		) VALUES ($1, $2, $3, $4, $5, $6, $7, 'scheduled')
		RETURNING id, created_at
	`, inv.SchoolID, inv.ApplicationID, inv.Kind, inv.StartsAt, inv.EndsAt,
		inv.LocationOrLink, inv.TemplateID).Scan(&id, &inv.CreatedAt)
	if err != nil {
		return nil, err
	}
	inv.ID = id

	for _, uid := range panelUserIDs {
		_, err = tx.ExecContext(ctx, `
			INSERT INTO interview_panel (school_id, interview_id, user_id)
			VALUES ($1, $2, $3)
			ON CONFLICT DO NOTHING
		`, inv.SchoolID, id, uid)
		if err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &inv, nil
}

func (s *Store) MyInterviews(ctx context.Context, schoolID, userID string) ([]Interview, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT i.id, i.school_id, i.application_id, c.full_name as candidate_name,
		       p.title as posting_title, i.kind, i.starts_at, i.ends_at, i.location_or_link,
		       i.template_id, i.status, i.created_at
		FROM interviews i
		JOIN interview_panel ip ON i.id = ip.interview_id
		JOIN applications a ON i.application_id = a.id
		JOIN candidates c ON a.candidate_id = c.id
		JOIN job_postings p ON a.posting_id = p.id
		WHERE i.school_id = $1 AND ip.user_id = $2
		ORDER BY i.starts_at ASC`, schoolID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []Interview
	for rows.Next() {
		var inv Interview
		if err := rows.Scan(
			&inv.ID, &inv.SchoolID, &inv.ApplicationID, &inv.CandidateName,
			&inv.PostingTitle, &inv.Kind, &inv.StartsAt, &inv.EndsAt,
			&inv.LocationOrLink, &inv.TemplateID, &inv.Status, &inv.CreatedAt,
		); err != nil {
			return nil, err
		}
		list = append(list, inv)
	}
	return list, nil
}

// -----------------------------------------------------------------------------
// Offers & Hiring
// -----------------------------------------------------------------------------

func (s *Store) CreateOffer(ctx context.Context, off Offer) (*Offer, error) {
	// Check salary range from position
	var minSalary, maxSalary int64
	_ = s.DB.QueryRowContext(ctx, `
		SELECT COALESCE(salary_min_idr, 0), COALESCE(salary_max_idr, 0)
		FROM positions WHERE id = $1 AND school_id = $2
	`, off.PositionID, off.SchoolID).Scan(&minSalary, &maxSalary)

	off.OutOfRange = calc.SalaryOutOfRange(off.BaseSalaryIDR, minSalary, maxSalary)
	off.Status = "draft"

	var id string
	err := s.DB.QueryRowContext(ctx, `
		INSERT INTO offers (
			school_id, application_id, position_id, start_date, employment_type,
			base_salary_enc, out_of_range, status
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id, created_at, updated_at
	`, off.SchoolID, off.ApplicationID, off.PositionID, off.StartDate,
		off.EmploymentType, []byte(fmt.Sprintf("%d", off.BaseSalaryIDR)),
		off.OutOfRange, off.Status).Scan(&id, &off.CreatedAt, &off.UpdatedAt)
	if err != nil {
		return nil, err
	}
	off.ID = id
	return &off, nil
}

// AcceptOffer processes candidate acceptance in one transaction
func (s *Store) AcceptOffer(ctx context.Context, offerID string) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var o Offer
	err = tx.QueryRowContext(ctx, `
		SELECT id, school_id, application_id, position_id
		FROM offers WHERE id = $1 AND status IN ('sent', 'approved')
	`, offerID).Scan(&o.ID, &o.SchoolID, &o.ApplicationID, &o.PositionID)
	if err != nil {
		return err
	}

	// 1. Update offer status to accepted
	now := time.Now()
	_, err = tx.ExecContext(ctx, `
		UPDATE offers SET status = 'accepted', responded_at = $2, updated_at = $2
		WHERE id = $1
	`, offerID, now)
	if err != nil {
		return err
	}

	// 2. Mark application status hired
	_, err = tx.ExecContext(ctx, `
		UPDATE applications SET status = 'hired', updated_at = $2
		WHERE id = $1
	`, o.ApplicationID, now)
	if err != nil {
		return err
	}

	return tx.Commit()
}

// -----------------------------------------------------------------------------
// ATS Settings & Talent Pool
// -----------------------------------------------------------------------------

func (s *Store) GetSettings(ctx context.Context, schoolID string) (*AtsSettings, error) {
	var st AtsSettings
	err := s.DB.QueryRowContext(ctx, `
		SELECT school_id, retention_months, career_page_enabled, career_intro
		FROM ats_settings WHERE school_id = $1`, schoolID).Scan(
		&st.SchoolID, &st.RetentionMonths, &st.CareerPageEnabled, &st.CareerIntro,
	)
	if errors.Is(err, sql.ErrNoRows) {
		// return default
		return &AtsSettings{
			SchoolID:          schoolID,
			RetentionMonths:   6,
			CareerPageEnabled: true,
			CareerIntro:       "Bergabunglah bersama kami untuk memajukan pendidikan berkualitas.",
		}, nil
	} else if err != nil {
		return nil, err
	}
	return &st, nil
}

func (s *Store) ListTalentPool(ctx context.Context, schoolID string) ([]Candidate, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT id, school_id, full_name, email, email_norm, consent_processing_at,
		       consent_talent_pool, anonymized_at, created_at, updated_at
		FROM candidates
		WHERE school_id = $1 AND consent_talent_pool = true AND anonymized_at IS NULL
		ORDER BY created_at DESC`, schoolID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []Candidate
	for rows.Next() {
		var c Candidate
		if err := rows.Scan(
			&c.ID, &c.SchoolID, &c.FullName, &c.Email, &c.EmailNorm,
			&c.ConsentProcessingAt, &c.ConsentTalentPool, &c.AnonymizedAt,
			&c.CreatedAt, &c.UpdatedAt,
		); err != nil {
			return nil, err
		}
		list = append(list, c)
	}
	return list, nil
}
