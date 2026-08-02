-- A fresh session must start every student as "belum presensi" (not yet
-- marked), not implicitly "hadir" — see CheckIn/InitStudentAttendances,
-- which now inserts one row per roster student in this status at check-in
-- time, so "unmarked" is a real, queryable value instead of a row that
-- simply doesn't exist yet (and gets rendered as "hadir" by callers).
ALTER TABLE student_attendances DROP CONSTRAINT student_attendances_status_check;
ALTER TABLE student_attendances ALTER COLUMN status SET DEFAULT 'belum_presensi';
ALTER TABLE student_attendances ADD CONSTRAINT student_attendances_status_check
    CHECK (status IN ('belum_presensi', 'hadir', 'sakit', 'izin', 'tanpa_keterangan'));
