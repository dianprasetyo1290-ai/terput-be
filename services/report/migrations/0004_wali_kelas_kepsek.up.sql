-- Wali Kelas (homeroom teacher) and Kepala Sekolah (principal) names, same
-- header-snapshot pattern as student_name/classroom_name/term_name: pulled
-- from academic-service (classrooms.homeroom_teacher_user_id) and
-- identity-service (Organisasi Sekolah position holder) at generate time,
-- cached here for display resiliency.
ALTER TABLE report_cards ADD COLUMN homeroom_teacher_name TEXT;
ALTER TABLE report_cards ADD COLUMN principal_name TEXT;
