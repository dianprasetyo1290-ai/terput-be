// Phase 14: BI & analytics of SD and SMK — metric snapshots computed from the
// unit's real attendance, billing, achievements, staff and PPDB data (what
// the dashboards draw before the first live refresh), refresh history, the
// unit's metric parameters, predictive analytics switched on with its risk
// rules (the insight scheduler then scores the students itself), dashboards
// for the principal and the BI analyst, saved and scheduled reports, the
// foundation head's analytics access, and a few AI-assistant conversations.
import { ACCOUNTS, at, dbRows, insert, merge, pgArray, uid } from './lib.mjs'
import { facts } from './facts.mjs'
import { MODEL } from './model.mjs'

const SMP = '11111111-1111-1111-1111-111111111111'
const { admin: ADMIN, kepala_sekolah: KEPSEK, kepala_yayasan: YAYASAN, bi_analyst: ANALIS, finance_manager: FINANCE } = ACCOUNTS
const STAFF_PER_UNIT = 5 // staff without a login of SD / SMK (06_hr.mjs)
const MONTHS = [['2026-07', 7, '2026-07-31'], ['2026-08', 8, '2026-08-31'], ['2026-09', 9, '2026-09-30']]

export function generate() {
  const out = []
  const ppdb = dbRows('teratai_admission', `SELECT w.school_id, w.name AS wave, count(*) AS n,
      count(*) FILTER (WHERE a.status IN ('reregistration','enrolled')) AS converted
    FROM applications a JOIN admission_waves w ON w.id = a.wave_id WHERE a.status <> 'draft' GROUP BY 1, 2`)
  for (const u of MODEL.filter((x) => x.schoolId !== SMP)) {
    const S = u.schoolId
    const f = facts(u)
    const id = (...k) => uid('bi', u.key, ...k)
    out.push(`-- ===== ${u.level} =====`)

    // ---- metric snapshots ----
    const asOf = at('2026-10-15', '01:00')
    const snaps = []
    const snap = (metric, start, end, value, { num = null, den = null, group = '', dim = '', kind = 'month' } = {}) =>
      snaps.push({ school_id: S, metric_key: metric, period_kind: kind, period_start: start, period_end: end, group_by: group, dim_value: dim,
        numerator: num, denominator: den, value, source_as_of: asOf, computed_at: asOf })
    for (const [m, mon, end] of MONTHS) {
      const a = f.attendance[m]
      if (a) snap('attendance_rate', `${m}-01`, end, a.rate, { num: a.present, den: a.marked })
      const b = f.billing[mon]
      if (b) {
        snap('collection_rate', `${m}-01`, end, b.rate, { num: b.paidBills, den: b.bills })
        snap('billed_idr', `${m}-01`, end, b.billed)
        snap('paid_idr', `${m}-01`, end, b.paid)
      }
    }
    for (const [g, a] of Object.entries(f.attendanceByGrade['2026-09'] ?? {})) {
      snap('attendance_rate', '2026-09-01', '2026-09-30', a.rate, { num: a.present, den: a.marked, group: 'grade', dim: g })
    }
    // Arrears by age on 15 October: each month's unpaid remainder ages from its due date (the 10th).
    const buckets = { '0-30': 10, '31-60': 9, '61-90': 8, '>90': 7 }
    for (const [bucket, mon] of Object.entries(buckets)) {
      const b = f.billing[mon]
      if (b) snap('outstanding_idr', '2026-09-01', '2026-09-30', Math.max(0, b.billed - b.paid), { group: 'aging_bucket', dim: bucket })
    }
    snap('active_students', '2026-09-01', '2026-09-30', f.students)
    snap('active_teachers', '2026-09-01', '2026-09-30', f.teachers)
    snap('active_staff', '2026-09-01', '2026-09-30', f.teachers + STAFF_PER_UNIT)
    snap('verified_achievements', '2026-09-01', '2026-09-30', f.achievementsTotal)
    for (const [level, n] of Object.entries(f.achievements)) snap('verified_achievements', '2026-09-01', '2026-09-30', n, { group: 'level', dim: level })
    for (const w of ppdb.filter((x) => x.school_id === S)) {
      snap('ppdb_applicants', '2026-07-01', '2027-06-30', Number(w.n), { kind: 'academic_year', group: 'wave', dim: w.wave })
      snap('ppdb_conversion', '2026-07-01', '2027-06-30', Math.round((Number(w.converted) * 10000) / Number(w.n)), { num: Number(w.converted), den: Number(w.n), kind: 'academic_year', group: 'wave', dim: w.wave })
    }
    out.push(insert('metric_snapshots', snaps))
    out.push(insert('metric_refresh_runs', [
      ['attendance_rate', '2026-09-01', 'ok', null, '2026-09-30T18:00', '2026-09-30T18:04'],
      ['collection_rate', '2026-09-01', 'ok', null, '2026-09-30T18:00', '2026-09-30T18:02'],
      ['active_staff', '2026-09-01', 'failed', 'Layanan hr tidak merespons dalam 30 detik', '2026-09-30T18:00', '2026-09-30T18:00'],
      ['active_staff', '2026-09-01', 'ok', null, '2026-10-01T06:00', '2026-10-01T06:01'],
      ['attendance_rate', '2026-10-01', 'ok', null, '2026-10-14T18:00', '2026-10-14T18:03'],
    ].map(([metric, period, status, error, s, e], i) => ({ id: id('refresh', i), school_id: S, metric_key: metric, period_start: period, status, error,
      started_at: `${s}:00+00:00`, finished_at: `${e}:30+00:00` }))))
    out.push(insert('metric_school_params', [
      { id: id('param', 'att'), school_id: S, metric_key: 'attendance_rate', params: { hari_efektif_per_minggu: u.level === 'SD' ? 5 : 5, hitung_terlambat_sebagai_hadir: true },
        effective_from: '2026-07-13', created_at: at('2026-07-12') },
      { id: id('param', 'col'), school_id: S, metric_key: 'collection_rate', params: { tanggal_jatuh_tempo: 10, abaikan_tagihan_dispensasi: true }, effective_from: '2026-07-01',
        created_at: at('2026-07-07') }], { conflict: '(school_id, metric_key, effective_from) DO NOTHING' }))

    // ---- predictive analytics ----
    out.push(insert('insight_settings', [{ school_id: S, predictive_enabled: true, enabled_by: YAYASAN, enabled_at: at('2026-09-01'),
      review_note: 'Diaktifkan untuk uji coba satu semester; hasil hanya bahan tinjauan wali kelas dan keuangan, bukan dasar keputusan otomatis.', created_at: at('2026-09-01') }]))
    const rules = [
      ['arrears_risk', { min_amount: u.level === 'SD' ? 450000 : 700000, threshold_days: 30 }],
      ['absence_risk', { consecutive_days: 3, min_attendance_pct: 85 }],
      ['ppdb_forecast', { seasonality: 'wave', lookback_years: 2 }],
      ['cashflow_forecast', { include_ppdb: true, horizon_months: 3 }],
    ].map(([model, params]) => ({ id: id('rule', model), school_id: S, model, version: 1, params, effective_from: '2026-09-01', created_by: ANALIS, status: 'active',
      created_at: at('2026-09-01') }))
    out.push(insert('risk_rules', rules, { conflict: '(school_id, model, version) DO NOTHING' }))
    out.push(insert('prediction_accuracy', [
      ['arrears_risk', 'precision_bp', 7400, 58], ['arrears_risk', 'recall_bp', 6900, 58], ['absence_risk', 'precision_bp', 8100, 31], ['cashflow_forecast', 'mape_bp', 720, 3],
    ].map(([model, measure, v, n], i) => ({ id: id('accuracy', i), school_id: S, model, rule_id: id('rule', model), period_start: '2026-07-01', period_end: '2026-09-30',
      measure, value_bp: v, sample_size: n, computed_at: at('2026-10-01', '07:00') }))))

    // ---- dashboards ----
    const W = (title, metric, chart, group, period) => ({ title, metric, chart, group, period })
    const dashboards = [
      ['kepsek', KEPSEK, `Dasbor Kepala Sekolah ${u.level}`, 'unit', ['kepala_sekolah', 'admin', 'kepala_yayasan'], [
        W('Total Peserta Didik Aktif', 'active_students', 'stat', null, { kind: 'latest' }),
        W('Tingkat Kehadiran Peserta Didik', 'attendance_rate', 'line', 'month', { n: 3, kind: 'last_n_months' }),
        W('Kehadiran per Tingkat', 'attendance_rate', 'bar', 'grade', { kind: 'latest' }),
        W('Kolektibilitas SPP', 'collection_rate', 'stat', null, { kind: 'current_month' }),
        W('Prestasi Terverifikasi per Tingkat', 'verified_achievements', 'bar', 'level', { kind: 'latest' })]],
      ['keu', ANALIS, `Analitik Keuangan ${u.level}`, 'unit', ['finance_manager', 'kepala_sekolah', 'bi_analyst'], [
        W('Kolektibilitas SPP Bulanan', 'collection_rate', 'line', 'month', { n: 3, kind: 'last_n_months' }),
        W('Tagihan Diterbitkan', 'billed_idr', 'bar', 'month', { n: 3, kind: 'last_n_months' }),
        W('Penerimaan Kas', 'paid_idr', 'bar', 'month', { n: 3, kind: 'last_n_months' }),
        W('Tunggakan per Umur', 'outstanding_idr', 'bar', 'aging_bucket', { kind: 'latest' })]],
      ['ppdb', ANALIS, `Pantauan PPDB ${u.level} 2027/2028`, 'unit', ['kepala_sekolah', 'admin', 'humas'], [
        W('Pendaftar per Gelombang', 'ppdb_applicants', 'bar', 'wave', { kind: 'academic_year' }),
        W('Konversi Daftar Ulang', 'ppdb_conversion', 'table', 'wave', { kind: 'academic_year' })]],
    ]
    out.push(insert('dashboards', dashboards.map(([k, owner, title, scope, roles]) => ({ id: id('dash', k), school_id: S, owner_user_id: owner, title, scope_kind: scope,
      shared_roles: pgArray(roles), created_at: at('2026-09-02'), updated_at: at('2026-10-05') }))))
    out.push(insert('dashboard_widgets', dashboards.flatMap(([k, , , , , widgets]) => widgets.map((w, i) => ({ id: id('widget', k, i), school_id: S, dashboard_id: id('dash', k),
      sort: i + 1, title: w.title, metric_key: w.metric, chart: w.chart, group_by: w.group, period: w.period, created_at: at('2026-09-02') })))))

    // ---- saved & scheduled reports ----
    const reports = [
      ['spp', FINANCE, `Rekap Tagihan & Tunggakan SPP ${u.level}`, ['billed_idr', 'paid_idr', 'outstanding_idr', 'collection_rate'], { period: 'last_month', status: 'all' },
        ['finance_manager', 'kepala_sekolah', 'akuntan']],
      ['hadir', KEPSEK, `Kehadiran Peserta Didik per Tingkat ${u.level}`, ['attendance_rate'], { period: 'last_month', group_by: 'grade', metric_keys: ['attendance_rate'] },
        ['kepala_sekolah', 'walikelas', 'kesiswaan']],
      ['yayasan', ANALIS, `Ringkasan Bulanan ${u.level} untuk Yayasan`, ['active_students', 'attendance_rate', 'collection_rate', 'paid_idr'],
        { period: 'last_month', metric_keys: ['active_students', 'attendance_rate', 'collection_rate', 'paid_idr'] }, ['kepala_yayasan', 'bi_analyst']],
    ]
    out.push(insert('saved_reports', reports.map(([k, owner, title, columns, filters, roles]) => ({ id: id('report', k), school_id: S, owner_user_id: owner, title, columns, filters,
      shared_roles: pgArray(roles), created_at: at('2026-08-20') }))))
    const schedules = [
      ['spp', 'weekly', 1, null, 7, [FINANCE, KEPSEK], true, '2026-10-19T07:00', '2026-10-12T07:00'],
      ['yayasan', 'monthly', null, 1, 7, [YAYASAN, KEPSEK], true, '2026-11-01T07:00', '2026-10-01T07:00'],
      ['hadir', 'weekly', 5, null, 15, [KEPSEK], false, '2026-10-16T15:00', '2026-09-25T15:00'],
    ]
    out.push(insert('scheduled_reports', schedules.map(([k, freq, wd, md, hour, recips, active, next, last]) => ({ id: id('schedule', k), school_id: S, saved_report_id: id('report', k),
      frequency: freq, weekday: wd, month_day: md, send_hour: hour, recipient_user_ids: pgArray(recips, 'uuid'), active, next_run_at: `${next}:00+07:00`,
      last_run_at: `${last}:00+07:00`, created_by: ANALIS, created_at: at('2026-08-20') }))))
    const runs = []
    for (const [k, freq, , , , recips, , , last] of schedules) {
      const dates = freq === 'monthly' ? ['2026-09-01', '2026-10-01'] : k === 'spp' ? ['2026-09-28', '2026-10-05', '2026-10-12'] : ['2026-09-18', '2026-09-25']
      for (const d of dates) for (const rcp of recips) {
        const failed = k === 'spp' && d === '2026-10-05' && rcp === KEPSEK
        runs.push({ id: id('run', k, d, rcp), school_id: S, schedule_id: id('schedule', k), recipient_user_id: rcp, status: failed ? 'failed' : 'sent',
          reason: failed ? 'Email penerima tidak valid' : null, deliveries: failed ? { email: 'failed', in_app: 'sent' } : { email: 'sent', in_app: 'sent' },
          idempotency_key: `demo:bi-run:${u.key}:${k}:${d}:${rcp}`, created_at: `${d}T${last.slice(11)}:00+07:00` })
      }
    }
    out.push(insert('scheduled_report_runs', runs, { conflict: '(school_id, idempotency_key) DO NOTHING' }))
    out.push(insert('analytics_scope_grants', [{ id: id('grant', 'yayasan'), school_id: S, user_id: YAYASAN, granted_school_id: S, granted_by: ADMIN, created_at: at('2026-08-16') },
      { id: id('grant', 'analis'), school_id: S, user_id: ANALIS, granted_school_id: S, granted_by: ADMIN, created_at: at('2026-09-01') }]))

    // ---- AI assistant ----
    const a9 = f.attendance['2026-09']?.rate ?? 0
    const b9 = f.billing[9] ?? { rate: 0, billed: 0, paid: 0 }
    const pct = (bp) => (bp / 100).toFixed(1).replace('.', ',')
    const rp = (n) => 'Rp' + n.toLocaleString('id-ID')
    const conv = [
      ['kepsek', KEPSEK, 'kepala_sekolah', '2026-10-14T08:10', [
        ['user', `berapa kehadiran siswa ${u.level} bulan september?`],
        ['assistant', `Kehadiran peserta didik ${u.level} Teratai Putih bulan September 2026 sebesar ${pct(a9)}% (${f.attendance['2026-09']?.present ?? 0} dari ${f.attendance['2026-09']?.marked ?? 0} catatan presensi).`,
          [{ key: 'attendance_rate', kind: 'metric' }], [{ tool: 'metric_lookup', status: 'ok' }], 'rules', 240]]],
      ['analis', ANALIS, 'bi_analyst', '2026-10-13T10:05', [
        ['user', 'bandingkan tagihan dan penerimaan spp september'],
        ['assistant', `Tagihan SPP September ${rp(b9.billed)}, penerimaan ${rp(b9.paid)}; kolektibilitas ${pct(b9.rate)}%. Sisa tunggakan ${rp(Math.max(0, b9.billed - b9.paid))}.`,
          [{ key: 'billed_idr', kind: 'metric' }, { key: 'paid_idr', kind: 'metric' }], [{ tool: 'metric_lookup', status: 'ok' }], 'llm', 1620]]],
    ]
    out.push(insert('assistant_settings', [{ school_id: S, log_retention_days: 180, created_at: at('2026-09-01') }]))
    out.push(insert('assistant_conversations', conv.map(([k, user, role, t]) => ({ id: id('conv', k), school_id: S, user_id: user, active_role: role,
      started_at: `${t}:00+07:00`, last_message_at: `${t}:40+07:00` }))))
    out.push(insert('assistant_messages', conv.flatMap(([k, , , t, msgs]) => msgs.map(([sender, content, sources = [], tools = [], provider = null, latency = null], i) => ({
      id: id('msg', k, i), school_id: S, conversation_id: id('conv', k), sender, content, ai_generated: provider === 'llm', sources, tool_calls: tools, provider,
      latency_ms: latency, created_at: `${t}:${String(10 + i * 20).padStart(2, '0')}+07:00` })))))
  }
  return merge({ teratai_insight: out.join('\n\n') })
}
