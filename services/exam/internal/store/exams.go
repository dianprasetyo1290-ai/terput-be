package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"skci/exam/internal/model"
)

// CreateExam persists a single exam row related to one or more classrooms
// (e.ClassroomIDs — falls back to []string{e.ClassroomID} if unset). See
// migration 0005: exam_classrooms is the real many-to-many target-audience
// table; exams.classroom_id is kept as ClassroomIDs[0] for backward compat.
func (s *Store) CreateExam(ctx context.Context, e *model.Exam) error {
	classroomIDs := e.ClassroomIDs
	if len(classroomIDs) == 0 {
		classroomIDs = []string{e.ClassroomID}
	}
	e.ClassroomID = classroomIDs[0]

	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if err := tx.QueryRowContext(ctx, `
		INSERT INTO exams (school_id, academic_year_id, term_id, exam_type_id, title, description,
			classroom_id, subject_id, teacher_id, start_at, end_at, is_shuffled, created_by, folder_id)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)
		RETURNING id, is_published, created_at, updated_at`,
		e.SchoolID, e.AcademicYearID, e.TermID, e.ExamTypeID, e.Title, e.Description,
		e.ClassroomID, e.SubjectID, e.TeacherID, e.StartAt, e.EndAt, e.IsShuffled, e.CreatedBy, e.FolderID,
	).Scan(&e.ID, &e.IsPublished, &e.CreatedAt, &e.UpdatedAt); err != nil {
		return err
	}
	for _, cid := range classroomIDs {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO exam_classrooms (exam_id, classroom_id) VALUES ($1,$2)
			ON CONFLICT (exam_id, classroom_id) DO NOTHING`, e.ID, cid); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	e.ClassroomIDs = classroomIDs
	return nil
}

// examClassroomIDs returns the full target-audience classroom list for one
// exam, ordered by insertion (so index 0 matches exams.classroom_id for
// exams created after migration 0005).
func (s *Store) examClassroomIDs(ctx context.Context, examID string) ([]string, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT classroom_id FROM exam_classrooms WHERE exam_id=$1 ORDER BY created_at`, examID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// examClassroomIDsBatch is examClassroomIDs for a whole page of exams at
// once (used by ListExams to avoid N+1 queries).
func (s *Store) examClassroomIDsBatch(ctx context.Context, examIDs []string) (map[string][]string, error) {
	out := make(map[string][]string, len(examIDs))
	if len(examIDs) == 0 {
		return out, nil
	}
	rows, err := s.DB.QueryContext(ctx, `
		SELECT exam_id, classroom_id FROM exam_classrooms WHERE exam_id = ANY($1) ORDER BY created_at`, examIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var examID, classroomID string
		if err := rows.Scan(&examID, &classroomID); err != nil {
			return nil, err
		}
		out[examID] = append(out[examID], classroomID)
	}
	return out, rows.Err()
}

type ExamFilter struct {
	SchoolID       string
	AcademicYearID string
	TermID         string
	ClassroomID    string
	TeacherID      string // "mine" scoping for guru
	IsPublished    *bool
	FolderID       string
	// Title: exact-match filter used by the live monitor's Kelas switcher
	// (item 6, V4) to find an exam's "siblings" — the classroom_assignments
	// fan-out (item 3/5) creates one Exam row per selected class, all
	// sharing the same title, so grouping by title is how the frontend
	// reconstructs "the same ujian, several classes" for that dropdown.
	Title string
}

func (s *Store) ListExams(ctx context.Context, f ExamFilter, page, perPage int) ([]model.Exam, int64, error) {
	where := "($1 = '' OR e.school_id::text = $1) AND e.deleted_at IS NULL"
	args := []interface{}{f.SchoolID}
	add := func(cond string, val interface{}) {
		args = append(args, val)
		where += fmt.Sprintf(" AND %s=$%d", cond, len(args))
	}
	if f.AcademicYearID != "" {
		add("e.academic_year_id", f.AcademicYearID)
	}
	if f.TermID != "" {
		add("e.term_id", f.TermID)
	}
	if f.ClassroomID != "" {
		// exam_classrooms is the full target-audience set (migration 0005) --
		// e.classroom_id alone would miss an exam for a student/admin whose
		// classroom isn't the "primary" one on a multi-classroom exam.
		args = append(args, f.ClassroomID)
		where += fmt.Sprintf(" AND EXISTS (SELECT 1 FROM exam_classrooms ec WHERE ec.exam_id = e.id AND ec.classroom_id::text = $%d)", len(args))
	}
	if f.TeacherID != "" {
		add("e.teacher_id", f.TeacherID)
	}
	if f.IsPublished != nil {
		add("e.is_published", *f.IsPublished)
	}
	// "none" means folder_id IS NULL. An exam created outside a Bank Soal
	// folder used to be unreachable in the UI -- the sidebar has no Kelola
	// Ujian entry (V4 item 1), so the only way in is a folder, and an exam
	// with no folder belongs to none of them. The guru dashboard counted it
	// anyway, which is how it surfaced: "terlihat di luar 4, pas masuk hanya
	// ada 3". Bank Soal now shows a "Tanpa Folder" entry backed by this.
	if f.FolderID == "none" {
		where += " AND e.folder_id IS NULL"
	} else if f.FolderID != "" {
		add("e.folder_id", f.FolderID)
	}
	if f.Title != "" {
		add("e.title", f.Title)
	}

	var total int64
	if err := s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM exams e WHERE `+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	args = append(args, perPage, (page-1)*perPage)
	q := fmt.Sprintf(`
		SELECT e.id, e.school_id, e.academic_year_id, e.term_id, e.exam_type_id, e.title, e.description,
			e.classroom_id, e.subject_id, e.teacher_id, e.start_at, e.end_at, e.is_shuffled, e.is_published,
			e.published_at, e.created_by, e.created_at, e.updated_at, e.folder_id,
			COALESCE((SELECT SUM(points) FROM exam_questions eq WHERE eq.exam_id = e.id), 0),
			COALESCE((SELECT COUNT(*) FROM exam_questions eq WHERE eq.exam_id = e.id), 0)
		FROM exams e WHERE %s ORDER BY e.created_at DESC LIMIT $%d OFFSET $%d`,
		where, len(args)-1, len(args))

	rows, err := s.DB.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []model.Exam
	for rows.Next() {
		var e model.Exam
		if err := rows.Scan(&e.ID, &e.SchoolID, &e.AcademicYearID, &e.TermID, &e.ExamTypeID, &e.Title, &e.Description,
			&e.ClassroomID, &e.SubjectID, &e.TeacherID, &e.StartAt, &e.EndAt, &e.IsShuffled, &e.IsPublished,
			&e.PublishedAt, &e.CreatedBy, &e.CreatedAt, &e.UpdatedAt, &e.FolderID, &e.TotalPoints, &e.QuestionCount); err != nil {
			return nil, 0, err
		}
		out = append(out, e)
	}
	if len(out) > 0 {
		ids := make([]string, len(out))
		for i := range out {
			ids[i] = out[i].ID
		}
		batch, err := s.examClassroomIDsBatch(ctx, ids)
		if err != nil {
			return nil, 0, err
		}
		for i := range out {
			out[i].ClassroomIDs = batch[out[i].ID]
		}
	}
	return out, total, rows.Err()
}

func (s *Store) GetExam(ctx context.Context, id, schoolID string) (*model.Exam, error) {
	var e model.Exam
	err := s.DB.QueryRowContext(ctx, `
		SELECT e.id, e.school_id, e.academic_year_id, e.term_id, e.exam_type_id, e.title, e.description,
			e.classroom_id, e.subject_id, e.teacher_id, e.start_at, e.end_at, e.is_shuffled, e.is_published,
			e.published_at, e.created_by, e.created_at, e.updated_at, e.folder_id,
			COALESCE((SELECT SUM(points) FROM exam_questions eq WHERE eq.exam_id = e.id), 0),
			COALESCE((SELECT COUNT(*) FROM exam_questions eq WHERE eq.exam_id = e.id), 0)
		FROM exams e WHERE e.id=$1 AND ($2 = '' OR e.school_id::text=$2) AND e.deleted_at IS NULL`, id, schoolID,
	).Scan(&e.ID, &e.SchoolID, &e.AcademicYearID, &e.TermID, &e.ExamTypeID, &e.Title, &e.Description,
		&e.ClassroomID, &e.SubjectID, &e.TeacherID, &e.StartAt, &e.EndAt, &e.IsShuffled, &e.IsPublished,
		&e.PublishedAt, &e.CreatedBy, &e.CreatedAt, &e.UpdatedAt, &e.FolderID, &e.TotalPoints, &e.QuestionCount)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	ids, err := s.examClassroomIDs(ctx, e.ID)
	if err != nil {
		return nil, err
	}
	e.ClassroomIDs = ids
	return &e, nil
}

func (s *Store) UpdateExam(ctx context.Context, e *model.Exam) error {
	classroomIDs := e.ClassroomIDs
	if len(classroomIDs) == 0 {
		classroomIDs = []string{e.ClassroomID}
	}
	e.ClassroomID = classroomIDs[0]

	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	res, err := tx.ExecContext(ctx, `
		UPDATE exams SET term_id=$1, exam_type_id=$2, title=$3, description=$4, classroom_id=$5, subject_id=$6,
			teacher_id=$7, start_at=$8, end_at=$9, is_shuffled=$10, updated_at=now()
		WHERE id=$11 AND school_id=$12 AND is_published=false AND deleted_at IS NULL`,
		e.TermID, e.ExamTypeID, e.Title, e.Description, e.ClassroomID, e.SubjectID,
		e.TeacherID, e.StartAt, e.EndAt, e.IsShuffled, e.ID, e.SchoolID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotEditable
	}

	if _, err := tx.ExecContext(ctx, `DELETE FROM exam_classrooms WHERE exam_id=$1`, e.ID); err != nil {
		return err
	}
	for _, cid := range classroomIDs {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO exam_classrooms (exam_id, classroom_id) VALUES ($1,$2)
			ON CONFLICT (exam_id, classroom_id) DO NOTHING`, e.ID, cid); err != nil {
			return err
		}
	}

	return tx.Commit()
}

