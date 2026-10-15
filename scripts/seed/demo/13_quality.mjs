// Phase 13: quality assurance & accreditation of SD and SMK (SMP has its own
// from the base seeds) — the foundation's EDS/SPMI instrument (this year's
// active version and last year's retired one) and BAN-S/M's IASP, their
// standards, items and indicators with yearly targets, three self-evaluations
// (start of year finalized, mid-term waiting for the principal, end of term
// in draft) scored from the unit's real attendance / billing / achievement
// figures, action plans for what fell below target, evidence, regulatory
// reports to the education office, and the Dapodik reconciliation.
import { ACCOUNTS, approvals, at, cloneConfig, insert, merge, rng, uid } from './lib.mjs'
import { facts } from './facts.mjs'
import { MODEL } from './model.mjs'

const SMP = '11111111-1111-1111-1111-111111111111'
const { admin: ADMIN, kepala_sekolah: KEPSEK, qa_coordinator: MUTU, operator_dapodik: DAPODIK } = ACCOUNTS

// [code, name, weight_bp, items: [code, statement, indicators: [code, name, weight, unit, direction, metric|null, target]]]
function edsStandards(u, f) {
  const smk = u.level === 'SMK'
  return [
    ['SKL', 'Standar Kompetensi Lulusan', 3500, [
      ['SKL-1', 'Peserta didik hadir dan disiplin mengikuti pembelajaran.', [
        ['SKL-1.1', 'Rata-rata kehadiran peserta didik', 2000, 'percent_bp', 'higher_better', 'attendance_rate', 9500]]],
      ['SKL-2', 'Peserta didik meraih prestasi akademik dan non-akademik.', [
        ['SKL-2.1', 'Prestasi terverifikasi per semester', 1500, 'count', 'higher_better', 'verified_achievements', smk ? 8 : 6]]],
      smk
        ? ['SKL-3', 'Lulusan terserap di dunia kerja, melanjutkan studi, atau berwirausaha (BMW).', [
          ['SKL-3.1', 'Keterserapan lulusan tahun lalu (BMW)', 1500, 'percent_bp', 'higher_better', null, 8500]]]
        : ['SKL-3', 'Peserta didik mencapai kompetensi minimum literasi dan numerasi.', [
          ['SKL-3.1', 'Peserta didik kelas 5 di atas kompetensi minimum literasi (Rapor Pendidikan)', 1500, 'percent_bp', 'higher_better', null, 7500]]],
    ]],
    ['PRS', 'Standar Proses', 2500, [
      ['PRS-1', 'Guru merencanakan pembelajaran dengan Rencana Pembelajaran Mendalam (RPM).', [
        ['PRS-1.1', 'Guru dengan RPM terbit untuk semester berjalan', 1500, 'percent_bp', 'higher_better', null, 9000]]],
      smk
        ? ['PRS-2', 'Pembelajaran kejuruan selaras dengan dunia usaha dan dunia industri (DUDI).', [
          ['PRS-2.1', 'Mitra DUDI aktif (PKL, guru tamu, kelas industri)', 1000, 'count', 'higher_better', null, 10]]]
        : ['PRS-2', 'Pembelajaran berdiferensiasi sesuai tahap perkembangan peserta didik.', [
          ['PRS-2.1', 'Kelas yang menerapkan asesmen diagnostik awal', 1000, 'count', 'higher_better', null, 6]]],
    ]],
    ['PTK', 'Standar Pendidik dan Tenaga Kependidikan', 2000, [
      ['PTK-1', 'Pendidik mengikuti pengembangan keprofesian berkelanjutan.', [
        ['PTK-1.1', 'Guru menuntaskan minimal 32 JP pelatihan per tahun', 2000, 'percent_bp', 'higher_better', null, 8000]]],
    ]],
    ['PBY', 'Standar Pembiayaan', 2000, [
      ['PBY-1', 'Pembiayaan operasional sekolah terkelola dan berkelanjutan.', [
        ['PBY-1.1', 'Kolektibilitas SPP bulanan', 2000, 'percent_bp', 'higher_better', 'collection_rate', 9200]]],
    ]],
  ]
}

