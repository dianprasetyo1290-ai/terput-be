package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"skci/exam/internal/model"
)

// ErrAdmissionExamNotFound is returned when an admission-only operation is
// pointed at an exam that does not exist, is not in the caller's school, or is
// not an admission exam (audience='admission'). A typed error so the handler
// answers 404 instead of leaking a 500 for what is really a caller mistake.
var ErrAdmissionExamNotFound = errors.New("admission exam not found")

type AdmissionExamInput struct {
	SchoolID         string
	ExternalRef      string
	Title            string
	Description      *string
	StartAt          time.Time
	EndAt            time.Time
	IsShuffled       bool
	CreatedBy        string
	AdmissionContext map[string]interface{}
}

// ErrAdmissionExternalRefRequired is returned when an admission exam upsert
// arrives without the external reference that identifies it. Without it the
// uniqueness index (on admission_context->>'external_reference') cannot apply
// and every retry would create another exam row.
var ErrAdmissionExternalRefRequired = errors.New("admission exam external_reference is required")

// UpsertAdmissionExam is idempotent on (school_id, admission external_ref):
// retrying a provisioning call with the same external reference updates the
// existing exam and returns it instead of creating a duplicate.
//
// The store — not the caller — mirrors ExternalRef into AdmissionContext,
// because the unique index that provides that guarantee lives on the JSONB
// field (idx_exams_admission_external_ref). An earlier version relied on
// InternalUpsertAdmissionExam doing the mirroring itself, so any other caller
// (a worker, a future route, a test) silently lost idempotency and produced a
// second exam for the same external reference.
func (s *Store) UpsertAdmissionExam(ctx context.Context, in AdmissionExamInput) (*model.Exam, error) {
	if in.ExternalRef == "" {
		return nil, ErrAdmissionExternalRefRequired
	}
	if in.AdmissionContext == nil {
		in.AdmissionContext = map[string]interface{}{}
	}
	in.AdmissionContext["external_reference"] = in.ExternalRef
	contextJSON, err := json.Marshal(in.AdmissionContext)
	if err != nil {
		return nil, err
	}
	var e model.Exam
	var raw []byte
	err = s.DB.QueryRowContext(ctx, `
		INSERT INTO exams (school_id, audience, title, description, start_at, end_at, is_shuffled,
			created_by, admission_context)
		VALUES ($1,'admission',$2,$3,$4,$5,$6,$7,$8)
		ON CONFLICT ((school_id), ((admission_context->>'external_reference'))) WHERE audience='admission'
		DO UPDATE SET title=EXCLUDED.title, description=EXCLUDED.description, start_at=EXCLUDED.start_at,
			end_at=EXCLUDED.end_at, is_shuffled=EXCLUDED.is_shuffled, updated_at=now()
		RETURNING id, school_id, audience, title, description, start_at, end_at, is_shuffled,
			is_published, published_at, created_by, created_at, updated_at, admission_context`,
		in.SchoolID, in.Title, in.Description, in.StartAt, in.EndAt, in.IsShuffled, in.CreatedBy, contextJSON,
	).Scan(&e.ID, &e.SchoolID, &e.Audience, &e.Title, &e.Description, &e.StartAt, &e.EndAt,
		&e.IsShuffled, &e.IsPublished, &e.PublishedAt, &e.CreatedBy, &e.CreatedAt, &e.UpdatedAt, &raw)
	if err != nil {
		return nil, err
	}
	_ = json.Unmarshal(raw, &e.AdmissionContext)
	return &e, nil
}

func (s *Store) GetAdmissionExam(ctx context.Context, examID, schoolID string) (*model.Exam, error) {
	var e model.Exam
	var raw []byte
	err := s.DB.QueryRowContext(ctx, `
		SELECT id, school_id, audience, title, description, start_at, end_at, is_shuffled,
			is_published, published_at, created_by, created_at, updated_at, admission_context
		FROM exams WHERE id=$1 AND school_id=$2 AND audience='admission' AND deleted_at IS NULL`, examID, schoolID,
	).Scan(&e.ID, &e.SchoolID, &e.Audience, &e.Title, &e.Description, &e.StartAt, &e.EndAt,
		&e.IsShuffled, &e.IsPublished, &e.PublishedAt, &e.CreatedBy, &e.CreatedAt, &e.UpdatedAt, &raw)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	_ = json.Unmarshal(raw, &e.AdmissionContext)
	return &e, nil
}