var ErrNotEditable = fmt.Errorf("exam is not editable (published or not found)")
var ErrHasAttempts = fmt.Errorf("exam already has student attempts and cannot be deleted")

// UpdateExamSchedule moves a PUBLISHED exam's window (Yadika13 item 2: "Guru
// harus bisa mengedit durasi/waktu ujian meskipun ujian sedang berjalan").
// UpdateExam deliberately refuses to touch a published row -- students
// already hold a per-attempt shuffled question_order, so changing the
// question set or the class under them would corrupt in-flight attempts.
// Times carry no such risk, so they get their own narrow statement rather
// than a relaxed guard on the wide one.
//
// Students pick the change up on their own: the dashboard's "today" bucket
// filters on end_at >= now(), and ExamRunnerView re-reads the attempt every
// 10s, so no push channel is needed.
func (s *Store) UpdateExamSchedule(ctx context.Context, id, schoolID string, start, end time.Time) error {
	res, err := s.DB.ExecContext(ctx, `
		UPDATE exams SET start_at=$1, end_at=$2, updated_at=now()
		WHERE id=$3 AND school_id=$4 AND deleted_at IS NULL`,
		start, end, id, schoolID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// DeleteExam allows deleting a draft exam freely, and a published exam too as
// long as no student has started it yet (e.g. published by mistake before
// questions were finished, see Report sekoci: SMK GPM's "Ujian Pendidikan
// Agama Katolik" stuck published with no way to remove it). Once any
// exam_attempts row exists, the exam is locked from deletion regardless of
// published state — matches the existing has_attempts guard already used by
// AddExamQuestion/RemoveExamQuestion.
func (s *Store) DeleteExam(ctx context.Context, id, schoolID string) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var hasAttempt bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM exam_attempts WHERE exam_id=$1)`, id).Scan(&hasAttempt); err != nil {
		return err
	}
	if hasAttempt {
		return ErrHasAttempts
	}

	res, err := tx.ExecContext(ctx, `UPDATE exams SET deleted_at=now() WHERE id=$1 AND school_id=$2 AND deleted_at IS NULL`, id, schoolID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotEditable
	}
	return tx.Commit()
}

// --- exam_questions ---

// AddExamQuestion appends a question at the end of an exam's order. The
// next order_index is read-then-written, so two concurrent calls for the
// same exam (two admins importing at once, or a doubled-up click) could
// both read the same MAX and collide on exam_questions_exam_id_order_index_key
// -- root-caused via a 500 on POST /exams/{id}/questions/import that
// happened mid live testing. pg_advisory_xact_lock serializes callers on
// the same exam_id (auto-released at commit/rollback) so the read+insert
// is effectively atomic instead of racing.
func (s *Store) AddExamQuestion(ctx context.Context, examID, questionID string, points float64) (*model.ExamQuestion, error) {
	var eq model.ExamQuestion
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtext($1))`, examID); err != nil {
		return nil, err
	}
	var nextIdx int
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(order_index)+1, 0) FROM exam_questions WHERE exam_id=$1`, examID).Scan(&nextIdx); err != nil {
		return nil, err
	}
	if err := tx.QueryRowContext(ctx, `
		INSERT INTO exam_questions (exam_id, question_id, order_index, points)
		VALUES ($1,$2,$3,$4) RETURNING id, created_at`,
		examID, questionID, nextIdx, points).Scan(&eq.ID, &eq.CreatedAt); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	eq.ExamID = examID
	eq.QuestionID = questionID
	eq.OrderIndex = nextIdx
	eq.Points = points
	return &eq, nil
}

func (s *Store) RemoveExamQuestion(ctx context.Context, examID, examQuestionID string) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM exam_questions WHERE id=$1 AND exam_id=$2`, examQuestionID, examID)
	return err
}

