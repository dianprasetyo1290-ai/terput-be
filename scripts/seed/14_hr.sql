-- 14_hr.sql
-- Seed HR, departments, positions, employees, attendance, leave, training, and recruitment

DO $$
DECLARE
    v_school_id UUID := '11111111-1111-1111-1111-111111111111';
    v_admin_id UUID := '22222222-2222-2222-2222-222222222222';
    v_guru_user_id UUID := '77777777-7777-7777-7777-777777777777';
    v_pegawai_user_id UUID := '54277dd2-37e1-43f6-a3c9-d4b754e58676';
    v_dept_akad UUID := '91000000-0000-0000-0000-000000000001';
    v_dept_tu UUID := '91000000-0000-0000-0000-000000000002';
    v_pos_guru UUID := '92000000-0000-0000-0000-000000000001';
    v_pos_tu UUID := '92000000-0000-0000-0000-000000000002';
    v_emp_guru UUID := '93000000-0000-0000-0000-000000000001';
    v_emp_pegawai UUID := '93000000-0000-0000-0000-000000000002';
    v_leave_type_id UUID := '94000000-0000-0000-0000-000000000001';
    v_train_cat UUID := '95000000-0000-0000-0000-000000000001';
    v_req_id UUID := '96000000-0000-0000-0000-000000000001';
