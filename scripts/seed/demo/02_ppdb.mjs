// Phase 2: PPDB 2027/2028 for the three units, with applicants spread over
// every step of the flow so each admin screen has something to work on:
//   Gelombang 1 (closed, tests done)
//     Reguler  — all applicants tested with final scores: ready for
//                "Jalankan Ranking" (the next step after test results).
//     Prestasi — ranked, decisions approved and published, offered
//                applicants re-registering and paying.
//     Afirmasi — ranked, no decision yet: the admin picks Diterima /
//                Cadangan / Tidak diterima per applicant, then submits.
//   Gelombang 2 (open now) — draft, submitted (form fee being paid),
//     document review and scheduled for the test.
// Also gives the already-open legacy period its selection components and
// formula, which it lacked (the flow stopped after the test results).
import { createHash } from 'node:crypto'
import { insert, person, place, address, phone, q, rng, slug, uid } from './lib.mjs'
import { MODEL } from './model.mjs'

const ADMIN = '22222222-2222-2222-2222-222222222222'
const ADMIN_NAME = 'Admin Teratai'

const LEVEL = {
  SD: { entry: 1, form: 250000, pangkal: 12000000, ulang: 2000000, age: 6, prev: ['TK Islam Al-Hikmah', 'TK Pertiwi', 'TK Kartika', 'RA Nurul Iman', 'TK Tunas Bangsa'],
    rapor: null,
    components: [['OBSERVASI', 'Observasi Kesiapan Sekolah', 'manual'], ['WAWANCARA', 'Wawancara Orang Tua', 'manual'], ['PRESTASI', 'Portofolio & Prestasi', 'manual']],
    formulas: { REG: [['OBSERVASI', 70, 5500], ['WAWANCARA', 30, 0]], PRS: [['PRESTASI', 50, 7000], ['OBSERVASI', 30, 5500], ['WAWANCARA', 20, 0]],
      AFR: [['OBSERVASI', 60, 5000], ['WAWANCARA', 40, 0]] } },
  SMP: { entry: 7, form: 300000, pangkal: 15000000, ulang: 2500000, age: 12, prev: ['SD Negeri 01 Bekasi', 'SD Islam Al-Azhar 5', 'SD Negeri 03 Jatiasih', 'SD Kristen Penabur', 'SD Muhammadiyah 2', 'MI Nurul Huda'],
    rapor: 'Rapor Kelas 4-6 SD',
    components: [['TES', 'Tes Potensi Akademik', 'exam'], ['RAPOR', 'Rata-rata Nilai Rapor', 'manual'], ['WAWANCARA', 'Wawancara Siswa & Orang Tua', 'manual'], ['PRESTASI', 'Nilai Prestasi', 'manual']],
    formulas: { REG: [['TES', 50, 6000], ['RAPOR', 30, 7000], ['WAWANCARA', 20, 0]], PRS: [['PRESTASI', 40, 7500], ['RAPOR', 40, 8000], ['WAWANCARA', 20, 0]],
      AFR: [['TES', 40, 5500], ['RAPOR', 40, 6500], ['WAWANCARA', 20, 0]] } },
  SMK: { entry: 10, form: 300000, pangkal: 10000000, ulang: 3000000, age: 15, prev: ['SMP Negeri 12 Bekasi', 'SMP Islam Al-Azhar 8', 'SMP Negeri 5 Jatiasih', 'MTs Negeri 2 Bekasi', 'SMP PGRI 1', 'SMP Teratai Putih'],
    rapor: 'Rapor SMP Kelas 7-9',
    components: [['TES', 'Tes Potensi Akademik', 'exam'], ['RAPOR', 'Rata-rata Nilai Rapor', 'manual'], ['WAWANCARA', 'Wawancara & Minat Jurusan', 'manual'], ['PRESTASI', 'Nilai Prestasi', 'manual']],
    formulas: { REG: [['TES', 45, 6000], ['RAPOR', 35, 7000], ['WAWANCARA', 20, 0]], PRS: [['PRESTASI', 40, 7500], ['RAPOR', 40, 8000], ['WAWANCARA', 20, 0]],
      AFR: [['TES', 40, 5500], ['RAPOR', 40, 6500], ['WAWANCARA', 20, 0]] } },
}

// The Go service stores sha256 of json.Marshal(FormulaSpec): same field order.
function formulaSpec(name, comps, components) {
  const byCode = Object.fromEntries(components.map(([c, n, s]) => [c, { n, s }]))
  const spec = {
    name, version: 0, status: 'active', minimum_total: 6000,
    components: comps.map(([code, pct, min]) => ({ code, name: byCode[code].n, source: byCode[code].s, weight: pct * 100, minimum: min })),
    tie_breakers: comps.slice(0, 2).map(([c]) => c),
  }
  const raw = JSON.stringify(spec)
  return { spec, checksum: createHash('sha256').update(raw).digest('hex') }
}

const ts = (d, h = 9, m = 0) => `${d}T${String(h).padStart(2, '0')}:${String(m).padStart(2, '0')}:00+07:00`

