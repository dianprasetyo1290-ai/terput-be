-- T9: semua peran baru PRD Bagian 3–4 (operasional sekolah) sekaligus.
-- Satu-satunya migrasi yang memperlebar CHECK peran untuk Bagian 3–4;
-- task lain memakai nama peran persis dari tabel T9 §2.
ALTER TABLE user_school_roles DROP CONSTRAINT IF EXISTS user_school_roles_role_check;
ALTER TABLE user_school_roles ADD CONSTRAINT user_school_roles_role_check
    CHECK (role IN (
        'superadmin', 'admin', 'guru', 'siswa', 'orang_tua', 'walikelas', 'kepala_sekolah', 'kepala_yayasan', 'pengawas',
        'hr_admin', 'hr_manager', 'payroll_officer', 'recruiter', 'training_admin', 'auditor', 'finance_manager', 'pegawai',
        'inventory_officer', 'procurement_officer', 'asset_officer', 'pustakawan', 'facility_officer', 'teknisi',
        'transport_officer', 'pendamping_transport', 'pengemudi', 'canteen_admin', 'canteen_operator', 'petugas_uks',
        'konselor', 'kesiswaan', 'event_coordinator', 'pembina_ekskul', 'servicedesk_agent', 'servicedesk_manager',
        'koordinator_pkl', 'pembimbing_pkl', 'pembimbing_industri', 'qa_coordinator', 'operator_dapodik', 'humas', 'bi_analyst'
    ));