// UpdateExamQuestionPoints updates this exam's own point snapshot for a
// question, independent of questions.poin (the bank-wide default). Needed
// because editing a question's poin from inside an exam's question list
// only ever touched the bank record -- exam_questions.points (what's
// actually displayed/summed for that exam) stayed stale.
func (s *Store) UpdateExamQuestionPoints(ctx context.Context, examID, examQuestionID string, points float64) error {
	res, err := s.DB.ExecContext(ctx, `UPDATE exam_questions SET points=$1 WHERE id=$2 AND exam_id=$3`, points, examQuestionID, examID)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// ListExamQuestions returns exam_questions joined with their question, ordered
// by order_index (the natural/base sequence; display numbering follows this
// unless a per-attempt shuffle order overrides it).
func (s *Store) ListExamQuestions(ctx context.Context, examID string) ([]model.ExamQuestion, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT eq.id, eq.exam_id, eq.question_id, eq.order_index, eq.points, eq.created_at,
			q.id, q.school_id, q.academic_year_id, q.folder_id, q.created_by, q.question_type, q.content, q.image_url,
			q.no_kd, q.konten_kd, q.indikator_pencapaian, q.level_kognitif, q.poin, q.is_published, q.created_at, q.updated_at
		FROM exam_questions eq JOIN questions q ON q.id = eq.question_id
		WHERE eq.exam_id=$1 ORDER BY eq.order_index`, examID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.ExamQuestion
	for rows.Next() {
		var eq model.ExamQuestion
		var q model.Question
		if err := rows.Scan(&eq.ID, &eq.ExamID, &eq.QuestionID, &eq.OrderIndex, &eq.Points, &eq.CreatedAt,
			&q.ID, &q.SchoolID, &q.AcademicYearID, &q.FolderID, &q.CreatedBy, &q.QuestionType, &q.Content, &q.ImageURL,
			&q.NoKD, &q.KontenKD, &q.IndikatorPencapaian, &q.LevelKognitif, &q.Poin, &q.IsPublished, &q.CreatedAt, &q.UpdatedAt); err != nil {
			return nil, err
		}
		eq.Question = &q
		out = append(out, eq)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range out {
		if err := s.loadQuestionOptions(ctx, out[i].Question); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// StudentExamRow is the siswa-dashboard view: an exam joined with that
// student's own attempt (if any voucher was ever issued to them for it).
type StudentExamRow struct {
	Exam    model.Exam         `json:"exam"`
	Attempt *model.ExamAttempt `json:"attempt"`
}

// ListMyExams buckets a student's PUBLISHED exams (scoped to their own
// classroom_id — resolved cross-service via academic-service, see
// httpclient.StudentClassroomID and handler/student.go's MyExams) into:
//   - "today": start_at falls on today's calendar date, and the exam hasn't
//     already been completed or missed (end_at passed) — a same-day exam
//     that hasn't opened yet still belongs here, just with a "belum_mulai"
//     attempt (or none at all).
//   - "upcoming": start_at falls on a future calendar date.
//   - "history": the student's attempt is 'selesai', OR the exam's window
//     has already closed (end_at < now) whether or not they ever attempted it.
//
// This LEFT JOINs exam_attempts rather than requiring one to already exist —
// the old INNER JOIN meant a freshly published exam was invisible on the
// dashboard until the student had already voucher-entered it once, which is
// backwards (see client bug report: "nothing shows up after guru
// publishes"). attempt is nil in the returned row until the student enters.
func (s *Store) ListMyExams(ctx context.Context, userID, classroomID, bucket string) ([]StudentExamRow, error) {
	if classroomID == "" {
		return nil, nil
	}
	// exam_classrooms (migration 0005) is the real target-audience set for a
	// multi-classroom exam -- e.classroom_id alone is only the "primary"
	// classroom and would hide the exam from students in any OTHER selected
	// classroom.
	where := "EXISTS (SELECT 1 FROM exam_classrooms ec WHERE ec.exam_id = e.id AND ec.classroom_id::text = $1) AND e.is_published = true AND e.deleted_at IS NULL"
	switch bucket {
	case "today":
		where += " AND e.start_at::date = now()::date AND (ea.status IS NULL OR ea.status != 'selesai') AND e.end_at >= now()"
	case "upcoming":
		where += " AND e.start_at::date > now()::date"
	case "history":
		where += " AND (ea.status = 'selesai' OR e.end_at < now())"
	}
	rows, err := s.DB.QueryContext(ctx, `
		SELECT e.id, e.school_id, e.academic_year_id, e.term_id, e.exam_type_id, e.title, e.description,
			e.classroom_id, e.subject_id, e.teacher_id, e.start_at, e.end_at, e.is_shuffled, e.is_published,
			e.published_at, e.created_by, e.created_at, e.updated_at,
			ea.id, ea.school_id, ea.academic_year_id, ea.exam_id, ea.user_id, ea.classroom_id, ea.token_id, ea.status,
			ea.question_order, ea.current_position, ea.focus_lost_count, ea.started_at, ea.submitted_at,
			ea.blocked_at, ea.blocked_reason, ea.blocked_by, ea.banned_at, ea.banned_reason, ea.banned_by,
			ea.score, ea.correct_count, ea.graded_at, ea.created_at, ea.updated_at
		FROM exams e
		LEFT JOIN exam_attempts ea ON ea.exam_id = e.id AND ea.user_id = $2
		WHERE `+where+`
		ORDER BY e.start_at`, classroomID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []StudentExamRow
	for rows.Next() {
		var e model.Exam
		var a model.ExamAttempt
		var qOrderJSON []byte
		var attemptID, aSchoolID, aAcademicYearID, aExamID, aUserID, aClassroomID, aStatus sql.NullString
		var tokenID sql.NullString
		var currentPosition, focusLostCount, correctCount sql.NullInt64
		var startedAt, submittedAt, blockedAt, bannedAt, gradedAt, aCreatedAt, aUpdatedAt sql.NullTime
		var blockedReason, blockedBy, bannedReason, bannedBy sql.NullString
		var score sql.NullFloat64
		if err := rows.Scan(&e.ID, &e.SchoolID, &e.AcademicYearID, &e.TermID, &e.ExamTypeID, &e.Title, &e.Description,
			&e.ClassroomID, &e.SubjectID, &e.TeacherID, &e.StartAt, &e.EndAt, &e.IsShuffled, &e.IsPublished,
			&e.PublishedAt, &e.CreatedBy, &e.CreatedAt, &e.UpdatedAt,
			&attemptID, &aSchoolID, &aAcademicYearID, &aExamID, &aUserID, &aClassroomID, &tokenID, &aStatus,
			&qOrderJSON, &currentPosition, &focusLostCount, &startedAt, &submittedAt,
			&blockedAt, &blockedReason, &blockedBy, &bannedAt, &bannedReason, &bannedBy,
			&score, &correctCount, &gradedAt, &aCreatedAt, &aUpdatedAt); err != nil {
			return nil, err
		}
		row := StudentExamRow{Exam: e}
		if attemptID.Valid {
			a = model.ExamAttempt{
				ID: attemptID.String, SchoolID: aSchoolID.String, AcademicYearID: aAcademicYearID.String,
				ExamID: aExamID.String, UserID: aUserID.String, ClassroomID: aClassroomID.String,
				Status: aStatus.String, CurrentPosition: int(currentPosition.Int64), FocusLostCount: int(focusLostCount.Int64),
			}
			if tokenID.Valid {
				a.TokenID = &tokenID.String
			}
			if len(qOrderJSON) > 0 {
				_ = json.Unmarshal(qOrderJSON, &a.QuestionOrder)
			}
			if startedAt.Valid {
				a.StartedAt = &startedAt.Time
			}
			if submittedAt.Valid {
				a.SubmittedAt = &submittedAt.Time
			}
			if blockedAt.Valid {
				a.BlockedAt = &blockedAt.Time
			}
			if blockedReason.Valid {
				a.BlockedReason = &blockedReason.String
			}
			if blockedBy.Valid {
				a.BlockedBy = &blockedBy.String
			}
			if bannedAt.Valid {
				a.BannedAt = &bannedAt.Time
			}
			if bannedReason.Valid {
				a.BannedReason = &bannedReason.String
			}
			if bannedBy.Valid {
				a.BannedBy = &bannedBy.String
			}
			if score.Valid {
				a.Score = &score.Float64
			}
			if correctCount.Valid {
				c := int(correctCount.Int64)
				a.CorrectCount = &c
			}
			if gradedAt.Valid {
				a.GradedAt = &gradedAt.Time
			}
			if aCreatedAt.Valid {
				a.CreatedAt = aCreatedAt.Time
			}
			if aUpdatedAt.Valid {
				a.UpdatedAt = aUpdatedAt.Time
			}
			row.Attempt = &a
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

// GetExamQuestion fetches one exam_questions row joined with its question
// (used when validating answers / manual grading overrides).
func (s *Store) GetExamQuestion(ctx context.Context, examQuestionID string) (*model.ExamQuestion, error) {
	var eq model.ExamQuestion
	var q model.Question
	err := s.DB.QueryRowContext(ctx, `
		SELECT eq.id, eq.exam_id, eq.question_id, eq.order_index, eq.points, eq.created_at,
			q.id, q.school_id, q.academic_year_id, q.folder_id, q.created_by, q.question_type, q.content, q.image_url,
			q.no_kd, q.konten_kd, q.indikator_pencapaian, q.level_kognitif, q.poin, q.is_published, q.created_at, q.updated_at
		FROM exam_questions eq JOIN questions q ON q.id = eq.question_id
		WHERE eq.id=$1`, examQuestionID,
	).Scan(&eq.ID, &eq.ExamID, &eq.QuestionID, &eq.OrderIndex, &eq.Points, &eq.CreatedAt,
		&q.ID, &q.SchoolID, &q.AcademicYearID, &q.FolderID, &q.CreatedBy, &q.QuestionType, &q.Content, &q.ImageURL,
		&q.NoKD, &q.KontenKD, &q.IndikatorPencapaian, &q.LevelKognitif, &q.Poin, &q.IsPublished, &q.CreatedAt, &q.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	eq.Question = &q

	rows, err := s.DB.QueryContext(ctx, `
		SELECT id, question_id, content, image_url, is_correct, order_index
		FROM answer_options WHERE question_id=$1 ORDER BY order_index`, q.ID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var o model.AnswerOption
		if err := rows.Scan(&o.ID, &o.QuestionID, &o.Content, &o.ImageURL, &o.IsCorrect, &o.OrderIndex); err != nil {
			rows.Close()
			return nil, err
		}
		q.AnswerOptions = append(q.AnswerOptions, o)
	}
	rows.Close()

	rows2, err := s.DB.QueryContext(ctx, `
		SELECT id, question_id, answer_text FROM essay_answer_options WHERE question_id=$1`, q.ID)
	if err != nil {
		return nil, err
	}
	for rows2.Next() {
		var e model.EssayAnswerOption
		if err := rows2.Scan(&e.ID, &e.QuestionID, &e.AnswerText); err != nil {
			rows2.Close()
			return nil, err
		}
		q.EssayOptions = append(q.EssayOptions, e)
	}
	rows2.Close()

	return &eq, nil
}

// NormalizeAndPublish proportionally scales exam_questions.points so they sum
// to exactly 100, then marks the exam published. See README "Score
// normalization" for the approach.
// scalePointsTo100 rescales an exam's existing exam_questions.points
// proportionally so they sum to exactly 100, preserving relative weights
// (last item absorbs rounding residue). Returns the question count so
// callers can decide how to treat zero questions -- NormalizeAndPublish
// treats it as a validation error (can't publish an empty exam),
// RenormalizePublishedPoints treats it as a harmless no-op (the guru just
// removed the last question, nothing left to scale).
func scalePointsTo100(ctx context.Context, tx *sql.Tx, examID string) (int, error) {
	rows, err := tx.QueryContext(ctx, `SELECT id, points FROM exam_questions WHERE exam_id=$1 ORDER BY order_index`, examID)
	if err != nil {
		return 0, err
	}
	type row struct {
		id     string
		points float64
	}
	var items []row
	var sum float64
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.id, &r.points); err != nil {
			rows.Close()
			return 0, err
		}
		items = append(items, r)
		sum += r.points
	}
	rows.Close()

	if len(items) == 0 {
		return 0, nil
	}
	if sum <= 0 {
		return len(items), fmt.Errorf("exam total raw points must be greater than zero")
	}

	var running float64
	for i, it := range items {
		var scaled float64
		if i == len(items)-1 {
			scaled = round2(100 - running)
		} else {
			scaled = round2(it.points / sum * 100)
			running += scaled
		}
		if _, err := tx.ExecContext(ctx, `UPDATE exam_questions SET points=$1 WHERE id=$2`, scaled, it.id); err != nil {
			return len(items), err
		}
	}
	return len(items), nil
}

func (s *Store) NormalizeAndPublish(ctx context.Context, examID, schoolID string) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	n, err := scalePointsTo100(ctx, tx, examID)
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("exam has no questions")
	}

	res, err := tx.ExecContext(ctx, `
		UPDATE exams SET is_published=true, published_at=now(), updated_at=now()
		WHERE id=$1 AND school_id=$2 AND deleted_at IS NULL`, examID, schoolID)
	if err != nil {
		return err
	}
	n2, _ := res.RowsAffected()
	if n2 == 0 {
		return ErrNotEditable
	}

	return tx.Commit()
}

// UnpublishExam reverts a published exam back to draft. Time-window gating
// (only before the exam's own start_at) is the caller's job -- see
// UnpublishExam handler -- this just flips the flag once permitted.
func (s *Store) UnpublishExam(ctx context.Context, examID, schoolID string) error {
	res, err := s.DB.ExecContext(ctx, `
		UPDATE exams SET is_published=false, published_at=NULL, updated_at=now()
		WHERE id=$1 AND school_id=$2 AND deleted_at IS NULL AND is_published=true`, examID, schoolID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotEditable
	}
	return nil
}

// RenormalizePublishedPoints: Bug Report SKCIXGPM 0.1 -- guru can keep
// adding/removing questions on a published exam (until any exam_attempts
// row exists, guarded at the handler level via HasAnyAttempt), so the
// "points sum to 100" invariant NormalizeAndPublish established at first
// publish must be re-applied after every such edit, not just once. Unlike
// NormalizeAndPublish this never touches is_published/published_at --
// the exam is already published, this only rescales points.
func (s *Store) RenormalizePublishedPoints(ctx context.Context, examID string) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := scalePointsTo100(ctx, tx, examID); err != nil {
		return err
	}
	return tx.Commit()
}

func round2(f float64) float64 {
	return float64(int64(f*100+0.5)) / 100
}

// NormalizeDraftPoints is the "Simpan sebagai Draft" action (client spec
// items 5/6): distribute 100 points evenly across the exam's current
// questions using distributeEqualPoints (integer remainder distribution, so
// the sum is always exactly 100 — never 99.99999999). Unlike
// NormalizeAndPublish, this does NOT flip is_published and may be called
// repeatedly (e.g. every time a question is added/removed) as long as the
// exam is still a draft; each call re-normalizes from scratch, discarding
// any previous scaling.
func (s *Store) NormalizeDraftPoints(ctx context.Context, examID, schoolID string) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var isPublished bool
	err = tx.QueryRowContext(ctx, `SELECT is_published FROM exams WHERE id=$1 AND ($2 = '' OR school_id::text=$2) AND deleted_at IS NULL`, examID, schoolID).Scan(&isPublished)
	if err == sql.ErrNoRows {
		return ErrNotEditable
	}
	if err != nil {
		return err
	}
	if isPublished {
		return ErrNotEditable
	}

	rows, err := tx.QueryContext(ctx, `SELECT id FROM exam_questions WHERE exam_id=$1 ORDER BY order_index`, examID)
	if err != nil {
		return err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	rows.Close()
	if len(ids) == 0 {
		return fmt.Errorf("exam has no questions")
	}

	points := distributeEqualPoints(len(ids))
	for i, id := range ids {
		if _, err := tx.ExecContext(ctx, `UPDATE exam_questions SET points=$1 WHERE id=$2`, points[i], id); err != nil {
			return err
		}
	}

	return tx.Commit()
}

// DuplicateExam copies an exam's settings and its exam_questions into a new,
// always-unpublished exam row (see ErrNotEditable-adjacent lifecycle rule:
// a duplicate is a fresh draft, not a clone of the source's publish state).
// Tokens/attempts are intentionally not copied — those belong to the source
// exam's live exam-taking data only.
func (s *Store) DuplicateExam(ctx context.Context, id, schoolID, actorUserID string) (*model.Exam, error) {
	src, err := s.GetExam(ctx, id, schoolID)
	if err != nil {
		return nil, err
	}
	if src == nil {
		return nil, nil
	}

	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	e := &model.Exam{}
	err = tx.QueryRowContext(ctx, `
		INSERT INTO exams (school_id, academic_year_id, term_id, exam_type_id, title, description,
			classroom_id, subject_id, teacher_id, start_at, end_at, is_shuffled, created_by, folder_id)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)
		RETURNING id, school_id, academic_year_id, term_id, exam_type_id, title, description,
			classroom_id, subject_id, teacher_id, start_at, end_at, is_shuffled, is_published,
			published_at, created_by, created_at, updated_at, folder_id`,
		src.SchoolID, src.AcademicYearID, src.TermID, src.ExamTypeID, src.Title+" (Salinan)", src.Description,
		src.ClassroomID, src.SubjectID, src.TeacherID, src.StartAt, src.EndAt, src.IsShuffled, actorUserID, src.FolderID,
	).Scan(&e.ID, &e.SchoolID, &e.AcademicYearID, &e.TermID, &e.ExamTypeID, &e.Title, &e.Description,
		&e.ClassroomID, &e.SubjectID, &e.TeacherID, &e.StartAt, &e.EndAt, &e.IsShuffled, &e.IsPublished,
		&e.PublishedAt, &e.CreatedBy, &e.CreatedAt, &e.UpdatedAt, &e.FolderID)
	if err != nil {
		return nil, err
	}

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO exam_questions (exam_id, question_id, order_index, points)
		SELECT $1, question_id, order_index, points FROM exam_questions WHERE exam_id=$2`,
		e.ID, id); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	e.QuestionCount = src.QuestionCount
	e.TotalPoints = src.TotalPoints
	return e, nil
}

