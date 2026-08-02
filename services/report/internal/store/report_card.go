package store

import (
	"context"
	"database/sql"
	"time"

	"skci/report/internal/model"
)

// GetOrCreateReportCard returns the existing report card for
// (student, classroom, term) or creates a fresh draft one. The returned
// bool is true when a new row was inserted (used by generate to decide
// whether to log a "created" vs "refreshed" count).
func (s *Store) GetOrCreateReportCard(ctx context.Context, schoolID, academicYearID, termID, classroomID, studentID string) (*model.ReportCard, bool, error) {
	row := s.DB.QueryRowContext(ctx, `
		INSERT INTO report_cards (school_id, academic_year_id, term_id, classroom_id, student_id)
		VALUES ($1,$2,$3,$4,$5)
		ON CONFLICT (student_id, classroom_id, term_id)
		DO UPDATE SET updated_at = now()
		RETURNING id, school_id, academic_year_id, term_id, classroom_id, student_id, status,
			finalized_at, finalized_by, unlocked_at, unlocked_by, class_teacher_note,
			attendance_hadir, attendance_sakit, attendance_izin, attendance_tanpa_keterangan,
			attendance_synced_at, attendance_sync_error,
			student_name, student_nisn, classroom_name, term_name,
			homeroom_teacher_name, principal_name, header_synced_at,
			created_at, updated_at, (xmax = 0) AS inserted`,
		schoolID, academicYearID, termID, classroomID, studentID)
	rc, inserted, err := scanReportCard(row)
	if err != nil {
		return nil, false, err
	}
	return rc, inserted, nil
}

func scanReportCard(row *sql.Row) (*model.ReportCard, bool, error) {
	var rc model.ReportCard
	var finalizedBy, unlockedBy, classTeacherNote sql.NullString
	var finalizedAt, unlockedAt, attendanceSyncedAt, headerSyncedAt sql.NullTime
	var hadir, sakit, izin, tanpa sql.NullInt64
	var attendanceSyncErr sql.NullString
	var studentName, studentNISN, classroomName, termName, homeroomTeacherName, principalName sql.NullString
	var inserted bool

	err := row.Scan(
		&rc.ID, &rc.SchoolID, &rc.AcademicYearID, &rc.TermID, &rc.ClassroomID, &rc.StudentID, &rc.Status,
		&finalizedAt, &finalizedBy, &unlockedAt, &unlockedBy, &classTeacherNote,
		&hadir, &sakit, &izin, &tanpa,
		&attendanceSyncedAt, &attendanceSyncErr,
		&studentName, &studentNISN, &classroomName, &termName,
		&homeroomTeacherName, &principalName, &headerSyncedAt,
		&rc.CreatedAt, &rc.UpdatedAt, &inserted,
	)
	if err == sql.ErrNoRows {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}

	if finalizedAt.Valid {
		t := finalizedAt.Time
		rc.FinalizedAt = &t
	}
	if finalizedBy.Valid {
		rc.FinalizedBy = &finalizedBy.String
	}
	if unlockedAt.Valid {
		t := unlockedAt.Time
		rc.UnlockedAt = &t
	}
	if unlockedBy.Valid {
		rc.UnlockedBy = &unlockedBy.String
	}
	if classTeacherNote.Valid {
		rc.ClassTeacherNote = &classTeacherNote.String
	}
	if hadir.Valid {
		v := int(hadir.Int64)
		rc.Attendance.Hadir = &v
	}
	if sakit.Valid {
		v := int(sakit.Int64)
		rc.Attendance.Sakit = &v
	}
	if izin.Valid {
		v := int(izin.Int64)
		rc.Attendance.Izin = &v
	}
	if tanpa.Valid {
		v := int(tanpa.Int64)
		rc.Attendance.TanpaKeterangan = &v
	}
	if attendanceSyncedAt.Valid {
		t := attendanceSyncedAt.Time
		rc.Attendance.SyncedAt = &t
	}
	if attendanceSyncErr.Valid {
		rc.Attendance.SyncError = &attendanceSyncErr.String
	}
	if studentName.Valid {
		rc.StudentName = &studentName.String
	}
	if studentNISN.Valid {
		rc.StudentNISN = &studentNISN.String
	}
	if classroomName.Valid {
		rc.ClassroomName = &classroomName.String
	}
	if termName.Valid {
		rc.TermName = &termName.String
	}
	if homeroomTeacherName.Valid {
		rc.HomeroomTeacherName = &homeroomTeacherName.String
	}
	if principalName.Valid {
		rc.PrincipalName = &principalName.String
	}
	if headerSyncedAt.Valid {
		t := headerSyncedAt.Time
		rc.HeaderSyncedAt = &t
	}
	return &rc, inserted, nil
}

