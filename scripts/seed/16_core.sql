-- 16_core.sql
-- Seed approval flows, approval requests, service desk teams/categories/tickets, service requests, and audit events

DO $$
DECLARE
    v_school_id UUID := '11111111-1111-1111-1111-111111111111';
    v_admin_id UUID := '22222222-2222-2222-2222-222222222222';
    v_guru_id UUID := '77777777-7777-7777-7777-777777777777';
    v_wali_id UUID := '33333333-3333-3333-3333-333333333333';
    v_student_id UUID := '44444444-4444-4444-4444-444444444444';
    v_team_it UUID := '71000000-0000-0000-0000-000000000001';
    v_team_sarpras UUID := '71000000-0000-0000-0000-000000000002';
    v_cat_wifi UUID := '72000000-0000-0000-0000-000000000001';
    v_cat_ac UUID := '72000000-0000-0000-0000-000000000002';
    v_cat_sr UUID := '73000000-0000-0000-0000-000000000001';
    v_sr_id UUID := '74000000-0000-0000-0000-000000000001';
    v_flow_proc UUID := '75000000-0000-0000-0000-000000000001';
    v_flow_cuti UUID := '75000000-0000-0000-0000-000000000002';
BEGIN
    -- 1. Service Desk Teams
    INSERT INTO sd_teams (id, school_id, name, manager_user_id)
    VALUES
        (v_team_it, v_school_id, 'Tim IT Support & Jaringan', v_admin_id),
        (v_team_sarpras, v_school_id, 'Tim Sarana & Prasarana', v_admin_id)
    ON CONFLICT (id) DO NOTHING;

    INSERT INTO sd_team_members (team_id, user_id, school_id)
    VALUES
        (v_team_it, v_admin_id, v_school_id),
        (v_team_sarpras, v_admin_id, v_school_id)
    ON CONFLICT (team_id, user_id) DO NOTHING;

    -- 2. Service Desk Categories
    INSERT INTO sd_categories (
        id, school_id, name, description, team_id, is_facility, reporter_kinds, is_fallback, active
    ) VALUES
        (v_cat_wifi, v_school_id, 'Gangguan Jaringan & Akses Internet', 'Masalah koneksi Wi-Fi lab atau ruang kelas', v_team_it, false, ARRAY['pegawai','siswa']::text[], false, true),
        (v_cat_ac, v_school_id, 'Kerusakan AC & Kelistrikan', 'Laporan pendingin ruangan kurang dingin atau mati', v_team_sarpras, true, ARRAY['pegawai','orang_tua','siswa']::text[], true, true)
    ON CONFLICT (id) DO NOTHING;

    -- 3. Service Request Categories (Layanan Mandiri Siswa/Ortu)
    INSERT INTO service_request_categories (
        id, school_id, name, description, handler_role, sla_hours, requires_student, active
    ) VALUES
        (v_cat_sr, v_school_id, 'Permohonan Surat Keterangan Aktif Siswa', 'Penerbitan surat resmi keterangan siswa aktif untuk tunjangan atau beasiswa', 'admin', 24, true, true)
    ON CONFLICT (id) DO NOTHING;

    -- 4. Service Requests
    INSERT INTO service_requests (
        id, school_id, category_id, requester_user_id, student_user_id, subject, description, status, assignee_user_id
    ) VALUES
        (v_sr_id, v_school_id, v_cat_sr, v_wali_id, v_student_id, 'Surat Keterangan Aktif untuk Tunjangan Kantor', 'Mohon diterbitkan surat keterangan aktif belajar untuk keperluan verifikasi instansi.', 'in_progress', v_admin_id)
    ON CONFLICT (id) DO UPDATE SET
        status = 'in_progress';

    -- 5. Service Desk Tickets
    INSERT INTO sd_tickets (
        id, school_id, number, category_id, team_id, priority, subject, description, channel, reporter_user_id, status, assignee_user_id, idempotency_key
    ) VALUES
        ('76000000-0000-0000-0000-000000000001', v_school_id, 'TKT-2026-0001', v_cat_wifi, v_team_it, 'normal',
         'Access Point Ruang Kelas VII A Lambat', 'Kecepatan koneksi Wi-Fi di kelas VII A menurun saat jam ujian.', 'employee_portal', v_guru_id, 'in_progress', v_admin_id, 'tkt-idemp-001'),
        ('76000000-0000-0000-0000-000000000002', v_school_id, 'TKT-2026-0002', v_cat_ac, v_team_sarpras, 'normal',
         'Pembersihan Filter AC Lab Komputer 1', 'AC di Lab Komputer 1 berbunyi dan perlu dibersihkan berkala.', 'employee_portal', v_admin_id, 'assigned', v_admin_id, 'tkt-idemp-002')
    ON CONFLICT (id) DO NOTHING;

    -- 6. Approval Flows
    -- Additive only: never replace a flow the school already configured.
    
    INSERT INTO approval_flows (id, school_id, request_type, name, active, created_by)
    VALUES
        (v_flow_proc, v_school_id, 'procurement', 'Alur Persetujuan Pengadaan Barang Sekolah', true, v_admin_id),
        (v_flow_cuti, v_school_id, 'leave', 'Alur Persetujuan Cuti Pegawai', true, v_admin_id)
    ON CONFLICT DO NOTHING;

    INSERT INTO approval_flow_steps (school_id, flow_id, seq, approver_kind, approver_role, approver_user_id, sla_hours)
    SELECT v.* FROM (VALUES
        (v_school_id, v_flow_proc, 1, 'role', 'admin', null::uuid, 24),
        (v_school_id, v_flow_proc, 2, 'role', 'kepala_sekolah', null::uuid, 48),
        (v_school_id, v_flow_cuti, 1, 'role', 'admin', null::uuid, 24)
    ) AS v(school_id, flow_id, seq, approver_kind, approver_role, approver_user_id, sla_hours)
    WHERE EXISTS (SELECT 1 FROM approval_flows f WHERE f.id = v.flow_id)
    ON CONFLICT (flow_id, seq) DO NOTHING;

    -- 7. Approval Request (Pending in Inbox!)
    INSERT INTO approval_requests (
        id, school_id, request_type, source_service, source_id, title, summary, amount_idr,
        maker_user_id, status, current_seq, callback_url, correlation_id, idempotency_key, flow_snapshot
    ) VALUES (
        '77000000-0000-0000-0000-000000000001', v_school_id, 'procurement', 'ops', 'PO-2026-001',
        'Pengadaan 30 Unit Alat Peraga Matematika & IPA',
        jsonb_build_object('item', 'Alat Peraga Geometri & Mikroskop Siswa', 'qty', 30, 'vendor', 'PT Sarana Edukasi Mandiri'),
        4500000, v_admin_id, 'pending', 1, 'http://ops:8093/internal/approvals/callback',
        'corr-po-2026-001', 'idemp-po-2026-001',
        jsonb_build_object('flow_id', v_flow_proc, 'flow_name', 'Alur Persetujuan Pengadaan Barang Sekolah')
    ) ON CONFLICT (id) DO UPDATE SET
        status = 'pending';

    INSERT INTO approval_steps (id, school_id, request_id, seq, approver_kind, approver_role, status)
    SELECT '78000000-0000-0000-0000-000000000001', v_school_id, '77000000-0000-0000-0000-000000000001', 1, 'role', 'admin', 'pending'
    WHERE NOT EXISTS (SELECT 1 FROM approval_steps WHERE request_id = '77000000-0000-0000-0000-000000000001' AND seq = 1);

    -- 8. Audit Events
    INSERT INTO audit_events (
        school_id, service, module, entity_type, entity_id, action, actor_user_id, actor_role, reason, created_at
    ) VALUES
        (v_school_id, 'identity', 'auth', 'user', v_admin_id::text, 'login', v_admin_id, 'admin', 'Login berhasil via portal web', now() - interval '2 hours'),
        (v_school_id, 'ops', 'asset', 'fixed_asset', 'AST-LAB-001', 'create', v_admin_id, 'admin', 'Registrasi unit Mikroskop Binokuler Olympus', now() - interval '1 day'),
        (v_school_id, 'billing', 'invoice', 'student_bill', 'BILL-2026-07', 'paid', v_admin_id, 'finance', 'Verifikasi bukti transfer SPP Juli 2026', now() - interval '1 day'),
        (v_school_id, 'academic', 'schedule', 'classroom_subject', 'c3000000-0000-0000-0000-000000000001', 'assign', v_admin_id, 'admin', 'Penetapan guru pengampu mata pelajaran', now() - interval '3 days')
    ON CONFLICT DO NOTHING;

END $$;