BEGIN
    -- 1. HR Settings
    INSERT INTO hr_settings (school_id, contract_reminder_days, document_reminder_days, geolocation_enabled, timezone)
    VALUES (v_school_id, 30, 30, false, 'Asia/Jakarta')
    ON CONFLICT (school_id) DO NOTHING;

    -- 2. Departments
    INSERT INTO departments (id, school_id, code, name, active)
    VALUES
        (v_dept_akad, v_school_id, 'AKAD', 'Kurikulum & Pengajaran', true),
        (v_dept_tu, v_school_id, 'TU', 'Tata Usaha & Administrasi', true)
    ON CONFLICT (school_id, code) WHERE deleted_at IS NULL DO NOTHING;

    -- 3. Positions
    INSERT INTO positions (id, school_id, code, name, level, job_description, department_id, salary_min_idr, salary_max_idr)
    VALUES
        (v_pos_guru, v_school_id, 'GURU-MTK', 'Guru Matematika', 2, 'Pengajar mata pelajaran Matematika', v_dept_akad, 5000000, 8000000),
        (v_pos_tu, v_school_id, 'STAF-TU', 'Staf Administrasi TU', 1, 'Pengelolaan berkas dan surat menyurat', v_dept_tu, 4000000, 6000000)
    ON CONFLICT (school_id, code) DO NOTHING;

    -- 4. Headcount Plans
    INSERT INTO headcount_plans (school_id, position_id, department_id, planned, effective_from)
    VALUES (v_school_id, v_pos_guru, v_dept_akad, 5, '2026-01-01')
    ON CONFLICT DO NOTHING;

    -- 5. Employees
    INSERT INTO employees (
        id, school_id, user_id, employee_no, full_name, email, ptkp_status, hire_date, status
    ) VALUES
        (v_emp_guru, v_school_id, v_guru_user_id, 'EMP-2026-001', 'Guru Teratai', 'guru@teratai.sch.id', 'K/1', '2024-07-01', 'active'),
        (v_emp_pegawai, v_school_id, v_pegawai_user_id, 'EMP-2026-002', 'Staf Pegawai Teratai', 'pegawai@teratai.sch.id', 'TK/0', '2025-01-10', 'active')
    ON CONFLICT (school_id, employee_no) DO NOTHING;

    -- 6. Employment Assignments
    INSERT INTO employment_assignments (
        id, school_id, employee_id, position_id, department_id, employment_type, grade, effective_from
    ) VALUES
        ('93100000-0000-0000-0000-000000000001', v_school_id, v_emp_guru, v_pos_guru, v_dept_akad, 'tetap', 'III/a', '2024-07-01'),
        ('93100000-0000-0000-0000-000000000002', v_school_id, v_emp_pegawai, v_pos_tu, v_dept_tu, 'kontrak', 'II/c', '2025-01-10')
    ON CONFLICT DO NOTHING;

    -- 7. Leave Types
    INSERT INTO leave_types (id, school_id, code, name, has_quota, annual_quota_days, accrual, carry_over_max_days)
    VALUES
        (v_leave_type_id, v_school_id, 'CT', 'Cuti Tahunan', true, 12, 'none', 5),
        ('94000000-0000-0000-0000-000000000002', v_school_id, 'CS', 'Cuti Sakit', false, 0, 'none', 0)
    ON CONFLICT DO NOTHING;

    -- 8. Leave Requests
    INSERT INTO leave_requests (
        id, school_id, employee_id, leave_type_id, start_date, end_date, days, reason, status, idempotency_key
    ) VALUES
        ('94100000-0000-0000-0000-000000000001', v_school_id, v_emp_guru, v_leave_type_id, CURRENT_DATE + INTERVAL '10 days', CURRENT_DATE + INTERVAL '11 days', 2.0, 'Keperluan keluarga penting', 'approved', 'leave-req-001')
    ON CONFLICT (school_id, idempotency_key) DO NOTHING;

    -- 9. Employee Attendance
    INSERT INTO employee_attendance (
        id, school_id, employee_id, work_date, status, check_in_at, check_out_at, source, note
    ) VALUES
        ('94200000-0000-0000-0000-000000000001', v_school_id, v_emp_guru, CURRENT_DATE - INTERVAL '1 day', 'hadir', (CURRENT_DATE - INTERVAL '1 day') + TIME '06:55:00', (CURRENT_DATE - INTERVAL '1 day') + TIME '15:30:00', 'ess', 'Hadir tepat waktu'),
        ('94200000-0000-0000-0000-000000000002', v_school_id, v_emp_guru, CURRENT_DATE, 'hadir', CURRENT_DATE + TIME '06:58:00', NULL, 'ess', 'Check-in pagi hari'),
        ('94200000-0000-0000-0000-000000000003', v_school_id, v_emp_pegawai, CURRENT_DATE - INTERVAL '1 day', 'hadir', (CURRENT_DATE - INTERVAL '1 day') + TIME '07:10:00', (CURRENT_DATE - INTERVAL '1 day') + TIME '16:00:00', 'ess', 'Hadir tepat waktu'),
        ('94200000-0000-0000-0000-000000000004', v_school_id, v_emp_pegawai, CURRENT_DATE, 'hadir', CURRENT_DATE + TIME '07:05:00', NULL, 'ess', 'Check-in kantor')
    ON CONFLICT (employee_id, work_date) DO NOTHING;

    -- 10. Training Categories & Courses
    INSERT INTO training_categories (id, school_id, name)
    VALUES (v_train_cat, v_school_id, 'Pengembangan Pedagogik & Karakter')
    ON CONFLICT (id) DO NOTHING;

    INSERT INTO courses (
        id, school_id, title, objective, duration_minutes, provider, kind, category_id, cost_idr, passing_score, status
    ) VALUES (
        '95100000-0000-0000-0000-000000000001', v_school_id, 'Workshop Implementasi Kurikulum Merdeka Terpadu',
        'Meningkatkan kompetensi guru dalam merancang modul ajar berdiferensiasi.', 240, 'Kemendikbudristek & Teratai', 'offline', v_train_cat, 0, 80, 'published'
    ) ON CONFLICT (id) DO NOTHING;

    -- 11. Recruitment (Requisitions & Job Postings)
    INSERT INTO requisitions (
        id, school_id, position_id, department_id, quantity, reason, qualifications, status, requested_by, idempotency_key
    ) VALUES (
        v_req_id, v_school_id, v_pos_guru, v_dept_akad, 1, 'addition', 'S1 Pendidikan Matematika, IPK min 3.25, menguasai IT', 'approved', v_admin_id, 'req-guru-mtk-2026'
    ) ON CONFLICT (school_id, idempotency_key) DO NOTHING;

    INSERT INTO job_postings (
        id, school_id, requisition_id, slug, title, description, qualifications, employment_type, closes_on, status, published_at
    ) VALUES (
        '96100000-0000-0000-0000-000000000001', v_school_id, v_req_id, 'guru-matematika-smp-2026',
        'Lowongan Guru Matematika SMP Teratai Putih', 'Dibutuhkan pendidik berdedikasi tinggi untuk mengajar matematika di tingkat SMP.',
        'S1 Pendidikan Matematika dari PTN/PTS terakreditasi A, mampu berbahasa Inggris aktif.', 'full_time', CURRENT_DATE + INTERVAL '45 days', 'open', now()
    ) ON CONFLICT (school_id, slug) DO NOTHING;

END $$;