const reportCardSelectCols = `id, school_id, academic_year_id, term_id, classroom_id, student_id, status,
	finalized_at, finalized_by, unlocked_at, unlocked_by, class_teacher_note,
	attendance_hadir, attendance_sakit, attendance_izin, attendance_tanpa_keterangan,
	attendance_synced_at, attendance_sync_error,
	student_name, student_nisn, classroom_name, term_name,
	homeroom_teacher_name, principal_name, header_synced_at,
	created_at, updated_at, false AS inserted`

func (s *Store) GetReportCard(ctx context.Context, id string) (*model.ReportCard, error) {
	row := s.DB.QueryRowContext(ctx, `SELECT `+reportCardSelectCols+` FROM report_cards WHERE id = $1`, id)
	rc, _, err := scanReportCard(row)
	return rc, err
}

func (s *Store) ListReportCardsByStudent(ctx context.Context, studentID string, termID string) ([]model.ReportCard, error) {
	query := `SELECT ` + reportCardSelectCols + ` FROM report_cards WHERE student_id = $1`
	args := []interface{}{studentID}
	if termID != "" {
		query += ` AND term_id = $2`
		args = append(args, termID)
	}
	query += ` ORDER BY created_at DESC`
	rows, err := s.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanReportCardRows(rows)
}

func (s *Store) ListReportCardsByClassroomTerm(ctx context.Context, classroomID, termID string) ([]model.ReportCard, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+reportCardSelectCols+` FROM report_cards WHERE classroom_id = $1 AND term_id = $2 ORDER BY student_name NULLS LAST, created_at`, classroomID, termID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanReportCardRows(rows)
}

func scanReportCardRows(rows *sql.Rows) ([]model.ReportCard, error) {
	var out []model.ReportCard
	for rows.Next() {
		var rc model.ReportCard
		var finalizedBy, unlockedBy, classTeacherNote sql.NullString
		var finalizedAt, unlockedAt, attendanceSyncedAt, headerSyncedAt sql.NullTime
		var hadir, sakit, izin, tanpa sql.NullInt64
		var attendanceSyncErr sql.NullString
		var studentName, studentNISN, classroomName, termName, homeroomTeacherName, principalName sql.NullString
		var inserted bool

		if err := rows.Scan(
			&rc.ID, &rc.SchoolID, &rc.AcademicYearID, &rc.TermID, &rc.ClassroomID, &rc.StudentID, &rc.Status,
			&finalizedAt, &finalizedBy, &unlockedAt, &unlockedBy, &classTeacherNote,
			&hadir, &sakit, &izin, &tanpa,
			&attendanceSyncedAt, &attendanceSyncErr,
			&studentName, &studentNISN, &classroomName, &termName,
			&homeroomTeacherName, &principalName, &headerSyncedAt,
			&rc.CreatedAt, &rc.UpdatedAt, &inserted,
		); err != nil {
			return nil, err
		}
		if finalizedAt.Valid {
			t := finalizedAt.Time
			rc.FinalizedAt = &t
		}
		if finalizedBy.Valid {
			rc.FinalizedBy = &finalizedBy.String
		}
		if unlockedAt.Valid {
			t := unlockedAt.Time
			rc.UnlockedAt = &t
		}
		if unlockedBy.Valid {
			rc.UnlockedBy = &unlockedBy.String
		}
		if classTeacherNote.Valid {
			rc.ClassTeacherNote = &classTeacherNote.String
		}
		if hadir.Valid {
			v := int(hadir.Int64)
			rc.Attendance.Hadir = &v
		}
		if sakit.Valid {
			v := int(sakit.Int64)
			rc.Attendance.Sakit = &v
		}
		if izin.Valid {
			v := int(izin.Int64)
			rc.Attendance.Izin = &v
		}
		if tanpa.Valid {
			v := int(tanpa.Int64)
			rc.Attendance.TanpaKeterangan = &v
		}
		if attendanceSyncedAt.Valid {
			t := attendanceSyncedAt.Time
			rc.Attendance.SyncedAt = &t
		}
		if attendanceSyncErr.Valid {
			rc.Attendance.SyncError = &attendanceSyncErr.String
		}
		if studentName.Valid {
			rc.StudentName = &studentName.String
		}
		if studentNISN.Valid {
			rc.StudentNISN = &studentNISN.String
		}
		if classroomName.Valid {
			rc.ClassroomName = &classroomName.String
		}
		if termName.Valid {
			rc.TermName = &termName.String
		}
		if homeroomTeacherName.Valid {
			rc.HomeroomTeacherName = &homeroomTeacherName.String
		}
		if principalName.Valid {
			rc.PrincipalName = &principalName.String
		}
		if headerSyncedAt.Valid {
			t := headerSyncedAt.Time
			rc.HeaderSyncedAt = &t
		}
		out = append(out, rc)
	}
	return out, rows.Err()
}

