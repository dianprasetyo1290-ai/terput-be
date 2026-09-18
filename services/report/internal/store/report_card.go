package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/lib/pq"

	"skci/report/internal/model"
)

// GetOrCreateReportCard returns the existing report card for
// (student, classroom, term) or creates a fresh draft one. The returned
// bool is true when a new row was inserted (used by generate to decide
// whether to log a "created" vs "refreshed" count).
func (s *Store) GetOrCreateReportCard(ctx context.Context, schoolID, academicYearID, termID, classroomID, studentID, reportType, periodLabel string) (*model.ReportCard, bool, error) {
	row := s.DB.QueryRowContext(ctx, `
		INSERT INTO report_cards (school_id, academic_year_id, term_id, classroom_id, student_id, report_type, period_label)
		VALUES ($1,$2,$3,$4,$5,$6,$7)
		ON CONFLICT (student_id, classroom_id, term_id, report_type, period_label)
		DO UPDATE SET updated_at = now()
		RETURNING id, school_id, academic_year_id, term_id, classroom_id, student_id, status,
			finalized_at, finalized_by, unlocked_at, unlocked_by, class_teacher_note,
			kokurikuler_note, ekstrakurikuler, prestasi, kenaikan_status, kenaikan_kelas,
			attendance_hadir, attendance_sakit, attendance_izin, attendance_tanpa_keterangan,
			attendance_synced_at, attendance_sync_error,
			student_name, student_nisn, classroom_name, term_name,
			homeroom_teacher_name, principal_name, header_synced_at,
			report_type, period_label,
			created_at, updated_at, (xmax = 0) AS inserted`,
		schoolID, academicYearID, termID, classroomID, studentID, reportType, periodLabel)
	rc, inserted, err := scanReportCard(row)
	if err != nil {
		return nil, false, err
	}
	return rc, inserted, nil
}

// raporExtras holds the hand-entered rapor sections' raw scan targets
// (migrations 0010 + 0011). Grouped into one struct so the two scan sites
// below stay a single line each instead of growing a variable per column.
type raporExtras struct {
	kokurikulerNote sql.NullString
	ekskulJSON      []byte
	prestasiJSON    []byte
	kenaikanStatus  sql.NullString
	kenaikanKelas   sql.NullString
}

// assign decodes the scanned extras onto a report card. The two JSONB
// lists are normalized to non-nil slices so they marshal as [] rather than
// null -- the print view and the admin form both iterate them unguarded. A
// malformed JSONB value (only reachable by hand-editing the row) degrades
// to an empty list rather than failing the whole read.
func (e *raporExtras) assign(rc *model.ReportCard) {
	if e.kokurikulerNote.Valid {
		rc.KokurikulerNote = &e.kokurikulerNote.String
	}
	if e.kenaikanStatus.Valid {
		rc.KenaikanStatus = &e.kenaikanStatus.String
	}
	if e.kenaikanKelas.Valid {
		rc.KenaikanKelas = &e.kenaikanKelas.String
	}
	rc.Ekstrakurikuler = []model.Ekskul{}
	if len(e.ekskulJSON) > 0 {
		var list []model.Ekskul
		if json.Unmarshal(e.ekskulJSON, &list) == nil && list != nil {
			rc.Ekstrakurikuler = list
		}
	}
	rc.Prestasi = []model.Prestasi{}
	if len(e.prestasiJSON) > 0 {
		var list []model.Prestasi
		if json.Unmarshal(e.prestasiJSON, &list) == nil && list != nil {
			rc.Prestasi = list
		}
	}
}

// UpdateRaporExtras writes the rapor sections an admin/wali kelas fills in
// by hand -- Kokurikuler, Ekstrakurikuler, Catatan Wali Kelas, and (akhir
// semester only) Prestasi and Kenaikan Kelas. All are replaced wholesale;
// the caller passes the current value for anything it isn't changing.
// Returns sql.ErrNoRows if the report card doesn't exist.
type RaporExtrasInput struct {
	KokurikulerNote  *string
	ClassTeacherNote *string
	Ekstrakurikuler  []model.Ekskul
	Prestasi         []model.Prestasi
	KenaikanStatus   *string
	KenaikanKelas    *string
}