const IASP = [
  ['IASP-1', 'Kinerja Pendidik dalam Mengelola Proses Pembelajaran', 3500, 'Pendidik menciptakan pembelajaran yang aman, nyaman, dan bermakna.'],
  ['IASP-2', 'Kepemimpinan Kepala Sekolah dalam Pengelolaan Satuan Pendidikan', 2500, 'Kepala sekolah memimpin perencanaan berbasis data dan refleksi.'],
  ['IASP-3', 'Iklim Lingkungan Belajar', 2000, 'Satuan pendidikan aman, inklusif, dan bebas perundungan.'],
  ['IASP-4', 'Mutu Lulusan', 2000, 'Lulusan menunjukkan karakter Profil Pelajar Pancasila dan kompetensi sesuai jenjang.'],
]
const RUBRIC = [{ desc: 'Sangat Baik — praktik baik konsisten dan terdokumentasi', score: 4 }, { desc: 'Baik — praktik berjalan, dokumentasi sebagian', score: 3 },
  { desc: 'Cukup — praktik belum konsisten', score: 2 }, { desc: 'Kurang — belum ada praktik', score: 1 }]

const score = (actual, target, dir = 'higher_better') => {
  if (!target) return 80
  const ratio = dir === 'higher_better' ? actual / target : target / Math.max(actual, 1)
  return Math.max(40, Math.min(100, Math.round(ratio * 85)))
}

