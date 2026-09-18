-- SEKOCI V11: role baru "Pengawas" untuk panitia ujian.
--
-- Hak aksesnya hanya 3 hal (lihat services/exam/internal/handler): atur
-- jadwal ujian, pantau ujian + larang/blokir peserta, dan lihat bank soal
-- (read-only). Role ini TIDAK boleh mengelola user, nilai, atau pengaturan
-- sistem -- karena itu ia tidak diberi akses ke identity/academic/report
-- selain login.
ALTER TABLE user_school_roles DROP CONSTRAINT IF EXISTS user_school_roles_role_check;
ALTER TABLE user_school_roles ADD CONSTRAINT user_school_roles_role_check
    CHECK (role IN ('superadmin','admin','guru','siswa','walikelas','kepala_sekolah','kepala_yayasan','pengawas'));
