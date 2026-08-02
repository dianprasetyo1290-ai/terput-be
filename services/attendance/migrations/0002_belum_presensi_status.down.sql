ALTER TABLE student_attendances DROP CONSTRAINT student_attendances_status_check;
ALTER TABLE student_attendances ALTER COLUMN status SET DEFAULT 'hadir';
ALTER TABLE student_attendances ADD CONSTRAINT student_attendances_status_check
    CHECK (status IN ('hadir', 'sakit', 'izin', 'tanpa_keterangan'));
