// Phase 12: access the demo accounts need to open their screens — the
// internal auditor's active audit access period in core (run-all-seeds.sh
// grants it on the server with a shell step; locally it was missing, so the
// audit screens answered 403).
import { dbRows, insert, uid } from './lib.mjs'

const SMP = '11111111-1111-1111-1111-111111111111'

export function generate() {
  const auditor = dbRows('teratai_identity', `SELECT id FROM users WHERE email = 'auditor@teratai.sch.id'`)[0]?.id
  if (!auditor) return {}
  return {
    teratai_core: insert('audit_access_grants', [{ id: uid('audit-grant', auditor, '2026-ganjil'), school_id: SMP, user_id: auditor,
      valid_from: '2026-10-01T00:00:00+07:00', valid_to: '2027-03-31T23:59:59+07:00', reason: 'Masa akses akun UAT auditor (data demo).',
      granted_by: '22222222-2222-2222-2222-222222222222' }]),
  }
}
