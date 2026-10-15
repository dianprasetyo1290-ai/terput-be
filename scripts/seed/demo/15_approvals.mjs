// Phase 15: the Approval Center of SD and SMK — every approval flow of SMP
// (same request types, steps, SLAs and amount thresholds) so a request raised
// in either unit is routed, the principal's and finance manager's
// delegations, and the requests waiting on work the earlier generators left
// pending: the employees' leave requests (06_hr) and the hall booking for the
// school committee meeting (11_facilities), each linked back to its source.
// Approvals of the other modules are raised by their own generators.
import { ACCOUNTS, approvals, at, dbRows, insert, merge, uid } from './lib.mjs'
import { MODEL } from './model.mjs'

const SMP = '11111111-1111-1111-1111-111111111111'
const { admin: ADMIN, kepala_sekolah: KEPSEK, finance_manager: FINANCE } = ACCOUNTS

export function generate() {
  const ap = approvals()
  const core = []
  const hr = []
  const ops = []
  const flows = dbRows('teratai_core', `SELECT id, request_type, name FROM approval_flows WHERE school_id = '${SMP}' AND deleted_at IS NULL AND active`)
  const steps = dbRows('teratai_core', `SELECT flow_id, seq, approver_kind, approver_role, approver_user_id, min_amount_idr, condition_flag, sla_hours, escalate_to_role
    FROM approval_flow_steps WHERE school_id = '${SMP}'`)
  const units = MODEL.filter((x) => x.schoolId !== SMP)
  const leaves = dbRows('teratai_hr', `SELECT l.id, l.school_id, l.employee_id, l.start_date, l.end_date, l.days, l.reason, l.created_at, e.full_name, e.user_id, t.name AS type
    FROM leave_requests l JOIN employees e ON e.id = l.employee_id JOIN leave_types t ON t.id = l.leave_type_id
    WHERE l.status = 'pending' AND l.approval_id IS NULL AND l.school_id IN (${units.map((u) => `'${u.schoolId}'`).join(',')})`)
  const bookings = dbRows('teratai_ops', `SELECT b.id, b.school_id, b.purpose, b.starts_at, b.participants, r.name AS room FROM room_bookings b JOIN rooms r ON r.id = b.room_id
    WHERE b.status = 'pending_approval' AND b.approval_id IS NULL AND b.school_id IN (${units.map((u) => `'${u.schoolId}'`).join(',')})`)

  for (const u of units) {
    const S = u.schoolId
    const id = (...k) => uid('approval-cfg', u.key, ...k)
    core.push(`-- ===== ${u.level} =====`)
    // Flows: one per request type, matched on SMP's (unique per type there too).
    const flowId = Object.fromEntries(flows.map((f) => [f.id, id('flow', f.request_type)]))
    core.push(insert('approval_flows', flows.map((f) => ({ id: flowId[f.id], school_id: S, request_type: f.request_type, name: f.name, active: true, created_by: ADMIN,
      created_at: at('2026-07-01') })), { conflict: '(id) DO NOTHING' }))
    core.push(insert('approval_flow_steps', steps.filter((s) => flowId[s.flow_id]).map((s) => ({ id: id('flow-step', s.flow_id, s.seq), school_id: S, flow_id: flowId[s.flow_id],
      seq: s.seq, approver_kind: s.approver_kind, approver_role: s.approver_role, approver_user_id: s.approver_user_id, min_amount_idr: s.min_amount_idr,
      condition_flag: s.condition_flag, sla_hours: s.sla_hours, escalate_to_role: s.escalate_to_role, created_at: at('2026-07-01') })), { conflict: '(flow_id, seq) DO NOTHING' }))
    core.push(insert('approval_delegations', [
      // The principal attends the foundation's principals' meeting: the
      // homeroom teacher of the first class decides in the meantime.
      { id: id('delegation', 'rakor'), school_id: S, from_user_id: KEPSEK, to_user_id: u.classes[0].homeroom.id, valid_from: at('2026-10-20', '00:00'),
        valid_to: at('2026-10-23', '23:59'), created_at: at('2026-10-14', '15:00') },
      { id: id('delegation', 'cuti-fm'), school_id: S, from_user_id: FINANCE, to_user_id: ACCOUNTS.kepala_yayasan, valid_from: at('2026-09-07', '00:00'),
        valid_to: at('2026-09-11', '23:59'), created_at: at('2026-09-04', '10:00') },
      { id: id('delegation', 'umroh'), school_id: S, from_user_id: KEPSEK, to_user_id: ADMIN, valid_from: at('2026-08-03', '00:00'), valid_to: at('2026-08-14', '23:59'),
        created_at: at('2026-07-28', '09:00'), revoked_at: at('2026-08-10', '07:00') }]))

    // Leave requests of the unit's employees.
    for (const l of leaves.filter((x) => x.school_id === S)) {
      const appId = ap.add({ school: S, key: `leave:${l.id}`, type: 'leave', service: 'hr', sourceId: l.id, title: `${l.type}: ${l.full_name} (${l.days} hari)`,
        summary: { employee: l.full_name, leave_type: l.type, start_date: l.start_date, end_date: l.end_date, days: Number(l.days), reason: l.reason },
        maker: l.user_id ?? ADMIN, subject: l.user_id, steps: [['admin', 24]], createdAt: at('2026-10-13', '10:00') })
      hr.push(`UPDATE leave_requests SET approval_id = '${appId}' WHERE id = '${l.id}' AND approval_id IS NULL;`)
    }
    // Hall bookings that need the facility officer's approval.
    for (const b of bookings.filter((x) => x.school_id === S)) {
      const appId = ap.add({ school: S, key: `booking:${b.id}`, type: 'room_booking', service: 'ops', sourceId: b.id, title: `Peminjaman ${b.room}: ${b.purpose}`,
        summary: { room: b.room, purpose: b.purpose, starts_at: b.starts_at, participants: Number(b.participants) }, maker: ADMIN, steps: [['facility_officer', 24]],
        createdAt: at('2026-10-12', '13:00') })
      ops.push(`UPDATE room_bookings SET approval_id = '${appId}' WHERE id = '${b.id}' AND approval_id IS NULL;`)
    }
  }
  return merge({ teratai_core: core.join('\n\n'), teratai_hr: hr.join('\n'), teratai_ops: ops.join('\n') }, ap.sql())
}
