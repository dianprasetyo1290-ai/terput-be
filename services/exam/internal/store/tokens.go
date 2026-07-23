package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"strings"
	"time"

	"skci/exam/internal/model"
)

const codeAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789" // no 0/O/1/I to avoid confusion

func GenerateCode(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	var sb strings.Builder
	for _, c := range b {
		sb.WriteByte(codeAlphabet[int(c)%len(codeAlphabet)])
	}
	return sb.String()
}

// IssueToken creates (or returns the existing) voucher for a student for this
// exam, and ensures a corresponding exam_attempts row exists in
// belum_mulai so the live monitor roster can list the student before they
// ever open the exam.
func (s *Store) IssueToken(ctx context.Context, schoolID, academicYearID, examID, userID, classroomID, createdBy string, expiresAt time.Time) (*model.ExamToken, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	t := &model.ExamToken{
		SchoolID:       schoolID,
		AcademicYearID: academicYearID,
		ExamID:         examID,
		UserID:         userID,
		ClassroomID:    classroomID,
		Code:           GenerateCode(8),
		ExpiresAt:      expiresAt,
		CreatedBy:      createdBy,
	}
	err = tx.QueryRowContext(ctx, `
		INSERT INTO exam_tokens (school_id, academic_year_id, exam_id, user_id, classroom_id, code, expires_at, created_by)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		ON CONFLICT (exam_id, user_id) DO UPDATE SET expires_at=EXCLUDED.expires_at, classroom_id=EXCLUDED.classroom_id, updated_at=now()
		RETURNING id, code, regenerated_count, created_at, updated_at`,
		t.SchoolID, t.AcademicYearID, t.ExamID, t.UserID, t.ClassroomID, t.Code, t.ExpiresAt, t.CreatedBy,
	).Scan(&t.ID, &t.Code, &t.RegeneratedCount, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		return nil, err
	}

	_, err = tx.ExecContext(ctx, `
		INSERT INTO exam_attempts (school_id, academic_year_id, exam_id, user_id, classroom_id, token_id, status)
		VALUES ($1,$2,$3,$4,$5,$6,'belum_mulai')
		ON CONFLICT (exam_id, user_id) DO UPDATE SET token_id=EXCLUDED.token_id, classroom_id=EXCLUDED.classroom_id`,
		schoolID, academicYearID, examID, userID, classroomID, t.ID)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return t, nil
}

func (s *Store) RegenerateToken(ctx context.Context, id, schoolID string, expiresAt *time.Time) (*model.ExamToken, error) {
	newCode := GenerateCode(8)
	var t model.ExamToken
	var err error
	if expiresAt != nil {
		err = s.DB.QueryRowContext(ctx, `
			UPDATE exam_tokens SET code=$1, expires_at=$2, regenerated_count=regenerated_count+1, updated_at=now()
			WHERE id=$3 AND school_id=$4
			RETURNING id, school_id, academic_year_id, exam_id, user_id, classroom_id, code, expires_at, regenerated_count, created_by, created_at, updated_at`,
			newCode, *expiresAt, id, schoolID,
		).Scan(&t.ID, &t.SchoolID, &t.AcademicYearID, &t.ExamID, &t.UserID, &t.ClassroomID, &t.Code, &t.ExpiresAt, &t.RegeneratedCount, &t.CreatedBy, &t.CreatedAt, &t.UpdatedAt)
	} else {
		err = s.DB.QueryRowContext(ctx, `
			UPDATE exam_tokens SET code=$1, regenerated_count=regenerated_count+1, updated_at=now()
			WHERE id=$2 AND school_id=$3
			RETURNING id, school_id, academic_year_id, exam_id, user_id, classroom_id, code, expires_at, regenerated_count, created_by, created_at, updated_at`,
			newCode, id, schoolID,
		).Scan(&t.ID, &t.SchoolID, &t.AcademicYearID, &t.ExamID, &t.UserID, &t.ClassroomID, &t.Code, &t.ExpiresAt, &t.RegeneratedCount, &t.CreatedBy, &t.CreatedAt, &t.UpdatedAt)
	}
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func (s *Store) ListTokens(ctx context.Context, examID, schoolID string) ([]model.ExamToken, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT id, school_id, academic_year_id, exam_id, user_id, classroom_id, code, expires_at, regenerated_count, created_by, created_at, updated_at
		FROM exam_tokens WHERE exam_id=$1 AND school_id=$2 ORDER BY created_at`, examID, schoolID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.ExamToken
	for rows.Next() {
		var t model.ExamToken
		if err := rows.Scan(&t.ID, &t.SchoolID, &t.AcademicYearID, &t.ExamID, &t.UserID, &t.ClassroomID, &t.Code, &t.ExpiresAt, &t.RegeneratedCount, &t.CreatedBy, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (s *Store) FindTokenByCode(ctx context.Context, examID, code string) (*model.ExamToken, error) {
	var t model.ExamToken
	err := s.DB.QueryRowContext(ctx, `
		SELECT id, school_id, academic_year_id, exam_id, user_id, classroom_id, code, expires_at, regenerated_count, created_by, created_at, updated_at
		FROM exam_tokens WHERE exam_id=$1 AND code=$2`, examID, code,
	).Scan(&t.ID, &t.SchoolID, &t.AcademicYearID, &t.ExamID, &t.UserID, &t.ClassroomID, &t.Code, &t.ExpiresAt, &t.RegeneratedCount, &t.CreatedBy, &t.CreatedAt, &t.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &t, nil
}