// Applicants per wave/track and the status each one ends in.
const PLAN = [
  { wave: 1, track: 'REG', n: 26, status: (i) => (i < 22 ? 'tested' : i < 24 ? 'rejected' : 'withdrawn') },
  { wave: 1, track: 'PRS', n: 12, status: (i) => (i < 5 ? 'reregistration' : i < 8 ? 'offered' : i < 10 ? 'waitlisted' : 'rejected') },
  { wave: 1, track: 'AFR', n: 8, status: (i) => (i < 6 ? 'ranked' : 'document_review') },
  { wave: 2, track: 'REG', n: 16, status: (i) => (i < 3 ? 'draft' : i < 8 ? 'submitted' : i < 13 ? 'document_review' : 'eligible_for_test') },
  { wave: 2, track: 'PRS', n: 6, status: (i) => (i < 2 ? 'submitted' : i < 4 ? 'document_review' : 'eligible_for_test') },
]

// Status path from draft to each final status (the events written).
const PATH = {
  draft: ['draft'],
  submitted: ['draft', 'submitted'],
  document_review: ['draft', 'submitted', 'document_review'],
  eligible_for_test: ['draft', 'submitted', 'document_review', 'eligible_for_test'],
  tested: ['draft', 'submitted', 'document_review', 'eligible_for_test', 'tested'],
  ranked: ['draft', 'submitted', 'document_review', 'eligible_for_test', 'tested', 'ranked'],
  offered: ['draft', 'submitted', 'document_review', 'eligible_for_test', 'tested', 'ranked', 'offered'],
  waitlisted: ['draft', 'submitted', 'document_review', 'eligible_for_test', 'tested', 'ranked', 'waitlisted'],
  reregistration: ['draft', 'submitted', 'document_review', 'eligible_for_test', 'tested', 'ranked', 'offered', 'reregistration'],
  rejected: ['draft', 'submitted', 'document_review', 'rejected'],
  withdrawn: ['draft', 'submitted', 'document_review', 'eligible_for_test', 'withdrawn'],
}
const AFTER = (s, step) => PATH[s].includes(step)