// ProvisionApplicant is idempotent: one row per
// (exam_id, participant_type='applicant', participant_id) — enforced by the
// UNIQUE constraint from migration 0008 — so admission can retry a provisioning
// call after a timeout without creating a second participant. Re-provisioning
// only ever fills in a display_name/external_reference that was missing (COALESCE
// keeps the previous non-empty value), so a retry can never blank out data a
// previous call already recorded. The applicant is only inserted when the exam
// really is an admission exam in the same school, which is also what keeps an
// applicant id from being attached to an academic exam.
func (s *Store) ProvisionApplicant(ctx context.Context, schoolID, examID, applicantID, externalRef string, displayName *string) (*model.ExamParticipant, error) {
	var p model.ExamParticipant
	err := s.DB.QueryRowContext(ctx, `
		INSERT INTO exam_participants (school_id, exam_id, participant_type, participant_id, display_name, external_reference)
		SELECT $1,$2,'applicant',$3,$4,$5
		WHERE EXISTS (SELECT 1 FROM exams WHERE id=$2 AND school_id=$1 AND audience='admission' AND deleted_at IS NULL)
		ON CONFLICT (exam_id, participant_type, participant_id) DO UPDATE SET
			display_name=COALESCE(EXCLUDED.display_name, exam_participants.display_name),
			external_reference=COALESCE(EXCLUDED.external_reference, exam_participants.external_reference), updated_at=now()
		RETURNING id, school_id, exam_id, participant_type, participant_id, display_name, external_reference, created_at, updated_at`,
		schoolID, examID, applicantID, displayName, nullableString(externalRef),
	).Scan(&p.ID, &p.SchoolID, &p.ExamID, &p.ParticipantType, &p.ParticipantID, &p.DisplayName,
		&p.ExternalReference, &p.CreatedAt, &p.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, ErrAdmissionExamNotFound
	}
	return &p, err
}

func nullableString(v string) interface{} {
	if v == "" {
		return nil
	}
	return v
}

// SaveParticipantToken stores only the SHA-256 hash of the signed applicant
// token (never the token itself), so a database read cannot mint a working
// credential and an operator can revoke a single token by its hash. The
// ON CONFLICT makes re-issuing the same token a no-op instead of a constraint
// violation, which keeps InternalIssueParticipantAccess retry-safe.
func (s *Store) SaveParticipantToken(ctx context.Context, hash, schoolID, examID, applicantID string, expiresAt time.Time) error {
	_, err := s.DB.ExecContext(ctx, `
		INSERT INTO participant_access_tokens (token_hash, school_id, exam_id, participant_type, participant_id, expires_at)
		VALUES ($1,$2,$3,'applicant',$4,$5)
		ON CONFLICT (token_hash) DO NOTHING`, hash, schoolID, examID, applicantID, expiresAt)
	return err
}

// ValidateParticipantToken reports whether this exact token (by hash) is a
// live applicant token for this school/exam/applicant: present, not revoked and
// not expired. A token that was never issued by this service — e.g. one signed
// with the same secret but minted elsewhere, or replayed after expiry — fails
// here even when its JWT signature/claims are valid.
func (s *Store) ValidateParticipantToken(ctx context.Context, hash, schoolID, examID, applicantID string) (bool, error) {
	var ok bool
	err := s.DB.QueryRowContext(ctx, `SELECT EXISTS(
		SELECT 1 FROM participant_access_tokens WHERE token_hash=$1 AND school_id=$2 AND exam_id=$3
		AND participant_type='applicant' AND participant_id=$4 AND revoked_at IS NULL AND expires_at > now())`,
		hash, schoolID, examID, applicantID).Scan(&ok)
	return ok, err
}

