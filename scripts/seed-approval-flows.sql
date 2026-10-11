-- Sample Approval Flows Seed (SEKOCI LMS Teratai Putih)
-- Demonstrates:
-- 1. leave -> direct_supervisor (single step)
-- 2. refund -> step 1: admin, step 2: kepala_sekolah if >= 5.000.000 (condition threshold)

-- Replace '5006c011-b21e-42ad-9fb5-c07de8d99f74' with your target school_id if testing a specific school.
DO $$
DECLARE
    v_school_id UUID := '5006c011-b21e-42ad-9fb5-c07de8d99f74';
    v_admin_id UUID := '00000000-0000-0000-0000-000000000001';
    v_leave_flow_id UUID;
    v_refund_flow_id UUID;
BEGIN
    -- 1. Alur Pengajuan Cuti (leave)
    DELETE FROM approval_flows WHERE school_id = v_school_id AND request_type = 'leave';
    INSERT INTO approval_flows (school_id, request_type, name, active, created_by)
    VALUES (v_school_id, 'leave', 'Alur Persetujuan Cuti Pegawai', true, v_admin_id)
    RETURNING id INTO v_leave_flow_id;

    -- Step 1: Atasan Langsung
    INSERT INTO approval_flow_steps (school_id, flow_id, seq, approver_kind, approver_role, approver_user_id, sla_hours, escalate_to_role)
    VALUES (v_school_id, v_leave_flow_id, 1, 'direct_supervisor', null, null, 48, 'hr_admin');

    -- 2. Alur Refund Tagihan (refund)
    DELETE FROM approval_flows WHERE school_id = v_school_id AND request_type = 'refund';
    INSERT INTO approval_flows (school_id, request_type, name, active, created_by)
    VALUES (v_school_id, 'refund', 'Alur Persetujuan Pengembalian Dana (Refund)', true, v_admin_id)
    RETURNING id INTO v_refund_flow_id;

    -- Step 1: Admin
    INSERT INTO approval_flow_steps (school_id, flow_id, seq, approver_kind, approver_role, approver_user_id, min_amount_idr, sla_hours, escalate_to_role)
    VALUES (v_school_id, v_refund_flow_id, 1, 'role', 'admin', null, null, 24, 'finance_manager');

    -- Step 2: Kepala Sekolah jika >= 5.000.000 IDR
    INSERT INTO approval_flow_steps (school_id, flow_id, seq, approver_kind, approver_role, approver_user_id, min_amount_idr, sla_hours, escalate_to_role)
    VALUES (v_school_id, v_refund_flow_id, 2, 'role', 'kepala_sekolah', null, 5000000, 48, 'superadmin');

END $$;