export function generate() {
  const ap = approvals()
  const out = []
  for (const u of MODEL.filter((x) => x.schoolId !== SMP)) {
    const r = rng('quality:' + u.key)
    const S = u.schoolId
    const Y = u.yearId
    const f = facts(u)
    const smk = u.level === 'SMK'
    const id = (...k) => uid('qa', u.key, ...k)
    out.push(`-- ===== ${u.level} =====`)

    // ---- instruments ----
    const instruments = [
      { id: id('inst', 'eds-2026'), school_id: S, code: 'EDS-SPMI-TPGS', name: 'Instrumen EDS SPMI', issuer: 'Yayasan Teratai Putih', version_label: 'v2026.1',
        score_min: 0, score_max: 100, status: 'active', effective_from: '2026-07-01', created_at: at('2026-06-17') },
      { id: id('inst', 'eds-2025'), school_id: S, code: 'EDS-SPMI-TPGS', name: 'Instrumen EDS SPMI', issuer: 'Yayasan Teratai Putih', version_label: 'v2025.1',
        score_min: 0, score_max: 100, status: 'retired', effective_from: '2025-07-01', created_at: at('2025-06-22') },
      { id: id('inst', 'iasp'), school_id: S, code: 'INST-BANSM-2026', name: 'Instrumen Akreditasi Satuan Pendidikan (IASP)', issuer: 'BAN-S/M', version_label: 'v2026.1',
        score_min: 0, score_max: 100, status: 'active', effective_from: '2026-01-01', created_at: at('2026-07-20') },
    ]
    const standards = []
    const items = []
    const indicators = [] // + _target, _metric for the evaluation below
    const addTree = (instId, tree, prefix) => tree.forEach(([sc, sn, sw, its], si) => {
      const sid = id('std', prefix, sc)
      standards.push({ id: sid, school_id: S, instrument_id: instId, sort: si + 1, code: sc, name: sn, weight_bp: sw, created_at: at('2026-06-17') })
      its.forEach(([ic, st, inds], ii) => {
        const iid = id('item', prefix, ic)
        items.push({ id: iid, school_id: S, standard_id: sid, sort: ii + 1, code: ic, statement: st, created_at: at('2026-06-17') })
        inds.forEach(([code, name, w, unit, dir, metric, target], k) => indicators.push({ id: id('ind', prefix, code), school_id: S, item_id: iid, sort: k + 1, code, name,
          weight_bp: w, unit, direction: dir, source_kind: metric ? 'metric' : 'manual', metric_key: metric, rubric: unit === 'score' ? RUBRIC : [],
          created_at: at('2026-06-17'), _target: target, _prefix: prefix }))
      })
    })
    const eds = edsStandards(u, f)
    addTree(instruments[0].id, eds, 'eds26')
    addTree(instruments[1].id, eds.map(([c, n, w, its]) => [c, n, w, its.map(([ic, st, inds]) => [ic, st, inds.map((x) => [...x.slice(0, 6), x[6]])])]), 'eds25')
    addTree(instruments[2].id, IASP.map(([c, n, w, st]) => [c, n, w, [[`${c}.1`, st, [[`${c}.1.a`, n, w, 'score', 'higher_better', null, 3]]]]]), 'iasp')
    const clean = (x) => Object.fromEntries(Object.entries(x).filter(([k]) => !k.startsWith('_')))
    out.push(insert('qa_instruments', instruments, { conflict: '(school_id, code, version_label) DO NOTHING' }), insert('qa_standards', standards), insert('qa_items', items),
      insert('qa_indicators', indicators.map(clean)))

    // ---- targets (this year's EDS) ----
    const eds26 = indicators.filter((i) => i._prefix === 'eds26')
    const owners = { 'SKL-1.1': u.classes[0].homeroom.id, 'SKL-2.1': ACCOUNTS.kepala_sekolah, 'SKL-3.1': MUTU, 'PRS-1.1': MUTU, 'PRS-2.1': smk ? KEPSEK : u.classes[0].homeroom.id,
      'PTK-1.1': ACCOUNTS.hr_manager, 'PBY-1.1': ACCOUNTS.finance_manager }
    out.push(insert('qa_targets', eds26.map((i) => ({ id: id('target', i.code), school_id: S, indicator_id: i.id, academic_year_id: Y, target_value: i._target,
      owner_user_id: owners[i.code] ?? MUTU, created_at: at('2026-07-01') })), { conflict: '(school_id, indicator_id, academic_year_id) DO NOTHING' }))

    // ---- self-evaluations ----
    const ach = f.achievementsTotal
    const att = (m) => f.attendance[m]?.rate ?? 9400
    const col = (m) => f.billing[m]?.rate ?? 9000
    const actuals = {
      awal: { 'SKL-1.1': att('2026-07'), 'SKL-2.1': Math.max(1, Math.round(ach / 3)), 'SKL-3.1': smk ? 8100 : 7120, 'PRS-1.1': 6500, 'PRS-2.1': smk ? 7 : 4,
        'PTK-1.1': 3500, 'PBY-1.1': col(7) },
      tengah: { 'SKL-1.1': att('2026-09'), 'SKL-2.1': ach, 'SKL-3.1': smk ? 8100 : 7120, 'PRS-1.1': 8200, 'PRS-2.1': smk ? 9 : 6, 'PTK-1.1': 6200, 'PBY-1.1': col(9) },
    }
    const notes = {
      'SKL-1.1': `Kehadiran dihitung dari presensi harian ${u.level}.`,
      'SKL-2.1': ach ? `${ach} prestasi terverifikasi di modul Kesiswaan.` : 'Belum ada prestasi terverifikasi.',
      'SKL-3.1': smk ? 'Data tracer study lulusan 2026 (BKK): 81% bekerja/melanjutkan/berwirausaha.' : 'Rapor Pendidikan 2026: literasi 71,2% di atas kompetensi minimum.',
      'PRS-1.1': 'Rekap RPM terbit dari modul Perangkat Ajar.',
      'PTK-1.1': 'Rekap jam pelatihan dari modul Pelatihan SDM.',
      'PBY-1.1': 'Kolektibilitas dari modul Billing (pembayaran teralokasi / tagihan).',
    }
    const evals = [
      { key: 'awal', label: 'Evaluasi Awal Tahun 2026/2027', from: '2026-07-13', to: '2026-08-14', status: 'finalized', created: '2026-08-14' },
      { key: 'tengah', label: 'Evaluasi Tengah Semester Ganjil 2026/2027', from: '2026-07-13', to: '2026-09-30', status: 'pending_finalization', created: '2026-09-30' },
      { key: 'akhir', label: 'Evaluasi Akhir Semester Ganjil 2026/2027', from: '2026-10-01', to: '2026-12-18', status: 'draft', created: '2026-10-14' },
    ]
    const selfEvals = []
    const evalItems = []
    const plans = []
    for (const e of evals) {
      const eid = id('eval', e.key)
      let approvalId = null
      if (e.status !== 'draft') {
        approvalId = ap.add({ school: S, key: `qa-final:${u.key}:${e.key}`, type: 'qa_self_eval_finalize', service: 'quality', sourceId: eid, title: `Finalisasi ${e.label}`,
          summary: { period: e.label, instrument: 'Instrumen EDS SPMI v2026.1', indicators: eds26.length }, maker: MUTU, steps: [['kepala_sekolah', 48]],
          decisions: e.status === 'finalized' ? [{ decision: 'approved', at: at('2026-08-21', '10:00'), comment: 'Disetujui; tindak lanjuti RPM dan pelatihan guru.' }] : [],
          createdAt: at(e.key === 'awal' ? '2026-08-15' : '2026-10-12', '08:30') })
      }
      selfEvals.push({ id: eid, school_id: S, instrument_id: instruments[0].id, academic_year_id: Y, period_label: e.label, period_from: e.from, period_to: e.to,
        status: e.status, approval_id: e.status === 'pending_finalization' ? approvalId : null, finalized_at: e.status === 'finalized' ? at('2026-08-21', '10:00') : null,
        finalized_by: e.status === 'finalized' ? KEPSEK : null, version: e.status === 'finalized' ? 1 : 0, created_at: at(e.created), updated_at: at(e.status === 'draft' ? e.created : '2026-10-12') })
      const vals = actuals[e.key]
      for (const ind of eds26) {
        if (!vals) {
          // The draft has the metric indicators pulled in, the manual ones still open.
          if (ind.source_kind !== 'metric') continue
        }
        const actual = vals ? vals[ind.code] : ind.code === 'SKL-1.1' ? att('2026-10') : ind.code === 'PBY-1.1' ? col(10) : ach
        const below = ind.direction === 'higher_better' ? actual < ind._target : actual > ind._target
        const itemId = id('eval-item', e.key, ind.code)
        evalItems.push({ id: itemId, school_id: S, self_evaluation_id: eid, indicator_id: ind.id, target_value: ind._target, actual_value: actual,
          value_source: ind.source_kind, metric_as_of: ind.source_kind === 'metric' ? at(e.to > '2026-10-14' ? '2026-10-14' : e.to, '01:00') : null,
          metric_fetched_at: ind.source_kind === 'metric' ? at(e.to > '2026-10-14' ? '2026-10-14' : e.to, '01:00') : null,
          score: vals ? score(actual, ind._target, ind.direction) : null, below_target: below, note: notes[ind.code] ?? null,
          assessed_by: vals ? (ind.source_kind === 'metric' ? MUTU : owners[ind.code] ?? MUTU) : null, created_at: at(e.created), updated_at: at(e.created) })
        if (below && e.key !== 'akhir') {
          const action = {
            'SKL-1.1': 'Program jemput sapa wali kelas untuk peserta didik dengan kehadiran < 90%.',
            'SKL-2.1': 'Bentuk tim pembina lomba (OSN, FLS2N, O2SN) dan jadwalkan seleksi internal.',
            'SKL-3.1': smk ? 'Perluas kerja sama BKK dengan 5 perusahaan mitra baru di kawasan industri Bekasi.' : 'Gerakan membaca 15 menit sebelum pelajaran dan pojok baca kelas.',
            'PRS-1.1': 'Pendampingan penyusunan RPM per KKG/MGMP setiap Jumat sore.',
            'PRS-2.1': smk ? 'Tambah kelas industri bersama mitra DUDI jurusan AKL.' : 'Pelatihan asesmen diagnostik untuk guru kelas 1-3.',
            'PTK-1.1': 'Daftarkan seluruh guru pada kursus wajib Kurikulum Merdeka di modul Pelatihan.',
            'PBY-1.1': 'Hubungi keluarga penunggak > 30 hari dan tawarkan skema cicilan.',
          }[ind.code]
          const done = e.key === 'awal' && r.chance(0.6)
          plans.push({ id: id('plan', e.key, ind.code), school_id: S, self_eval_item_id: itemId, action, owner_user_id: owners[ind.code] ?? MUTU,
            due_date: e.key === 'awal' ? '2026-09-30' : '2026-11-30', status: done ? 'done' : e.key === 'awal' ? 'in_progress' : 'open',
            done_at: done ? at('2026-09-26', '14:00') : null, created_at: at(e.key === 'awal' ? '2026-08-21' : '2026-10-12'), updated_at: at('2026-10-12') })
        }
      }
    }
    out.push(insert('qa_self_evaluations', selfEvals, { conflict: '(school_id, instrument_id, academic_year_id, period_label) DO NOTHING' }),
      insert('qa_self_eval_items', evalItems, { conflict: '(self_evaluation_id, indicator_id) DO NOTHING' }), insert('qa_action_plans', plans),
      insert('qa_self_eval_versions', [{ id: id('version', 'awal', 1), school_id: S, self_evaluation_id: id('eval', 'awal'), version: 1,
        snapshot: { comment: 'Disetujui; tindak lanjuti RPM dan pelatihan guru.', version: 1, decided_by: KEPSEK, finalized_at: at('2026-08-21', '10:00'),
          items: evalItems.filter((i) => i.self_evaluation_id === id('eval', 'awal')).map((i) => ({ indicator_id: i.indicator_id, target: i.target_value, actual: i.actual_value, score: i.score })) },
        finalized_at: at('2026-08-21', '10:00'), finalized_by: KEPSEK, created_at: at('2026-08-21', '10:00') }], { conflict: '(self_evaluation_id, version) DO NOTHING' }))

    // ---- evidence ----
    const ind = (code, prefix = 'eds26') => id('ind', prefix, code)
    const evidence = [
      ['sekoci_report', 'Rekap Kehadiran Peserta Didik Juli-September 2026', null, { metric_key: 'attendance_rate', period_from: '2026-07-01', period_to: '2026-09-30' }, null, 'verified', [ind('SKL-1.1')]],
      ['sekoci_report', 'Rekap Kolektibilitas SPP Semester Ganjil', null, { metric_key: 'collection_rate', period_from: '2026-07-01', period_to: '2026-09-30' }, null, 'verified', [ind('PBY-1.1')]],
      ['file', `Kumpulan RPM Guru ${u.level} Semester Ganjil 2026.pdf`, `qa/evidence/${u.key}/rpm-semester-ganjil-2026.pdf`, null, null, 'verified', [ind('PRS-1.1'), ind('IASP-1.1.a', 'iasp')]],
      ['file', 'Sertifikat Pelatihan Guru 2026.pdf', `qa/evidence/${u.key}/sertifikat-pelatihan-2026.pdf`, null, null, 'pending', [ind('PTK-1.1')]],
      smk
        ? ['file', 'Laporan Tracer Study Lulusan 2026 (BKK).pdf', `qa/evidence/${u.key}/tracer-study-2026.pdf`, null, null, 'verified', [ind('SKL-3.1'), ind('IASP-4.1.a', 'iasp')]]
        : ['link', 'Rapor Pendidikan 2026 — SD Teratai Putih', null, null, 'https://raporpendidikan.kemdikbud.go.id/', 'verified', [ind('SKL-3.1'), ind('IASP-4.1.a', 'iasp')]],
      ['file', 'Notulen Rapat Kerja Kepala Sekolah Juli 2026.pdf', `qa/evidence/${u.key}/notulen-raker-2026.pdf`, null, null, 'rejected', [ind('IASP-2.1.a', 'iasp')]],
      ['link', 'Dokumentasi Program Anti-Perundungan (Roots)', null, null, 'https://cerdasberkarakter.kemdikbud.go.id/', 'pending', [ind('IASP-3.1.a', 'iasp')]],
    ].map(([kind, title, key, ref, url, status, links], i) => ({ id: id('evidence', i), school_id: S, kind, title, object_key: key, mime: key ? 'application/pdf' : null,
      size_bytes: key ? 250000 + i * 73000 : null, checksum: key ? uid('qa-sum', u.key, i).replace(/-/g, '') : null, report_ref: ref, url, version: 1, uploaded_by: i % 2 ? MUTU : ADMIN,
      status, verified_by: status === 'pending' ? null : KEPSEK, verified_at: status === 'pending' ? null : at('2026-10-0' + (3 + (i % 5))),
      reject_reason: status === 'rejected' ? 'Notulen belum ditandatangani kepala sekolah; unggah ulang versi bertanda tangan.' : null,
      created_at: at('2026-10-0' + (1 + (i % 7))), _links: links }))
    out.push(insert('qa_evidence', evidence.map(clean)),
      insert('qa_evidence_links', evidence.flatMap((e) => e._links.map((l) => ({ evidence_id: e.id, indicator_id: l, academic_year_id: Y, created_at: e.created_at })))))

    // ---- regulatory reports ----
    const office = smk ? 'Kantor Cabang Dinas Pendidikan Wilayah III Provinsi Jawa Barat' : 'Dinas Pendidikan Kota Bekasi'
    const templates = [
      ['REKAP-PD-ROMBEL', 'Rekap Peserta Didik per Rombel', office, 'semester', [{ label: 'Jumlah Peserta Didik Aktif', metric_key: 'active_students' },
        { label: 'Rata-rata Kehadiran', group_by: 'grade', metric_key: 'attendance_rate' }], 'xlsx'],
      ['KEU-BULANAN', 'Laporan Keuangan Bulanan Yayasan', 'Pengurus Yayasan Teratai Putih', 'monthly', [{ label: 'Total Tagihan', metric_key: 'billed_idr' },
        { label: 'Total Penerimaan', metric_key: 'paid_idr' }, { label: 'Kolektibilitas', metric_key: 'collection_rate' }], 'xlsx'],
      ['PTK-TAHUNAN', 'Laporan Tahunan Pendidik & Tenaga Kependidikan', office, 'annual', [{ label: 'Guru Aktif', metric_key: 'active_teachers' },
        { label: 'Tenaga Kependidikan Aktif', metric_key: 'active_staff' }], 'csv'],
    ].map(([code, name, recipient, frequency, columns, fmt]) => ({ id: id('template', code), school_id: S, code, name, recipient, frequency, columns, export_format: fmt,
      status: 'active', created_at: at('2026-05-18') }))
    out.push(insert('regulatory_templates', templates, { conflict: '(school_id, code) DO NOTHING' }))
    const tpl = Object.fromEntries(templates.map((t) => [t.code, t]))
    const col_ = (label, key, unit, value, asOf) => ({ unit, as_of: at(asOf, '01:00'), label, value, metric_key: key, unavailable: false })
    const reports = []
    const report = (k, code, from, to, status, data, extra = {}) => reports.push({ id: id('report', k), school_id: S, template_id: tpl[code].id, period_from: from, period_to: to,
      status, template_snapshot: status === 'draft' ? null : tpl[code], data_snapshot: data, snapshot_at: data ? at(to < '2026-10-13' ? to : '2026-10-13', '10:00') : null,
      idempotency_key: `demo:qa-report:${u.key}:${k}`, created_at: at(to < '2026-10-12' ? to : '2026-10-12'), ...extra })
    const b8 = f.billing[8] ?? { billed: 0, paid: 0, rate: 0 }
    const b9 = f.billing[9] ?? { billed: 0, paid: 0, rate: 0 }
    report('keu-08', 'KEU-BULANAN', '2026-08-01', '2026-08-31', 'submitted', { columns: [col_('Total Tagihan', 'billed_idr', 'idr', b8.billed, '2026-09-01'),
      col_('Total Penerimaan', 'paid_idr', 'idr', b8.paid, '2026-09-01'), col_('Kolektibilitas', 'collection_rate', 'percent_bp', b8.rate, '2026-09-01')] },
    { submitted_at: at('2026-09-08', '10:00'), submitted_by: ADMIN, submission_note: 'Dikirim ke bendahara yayasan melalui email resmi.', export_object_key: `quality/reports/${u.key}/keu-2026-08.xlsx` })
    const keu9 = id('report', 'keu-09')
    const keu9App = ap.add({ school: S, key: `qa-report:${u.key}:keu-09`, type: 'regulatory_report', service: 'quality', sourceId: keu9,
      title: 'Persetujuan Laporan Keuangan Bulanan September 2026', summary: { template: 'KEU-BULANAN', period: 'September 2026' }, maker: ADMIN,
      steps: [['kepala_sekolah', 48]], decisions: [{ decision: 'approved', at: at('2026-10-06', '09:15'), comment: 'Sesuai rekap billing.' }], createdAt: at('2026-10-05', '10:00') })
    report('keu-09', 'KEU-BULANAN', '2026-09-01', '2026-09-30', 'approved', { columns: [col_('Total Tagihan', 'billed_idr', 'idr', b9.billed, '2026-10-01'),
      col_('Total Penerimaan', 'paid_idr', 'idr', b9.paid, '2026-10-01'), col_('Kolektibilitas', 'collection_rate', 'percent_bp', b9.rate, '2026-10-01')] }, { approval_id: keu9App })
    const pd = id('report', 'pd-ganjil')
    const pdApp = ap.add({ school: S, key: `qa-report:${u.key}:pd-ganjil`, type: 'regulatory_report', service: 'quality', sourceId: pd,
      title: 'Persetujuan Laporan Rekap Peserta Didik Semester Ganjil', summary: { template: 'REKAP-PD-ROMBEL', recipient: office }, maker: DAPODIK,
      steps: [['kepala_sekolah', 48]], createdAt: at('2026-10-13', '09:00') })
    report('pd-ganjil', 'REKAP-PD-ROMBEL', '2026-07-01', '2026-09-30', 'pending_approval', { columns: [col_('Jumlah Peserta Didik Aktif', 'active_students', 'count', f.students, '2026-10-01'),
      col_('Rata-rata Kehadiran', 'attendance_rate', 'percent_bp', att('2026-09'), '2026-10-01')] }, { approval_id: pdApp })
    report('ptk-2026', 'PTK-TAHUNAN', '2026-01-01', '2026-12-31', 'draft', null)
    out.push(insert('regulatory_reports', reports, { conflict: '(school_id, idempotency_key) DO NOTHING' }))

    // ---- Dapodik reconciliation ----
    out.push(cloneConfig('dapodik_field_mappings', S))
    const up = (s) => s.toUpperCase()
    const dob = (d) => d.split('-').reverse().join('/')
    const pick = [u.students[3], u.students[25], u.students[47], u.students[66], u.students[88]]
    const imp = { student: id('import', 'student'), ptk: id('import', 'ptk'), rombel: id('import', 'rombel') }
    const rows = pick.map((s, i) => {
      const cls = u.classes.find((c) => c.id === s.classId)
      const rombel = i === 2 ? u.classes[u.classes.indexOf(cls) + 1]?.name ?? cls.name : cls.name
      return { id: id('import-row', i), school_id: S, import_id: imp.student, row_no: i + 2, data: { NIK: '', NISN: s.nisn, Nama: up(s.name), 'Tanggal Lahir': dob(s.birthDate), 'Rombel Saat Ini': up(rombel) },
        created_at: at('2026-10-01', '10:20') }
    })
    rows.push({ id: id('import-row', 9), school_id: S, import_id: imp.student, row_no: 7, data: { NIK: '', NISN: String(3129000000 + (smk ? 77 : 11)), Nama: smk ? 'RAFI ADITYA NUGRAHA' : 'KIRANA AYU LESTARI',
      'Tanggal Lahir': smk ? '14/03/2010' : '22/08/2018', 'Rombel Saat Ini': up(u.classes[1].name) }, created_at: at('2026-10-01', '10:20') })
    const moved = u.students[110]
    const wrongClass = u.classes.find((c) => c.id === pick[2].classId)
    out.push(insert('dapodik_imports', [
      { id: imp.student, school_id: S, entity: 'student', file_name: 'dapodik_peserta_didik_20261001.xlsx', file_checksum: uid('dpd-sum', u.key, 1).replace(/-/g, '').padEnd(64, '0'), row_count: rows.length,
        imported_by: DAPODIK, status: 'compared', created_at: at('2026-10-01', '10:20'), updated_at: at('2026-10-02', '10:20') },
      { id: imp.ptk, school_id: S, entity: 'ptk', file_name: 'dapodik_ptk_20260815.xlsx', file_checksum: uid('dpd-sum', u.key, 2).replace(/-/g, '').padEnd(64, '0'), row_count: u.teachers.length,
        imported_by: DAPODIK, status: 'closed', created_at: at('2026-08-15', '10:20'), updated_at: at('2026-08-21', '10:20') },
      { id: imp.rombel, school_id: S, entity: 'rombel', file_name: 'dapodik_rombel_20261010.csv', file_checksum: uid('dpd-sum', u.key, 3).replace(/-/g, '').padEnd(64, '0'), row_count: u.classes.length,
        imported_by: DAPODIK, status: 'parsed', created_at: at('2026-10-10', '10:20'), updated_at: at('2026-10-10', '10:20') }]),
    insert('dapodik_import_rows', rows),
    insert('dapodik_diffs', [
      { id: id('diff', 1), school_id: S, import_id: imp.student, entity: 'student', sekoci_ref: pick[2].id, import_row_id: rows[2].id, kind: 'field_mismatch', field: 'classroom',
        sekoci_value: up(wrongClass.name), dapodik_value: rows[2].data['Rombel Saat Ini'], status: 'open', created_at: at('2026-10-01', '10:23') },
      { id: id('diff', 2), school_id: S, import_id: imp.student, entity: 'student', sekoci_ref: null, import_row_id: rows[5].id, kind: 'missing_in_sekoci', field: null,
        sekoci_value: null, dapodik_value: rows[5].data.Nama, status: 'open', created_at: at('2026-10-01', '10:23') },
      { id: id('diff', 3), school_id: S, import_id: imp.student, entity: 'student', sekoci_ref: moved.id, import_row_id: null, kind: 'missing_in_dapodik', field: null,
        sekoci_value: `${up(moved.name)} (belum diunggah ke Dapodik)`, dapodik_value: null, status: 'open', created_at: at('2026-10-01', '10:23') },
      { id: id('diff', 4), school_id: S, import_id: imp.student, entity: 'student', sekoci_ref: pick[0].id, import_row_id: rows[0].id, kind: 'field_mismatch', field: 'birth_date',
        sekoci_value: pick[0].birthDate, dapodik_value: pick[0].birthDate.slice(0, 8) + '01', status: 'decided', decision: 'update_dapodik',
        decision_reason: 'Tanggal lahir di SEKOCI sesuai akta kelahiran; Dapodik diperbaiki operator.', decided_by: DAPODIK, decided_at: at('2026-10-02', '09:00'), created_at: at('2026-10-01', '10:23') },
      { id: id('diff', 5), school_id: S, import_id: imp.ptk, entity: 'ptk', sekoci_ref: u.teachers[4].id, import_row_id: null, kind: 'field_mismatch', field: 'name',
        sekoci_value: up(u.teachers[4].name), dapodik_value: up(u.teachers[4].name) + ', S.PD', status: 'decided', decision: 'ignore',
        decision_reason: 'Perbedaan hanya gelar akademik.', decided_by: DAPODIK, decided_at: at('2026-08-21', '10:00'), created_at: at('2026-08-15', '10:23') }]),
    insert('dapodik_sync_logs', [
      ['ptk', 'import', imp.ptk, { rows: u.teachers.length }, '2026-08-15T10:20'], ['ptk', 'compare', imp.ptk, { field_mismatch: 1 }, '2026-08-15T10:23'],
      ['ptk', 'decide', imp.ptk, { ignore: 1 }, '2026-08-21T10:00'], ['ptk', 'export', imp.ptk, { rows: 0, format: 'xlsx' }, '2026-08-21T10:30'],
      ['student', 'import', imp.student, { rows: rows.length }, '2026-10-01T10:20'], ['student', 'compare', imp.student, { field_mismatch: 2, missing_in_sekoci: 1, missing_in_dapodik: 1 }, '2026-10-01T10:23'],
      ['student', 'decide', imp.student, { update_dapodik: 1 }, '2026-10-02T09:00'], ['rombel', 'import', imp.rombel, { rows: u.classes.length }, '2026-10-10T10:20'],
    ].map(([entity, action, importId, summary, t], i) => ({ id: id('sync', i), school_id: S, entity, action, actor_user_id: DAPODIK, import_id: importId, summary, created_at: `${t}:00+07:00` }))))
  }
  return merge({ teratai_quality: out.join('\n\n') }, ap.sql())
}