func (s *Store) EnterApplicantExam(ctx context.Context, schoolID, examID, applicantID string) (*model.ExamAttempt, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var isPublished, isShuffled bool
	var startAt, endAt time.Time
	err = tx.QueryRowContext(ctx, `SELECT is_published, is_shuffled, start_at, end_at FROM exams
		WHERE id=$1 AND school_id=$2 AND audience='admission' AND deleted_at IS NULL`, examID, schoolID).
		Scan(&isPublished, &isShuffled, &startAt, &endAt)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("admission exam not found")
	}
	if err != nil {
		return nil, err
	}
	now := time.Now()
	if !isPublished || now.Before(startAt) || now.After(endAt) {
		return nil, ErrInvalidTransition
	}
	res, err := tx.ExecContext(ctx, `INSERT INTO exam_attempts
		(school_id, exam_id, participant_type, participant_id, status)
		SELECT $1,$2,'applicant',$3,'belum_mulai' WHERE EXISTS (
			SELECT 1 FROM exam_participants WHERE exam_id=$2 AND school_id=$1 AND participant_type='applicant' AND participant_id=$3)
		ON CONFLICT (exam_id, participant_type, participant_id) DO NOTHING`, schoolID, examID, applicantID)
	if err != nil {
		return nil, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		var exists bool
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM exam_participants WHERE exam_id=$1 AND school_id=$2 AND participant_type='applicant' AND participant_id=$3)`, examID, schoolID, applicantID).Scan(&exists); err != nil {
			return nil, err
		}
		if !exists {
			return nil, fmt.Errorf("applicant is not provisioned for this exam")
		}
	}
	a, err := scanAttempt(tx.QueryRowContext(ctx, `SELECT `+attemptCols+` FROM exam_attempts
		WHERE exam_id=$1 AND participant_type='applicant' AND participant_id=$2 FOR UPDATE`, examID, applicantID))
	if err != nil {
		return nil, err
	}
	switch a.Status {
	case model.StatusAktif:
	case model.StatusBelumMulai:
		_, ids, err := s.examShuffleAndQuestionIDs(ctx, tx, examID)
		if err != nil {
			return nil, err
		}
		if len(ids) == 0 {
			return nil, fmt.Errorf("exam has no questions to attempt")
		}
		if isShuffled {
			randShuffle(ids)
		}
		raw, _ := json.Marshal(ids)
		_, err = tx.ExecContext(ctx, `UPDATE exam_attempts SET status='aktif', started_at=now(), question_order=$1, current_position=0, updated_at=now() WHERE id=$2`, raw, a.ID)
		if err != nil {
			return nil, err
		}
		a.Status, a.QuestionOrder = model.StatusAktif, ids
		t := time.Now()
		a.StartedAt = &t
	default:
		return nil, ErrInvalidTransition
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return a, nil
}

func randShuffle(ids []string) {
	// math/rand is sufficient: this is display order, not a secret/token.
	for i := len(ids) - 1; i > 0; i-- {
		j := int(time.Now().UnixNano() % int64(i+1))
		ids[i], ids[j] = ids[j], ids[i]
	}
}

// GetParticipantAttempt refuses anything but the two legal participant types
// before touching SQL: participantType reaches here straight from a token
// claim, so an over-long/padded value must never be interpolated into a lookup
// as if it were a real kind (see model.ValidParticipantType).
func (s *Store) GetParticipantAttempt(ctx context.Context, examID, participantType, participantID string) (*model.ExamAttempt, error) {
	if !model.ValidParticipantType(participantType) {
		return nil, fmt.Errorf("invalid participant type %q", participantType)
	}
	a, err := scanAttempt(s.DB.QueryRowContext(ctx, `SELECT `+attemptCols+` FROM exam_attempts
		WHERE exam_id=$1 AND participant_type=$2 AND participant_id=$3`, examID, participantType, participantID))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return a, err
}

func (s *Store) FocusLostParticipant(ctx context.Context, examID, participantType, participantID string) (*model.ExamAttempt, error) {
	if !model.ValidParticipantType(participantType) {
		return nil, fmt.Errorf("invalid participant type %q", participantType)
	}
	res, err := s.DB.ExecContext(ctx, `UPDATE exam_attempts SET status='blokir', blocked_at=now(),
		blocked_reason='Terdeteksi keluar dari halaman ujian (focus lost)', blocked_by=NULL,
		focus_lost_count=focus_lost_count+1, updated_at=now()
		WHERE exam_id=$1 AND participant_type=$2 AND participant_id=$3 AND status='aktif'`, examID, participantType, participantID)
	if err != nil {
		return nil, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, ErrInvalidTransition
	}
	return s.GetParticipantAttempt(ctx, examID, participantType, participantID)
}

// AdmissionResult is the ONLY shape an applicant's result may leave this
// service through. It is deliberately narrow and doubly filtered: the attempt
// must be participant_type='applicant' AND belong to an exam with
// audience='admission' in the same school. A student's attempt can therefore
// never be read out of the admission contract, and an applicant's attempt can
// never satisfy it through an academic exam. The academic score/report surface
// (ExamScores) applies the mirror-image filter.
type AdmissionResult struct {
	ExamID          string     `json:"exam_id"`
	ParticipantType string     `json:"participant_type"`
	ParticipantID   string     `json:"participant_id"`
	Status          string     `json:"status"`
	Score           *float64   `json:"score,omitempty"`
	CorrectCount    *int       `json:"correct_count,omitempty"`
	SubmittedAt     *time.Time `json:"submitted_at,omitempty"`
	Final           bool       `json:"final"`
}

func (s *Store) AdmissionResult(ctx context.Context, schoolID, examID, applicantID string) (*AdmissionResult, error) {
	var out AdmissionResult
	err := s.DB.QueryRowContext(ctx, `SELECT ea.exam_id, ea.participant_type, ea.participant_id, ea.status,
		ea.score, ea.correct_count, ea.submitted_at, ea.status='selesai'
		FROM exam_attempts ea JOIN exams e ON e.id=ea.exam_id
		WHERE ea.exam_id=$1 AND ea.participant_type='applicant' AND ea.participant_id=$2
		AND ea.school_id=$3 AND e.audience='admission'`, examID, applicantID, schoolID).
		Scan(&out.ExamID, &out.ParticipantType, &out.ParticipantID, &out.Status, &out.Score,
			&out.CorrectCount, &out.SubmittedAt, &out.Final)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return &out, err
}

func (s *Store) ListApplicantParticipants(ctx context.Context, examID string) ([]model.ExamParticipant, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id, school_id, exam_id, participant_type, participant_id,
		display_name, external_reference, created_at, updated_at FROM exam_participants
		WHERE exam_id=$1 AND participant_type='applicant' ORDER BY created_at`, examID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.ExamParticipant
	for rows.Next() {
		var p model.ExamParticipant
		if err := rows.Scan(&p.ID, &p.SchoolID, &p.ExamID, &p.ParticipantType, &p.ParticipantID, &p.DisplayName, &p.ExternalReference, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}
