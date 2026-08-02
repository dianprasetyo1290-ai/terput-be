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

// IssueToken creates (or reissues, keeping the same code) a student's
// voucher — one per student, valid across any exam within [validFrom,
// validUntil]. No longer creates an exam_attempts row: that's created
// lazily in EnterExam once the student actually enters a specific exam.
func (s *Store) IssueToken(ctx context.Context, schoolID, academicYearID, userID, createdBy string, validFrom, validUntil time.Time) (*model.ExamToken, error) {
	var ayID *string
	if academicYearID != "" {
		ayID = &academicYearID
	}
	t := &model.ExamToken{
		SchoolID:       schoolID,
		AcademicYearID: ayID,
		UserID:         userID,
		Code:           GenerateCode(8),
		ValidFrom:      validFrom,
		ValidUntil:     validUntil,
		CreatedBy:      createdBy,
	}
	err := s.DB.QueryRowContext(ctx, `
		INSERT INTO exam_tokens (school_id, academic_year_id, user_id, code, valid_from, valid_until, created_by)
		VALUES ($1,$2,$3,$4,$5,$6,$7)
		ON CONFLICT (school_id, user_id) DO UPDATE SET valid_from=EXCLUDED.valid_from, valid_until=EXCLUDED.valid_until, updated_at=now()
		RETURNING id, code, regenerated_count, created_at, updated_at`,
		t.SchoolID, t.AcademicYearID, t.UserID, t.Code, t.ValidFrom, t.ValidUntil, t.CreatedBy,
	).Scan(&t.ID, &t.Code, &t.RegeneratedCount, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return t, nil
}

func (s *Store) RegenerateToken(ctx context.Context, id, schoolID string, validFrom, validUntil *time.Time) (*model.ExamToken, error) {
	newCode := GenerateCode(8)
	var t model.ExamToken
	var err error
	if validFrom != nil && validUntil != nil {
		err = s.DB.QueryRowContext(ctx, `
			UPDATE exam_tokens SET code=$1, valid_from=$2, valid_until=$3, regenerated_count=regenerated_count+1, updated_at=now()
			WHERE id=$4 AND school_id=$5
			RETURNING id, school_id, academic_year_id, user_id, code, valid_from, valid_until, regenerated_count, created_by, created_at, updated_at`,
			newCode, *validFrom, *validUntil, id, schoolID,
		).Scan(&t.ID, &t.SchoolID, &t.AcademicYearID, &t.UserID, &t.Code, &t.ValidFrom, &t.ValidUntil, &t.RegeneratedCount, &t.CreatedBy, &t.CreatedAt, &t.UpdatedAt)
	} else {
		err = s.DB.QueryRowContext(ctx, `
			UPDATE exam_tokens SET code=$1, regenerated_count=regenerated_count+1, updated_at=now()
			WHERE id=$2 AND school_id=$3
			RETURNING id, school_id, academic_year_id, user_id, code, valid_from, valid_until, regenerated_count, created_by, created_at, updated_at`,
			newCode, id, schoolID,
		).Scan(&t.ID, &t.SchoolID, &t.AcademicYearID, &t.UserID, &t.Code, &t.ValidFrom, &t.ValidUntil, &t.RegeneratedCount, &t.CreatedBy, &t.CreatedAt, &t.UpdatedAt)
	}
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// ListTokens returns every student voucher issued for the school (Data
// Siswa's voucher column reads this).
func (s *Store) ListTokens(ctx context.Context, schoolID string) ([]model.ExamToken, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT id, school_id, academic_year_id, user_id, code, valid_from, valid_until, regenerated_count, created_by, created_at, updated_at
		FROM exam_tokens WHERE school_id=$1 ORDER BY created_at`, schoolID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.ExamToken
	for rows.Next() {
		var t model.ExamToken
		if err := rows.Scan(&t.ID, &t.SchoolID, &t.AcademicYearID, &t.UserID, &t.Code, &t.ValidFrom, &t.ValidUntil, &t.RegeneratedCount, &t.CreatedBy, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (s *Store) GetTokenForUser(ctx context.Context, schoolID, userID string) (*model.ExamToken, error) {
	var t model.ExamToken
	err := s.DB.QueryRowContext(ctx, `
		SELECT id, school_id, academic_year_id, user_id, code, valid_from, valid_until, regenerated_count, created_by, created_at, updated_at
		FROM exam_tokens WHERE school_id=$1 AND user_id=$2`, schoolID, userID,
	).Scan(&t.ID, &t.SchoolID, &t.AcademicYearID, &t.UserID, &t.Code, &t.ValidFrom, &t.ValidUntil, &t.RegeneratedCount, &t.CreatedBy, &t.CreatedAt, &t.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &t, nil
}
