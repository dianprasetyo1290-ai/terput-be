// Phase 6: HR & payroll for the three units — an employee record for every
// teacher and staff account (linked to its identity user, sensitive fields
// encrypted exactly like services/hr), positions, assignments with the base
// salary, allowances, attendance July-October, leave balances and requests,
// and payroll: July and August final (payslips published), September
// approved, October draft. Also relinks the two legacy employees whose
// account link was wrong (the demo teacher pointed at the principal).
import { addDays, date, dbRows, envLocal, fieldCrypto, insert, person, phone, address, q, rng, uid } from './lib.mjs'
import { MODEL } from './model.mjs'

const SMP = '11111111-1111-1111-1111-111111111111'
const ADMIN = '22222222-2222-2222-2222-222222222222'
const crypto = fieldCrypto(envLocal('HR_ENC_KEY'), envLocal('HR_BLIND_KEY'))

const DEPARTMENTS = [['AKAD', 'Kurikulum & Pengajaran', null], ['TU', 'Tata Usaha & Administrasi', null], ['UMUM', 'Sarana Prasarana & Umum', 'TU'],
  ['BK', 'Bimbingan & Konseling', 'AKAD']]
// code, name, level, department, salary min, max
const POSITIONS = [
  ['KEPSEK', 'Kepala Sekolah', 4, 'AKAD', 9000000, 13000000], ['WAKA', 'Wakil Kepala Sekolah', 3, 'AKAD', 7500000, 10000000],
  ['GURU', 'Guru Mata Pelajaran', 2, 'AKAD', 4500000, 8500000], ['GURU-BK', 'Guru Bimbingan Konseling', 2, 'BK', 4500000, 7500000],
  ['STAF-TU', 'Staf Administrasi TU', 1, 'TU', 4000000, 6000000], ['STAF-KEU', 'Staf Keuangan', 1, 'TU', 4000000, 6500000],
  ['KABAG', 'Kepala Bagian', 3, 'TU', 7000000, 9500000], ['STAF-SDM', 'Staf SDM', 1, 'TU', 4000000, 6500000],
  ['STAF-IT', 'Staf Teknologi Informasi', 1, 'TU', 4500000, 7000000], ['PUSTAKAWAN', 'Pustakawan', 1, 'AKAD', 3800000, 5500000],
  ['STAF-UMUM', 'Petugas Keamanan & Umum', 1, 'UMUM', 3500000, 5000000], ['TEKNISI', 'Teknisi Sarana', 1, 'UMUM', 3800000, 5500000],
  ['PETUGAS-UKS', 'Petugas UKS', 1, 'BK', 3800000, 5500000],
]
const ROLE_POSITION = {
  kepala_sekolah: 'KEPSEK', kesiswaan: 'WAKA', qa_coordinator: 'WAKA', konselor: 'GURU-BK', admin: 'STAF-TU', pegawai: 'STAF-TU', operator_dapodik: 'STAF-TU',
  humas: 'STAF-TU', event_coordinator: 'STAF-TU', finance_manager: 'KABAG', hr_manager: 'KABAG', akuntan: 'STAF-KEU', auditor: 'STAF-KEU',
  hr_admin: 'STAF-SDM', payroll_officer: 'STAF-SDM', recruiter: 'STAF-SDM', training_admin: 'STAF-SDM', servicedesk_agent: 'STAF-IT',
  servicedesk_manager: 'STAF-IT', bi_analyst: 'STAF-IT', pustakawan: 'PUSTAKAWAN', inventory_officer: 'STAF-UMUM', procurement_officer: 'STAF-UMUM',
  asset_officer: 'STAF-UMUM', transport_officer: 'STAF-UMUM', pengemudi: 'STAF-UMUM', pendamping_transport: 'STAF-UMUM', canteen_admin: 'STAF-UMUM',
  canteen_operator: 'STAF-UMUM', facility_officer: 'TEKNISI', teknisi: 'TEKNISI', petugas_uks: 'PETUGAS-UKS', pembina_ekskul: 'GURU',
  koordinator_pkl: 'GURU', pembimbing_pkl: 'GURU', pengawas: 'GURU', guru: 'GURU', walikelas: 'GURU',
}
const PTKP = ['TK/0', 'TK/0', 'TK/1', 'K/0', 'K/1', 'K/1', 'K/2', 'K/3']
const TER_CAT = { 'TK/0': 'A', 'TK/1': 'A', 'K/0': 'A', 'TK/2': 'B', 'TK/3': 'B', 'K/1': 'B', 'K/2': 'B', 'K/3': 'C' }
const BANKS = ['002', '008', '009', '014', '451']
const PERIODS = [['2026-07', 'final'], ['2026-08', 'final'], ['2026-09', 'approved'], ['2026-10', 'draft']]
const HOLIDAYS = new Set(['2026-08-17', '2026-08-25'])

