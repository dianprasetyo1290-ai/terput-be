package consent

import (
	"context"
	"database/sql"
	"errors"

	"skci/studentlife/internal/store"
)

const cols = `id, school_id, subject_type, subject_id, student_id, status, decided_by_guardian_id, decided_at, note, requested_by, created_at`

func scan(r interface{ Scan(...any) error }) (Consent, error) {
	var c Consent
	err := r.Scan(&c.ID, &c.SchoolID, &c.SubjectType, &c.SubjectID, &c.StudentID, &c.Status,
		&c.DecidedByGuardianID, &c.DecidedAt, &c.Note, &c.RequestedBy, &c.CreatedAt)
	return c, err
}

// Request creates (or returns the existing) consent for one student.
// created reports whether a new pending row was inserted.
func Request(ctx context.Context, q store.Q, schoolID, subjectType, subjectID, studentID, requestedBy string) (c Consent, created bool, err error) {
	c, err = scan(q.QueryRowContext(ctx, `INSERT INTO parent_consents (school_id, subject_type, subject_id, student_id, requested_by)
		VALUES ($1,$2,$3,$4,$5) ON CONFLICT (subject_type, subject_id, student_id) DO NOTHING RETURNING `+cols,
		schoolID, subjectType, subjectID, studentID, requestedBy))
	if err == nil {
		return c, true, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return c, false, err
	}
	c, err = scan(q.QueryRowContext(ctx, `SELECT `+cols+` FROM parent_consents
		WHERE school_id = $1 AND subject_type = $2 AND subject_id = $3 AND student_id = $4`,
		schoolID, subjectType, subjectID, studentID))
	return c, false, err
}

// Get reads one consent in the school.
func Get(ctx context.Context, q store.Q, schoolID, id string) (Consent, error) {
	c, err := scan(q.QueryRowContext(ctx, `SELECT `+cols+` FROM parent_consents WHERE school_id = $1 AND id = $2`, schoolID, id))
	return c, store.One(err)
}

// ListForStudents lists consents for the given students (optionally one subject type).
func ListForStudents(ctx context.Context, q store.Q, schoolID string, studentIDs []string, subjectType string) ([]Consent, error) {
	if len(studentIDs) == 0 {
		return []Consent{}, nil
	}
	rows, err := q.QueryContext(ctx, `SELECT `+cols+` FROM parent_consents
		WHERE school_id = $1 AND student_id::text = ANY($2) AND ($3 = '' OR subject_type = $3)
		ORDER BY (status = 'pending') DESC, created_at DESC`, schoolID, studentIDs, subjectType)
	return store.Collect(rows, err, func(r *sql.Rows) (Consent, error) { return scan(r) })
}

// SaveDecision persists a Decide result only if the row still has the
// status it was read with (the second of two concurrent guardians loses).
func SaveDecision(ctx context.Context, q store.Q, c Consent, fromStatus, note string) error {
	res, err := q.ExecContext(ctx, `UPDATE parent_consents SET status = $3, decided_by_guardian_id = $4, decided_at = $5,
		note = NULLIF($6,''), updated_at = now() WHERE school_id = $1 AND id = $2 AND status = $7`,
		c.SchoolID, c.ID, c.Status, c.DecidedByGuardianID, c.DecidedAt, note, fromStatus)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrFinal
	}
	return nil
}