func (s *Store) UpdateHeaderSnapshot(ctx context.Context, id string, studentName, studentNISN, classroomName, termName, homeroomTeacherName, principalName *string, syncedAt time.Time) error {
	_, err := s.DB.ExecContext(ctx, `
		UPDATE report_cards SET student_name = $1, student_nisn = $2, classroom_name = $3, term_name = $4,
			homeroom_teacher_name = $5, principal_name = $6,
			header_synced_at = $7, updated_at = now()
		WHERE id = $8`,
		studentName, studentNISN, classroomName, termName, homeroomTeacherName, principalName, syncedAt, id)
	return err
}

func (s *Store) UpdateAttendance(ctx context.Context, id string, hadir, sakit, izin, tanpa *int, syncedAt *time.Time, syncErr *string) error {
	_, err := s.DB.ExecContext(ctx, `
		UPDATE report_cards SET attendance_hadir = $1, attendance_sakit = $2, attendance_izin = $3,
			attendance_tanpa_keterangan = $4, attendance_synced_at = $5, attendance_sync_error = $6, updated_at = now()
		WHERE id = $7`,
		hadir, sakit, izin, tanpa, syncedAt, syncErr, id)
	return err
}

// UpsertSubjectComputed writes/refreshes the computed_score for one
// subject row on a report card. It never touches override_score, notes, or
// overridden_* columns — an admin's manual adjustment survives a regenerate.
func (s *Store) UpsertSubjectComputed(ctx context.Context, reportCardID, schoolID, subjectID string, subjectName, subjectCode *string, computedScore, stsScore, sasScore *float64, sourceCount int, syncedAt *time.Time, syncErr *string) error {
	_, err := s.DB.ExecContext(ctx, `
		INSERT INTO report_card_subjects (report_card_id, school_id, subject_id, subject_name, subject_code, computed_score, sts_score, sas_score, source_exam_count, last_synced_at, sync_error)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
		ON CONFLICT (report_card_id, subject_id) DO UPDATE SET
			subject_name = EXCLUDED.subject_name,
			subject_code = EXCLUDED.subject_code,
			computed_score = EXCLUDED.computed_score,
			sts_score = EXCLUDED.sts_score,
			sas_score = EXCLUDED.sas_score,
			source_exam_count = EXCLUDED.source_exam_count,
			last_synced_at = EXCLUDED.last_synced_at,
			sync_error = EXCLUDED.sync_error,
			updated_at = now()`,
		reportCardID, schoolID, subjectID, subjectName, subjectCode, computedScore, stsScore, sasScore, sourceCount, syncedAt, syncErr)
	return err
}