export function generate() {
  const out = []
  // Legacy open period: components + formula, so ranking works there too.
  out.push(`-- legacy open period "PPDB 2026/2027": selection components and formula it lacked`)
  const legacy = LEVEL.SMP
  out.push(insert('selection_components', legacy.components.slice(0, 3).map(([code, name, source]) => ({
    id: uid('ppdb-legacy-comp', code), school_id: '11111111-1111-1111-1111-111111111111', period_id: 'aaaaaaaa-0000-0000-0000-000000000001',
    code, name, source, required: true, max_score: 10000 })), { conflict: '(period_id, code) DO NOTHING' }))
  {
    const { spec, checksum } = formulaSpec('Formula Reguler 2026/2027', legacy.formulas.REG, legacy.components)
    out.push(insert('selection_formula_versions', [{ id: uid('ppdb-legacy-formula'), school_id: '11111111-1111-1111-1111-111111111111',
      period_id: 'aaaaaaaa-0000-0000-0000-000000000001', wave_id: 'aaaaaaaa-0000-0000-0000-000000000002', track_id: 'aaaaaaaa-0000-0000-0000-000000000003',
      version: 1, formula: spec, status: 'active', checksum }], { conflict: '(wave_id, track_id, version) DO NOTHING' }))
  }

  let nisn = 4100000000
  for (const u of MODEL) {
    const L = LEVEL[u.level]
    const r = rng('ppdb:' + u.key)
    const S = u.schoolId
    const periodId = uid('ppdb-period', u.key)
    const prefix = `PPDB-2027-${u.level}`
    out.push(`-- ===== ${u.level} =====`)

    out.push(insert('admission_periods', [{ id: periodId, school_id: S, name: `PPDB 2027/2028 ${u.level} Teratai Putih`, academic_year: '2027/2028',
      timezone: 'Asia/Jakarta', status: 'open', registration_open_at: ts('2026-09-01', 0), registration_close_at: ts('2027-02-28', 23, 59),
      discount_mode: 'stack', max_discount_idr: 5000000, max_payment_grace_days: 7, refund_policy_bp: 5000, refund_admin_fee_idr: 250000 }]))

    const waves = [
      { n: 1, id: uid('ppdb-wave', u.key, 1), name: `Gelombang 1 - ${u.level} Kelas ${L.entry}`, start: '2026-09-01', end: '2026-09-30', pay: '2026-10-05',
        test: ['2026-10-03', '2026-10-04'], ann: '2026-10-10', rereg: '2026-11-30' },
      { n: 2, id: uid('ppdb-wave', u.key, 2), name: `Gelombang 2 - ${u.level} Kelas ${L.entry}`, start: '2026-10-01', end: '2026-11-30', pay: '2026-12-05',
        test: ['2026-12-06', '2026-12-07'], ann: '2026-12-14', rereg: '2027-01-31' },
    ]
    out.push(insert('admission_waves', waves.map((w) => ({ id: w.id, school_id: S, period_id: periodId, name: w.name,
      registration_start: ts(w.start, 0), registration_end: ts(w.end, 23, 59), payment_end: ts(w.pay, 23, 59), test_start: ts(w.test[0], 8),
      test_end: ts(w.test[1], 15), announcement_at: ts(w.ann, 10), reregistration_end: ts(w.rereg, 23, 59), waitlist_reregistration_days: 3,
      education_level: u.level, entry_grade: L.entry }))))

    const tracks = [
      { code: 'REG', name: 'Jalur Reguler', quota: u.level === 'SMK' ? 50 : 40, desc: 'Seleksi umum berdasarkan tes dan nilai rapor.' },
      { code: 'PRS', name: 'Jalur Prestasi', quota: 8, desc: 'Untuk calon siswa dengan prestasi akademik atau non-akademik minimal tingkat kota.' },
      { code: 'AFR', name: 'Jalur Afirmasi', quota: 4, desc: 'Untuk calon siswa pemegang KIP/KKS atau dari keluarga kurang mampu.' },
    ].map((t) => ({ ...t, id: uid('ppdb-track', u.key, t.code) }))
    out.push(insert('admission_tracks', tracks.map((t) => ({ id: t.id, school_id: S, period_id: periodId, code: t.code, name: t.name,
      quota: t.quota, eligibility_config: {}, description: t.desc, is_active: true })), { conflict: '(period_id, code) DO NOTHING' }))

    // Form, custom fields, documents.
    out.push(insert('form_definitions', [{ id: uid('ppdb-form', u.key), school_id: S, period_id: periodId, version: 1, status: 'published',
      schema: { sections: [
        { title: 'Data Calon Siswa', fields: ['full_name', 'nickname', 'nisn', 'nik', 'birth_place', 'birth_date', 'gender', 'previous_school', 'address'] },
        { title: 'Data Orang Tua', fields: ['guardian_relationship'] },
        { title: 'Data Tambahan', fields: ['ukuran_seragam', 'minat_ekskul'] }] } }], { conflict: '(period_id, version) DO NOTHING' }))
    out.push(insert('admission_form_fields', [
      { id: uid('ppdb-field', u.key, 'seragam'), school_id: S, period_id: periodId, field_key: 'ukuran_seragam', label: 'Ukuran Seragam', field_type: 'select',
        options: ['S', 'M', 'L', 'XL'], required: true, help_text: 'Untuk pemesanan seragam saat daftar ulang.', sort_order: 1, active: true },
      { id: uid('ppdb-field', u.key, 'ekskul'), school_id: S, period_id: periodId, field_key: 'minat_ekskul', label: 'Minat Ekstrakurikuler', field_type: 'select',
        options: ['Pramuka', 'Futsal', 'Robotik', 'Paduan Suara', 'Tari Tradisional', 'Basket', 'English Club'], required: false, sort_order: 2, active: true },
    ], { conflict: '(school_id, period_id, field_key) DO NOTHING' }))
    const pdfImg = ['application/pdf', 'image/jpeg', 'image/png']
    const reqs = [
      { code: 'AKTA', name: 'Akta Kelahiran', track: null, mimes: pdfImg, stage: 'registration' },
      { code: 'KK', name: 'Kartu Keluarga', track: null, mimes: pdfImg, stage: 'registration' },
      { code: 'FOTO', name: 'Pas Foto 3x4', track: null, mimes: ['image/jpeg', 'image/png'], stage: 'registration' },
      ...(L.rapor ? [{ code: 'RAPOR', name: L.rapor, track: null, mimes: ['application/pdf'], stage: 'registration' }] : []),
      { code: 'PIAGAM', name: 'Piagam / Sertifikat Prestasi', track: 'PRS', mimes: ['application/pdf', 'image/jpeg'], stage: 'registration' },
      { code: 'KIP', name: 'Kartu Indonesia Pintar / KKS', track: 'AFR', mimes: pdfImg, stage: 'registration' },
      { code: 'SKL', name: u.level === 'SD' ? 'Surat Keterangan Lulus TK' : u.level === 'SMP' ? 'Surat Keterangan Lulus SD' : 'Surat Keterangan Lulus SMP',
        track: null, mimes: ['application/pdf'], stage: 'reregistration' },
    ].map((d) => ({ ...d, id: uid('ppdb-req', u.key, d.code) }))
    out.push(insert('document_requirements', reqs.map((d) => ({ id: d.id, school_id: S, period_id: periodId,
      track_id: d.track ? tracks.find((t) => t.code === d.track).id : null, code: d.code, name: d.name, required: true,
      allowed_mimes: `{${d.mimes.join(',')}}`, max_bytes: 5242880, version: 1, stage: d.stage })), { conflict: '(period_id, code, version) DO NOTHING' }))

    // Fees.
    const schemeId = uid('ppdb-fee', u.key)
    out.push(insert('fee_schemes', [{ id: schemeId, school_id: S, period_id: periodId, track_id: null, name: `Biaya PPDB 2027/2028 ${u.level}`,
      revision: 1, status: 'published', currency: 'IDR', registration_fee_idr: L.form, reregistration_fee_idr: L.ulang, installment_count: 2,
      entrance_fee_idr: L.pangkal, min_reregistration_pct: 50 }]))
    const feeItems = [['FORM', 'Formulir Pendaftaran', L.form, false], ['UANG_PANGKAL', 'Uang Pangkal', L.pangkal, true],
      ['DAFTAR_ULANG', 'Daftar Ulang (seragam, buku, MPLS)', L.ulang, false]]
    out.push(insert('fee_items', feeItems.map(([code, name, amount, disc]) => ({ id: uid('ppdb-fee-item', u.key, code), school_id: S,
      fee_scheme_id: schemeId, code, name, amount_idr: amount, discountable: disc })), { conflict: '(fee_scheme_id, code) DO NOTHING' }))
    const rereg = L.pangkal + L.ulang
    out.push(insert('installment_plans', [
      { id: uid('ppdb-plan', u.key, 1), school_id: S, fee_scheme_id: schemeId, sequence: 1, description: 'Cicilan 1 (minimal 50%)', amount_idr: rereg / 2, due_at: ts('2026-11-15', 23, 59) },
      { id: uid('ppdb-plan', u.key, 2), school_id: S, fee_scheme_id: schemeId, sequence: 2, description: 'Cicilan 2 (pelunasan)', amount_idr: rereg / 2, due_at: ts('2027-02-15', 23, 59) },
    ], { conflict: '(fee_scheme_id, sequence) DO NOTHING' }))

    // Re-registration checklist.
    const checklistId = uid('ppdb-checklist', u.key)
    out.push(insert('reregistration_checklists', [{ id: checklistId, school_id: S, period_id: periodId, name: 'Checklist Daftar Ulang', min_payment_idr: rereg / 2 }]))
    const rrItems = [['SKL', 'Menyerahkan surat keterangan lulus'], ['SERAGAM', 'Pengukuran seragam'], ['TATIB', 'Menandatangani pernyataan tata tertib'],
      ['MPLS', 'Konfirmasi kehadiran MPLS']].map(([code, label]) => ({ id: uid('ppdb-rr-item', u.key, code), school_id: S, checklist_id: checklistId, code, label, required: true }))
    out.push(insert('reregistration_items', rrItems, { conflict: '(checklist_id, code) DO NOTHING' }))

    // Selection components + one formula per wave and track.
    const comps = L.components.map(([code, name, source]) => ({ id: uid('ppdb-comp', u.key, code), school_id: S, period_id: periodId, code, name, source,
      required: true, max_score: 10000 }))
    out.push(insert('selection_components', comps, { conflict: '(period_id, code) DO NOTHING' }))
    const formulas = {}
    const formulaRows = []
    for (const w of waves) {
      for (const t of tracks) {
        const { spec, checksum } = formulaSpec(`Formula ${t.name} ${w.n === 1 ? 'Gel. 1' : 'Gel. 2'}`, L.formulas[t.code], L.components)
        const id = uid('ppdb-formula', u.key, w.n, t.code)
        formulas[`${w.n}:${t.code}`] = { id, comps: L.formulas[t.code] }
        formulaRows.push({ id, school_id: S, period_id: periodId, wave_id: w.id, track_id: t.id, version: 1, formula: spec, status: 'active', checksum })
      }
    }
    out.push(insert('selection_formula_versions', formulaRows, { conflict: '(wave_id, track_id, version) DO NOTHING' }))

    // ---- applicants ----
    const guardians = []
    const contacts = []
    const applicants = []
    const apps = []
    const events = []
    const docs = []
    const reviews = []
    const charges = []
    const installments = []
    const payments = []
    const payReviews = []
    const allocations = []
    const scores = []
    const schedules = []
    const rrDone = []
    const counters = {}
    const ranked = {} // `${wave}:${track}` -> [{ appId, total, scores }]

    for (const plan of PLAN) {
      const w = waves[plan.wave - 1]
      const t = tracks.find((x) => x.code === plan.track)
      for (let i = 0; i < plan.n; i++) {
        const status = plan.status(i)
        const p = person(r)
        const gp = person(r, { gender: r.chance(0.6) ? 'P' : 'L', last: p.last })
        const key = `${u.key}:${plan.wave}:${plan.track}:${i}`
        const guardianId = uid('ppdb-guardian', key)
        const applicantId = uid('ppdb-applicant', key)
        const appId = uid('ppdb-app', key)
        counters[w.id] = (counters[w.id] ?? 0) + 1
        const regNo = `${prefix}-G${plan.wave}-${String(counters[w.id]).padStart(4, '0')}`
        const email = `${slug(gp.name)}.${u.key}${plan.wave}${plan.track.toLowerCase()}${i}@ppdb-demo.test`
        const hp = phone(r)
        guardians.push({ id: guardianId, school_id: S, name: gp.name, status: 'active' })
        contacts.push({ id: uid('ppdb-contact', key, 'e'), school_id: S, guardian_id: guardianId, kind: 'email', normalized_value: email, verified_at: ts(w.start, 8) })
        contacts.push({ id: uid('ppdb-contact', key, 'p'), school_id: S, guardian_id: guardianId, kind: 'phone', normalized_value: '62' + hp.slice(1), verified_at: ts(w.start, 8) })
        const birthYear = 2027 - L.age - (r.chance(0.2) ? 1 : 0)
        applicants.push({ id: applicantId, school_id: S, guardian_id: guardianId, full_name: p.name, nickname: p.first, nisn: L.entry === 1 ? null : String(nisn++),
          nik: `3275${String(r.int(10000000, 99999999))}${String(r.int(1000, 9999))}`,
          birth_date: `${birthYear}-${String(r.int(1, 12)).padStart(2, '0')}-${String(r.int(1, 28)).padStart(2, '0')}`, birth_place: place(r), gender: p.gender,
          source: r.pick(['landing_page', 'referral', 'walk_in', 'instagram', 'landing_page']), profile: {}, previous_school: r.pick(L.prev), address: address(r),
          guardian_relationship: gp.gender === 'L' ? 'ayah' : 'ibu',
          custom_fields: { ukuran_seragam: r.pick(['S', 'M', 'M', 'L']), minat_ekskul: r.pick(['Pramuka', 'Futsal', 'Robotik', 'Paduan Suara', 'Tari Tradisional', 'Basket']) } })

        // Timeline inside the wave window.
        const dayOff = r.int(0, 20)
        const base = new Date(`${w.start}T09:00:00+07:00`).getTime() + dayOff * 86400000
        const at = (k) => new Date(base + k * 86400000 * 1.5 + r.int(0, 300) * 60000).toISOString()
        const path = PATH[status]
        path.slice(1).forEach((to, k) => {
          const from = path[k]
          const actorType = to === 'submitted' || to === 'withdrawn' ? 'guardian' : 'admin'
          events.push({ school_id: S, application_id: appId, from_status: from, to_status: to, actor_type: actorType,
            actor_id: actorType === 'guardian' ? guardianId : ADMIN, version: k + 2,
            reason: to === 'rejected' ? 'Dokumen persyaratan tidak lengkap setelah batas perbaikan.' : to === 'withdrawn' ? 'Diterima di sekolah lain.' : null,
            created_at: at(k + 1) })
        })
        const submitted = status !== 'draft'
        apps.push({ id: appId, school_id: S, applicant_id: applicantId, period_id: periodId, wave_id: w.id, track_id: t.id, registration_number: regNo, status,
          version: path.length, answers: {}, submitted_snapshot: submitted ? { full_name: p.name, previous_school: applicants.at(-1).previous_school, track: t.name } : null,
          submitted_at: submitted ? at(1) : null, reregistration_due_at: AFTER(status, 'offered') ? ts(w.rereg, 23, 59) : null, created_at: at(0) })

        if (!submitted) continue

        // Registration fee: paid (verified) once past document review; on
        // "submitted" half are waiting for verification, half not paid yet.
        const regChargeId = uid('ppdb-charge', key, 1)
        const regInstId = uid('ppdb-charge-inst', key, 1, 1)
        charges.push({ id: regChargeId, school_id: S, application_id: appId, revision: 1, fee_scheme_id: schemeId, gross_idr: L.form, discount_idr: 0, net_idr: L.form,
          items_snapshot: [{ code: 'FORM', name: 'Formulir Pendaftaran', amount_idr: L.form }], discount_snapshot: [], finalized_at: at(1), stage: 'registration',
          min_payment_idr: L.form, due_at: ts(w.pay, 23, 59) })
        installments.push({ id: regInstId, school_id: S, charge_id: regChargeId, application_id: appId, sequence: 1, description: 'Formulir Pendaftaran', amount_idr: L.form, due_at: ts(w.pay, 23, 59) })
        const regPaid = AFTER(status, 'document_review') || status === 'withdrawn'
        const regPending = status === 'submitted' && i % 2 === 0
        if (regPaid || regPending) {
          const payId = uid('ppdb-pay', key, 1)
          payments.push({ id: payId, school_id: S, application_id: appId, charge_id: regChargeId, amount_idr: L.form, object_key: `demo/ppdb/${appId}/bukti-formulir.jpg`,
            mime: 'image/jpeg', size_bytes: 184320, sha256: createHash('sha256').update(payId).digest('hex'), status: regPaid ? 'verified' : 'pending_review',
            uploader_type: 'guardian', uploader_id: guardianId, idempotency_key: `demo:${payId}`, file_name: 'bukti-transfer-formulir.jpg', transfer_at: at(1),
            bank_name: r.pick(['BCA', 'Bank Mandiri', 'BRI', 'BNI', 'BSI']), account_name: gp.name, reference: `TRF${r.int(100000, 999999)}`, created_at: at(1) })
          if (regPaid) {
            payReviews.push({ id: uid('ppdb-pay-review', key, 1), school_id: S, payment_id: payId, action: 'verified', reviewer_id: ADMIN, created_at: at(1.5) })
            allocations.push({ id: uid('ppdb-alloc', key, 1), school_id: S, payment_id: payId, charge_installment_id: regInstId, amount_idr: L.form })
          }
        }

        // Documents: uploaded once submitted; reviewed once past review.
        if (AFTER(status, 'document_review') || status === 'withdrawn') {
          const mine = reqs.filter((d) => d.stage === 'registration' && (!d.track || d.track === plan.track))
          for (const d of mine) {
            const docId = uid('ppdb-doc', key, d.code)
            docs.push({ id: docId, school_id: S, application_id: appId, requirement_id: d.id, object_key: `demo/ppdb/${appId}/${d.code.toLowerCase()}.pdf`,
              mime: d.code === 'FOTO' ? 'image/jpeg' : 'application/pdf', size_bytes: r.int(120000, 900000), sha256: createHash('sha256').update(docId).digest('hex'),
              scan_status: 'clean', uploaded_by: 'guardian', file_name: `${d.code.toLowerCase()}_${slug(p.first)}.${d.code === 'FOTO' ? 'jpg' : 'pdf'}`, version: 1, created_at: at(1.2) })
            const reviewed = status !== 'document_review'
            if (reviewed) {
              const rejected = status === 'rejected' && d.code === 'KK'
              reviews.push({ id: uid('ppdb-doc-review', key, d.code), school_id: S, document_id: docId, status: rejected ? 'rejected' : 'accepted',
                notes: rejected ? 'Kartu Keluarga tidak terbaca dan tidak diperbaiki sampai batas waktu.' : null, reviewer_id: ADMIN, reviewer_name: ADMIN_NAME,
                document_version: 1, created_at: at(2) })
            }
          }
        }

        // Test schedule for those cleared for the test.
        if (AFTER(status, 'eligible_for_test') && status !== 'withdrawn') {
          schedules.push({ id: uid('ppdb-sched', key, 'test'), school_id: S, application_id: appId, kind: 'test', scheduled_at: ts(w.test[0], 8 + (i % 3)),
            location: u.level === 'SD' ? 'Ruang Kelas 1A' : 'Lab Komputer Lt. 2', notes: 'Membawa alat tulis dan kartu peserta.', created_by: ADMIN, created_by_name: ADMIN_NAME })
          schedules.push({ id: uid('ppdb-sched', key, 'interview'), school_id: S, application_id: appId, kind: 'interview', scheduled_at: ts(w.test[1], 8 + (i % 6)),
            location: 'Ruang BK', notes: 'Didampingi orang tua/wali.', created_by: ADMIN, created_by_name: ADMIN_NAME })
        }

        // Final scores once tested; ranked groups get a total too.
        if (AFTER(status, 'tested')) {
          const f = formulas[`${plan.wave}:${plan.track}`]
          const sc = {}
          let total = 0
          for (const [code, pct] of f.comps) {
            const strong = AFTER(status, 'offered') ? 1000 : status === 'waitlisted' ? 300 : 0
            const v = Math.min(10000, r.int(6200, 9000) + strong)
            sc[code] = v
            total += Math.floor((v * pct * 100) / 10000)
            scores.push({ id: uid('ppdb-score', key, code), school_id: S, application_id: appId, component_id: comps.find((c) => c.code === code).id, score: v,
              final: true, entered_by: ADMIN, created_at: ts(w.test[1], 16), updated_at: ts(w.test[1], 16) })
          }
          if (AFTER(status, 'ranked')) (ranked[`${plan.wave}:${plan.track}`] ??= []).push({ appId, key, status, total, sc, w, t })
        }

        // Offered: re-registration charge; re-registering ones paid the first
        // instalment (some fully) and ticked part of the checklist.
        if (AFTER(status, 'offered')) {
          const chargeId = uid('ppdb-charge', key, 2)
          const disc = plan.track === 'PRS' && i % 2 === 0 ? 2500000 : 0
          const net = rereg - disc
          charges.push({ id: chargeId, school_id: S, application_id: appId, revision: 2, fee_scheme_id: schemeId, gross_idr: rereg, discount_idr: disc, net_idr: net,
            items_snapshot: [{ code: 'UANG_PANGKAL', name: 'Uang Pangkal', amount_idr: L.pangkal }, { code: 'DAFTAR_ULANG', name: 'Daftar Ulang (seragam, buku, MPLS)', amount_idr: L.ulang }],
            discount_snapshot: disc ? [{ origin: 'manual', source_id: uid('ppdb-disc', key), applied_idr: disc, label: 'Potongan Jalur Prestasi' }] : [],
            finalized_at: ts(w.ann, 11), stage: 'reregistration', min_payment_idr: net / 2, due_at: ts(w.rereg, 23, 59) })
          const inst = [1, 2].map((sq) => ({ id: uid('ppdb-charge-inst', key, 2, sq), school_id: S, charge_id: chargeId, application_id: appId, sequence: sq,
            description: sq === 1 ? 'Cicilan 1 (minimal 50%)' : 'Cicilan 2 (pelunasan)', amount_idr: net / 2, due_at: sq === 1 ? ts('2026-11-15', 23, 59) : ts('2027-02-15', 23, 59) }))
          installments.push(...inst)
          if (status === 'reregistration') {
            const full = i % 2 === 0
            const amount = full ? net : net / 2
            const payId = uid('ppdb-pay', key, 2)
            payments.push({ id: payId, school_id: S, application_id: appId, charge_id: chargeId, amount_idr: amount, object_key: `demo/ppdb/${appId}/bukti-daftar-ulang.jpg`,
              mime: 'image/jpeg', size_bytes: 201440, sha256: createHash('sha256').update(payId).digest('hex'), status: 'verified', uploader_type: 'guardian',
              uploader_id: guardianId, idempotency_key: `demo:${payId}`, file_name: 'bukti-transfer-daftar-ulang.jpg', transfer_at: ts('2026-10-20', 10),
              bank_name: r.pick(['BCA', 'Bank Mandiri', 'BRI', 'BNI', 'BSI']), account_name: gp.name, reference: `TRF${r.int(100000, 999999)}`, created_at: ts('2026-10-20', 10) })
            payReviews.push({ id: uid('ppdb-pay-review', key, 2), school_id: S, payment_id: payId, action: 'verified', reviewer_id: ADMIN, created_at: ts('2026-10-21', 9) })
            allocations.push({ id: uid('ppdb-alloc', key, 2, 1), school_id: S, payment_id: payId, charge_installment_id: inst[0].id, amount_idr: net / 2 })
            if (full) allocations.push({ id: uid('ppdb-alloc', key, 2, 2), school_id: S, payment_id: payId, charge_installment_id: inst[1].id, amount_idr: net / 2 })
            rrItems.slice(0, full ? 4 : 2).forEach((it) => rrDone.push({ id: uid('ppdb-rr-done', key, it.code), school_id: S, application_id: appId, item_id: it.id,
              completed_at: ts('2026-10-22', 10), completed_by: ADMIN }))
          } else if (i % 2 === 1) {
            // An offered applicant whose transfer is waiting for verification.
            const payId = uid('ppdb-pay', key, 2)
            payments.push({ id: payId, school_id: S, application_id: appId, charge_id: chargeId, amount_idr: net / 2, object_key: `demo/ppdb/${appId}/bukti-daftar-ulang.jpg`,
              mime: 'image/jpeg', size_bytes: 199880, sha256: createHash('sha256').update(payId).digest('hex'), status: 'pending_review', uploader_type: 'guardian',
              uploader_id: guardianId, idempotency_key: `demo:${payId}`, file_name: 'bukti-transfer-cicilan-1.jpg', transfer_at: ts('2026-10-14', 13),
              bank_name: 'BCA', account_name: gp.name, reference: `TRF${r.int(100000, 999999)}`, created_at: ts('2026-10-14', 13) })
          }
        }
      }
    }

    // Ranking runs: Prestasi published (decisions approved + announced);
    // Afirmasi waiting for approval (decisions pending_approval).
    const runs = []
    const entries = []
    const decisions = []
    const approvals = []
    const announcements = []
    const undecided = []
    const undecidedApprovals = []
    for (const [k, list] of Object.entries(ranked)) {
      const [waveN, trackCode] = k.split(':')
      const w = waves[Number(waveN) - 1]
      const t = tracks.find((x) => x.code === trackCode)
      const published = trackCode === 'PRS'
      const runId = uid('ppdb-run', u.key, k)
      const sorted = [...list].sort((a, b) => b.total - a.total)
      runs.push({ id: runId, school_id: S, period_id: periodId, wave_id: w.id, track_id: t.id, formula_version_id: formulas[k].id, version: 1,
        input_checksum: createHash('sha256').update(runId).digest('hex'), status: 'final', created_by: ADMIN, created_at: ts(w.ann === '2026-10-10' ? '2026-10-06' : w.ann, 10),
        published_at: published ? ts('2026-10-08', 14) : null })
      const approvalId = uid('ppdb-approval', u.key, k)
      // Afirmasi stops right after the ranking: no decision yet, so the admin
      // picks Diterima / Cadangan / Tidak diterima per applicant in the
      // "Ranking & keputusan" tab, then submits the wave for approval.
      if (published) {
        approvals.push({ id: approvalId, school_id: S, period_id: periodId, wave_id: w.id, status: 'approved', decision_count: sorted.length,
          created_by: ADMIN, created_by_name: ADMIN_NAME, decided_by: '77777777-7777-7777-7777-777777777777',
          decided_comment: 'Disetujui sesuai hasil rapat panitia.', decided_at: ts('2026-10-09', 10), created_at: ts('2026-10-08', 15) })
      } else {
        undecided.push(...sorted.map((x) => uid('ppdb-decision', x.key)))
        undecidedApprovals.push(approvalId)
      }
      sorted.forEach((x, idx) => {
        const accepted = published ? x.status !== 'waitlisted' : idx < t.quota
        entries.push({ school_id: S, ranking_run_id: runId, application_id: x.appId, rank: idx + 1, total_score: x.total,
          scores_snapshot: x.sc, recommendation: accepted ? 'accepted' : 'waitlisted' })
        if (!published) return
        decisions.push({ id: uid('ppdb-decision', x.key), school_id: S, application_id: x.appId,
          decision: accepted ? 'offered' : 'waitlisted', override: false, ranking_entry_id: null, actor_id: ADMIN, version: 1, status: 'published',
          selection_approval_id: approvalId, published_at: ts(w.ann, 10), created_at: ts('2026-10-08', 15) })
      })
      if (published) {
        announcements.push({ id: uid('ppdb-announce', u.key, k), school_id: S, period_id: periodId, wave_id: w.id,
          title: `Pengumuman Hasil Seleksi ${t.name} ${w.name}`,
          message: `Selamat kepada calon siswa yang dinyatakan diterima. Silakan melakukan daftar ulang paling lambat ${w.rereg}. Calon siswa cadangan akan dihubungi bila ada kuota kosong.`,
          publish_at: ts(w.ann, 10), status: 'published', created_by: ADMIN, created_by_name: ADMIN_NAME, approved_by: '77777777-7777-7777-7777-777777777777',
          approved_by_name: 'Kepala Sekolah Teratai', approved_at: ts('2026-10-09', 10), published_at: ts(w.ann, 10), notified_count: sorted.length, selection_approval_id: approvalId })
      }
    }
    // Upcoming announcement for Gelombang 2 (draft).
    announcements.push({ id: uid('ppdb-announce', u.key, 'w2'), school_id: S, period_id: periodId, wave_id: waves[1].id,
      title: `Jadwal Tes Seleksi ${waves[1].name}`, message: `Tes seleksi dilaksanakan ${waves[1].test[0]} pukul 08.00 WIB. Peserta wajib membawa kartu peserta dan alat tulis.`,
      publish_at: ts('2026-12-01', 9), status: 'draft', created_by: ADMIN, created_by_name: ADMIN_NAME })

    out.push(insert('guardians', guardians))
    out.push(insert('guardian_contacts', contacts, { conflict: '(school_id, kind, normalized_value) DO NOTHING' }))
    out.push(insert('applicants', applicants))
    out.push(insert('applications', apps))
    // Auto-increment ids: the demo applications' history is rewritten.
    out.push(`DELETE FROM application_state_events WHERE application_id IN (${apps.map((a) => q(a.id)).join(', ')});`)
    out.push(insert('application_state_events', events))
    out.push(insert('registration_counters', Object.entries(counters).map(([wave_id, n]) => ({ wave_id, school_id: S, prefix: `${prefix}-G${waves.find((w) => w.id === wave_id).n}`, last_value: n })),
      { conflict: '(wave_id) DO UPDATE SET last_value = GREATEST(registration_counters.last_value, EXCLUDED.last_value)' }))
    out.push(insert('application_documents', docs, { conflict: '(application_id, requirement_id) DO NOTHING' }))
    out.push(insert('document_reviews', reviews))
    out.push(insert('charges', charges, { conflict: '(application_id, revision) DO NOTHING' }))
    out.push(insert('charge_installments', installments, { conflict: '(charge_id, sequence) DO NOTHING' }))
    out.push(insert('payment_submissions', payments, { conflict: '(school_id, idempotency_key) DO NOTHING' }))
    out.push(insert('payment_reviews', payReviews))
    out.push(insert('payment_allocations', allocations))
    out.push(insert('selection_schedules', schedules, { conflict: '(application_id, kind) DO NOTHING' }))
    out.push(insert('component_scores', scores, { conflict: '(application_id, component_id) DO NOTHING' }))
    out.push(insert('application_reregistration_items', rrDone, { conflict: '(application_id, item_id) DO NOTHING' }))
    // Decisions an earlier run pre-filled for Afirmasi are removed, so the
    // admin makes them (see above).
    if (undecided.length) out.push(`DELETE FROM decisions WHERE id IN (${undecided.map(q).join(', ')});`)
    if (undecidedApprovals.length) out.push(`DELETE FROM selection_approvals WHERE id IN (${undecidedApprovals.map(q).join(', ')});`)
    out.push(insert('selection_approvals', approvals))
    out.push(insert('ranking_runs', runs, { conflict: '(wave_id, track_id, version) DO NOTHING' }))
    out.push(insert('ranking_entries', entries, { conflict: '(ranking_run_id, application_id) DO NOTHING' }))
    out.push(insert('decisions', decisions, { conflict: '(application_id, version) DO NOTHING' }))
    if (decisions.length) out.push(`UPDATE decisions d SET ranking_entry_id = e.id FROM ranking_entries e WHERE e.application_id = d.application_id AND d.ranking_entry_id IS NULL AND d.id IN (${decisions.map((x) => q(x.id)).join(', ')});`)
    out.push(insert('ppdb_announcements', announcements))
  }
  return { teratai_admission: out.join('\n\n') }
}
