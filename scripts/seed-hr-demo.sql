-- Seed script for local development only: skci-teratai HR service demo data.
-- Run with: psql postgres://localhost/skci_t2_hr?sslmode=disable -f scripts/seed-hr-demo.sql

DO $$
DECLARE
    v_school_id UUID := '00000000-0000-0000-0000-000000000001';
    v_dept_id UUID := '00000000-0000-0000-0000-000000000010';
    v_pos_guru UUID := '00000000-0000-0000-0000-000000000020';
    v_pos_staf UUID := '00000000-0000-0000-0000-000000000021';
    v_emp_id UUID := '00000000-0000-0000-0000-000000000030';
    v_leave_type_id UUID := '00000000-0000-0000-0000-000000000040';
BEGIN
    INSERT INTO hr_settings (school_id, contract_reminder_days, document_reminder_days, geolocation_enabled, timezone)
    VALUES (v_school_id, 30, 30, false, 'Asia/Jakarta')
    ON CONFLICT (school_id) DO NOTHING;

    INSERT INTO departments (id, school_id, code, name, active)
    VALUES (v_dept_id, v_school_id, 'AKAD', 'Akademik', true)
    ON CONFLICT DO NOTHING;

    INSERT INTO positions (id, school_id, code, name, level, job_description, department_id, salary_min_idr, salary_max_idr)
    VALUES
        (v_pos_guru, v_school_id, 'GURU-MAT', 'Guru Matematika', 2, 'Pengajar mata pelajaran Matematika', v_dept_id, 4500000, 7500000),
        (v_pos_staf, v_school_id, 'STAF-TU', 'Staf Tata Usaha', 1, 'Administrasi operasional sekolah', v_dept_id, 3500000, 5500000)
    ON CONFLICT DO NOTHING;

    INSERT INTO headcount_plans (school_id, position_id, department_id, planned, effective_from)
    VALUES (v_school_id, v_pos_guru, v_dept_id, 3, '2026-01-01')
    ON CONFLICT DO NOTHING;

    INSERT INTO leave_types (id, school_id, code, name, has_quota, annual_quota_days, accrual, carry_over_max_days)
    VALUES (v_leave_type_id, v_school_id, 'CT', 'Cuti Tahunan', true, 12, 'none', 5)
    ON CONFLICT DO NOTHING;

    INSERT INTO document_types (school_id, code, name, has_expiry)
    VALUES
        (v_school_id, 'KTP', 'Kartu Tanda Penduduk', false),
        (v_school_id, 'IJAZAH', 'Ijazah Pendidikan Terakhir', false),
        (v_school_id, 'KONTRAK', 'Surat Perjanjian Kerja / Kontrak', true)
    ON CONFLICT DO NOTHING;

    INSERT INTO required_documents (school_id, employment_type, document_type_id)
    SELECT v_school_id, 'kontrak', id FROM document_types WHERE school_id = v_school_id AND code IN ('KTP', 'IJAZAH', 'KONTRAK')
    ON CONFLICT DO NOTHING;
END $$;