func (s *Store) ListSubjects(ctx context.Context, reportCardID string) ([]model.ReportCardSubject, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT id, report_card_id, school_id, subject_id, subject_name, subject_code, computed_score,
			sts_score, sas_score,
			override_score, source_exam_count, grade_label, notes, mid_note, final_note, overridden_by, overridden_at,
			last_synced_at, sync_error, created_at, updated_at
		FROM report_card_subjects WHERE report_card_id = $1 ORDER BY subject_name NULLS LAST, created_at`, reportCardID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []model.ReportCardSubject
	for rows.Next() {
		var sub model.ReportCardSubject
		var subjectName, subjectCode, gradeLabel, notes, midNote, finalNote, overriddenBy, syncError sql.NullString
		var computedScore, stsScore, sasScore, overrideScore sql.NullFloat64
		var overriddenAt, lastSyncedAt sql.NullTime

		if err := rows.Scan(
			&sub.ID, &sub.ReportCardID, &sub.SchoolID, &sub.SubjectID, &subjectName, &subjectCode, &computedScore,
			&stsScore, &sasScore,
			&overrideScore, &sub.SourceExamCount, &gradeLabel, &notes, &midNote, &finalNote, &overriddenBy, &overriddenAt,
			&lastSyncedAt, &syncError, &sub.CreatedAt, &sub.UpdatedAt,
		); err != nil {
			return nil, err
		}
		if stsScore.Valid {
			v := stsScore.Float64
			sub.StsScore = &v
		}
		if sasScore.Valid {
			v := sasScore.Float64
			sub.SasScore = &v
		}
		if midNote.Valid {
			sub.MidNote = &midNote.String
		}
		if finalNote.Valid {
			sub.FinalNote = &finalNote.String
		}
		if subjectName.Valid {
			sub.SubjectName = &subjectName.String
		}
		if subjectCode.Valid {
			sub.SubjectCode = &subjectCode.String
		}
		if computedScore.Valid {
			v := computedScore.Float64
			sub.ComputedScore = &v
		}
		if overrideScore.Valid {
			v := overrideScore.Float64
			sub.OverrideScore = &v
		}
		if gradeLabel.Valid {
			sub.GradeLabel = &gradeLabel.String
		}
		if notes.Valid {
			sub.Notes = &notes.String
		}
		if overriddenBy.Valid {
			sub.OverriddenBy = &overriddenBy.String
		}
		if overriddenAt.Valid {
			t := overriddenAt.Time
			sub.OverriddenAt = &t
		}
		if lastSyncedAt.Valid {
			t := lastSyncedAt.Time
			sub.LastSyncedAt = &t
		}
		if syncError.Valid {
			sub.SyncError = &syncError.String
		}
		sub.FinalScore = sub.Final()
		out = append(out, sub)
	}
	return out, rows.Err()
}

// PatchSubjectOverride sets (or clears, if overrideScore is nil) the manual
// override for a subject row. Returns sql.ErrNoRows if no such subject row
// exists on that report card.
func (s *Store) PatchSubjectOverride(ctx context.Context, reportCardID, subjectID string, overrideScore *float64, notes *string, overriddenBy string) error {
	res, err := s.DB.ExecContext(ctx, `
		UPDATE report_card_subjects SET override_score = $1, notes = COALESCE($2, notes),
			overridden_by = $3, overridden_at = now(), updated_at = now()
		WHERE report_card_id = $4 AND subject_id = $5`,
		overrideScore, notes, overriddenBy, reportCardID, subjectID)
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

// PatchSubjectNotes upserts the Deskripsi Rapor mid/final narrative notes
// for a subject row. Unlike PatchSubjectOverride, a nil value here means
// "leave unchanged", not "clear" — the two note fields are independent
// (guru may set one without touching the other).
func (s *Store) PatchSubjectNotes(ctx context.Context, reportCardID, subjectID string, midNote, finalNote *string) error {
	res, err := s.DB.ExecContext(ctx, `
		UPDATE report_card_subjects SET
			mid_note = COALESCE($1, mid_note),
			final_note = COALESCE($2, final_note),
			updated_at = now()
		WHERE report_card_id = $3 AND subject_id = $4`,
		midNote, finalNote, reportCardID, subjectID)
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

func (s *Store) FinalizeReportCard(ctx context.Context, id, finalizedBy string) (bool, error) {
	res, err := s.DB.ExecContext(ctx, `
		UPDATE report_cards SET status = 'finalized', finalized_at = now(), finalized_by = $1, updated_at = now()
		WHERE id = $2 AND status = 'draft'`, finalizedBy, id)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

func (s *Store) UnlockReportCard(ctx context.Context, id, unlockedBy string) (bool, error) {
	res, err := s.DB.ExecContext(ctx, `
		UPDATE report_cards SET status = 'draft', unlocked_at = now(), unlocked_by = $1, updated_at = now()
		WHERE id = $2 AND status = 'finalized'`, unlockedBy, id)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n > 0, err
}