// MoveExamsToFolder re-files exams into a Bank Soal folder (or out of one
// when folderID is empty). UpdateExam deliberately never touches folder_id
// -- editing an exam must not silently re-file it -- so this is the only
// way an exam's folder changes after creation, which is what SMK GPM's
// admin asked for after the "Tanpa Folder" listing made stray exams
// visible: they belong in the STS folder, retyping them is not an option.
//
// onlyCreatedBy scopes the update to one author (guru moving their own);
// empty means no author restriction (admin). Returns rows actually moved,
// so a caller passing an id from another school gets a count, not a lie.
func (s *Store) MoveExamsToFolder(ctx context.Context, examIDs []string, folderID, schoolID, onlyCreatedBy string) (int64, error) {
	if len(examIDs) == 0 {
		return 0, nil
	}
	var folder interface{}
	if folderID != "" {
		folder = folderID
	}
	args := []interface{}{folder, schoolID}
	ph := make([]string, 0, len(examIDs))
	for _, id := range examIDs {
		args = append(args, id)
		ph = append(ph, fmt.Sprintf("$%d", len(args)))
	}
	q := fmt.Sprintf(`UPDATE exams SET folder_id=$1, updated_at=now()
		WHERE school_id=$2 AND deleted_at IS NULL AND id::text IN (%s)`, strings.Join(ph, ","))
	if onlyCreatedBy != "" {
		args = append(args, onlyCreatedBy)
		q += fmt.Sprintf(" AND created_by=$%d", len(args))
	}
	res, err := s.DB.ExecContext(ctx, q, args...)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