export function generate() {
  const out = []
  const ter = dbRows('teratai_hr', `SELECT params_snapshot->'params'->'ter' t FROM payroll_runs WHERE params_snapshot ? 'params' LIMIT 1`)[0]?.t
  const smpStaff = dbRows('teratai_identity', `SELECT u.id, u.name, u.email, array_agg(r.role) roles FROM users u JOIN user_school_roles r ON r.user_id = u.id
    WHERE r.school_id = '${SMP}' AND r.status = 'active' AND u.email LIKE '%@teratai.sch.id' AND u.email NOT LIKE '%@%.teratai.sch.id'
      AND NOT (r.role = ANY(ARRAY['siswa','orang_tua','kepala_yayasan','pembimbing_industri'])) GROUP BY u.id, u.name, u.email`)
  // Accounts that already have a legacy (non-generated) employee record are
  // skipped; generated ones are rebuilt identically and skipped by ON CONFLICT.
  const linked = new Set(dbRows('teratai_hr', `SELECT user_id FROM employees WHERE user_id IS NOT NULL AND id::text !~ '^[0-9a-f]{8}-[0-9a-f]{4}-5'`).map((x) => x.user_id))
  // The legacy demo teacher / staff employees get their right accounts.
  out.push(`UPDATE employees SET user_id = NULL WHERE employee_no IN ('EMP-2026-001','EMP-2026-002') AND school_id = '${SMP}'
  AND user_id IS DISTINCT FROM CASE employee_no WHEN 'EMP-2026-001' THEN '55555555-5555-5555-5555-555555555555'::uuid ELSE '720a589f-e673-4ca2-8afa-51ed555ebcbb'::uuid END;
UPDATE employees SET user_id = '55555555-5555-5555-5555-555555555555' WHERE employee_no = 'EMP-2026-001' AND school_id = '${SMP}' AND user_id IS NULL;
UPDATE employees SET user_id = '720a589f-e673-4ca2-8afa-51ed555ebcbb' WHERE employee_no = 'EMP-2026-002' AND school_id = '${SMP}' AND user_id IS NULL;`)
  linked.delete('77777777-7777-7777-7777-777777777777')
  linked.add('55555555-5555-5555-5555-555555555555').add('720a589f-e673-4ca2-8afa-51ed555ebcbb')

  for (const u of MODEL) {
    const r = rng('hr:' + u.key)
    const S = u.schoolId
    out.push(`-- ===== ${u.level} =====`)

    // ---- master data (SMP keeps its ids, SD/SMK get the same set) ----
    const existingDept = S === SMP ? Object.fromEntries(dbRows('teratai_hr', `SELECT code, id FROM departments WHERE school_id = '${S}'`).map((x) => [x.code, x.id])) : {}
    const dept = Object.fromEntries(DEPARTMENTS.map(([c]) => [c, existingDept[c] ?? uid('hr-dept', u.key, c)]))
    out.push(insert('departments', DEPARTMENTS.filter(([c]) => !existingDept[c]).map(([c, name, parent]) => ({ id: dept[c], school_id: S, code: c, name,
      parent_id: parent ? dept[parent] : null, active: true }))))
    const existingPos = S === SMP ? Object.fromEntries(dbRows('teratai_hr', `SELECT code, id FROM positions WHERE school_id = '${S}'`).map((x) => [x.code, x.id])) : {}
    const pos = Object.fromEntries(POSITIONS.map(([c]) => [c, existingPos[c] ?? uid('hr-pos', u.key, c)]))
    out.push(insert('positions', POSITIONS.filter(([c]) => !existingPos[c]).map(([c, name, level, d, min, max]) => ({ id: pos[c], school_id: S, code: c, name,
      level, department_id: dept[d], salary_min_idr: min, salary_max_idr: max })), { conflict: '(school_id, code) DO NOTHING' }))

    const smpComps = dbRows('teratai_hr', `SELECT * FROM pay_components WHERE school_id = '${SMP}' AND deleted_at IS NULL`)
    const comps = S === SMP ? smpComps : smpComps.map((c) => ({ ...c, id: uid('hr-comp', u.key, c.code), school_id: S }))
    if (S !== SMP) out.push(insert('pay_components', comps.map(({ created_at, updated_at, approval_id, deleted_at, ...c }) => c)))
    const comp = Object.fromEntries(comps.map((c) => [c.code, c]))
    const smpLeave = dbRows('teratai_hr', `SELECT * FROM leave_types WHERE school_id = '${SMP}' AND deleted_at IS NULL`)
    const leaveTypes = S === SMP ? smpLeave : smpLeave.map((l) => ({ ...l, id: uid('hr-leave-type', u.key, l.code), school_id: S }))
    if (S !== SMP) out.push(insert('leave_types', leaveTypes.map(({ created_at, updated_at, deleted_at, ...l }) => l)))
    const lt = Object.fromEntries(leaveTypes.map((l) => [l.code, l.id]))
    if (S !== SMP) {
      for (const t of ['hr_settings', 'work_schedules', 'holidays', 'payroll_calendars', 'attendance_periods', 'payroll_accounts', 'pay_component_accounts', 'document_types', 'required_documents']) {
        out.push(`DO $$ DECLARE cols text; vals text; BEGIN
  IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema = 'public' AND table_name = '${t}' AND column_name = 'school_id') THEN RETURN; END IF;
  SELECT string_agg(quote_ident(column_name), ', ' ORDER BY ordinal_position),
         string_agg(CASE column_name WHEN 'school_id' THEN quote_literal('${S}') || '::uuid' WHEN 'id' THEN 'gen_random_uuid()'
           WHEN 'position_id' THEN 'NULL' WHEN 'department_id' THEN 'NULL' ELSE quote_ident(column_name) END, ', ' ORDER BY ordinal_position)
    INTO cols, vals FROM information_schema.columns WHERE table_schema = 'public' AND table_name = '${t}';
  IF NOT EXISTS (SELECT 1 FROM ${t} WHERE school_id = '${S}') THEN
    EXECUTE format('INSERT INTO ${t} (%s) SELECT %s FROM ${t} WHERE school_id = %L ON CONFLICT DO NOTHING', cols, vals, '${SMP}');
  END IF;
END $$;`)
      }
    }

    // ---- people ----
    const homeroomIds = new Set(u.classes.map((c) => c.homeroom.id))
    const people = u.teachers.map((t) => ({ userId: t.id, name: t.name, email: t.email ?? null, gender: t.gender, position: 'GURU', roles: ['guru'], birth: t.birthDate, nik: t.nik }))
    if (S === SMP) {
      for (const s of smpStaff) {
        if (people.some((p) => p.userId === s.id)) continue
        const role = s.roles.find((x) => ROLE_POSITION[x] && x !== 'guru') ?? s.roles[0]
        people.push({ userId: s.id, name: s.name, email: s.email, position: ROLE_POSITION[role] ?? 'STAF-TU', roles: s.roles })
      }
    } else {
      // Staff of SD / SMK without a login.
      for (const [i, p] of ['STAF-TU', 'STAF-KEU', 'PUSTAKAWAN', 'STAF-UMUM', 'STAF-UMUM'].entries()) {
        const pp = person(r)
        people.push({ userId: null, key: `${u.key}-staff-${i}`, name: pp.name, gender: pp.gender, position: p, roles: [] })
      }
    }

    const employees = []
    const assignments = []
    const empComps = []
    const attendance = []
    const balances = []
    const leaves = []
    const payrollRows = [] // { emp, base, items }
    let seq = 0
    for (const p of people) {
      if (p.userId && linked.has(p.userId)) continue
      // One generator per person: skipping someone never shifts the others.
      const r = rng('hr-person:' + (p.userId ?? p.key))
      const empId = uid('hr-emp', p.userId ?? p.key)
      const posDef = POSITIONS.find(([c]) => c === p.position)
      const tenure = r.int(0, 14)
      const hire = `${2026 - tenure}-${tenure === 0 ? '07' : String(r.pick([1, 7, 7, 8])).padStart(2, '0')}-01`
      const type = p.position === 'KEPSEK' || tenure >= 4 ? 'tetap' : tenure >= 1 ? 'kontrak' : r.chance(0.5) ? 'kontrak' : 'honorer'
      const base = Math.round((posDef[4] + (posDef[5] - posDef[4]) * Math.min(1, tenure / 14) + r.int(-2, 2) * 100000) / 50000) * 50000
      const ptkp = r.pick(PTKP)
      const nik = p.nik ?? `3275${String(r.int(10000000, 99999999))}${String(r.int(1000, 9999))}`
      const npwp = `${r.int(10, 99)}.${r.int(100, 999)}.${r.int(100, 999)}.${r.int(1, 9)}-${r.int(100, 999)}.000`
      const bankAcc = String(r.int(1000000000, 9999999999))
      const gender = p.gender ?? (r.chance(0.5) ? 'L' : 'P')
      employees.push({ id: empId, school_id: S, user_id: p.userId, employee_no: `EMP-${u.level}-${String(++seq).padStart(3, '0')}`, full_name: p.name, email: p.email ?? null,
        phone_enc: crypto.encrypt(phone(r), S, empId + 'phone'), nik_enc: crypto.encrypt(nik, S, empId + 'nik'), nik_bidx: crypto.blindIndex(nik, S),
        npwp_enc: crypto.encrypt(npwp, S, empId + 'npwp'), npwp_bidx: crypto.blindIndex(npwp, S), ptkp_status: ptkp, bank_code: r.pick(BANKS),
        bank_account_enc: crypto.encrypt(bankAcc, S, empId + 'bank'), bank_account_name: p.name.replace(/\s*\(.*\)/, ''), birth_date: p.birth ?? `19${r.int(70, 98)}-0${r.int(1, 9)}-1${r.int(0, 9)}`,
        gender, address_enc: crypto.encrypt(address(r), S, empId + 'addr'), hire_date: hire, status: 'active' })
      assignments.push({ id: uid('hr-assign', empId), school_id: S, employee_id: empId, position_id: pos[p.position], department_id: dept[posDef[3]],
        employment_type: type, grade: type === 'tetap' ? r.pick(['III/a', 'III/b', 'III/c', 'IV/a']) : type === 'kontrak' ? r.pick(['II/c', 'II/d', 'III/a']) : null,
        base_salary_enc: crypto.encrypt(String(base), S, empId + 'salary'), contract_end: type === 'kontrak' ? '2027-06-30' : null, effective_from: hire })
      // Allowances.
      const mine = [['TJ-TRANS', comp['TJ-TRANS']?.default_amount_idr ?? 400000], ['TJ-MAKAN', comp['TJ-MAKAN']?.default_amount_idr ?? 300000]]
      if (p.userId && homeroomIds.has(p.userId)) mine.push(['TJ-WALI', comp['TJ-WALI']?.default_amount_idr ?? 500000])
      if (['KEPSEK', 'WAKA', 'KABAG'].includes(p.position)) mine.push(['TJ-JAB', p.position === 'KEPSEK' ? 2000000 : 1000000])
      for (const [code, amount] of mine) {
        if (!comp[code]) continue
        empComps.push({ id: uid('hr-emp-comp', empId, code), school_id: S, employee_id: empId, component_id: comp[code].id, amount_idr: amount, effective_from: hire < '2026-07-01' ? '2026-07-01' : hire })
      }
      payrollRows.push({ empId, base, ptkp, items: mine.filter(([c]) => comp[c]).map(([code, amount]) => ({ code, amount, c: comp[code] })),
        coop: r.chance(0.3) ? 250000 : 0, hire })

      // Attendance 13 Jul - 14 Oct (weekdays), leave balance and requests.
      let used = 0
      for (let d = new Date('2026-07-13T00:00:00Z'); d <= new Date('2026-10-14T00:00:00Z'); d = addDays(d, 1)) {
        const dow = d.getUTCDay()
        const ds = date(d)
        if (dow === 0 || dow === 6 || HOLIDAYS.has(ds)) continue
        if (ds < hire) continue
        const x = r.next()
        const status = x < 0.9 ? 'hadir' : x < 0.95 ? 'terlambat' : x < 0.97 ? 'sakit' : x < 0.985 ? 'izin' : x < 0.995 ? 'cuti' : 'dinas_luar'
        if (status === 'cuti') used++
        const inMin = status === 'terlambat' ? r.int(431, 470) : r.int(395, 425)
        attendance.push({ id: uid('hr-att', empId, ds), school_id: S, employee_id: empId, work_date: ds, status,
          check_in_at: ['hadir', 'terlambat', 'dinas_luar'].includes(status) ? `${ds}T${String(Math.floor(inMin / 60)).padStart(2, '0')}:${String(inMin % 60).padStart(2, '0')}:00+07:00` : null,
          check_out_at: ['hadir', 'terlambat'].includes(status) ? `${ds}T15:${String(r.int(0, 40)).padStart(2, '0')}:00+07:00` : null,
          source: ['sakit', 'izin', 'cuti'].includes(status) ? (status === 'izin' ? 'ess' : 'leave') : status === 'dinas_luar' ? 'manual' : 'import',
          note: status === 'dinas_luar' ? 'Pendampingan lomba siswa' : null })
        if (status === 'cuti' || status === 'sakit') {
          leaves.push({ id: uid('hr-leave', empId, ds), school_id: S, employee_id: empId, leave_type_id: lt[status === 'cuti' ? 'CT' : 'CS'], start_date: ds, end_date: ds, days: 1,
            reason: status === 'cuti' ? 'Keperluan keluarga' : 'Sakit, istirahat sesuai saran dokter', status: 'approved', idempotency_key: `demo:leave:${empId}:${ds}` })
        }
      }
      if (lt.CT) balances.push({ id: uid('hr-balance', empId, 2026), school_id: S, employee_id: empId, leave_type_id: lt.CT, year: 2026, entitled: 12, carried: r.int(0, 3), used })
      if (seq % 6 === 2 && lt.CT) {
        leaves.push({ id: uid('hr-leave-pending', empId), school_id: S, employee_id: empId, leave_type_id: lt.CT, start_date: '2026-10-26', end_date: '2026-10-27', days: 2,
          reason: 'Menghadiri pernikahan saudara di luar kota', status: 'pending', idempotency_key: `demo:leave-pending:${empId}` })
      }
    }

    out.push(insert('employees', employees), insert('employment_assignments', assignments), insert('employee_components', empComps))
    out.push(insert('employee_attendance', attendance, { conflict: '(employee_id, work_date) DO NOTHING', chunk: 1000 }))
    out.push(insert('leave_balances', balances, { conflict: '(employee_id, leave_type_id, year) DO NOTHING' }), insert('leave_requests', leaves, { conflict: '(school_id, idempotency_key) DO NOTHING' }))

    // ---- payroll ----
    const smpRuns = S === SMP ? Object.fromEntries(dbRows('teratai_hr', `SELECT period, id FROM payroll_runs WHERE school_id = '${S}' AND kind = 'regular'`).map((x) => [x.period, x.id])) : {}
    const runs = []
    const lines = []
    const slips = []
    for (const [period, status] of PERIODS) {
      const runId = smpRuns[period] ?? uid('hr-run', u.key, period)
      if (!smpRuns[period]) {
        runs.push({ id: runId, school_id: S, period, kind: 'regular', status, calculated_at: status !== 'draft' ? `${period}-24T10:00:00+07:00` : null,
          finalized_at: status === 'final' ? `${period}-27T09:00:00+07:00` : null, finalized_by: status === 'final' ? ADMIN : null,
          params_snapshot: ter ? { params: { ter } } : {}, totals: {}, idempotency_key: `demo:payroll:${u.key}:${period}` })
      }
      if (status === 'draft') continue
      for (const pr of payrollRows) {
        if (pr.hire > `${period}-25`) continue
        const items = [{ code: 'GAPOK', kind: 'earning', name: 'Gaji Pokok', taxable: true, bpjs_base: true, amount_idr: pr.base },
          ...pr.items.map((it) => ({ code: it.code, kind: 'earning', name: it.c.name, taxable: it.c.taxable, ...(it.c.bpjs_base ? { bpjs_base: true } : {}), amount_idr: it.amount, component_id: it.c.id }))]
        if (pr.coop) items.push({ code: 'POT-KOP', kind: 'deduction', name: 'Potongan Koperasi Pegawai', amount_idr: pr.coop, component_id: comp['POT-KOP']?.id })
        const gross = items.filter((i) => i.kind === 'earning').reduce((t, i) => t + i.amount_idr, 0)
        const bpjsBase = Math.min(12000000, items.filter((i) => i.bpjs_base).reduce((t, i) => t + i.amount_idr, 0))
        const jpBase = Math.min(10042300, bpjsBase)
        const emp = { kesehatan: Math.round(bpjsBase * 0.01), jht: Math.round(bpjsBase * 0.02), jp: Math.round(jpBase * 0.01), jkk: 0, jkm: 0 }
        const er = { kesehatan: Math.round(bpjsBase * 0.04), jht: Math.round(bpjsBase * 0.037), jp: Math.round(jpBase * 0.02), jkk: Math.round(bpjsBase * 0.0024), jkm: Math.round(bpjsBase * 0.003) }
        const empTotal = emp.kesehatan + emp.jht + emp.jp
        const erTaxed = er.kesehatan + er.jkk + er.jkm
        const taxable = gross + erTaxed
        const cat = TER_CAT[pr.ptkp] ?? 'A'
        const bracket = (ter?.[cat] ?? []).find((b) => taxable >= b.lower_idr && taxable <= b.upper_idr)
        const rateBp = bracket?.rate_bp ?? 0
        const pph = Math.floor((taxable * rateBp) / 10000)
        const net = gross - empTotal - pph - pr.coop
        const lineId = uid('hr-line', runId, pr.empId)
        lines.push({ id: lineId, school_id: S, run_id: runId, employee_id: pr.empId, gross_idr: gross, taxable_idr: taxable, bpjs_base_idr: bpjsBase,
          bpjs_employee_idr: empTotal, bpjs_employer_idr: Object.values(er).reduce((a, b) => a + b, 0), pph21_idr: pph, other_deductions_idr: pr.coop, net_idr: net,
          flagged: false, payment_status: status === 'final' ? 'paid' : 'unpaid',
          details: { bpjs: { pension: emp.jht + emp.jp, employee: emp, employer: er, employee_total: empTotal, employer_taxed: erTaxed,
            employer_total: Object.values(er).reduce((a, b) => a + b, 0) }, ptkp: pr.ptkp, items, pension_idr: emp.jht + emp.jp, ter_rate_bp: rateBp,
            ter_category: cat, base_salary_idr: pr.base } })
        if (status === 'final') slips.push({ id: uid('hr-slip', lineId), school_id: S, line_id: lineId, object_key: `payslips/${runId}/${lineId}.pdf`,
          sha256: lineId.replace(/-/g, '').padEnd(64, '0'), published_at: `${period}-27T10:00:00+07:00` })
      }
    }
    out.push(insert('payroll_runs', runs, { conflict: '(school_id, idempotency_key) DO NOTHING' }))
    out.push(insert('payroll_lines', lines, { conflict: '(run_id, employee_id) DO NOTHING', chunk: 500 }), insert('payslips', slips, { conflict: '(line_id) DO NOTHING' }))
    // Run totals from their lines (SMP's existing runs gained lines too).
    out.push(`UPDATE payroll_runs pr SET totals = t.totals FROM (
  SELECT run_id, jsonb_build_object('lines', count(*), 'flagged', count(*) FILTER (WHERE flagged), 'net_idr', sum(net_idr), 'skipped', '[]'::jsonb,
    'gross_idr', sum(gross_idr), 'pph21_idr', sum(pph21_idr), 'bpjs_employee_idr', sum(bpjs_employee_idr), 'bpjs_employer_idr', sum(bpjs_employer_idr),
    'other_deductions_idr', sum(other_deductions_idr)) totals
  FROM payroll_lines WHERE school_id = ${q(S)} GROUP BY run_id) t
WHERE pr.id = t.run_id AND pr.status IN ('calculated','pending_approval','approved','final');`)
  }
  return { teratai_hr: out.join('\n\n') }
}
