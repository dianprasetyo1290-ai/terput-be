-- 17_insight_extra.sql
-- Seed custom dashboards, widgets, saved reports, scheduled reports, risk predictions, and help articles

DO $$
DECLARE
    v_school_id UUID := '11111111-1111-1111-1111-111111111111';
    v_admin_id UUID := '22222222-2222-2222-2222-222222222222';
    v_student_id UUID := '44444444-4444-4444-4444-444444444444';
    v_dash_id UUID := '51000000-0000-0000-0000-000000000001';
    v_report_id UUID := '52000000-0000-0000-0000-000000000001';
    v_rule_arrears UUID := '53000000-0000-0000-0000-000000000001';
    v_run_arrears UUID := '54000000-0000-0000-0000-000000000001';
    v_pred_arrears UUID := '55000000-0000-0000-0000-000000000001';
    v_rule_absence UUID := '53000000-0000-0000-0000-000000000002';
    v_run_absence UUID := '54000000-0000-0000-0000-000000000002';
    v_pred_absence UUID := '55000000-0000-0000-0000-000000000002';
BEGIN
    -- 1. Custom Dashboard
    INSERT INTO dashboards (id, school_id, owner_user_id, title, scope_kind, shared_roles)
    VALUES (v_dash_id, v_school_id, v_admin_id, 'Executive Overview TPGS', 'unit', ARRAY['admin', 'kepala_sekolah', 'yayasan']::text[])
    ON CONFLICT (id) DO NOTHING;

    INSERT INTO dashboard_widgets (id, school_id, dashboard_id, sort, title, metric_key, chart, period)
    VALUES
        ('51100000-0000-0000-0000-000000000001', v_school_id, v_dash_id, 1, 'Tingkat Kolektibilitas SPP', 'collection_rate', 'stat', '{"kind": "current_month"}'),
        ('51100000-0000-0000-0000-000000000002', v_school_id, v_dash_id, 2, 'Tingkat Presensi Siswa', 'attendance_rate', 'line', '{"kind": "last_30_days"}'),
        ('51100000-0000-0000-0000-000000000003', v_school_id, v_dash_id, 3, 'Total Siswa Aktif', 'active_students', 'stat', '{"kind": "latest"}')
    ON CONFLICT (id) DO NOTHING;

    -- 2. Saved Reports
    INSERT INTO saved_reports (id, school_id, owner_user_id, title, columns, filters, shared_roles)
    VALUES (
        v_report_id, v_school_id, v_admin_id, 'Laporan Rekapitulasi Tagihan & Tunggakan SPP',
        '[{"name": "student_name", "label": "Nama Siswa"}, {"name": "classroom", "label": "Kelas"}, {"name": "amount", "label": "Nominal"}, {"name": "status", "label": "Status"}]'::jsonb,
        '{"status": "all"}'::jsonb,
        ARRAY['admin', 'bendahara', 'kepala_sekolah']::text[]
    ) ON CONFLICT (id) DO NOTHING;

    -- 3. Scheduled Reports
    INSERT INTO scheduled_reports (
        id, school_id, saved_report_id, frequency, send_hour, recipient_user_ids, active, next_run_at, created_by
    ) VALUES (
        '52100000-0000-0000-0000-000000000001', v_school_id, v_report_id, 'weekly', 8, ARRAY[v_admin_id]::uuid[],
        true, now() + interval '5 days', v_admin_id
    ) ON CONFLICT (id) DO NOTHING;

    -- 4. Risk Prediction - Arrears (Risiko Tunggakan)
    INSERT INTO risk_rules (id, school_id, model, version, params, effective_from, created_by, status)
    VALUES (v_rule_arrears, v_school_id, 'arrears_risk', 1, '{"threshold_days": 30, "min_amount": 500000}'::jsonb, CURRENT_DATE - INTERVAL '30 days', v_admin_id, 'active')
    ON CONFLICT (school_id, model, version) DO NOTHING;

    INSERT INTO prediction_runs (id, school_id, model, rule_id, run_at, status)
    VALUES (v_run_arrears, v_school_id, 'arrears_risk', v_rule_arrears, now() - interval '1 hour', 'ok')
    ON CONFLICT (id) DO NOTHING;

    INSERT INTO predictions (
        id, school_id, run_id, model, subject_kind, subject_id, score, level, projected_value, factors, review_label
    ) VALUES (
        v_pred_arrears, v_school_id, v_run_arrears, 'arrears_risk', 'student', v_student_id::text,
        35, 'low', 750000,
        '[{"factor": "Riwayat Pembayaran", "weight": 0.4, "note": "2 bulan awal lunas tepat waktu"}]'::jsonb,
        'perlu_tinjauan'
    ) ON CONFLICT (id) DO NOTHING;

    -- 5. Risk Prediction - Absence (Risiko Ketidakhadiran)
    INSERT INTO risk_rules (id, school_id, model, version, params, effective_from, created_by, status)
    VALUES (v_rule_absence, v_school_id, 'absence_risk', 1, '{"consecutive_days": 3, "min_attendance_pct": 80}'::jsonb, CURRENT_DATE - INTERVAL '30 days', v_admin_id, 'active')
    ON CONFLICT (school_id, model, version) DO NOTHING;

    INSERT INTO prediction_runs (id, school_id, model, rule_id, run_at, status)
    VALUES (v_run_absence, v_school_id, 'absence_risk', v_rule_absence, now() - interval '1 hour', 'ok')
    ON CONFLICT (id) DO NOTHING;

    INSERT INTO predictions (
        id, school_id, run_id, model, subject_kind, subject_id, score, level, projected_value, factors, review_label
    ) VALUES (
        v_pred_absence, v_school_id, v_run_absence, 'absence_risk', 'student', v_student_id::text,
        15, 'low', 0,
        '[{"factor": "Tingkat Kehadiran", "weight": 0.8, "note": "Kehadiran 98% konsisten"}]'::jsonb,
        'perlu_tinjauan'
    ) ON CONFLICT (id) DO NOTHING;

    -- 6. Help Articles (Panduan Asisten / Sistem)
    INSERT INTO help_articles (id, school_id, title, body_md, tags, route_name, status, updated_by)
    VALUES
        ('56000000-0000-0000-0000-000000000001', v_school_id,
         'Panduan Pengelolaan Tagihan SPP & Pembayaran Online',
         '# Panduan Billing SPP\n\n1. Buka menu **Keuangan & Billing > Skema SPP**\n2. Tetapkan nominal dan periode jatuh tempo bulanan.\n3. Cetak kuitansi atau unduh bukti pembayaran langsung dari dashboard siswa.',
         ARRAY['keuangan', 'spp', 'panduan']::text[], 'billing-list', 'published', v_admin_id),
        ('56000000-0000-0000-0000-000000000002', v_school_id,
         'Cara Menginput Nilai Rapor Kurikulum Merdeka',
         '# Panduan Penginputan E-Rapor\n\n1. Masuk sebagai **Guru Mapel** atau **Wali Kelas**.\n2. Buka menu **Operasional Akademik > E-Rapor**.\n3. Masukkan capaian kompetensi dan nilai sumatif siswa.',
         ARRAY['akademik', 'rapor', 'guru']::text[], 'rapor-input', 'published', v_admin_id)
    ON CONFLICT (id) DO NOTHING;

END $$;