func (s *Store) UpdateRaporExtras(ctx context.Context, id string, in RaporExtrasInput) error {
	if in.Ekstrakurikuler == nil {
		in.Ekstrakurikuler = []model.Ekskul{}
	}
	if in.Prestasi == nil {
		in.Prestasi = []model.Prestasi{}
	}
	raw, err := json.Marshal(in.Ekstrakurikuler)
	if err != nil {
		return err
	}
	rawPrestasi, err := json.Marshal(in.Prestasi)
	if err != nil {
		return err
	}
	res, err := s.DB.ExecContext(ctx, `
		UPDATE report_cards
		SET kokurikuler_note = $2, class_teacher_note = $3, ekstrakurikuler = $4::jsonb,
			prestasi = $5::jsonb, kenaikan_status = $6, kenaikan_kelas = $7, updated_at = now()
		WHERE id = $1`, id, in.KokurikulerNote, in.ClassTeacherNote, string(raw),
		string(rawPrestasi), in.KenaikanStatus, in.KenaikanKelas)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func scanReportCard(row *sql.Row) (*model.ReportCard, bool, error) {
	var rc model.ReportCard
	var finalizedBy, unlockedBy, classTeacherNote sql.NullString
	var extras raporExtras
	var finalizedAt, unlockedAt, attendanceSyncedAt, headerSyncedAt sql.NullTime
	var hadir, sakit, izin, tanpa sql.NullInt64
	var attendanceSyncErr sql.NullString
	var studentName, studentNISN, classroomName, termName, homeroomTeacherName, principalName sql.NullString
	var inserted bool

	err := row.Scan(
		&rc.ID, &rc.SchoolID, &rc.AcademicYearID, &rc.TermID, &rc.ClassroomID, &rc.StudentID, &rc.Status,
		&finalizedAt, &finalizedBy, &unlockedAt, &unlockedBy, &classTeacherNote,
		&extras.kokurikulerNote, &extras.ekskulJSON, &extras.prestasiJSON, &extras.kenaikanStatus, &extras.kenaikanKelas,
		&hadir, &sakit, &izin, &tanpa,
		&attendanceSyncedAt, &attendanceSyncErr,
		&studentName, &studentNISN, &classroomName, &termName,
		&homeroomTeacherName, &principalName, &headerSyncedAt,
		&rc.ReportType, &rc.PeriodLabel,
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
	extras.assign(&rc)
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
	kokurikuler_note, ekstrakurikuler, prestasi, kenaikan_status, kenaikan_kelas,
	attendance_hadir, attendance_sakit, attendance_izin, attendance_tanpa_keterangan,
	attendance_synced_at, attendance_sync_error,
	student_name, student_nisn, classroom_name, term_name,
	homeroom_teacher_name, principal_name, header_synced_at,
	report_type, period_label,
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

// ListReportCardsByClassroomTerm lists report cards for a classroom/term,
// scoped to one report_type ("semester" or "bayangan" -- item 1/3: Leger
// is semester-only, Generate Rapor's list switches per the admin's chosen
// report type). periodLabel further scopes a bayangan listing to one
// quarterly period; ignored (any) for "semester".
func (s *Store) ListReportCardsByClassroomTerm(ctx context.Context, classroomID, termID, reportType, periodLabel string) ([]model.ReportCard, error) {
	query := `SELECT ` + reportCardSelectCols + ` FROM report_cards WHERE classroom_id = $1 AND term_id = $2 AND report_type = $3`
	args := []interface{}{classroomID, termID, reportType}
	if reportType == "bayangan" && periodLabel != "" {
		query += ` AND period_label = $4`
		args = append(args, periodLabel)
	}
	query += ` ORDER BY student_name NULLS LAST, created_at`
	rows, err := s.DB.QueryContext(ctx, query, args...)
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
		var extras raporExtras
		var finalizedAt, unlockedAt, attendanceSyncedAt, headerSyncedAt sql.NullTime
		var hadir, sakit, izin, tanpa sql.NullInt64
		var attendanceSyncErr sql.NullString
		var studentName, studentNISN, classroomName, termName, homeroomTeacherName, principalName sql.NullString
		var inserted bool

		if err := rows.Scan(
			&rc.ID, &rc.SchoolID, &rc.AcademicYearID, &rc.TermID, &rc.ClassroomID, &rc.StudentID, &rc.Status,
			&finalizedAt, &finalizedBy, &unlockedAt, &unlockedBy, &classTeacherNote,
			&extras.kokurikulerNote, &extras.ekskulJSON, &extras.prestasiJSON, &extras.kenaikanStatus, &extras.kenaikanKelas,
			&hadir, &sakit, &izin, &tanpa,
			&attendanceSyncedAt, &attendanceSyncErr,
			&studentName, &studentNISN, &classroomName, &termName,
			&homeroomTeacherName, &principalName, &headerSyncedAt,
			&rc.ReportType, &rc.PeriodLabel,
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
		extras.assign(&rc)
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
func (s *Store) UpsertSubjectComputed(ctx context.Context, reportCardID, schoolID, subjectID string, subjectName, subjectCode *string, computedScore, formatifScore, stsScore, sasScore, praktikScore *float64, sourceCount int, syncedAt *time.Time, syncErr *string) error {
	_, err := s.DB.ExecContext(ctx, `
		INSERT INTO report_card_subjects (report_card_id, school_id, subject_id, subject_name, subject_code, computed_score, formatif_score, sts_score, sas_score, praktik_score, source_exam_count, last_synced_at, sync_error)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
		ON CONFLICT (report_card_id, subject_id) DO UPDATE SET
			subject_name = EXCLUDED.subject_name,
			subject_code = EXCLUDED.subject_code,
			computed_score = EXCLUDED.computed_score,
			formatif_score = EXCLUDED.formatif_score,
			sts_score = EXCLUDED.sts_score,
			sas_score = EXCLUDED.sas_score,
			praktik_score = EXCLUDED.praktik_score,
			source_exam_count = EXCLUDED.source_exam_count,
			last_synced_at = EXCLUDED.last_synced_at,
			sync_error = EXCLUDED.sync_error,
			updated_at = now()`,
		reportCardID, schoolID, subjectID, subjectName, subjectCode, computedScore, formatifScore, stsScore, sasScore, praktikScore, sourceCount, syncedAt, syncErr)
	return err
}

// DeleteSubjectsNotIn removes report_card_subjects rows for subjects that
// are no longer in the student's allowed set (e.g. a religion-mismatched
// Agama variant provisioned before the allowed-subjects filter existed, or
// before a student's religion was corrected) -- otherwise a subject the
// filter now skips upserting still lingers forever with its old row.
func (s *Store) DeleteSubjectsNotIn(ctx context.Context, reportCardID string, allowedSubjectIDs []string) error {
	if len(allowedSubjectIDs) == 0 {
		_, err := s.DB.ExecContext(ctx, `DELETE FROM report_card_subjects WHERE report_card_id = $1`, reportCardID)
		return err
	}
	_, err := s.DB.ExecContext(ctx, `DELETE FROM report_card_subjects WHERE report_card_id = $1 AND NOT (subject_id::text = ANY($2))`,
		reportCardID, pq.Array(allowedSubjectIDs))
	return err
}

func (s *Store) ListSubjects(ctx context.Context, reportCardID string) ([]model.ReportCardSubject, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT id, report_card_id, school_id, subject_id, subject_name, subject_code, computed_score,
			formatif_score, sts_score, sas_score, praktik_score,
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
		var computedScore, formatifScore, stsScore, sasScore, praktikScore, overrideScore sql.NullFloat64
		var overriddenAt, lastSyncedAt sql.NullTime

		if err := rows.Scan(
			&sub.ID, &sub.ReportCardID, &sub.SchoolID, &sub.SubjectID, &subjectName, &subjectCode, &computedScore,
			&formatifScore, &stsScore, &sasScore, &praktikScore,
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
		if praktikScore.Valid {
			v := praktikScore.Float64
			sub.PraktikScore = &v
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
		if formatifScore.Valid {
			v := formatifScore.Float64
			sub.FormatifScore = &v
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
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// SEKOCI V10 item A: the rapor's mapel order is the curriculum's, not
	// the alphabetical one the ORDER BY above gives (that's what put
	// "Bahasa Indonesia" above "Pendidikan Agama"). Applied here, in the
	// one place every reader of a report card's subjects goes through.
	model.SortSubjectsCurriculum(out)
	return out, nil
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
