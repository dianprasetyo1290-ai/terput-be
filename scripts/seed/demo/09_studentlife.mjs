// Phase 9: student life of the three units — health profiles (encrypted
// like services/studentlife) and UKS visits, discipline points, counseling
// cases with sessions, extracurricular members / sessions / attendance,
// events with registrations, achievements, and PKL for the SMK grade XII
// (majors, partners, placements since September, attendance and journals).
// SD / SMK first get SMP's master data.
import { addDays, date, dbRows, envLocal, fieldCrypto, insert, rng, uid } from './lib.mjs'
import { MODEL } from './model.mjs'

const SMP = '11111111-1111-1111-1111-111111111111'
const ADMIN = '22222222-2222-2222-2222-222222222222'
const UKS = 'a2600000-0000-0000-0000-00000000000b'
const KONSELOR = 'a2600000-0000-0000-0000-00000000000c'
const KESISWAAN = 'a2600000-0000-0000-0000-00000000000d'
const crypto = fieldCrypto(envLocal('STUDENTLIFE_ENC_KEY'), envLocal('STUDENTLIFE_BLIND_KEY'))
const seal = (obj, table, school, id) => crypto.encrypt(JSON.stringify(obj), `${table}:${school}:${id}`, id)
const HOLIDAYS = new Set(['2026-08-17', '2026-08-25'])

const DISCIPLINE = [['DISC-LATE', 'Terlambat Masuk Sekolah (>15 menit)', 'violation', 5], ['DISC-UNIFORM', 'Seragam / Atribut Tidak Lengkap', 'violation', 3],
  ['DISC-PHONE', 'Menggunakan Ponsel Saat Pelajaran', 'violation', 5], ['DISC-ABSENT', 'Membolos Jam Pelajaran', 'violation', 15],
  ['DISC-MERIT-ROBOT', 'Juara Lomba Akademik / Sains', 'merit', 20], ['DISC-MERIT-HELP', 'Membantu Kegiatan Sekolah', 'merit', 5]]
const COUNSELING = ['Akademik & Motivasi Belajar', 'Pribadi & Emosional', 'Karier & Perencanaan Studi', 'Sosial & Pertemanan']
const ECC = { SD: ['Pramuka Siaga', 'Tari Tradisional', 'Futsal Junior', 'Menggambar & Mewarnai'], SMP: ['Klub Robotik & Coding', 'Futsal Teratai Putih', 'Pramuka Inti', 'Paduan Suara'],
  SMK: ['Pramuka Penegak', 'Futsal', 'Klub Cisco Networking', 'English Club'] }
const KINDS = ['Lomba', 'Perkemahan', 'Seminar', 'Pentas Seni', 'Kunjungan Edukasi']
const MEDS = [['Paracetamol 500mg', 'tablet'], ['Minyak Kayu Putih 60ml', 'botol'], ['Betadine Antiseptik 30ml', 'botol'], ['Kasa Steril & Plester Cepat', 'kotak']]
const COMPLAINTS = [['Pusing dan lemas', 'Suhu 37,2°C, tensi normal', 'Istirahat 30 menit, diberi teh hangat', 'back_to_class'],
  ['Demam', 'Suhu 38,4°C', 'Kompres, paracetamol 1 tablet, orang tua dihubungi', 'sent_home'],
  ['Luka lecet saat olahraga', 'Lecet di lutut kanan ±3 cm', 'Dibersihkan, betadine, plester', 'back_to_class'],
  ['Sakit perut', 'Nyeri ulu hati, belum sarapan', 'Istirahat, diberi roti dan air hangat', 'back_to_class'],
  ['Sesak napas ringan', 'Riwayat asma, napas berbunyi', 'Inhaler milik siswa, dirujuk ke klinik terdekat', 'referred']]
const ALLERGIES = [['Udang', 'Gatal-gatal dan bentol'], ['Kacang', 'Bengkak di bibir'], ['Debu', 'Bersin dan hidung tersumbat'], ['Antibiotik golongan penisilin', 'Ruam kulit']]

const days = () => {
  const out = []
  for (let d = new Date('2026-07-13T00:00:00Z'); d <= new Date('2026-10-14T00:00:00Z'); d = addDays(d, 1)) {
    const ds = date(d)
    if (d.getUTCDay() > 0 && d.getUTCDay() < 6 && !HOLIDAYS.has(ds)) out.push(ds)
  }
  return out
}

export function generate() {
  const out = []
  const schoolDays = days()
  const byName = (table, school, col = 'name') => Object.fromEntries(dbRows('teratai_studentlife', `SELECT ${col} k, id FROM ${table} WHERE school_id = '${school}'`).map((x) => [x.k, x.id]))
  const haveProfile = new Set(dbRows('teratai_studentlife', `SELECT student_id FROM health_profiles`).map((x) => x.student_id))

  for (const u of MODEL) {
    // Each section has its own generator, so a skipped row never shifts another section.
    let r = rng('studentlife:' + u.key)
    const S = u.schoolId
    out.push(`-- ===== ${u.level} =====`)
    const ex = S === SMP
    const discEx = ex ? byName('discipline_types', S, 'code') : {}
    const catEx = ex ? byName('counseling_categories', S) : {}
    const eccEx = ex ? byName('act_extracurriculars', S) : {}
    const kindEx = ex ? byName('act_event_kinds', S) : {}
    const medEx = ex ? byName('uks_medicines', S) : {}
    const disc = Object.fromEntries(DISCIPLINE.map(([c]) => [c, discEx[c] ?? uid('sl-disc', u.key, c)]))
    const cat = Object.fromEntries(COUNSELING.map((c) => [c, catEx[c] ?? uid('sl-cat', u.key, c)]))
    const ecc = Object.fromEntries(ECC[u.level].map((c) => [c, eccEx[c] ?? uid('sl-ecc', u.key, c)]))
    const kind = Object.fromEntries(KINDS.map((c) => [c, kindEx[c] ?? uid('sl-kind', u.key, c)]))
    const med = Object.fromEntries(MEDS.map(([c]) => [c, medEx[c] ?? uid('sl-med', u.key, c)]))
    out.push(insert('studentlife_settings', [{ school_id: S, timezone: 'Asia/Jakarta' }], { conflict: '(school_id) DO NOTHING' }))
    out.push(insert('discipline_types', DISCIPLINE.filter(([c]) => !discEx[c]).map(([code, name, k, points]) => ({ id: disc[code], school_id: S, code, name, kind: k, points, active: true }))))
    out.push(insert('counseling_categories', COUNSELING.filter((c) => !catEx[c]).map((name) => ({ id: cat[name], school_id: S, name, active: true }))))
    out.push(insert('act_extracurriculars', ECC[u.level].filter((c) => !eccEx[c]).map((name, i) => ({ id: ecc[name], school_id: S, name, description: `Ekstrakurikuler ${name} ${u.level} Teratai Putih.`,
      schedule: [{ day: ['Senin', 'Rabu', 'Jumat', 'Sabtu'][i % 4], start: '14:00', end: '16:00' }], location_text: ['Lapangan', 'Aula', 'Lab Komputer', 'Ruang Kesenian'][i % 4],
      quota: 40, fee_idr: i % 2 ? 25000 : 0, full_policy: 'waitlist', active: true }))))
    out.push(insert('act_event_kinds', KINDS.filter((c) => !kindEx[c]).map((name) => ({ id: kind[name], school_id: S, name }))))
    out.push(insert('uks_medicines', MEDS.filter(([c]) => !medEx[c]).map(([name, unit]) => ({ id: med[name], school_id: S, name, unit, active: true }))))
    const periodId = ex ? byName('act_ecc_periods', S)['Pendaftaran Ekskul Semester Ganjil 2026/2027'] : uid('sl-ecc-period', u.key, 'ganjil')
    if (!ex) out.push(insert('act_ecc_periods', [{ id: periodId, school_id: S, academic_year_id: u.yearId, name: 'Pendaftaran Ekskul Semester Ganjil 2026/2027',
      opens_at: '2026-07-20T07:00:00+07:00', closes_at: '2026-08-08T23:59:00+07:00' }]))

    // ---- health ----
    const profiles = []
    const visits = []
    const visitMeds = []
    for (const st of u.students) {
      if (haveProfile.has(st.id)) continue
      r = rng('sl-profile:' + st.id)
      const id = uid('sl-profile', st.id)
      const allergies = r.chance(0.12) ? [r.pick(ALLERGIES)].map(([name, reaction]) => ({ name, reaction, is_alert: true })) : []
      const conditions = r.chance(0.05) ? [{ name: r.pick(['Asma ringan', 'Maag', 'Mimisan berulang']), note: 'Kontrol rutin; bawa obat pribadi', is_alert: true }] : []
      const parent = u.parents.find((p) => p.id === st.parentId)
      const contacts = [{ name: parent?.name ?? 'Orang tua', relationship: parent?.relationship ?? 'wali', phone: parent?.phone ?? '081200000000' }]
      const profile = { blood_type: r.pick(['A', 'B', 'AB', 'O', 'O']), rhesus: '+', allergies, conditions, routine_medications: conditions.length ? [{ name: 'Inhaler salbutamol', dose: '1-2 semprot', schedule: 'Saat sesak' }] : [] }
      const alert = { items: [...allergies.map((a) => ({ kind: 'allergy', name: a.name, detail: a.reaction })), ...conditions.map((c) => ({ kind: 'condition', name: c.name, detail: c.note }))], emergency_contacts: contacts }
      profiles.push({ id, school_id: S, student_id: st.id, profile_enc: seal(profile, 'health_profiles', S, id), alert_enc: seal(alert, 'health_profiles', S, id),
        emergency_enc: seal(contacts, 'health_profiles', S, id), verified_by: UKS, verified_at: '2026-07-20T10:00:00+07:00', version: 1 })
    }
    r = rng('sl-visits:' + u.key)
    for (let i = 0; i < 30; i++) {
      const st = u.students[r.int(0, u.students.length - 1)]
      const day = schoolDays[r.int(0, schoolDays.length - 1)]
      const [complaint, exam, action, outcome] = r.pick(COMPLAINTS)
      const id = uid('sl-visit', u.key, i)
      const hh = r.int(8, 13)
      visits.push({ id, school_id: S, student_id: st.id, visited_at: `${day}T${String(hh).padStart(2, '0')}:${String(r.int(0, 59)).padStart(2, '0')}:00+07:00`,
        left_at: `${day}T${String(hh + 1).padStart(2, '0')}:00:00+07:00`, details_enc: seal({ complaint, examination: exam, action, note: '' }, 'clinic_visits', S, id), outcome,
        referred_to_enc: outcome === 'referred' ? crypto.encrypt('Klinik Pratama Sehat Bersama', `clinic_visits:${S}:${id}`, id + 'ref') : null, handled_by: UKS,
        attendance_sync: outcome === 'back_to_class' ? 'not_needed' : 'done', parent_notified_at: outcome === 'back_to_class' ? null : `${day}T${String(hh).padStart(2, '0')}:30:00+07:00`,
        idempotency_key: `demo:visit:${id}` })
      const m = complaint.startsWith('Luka') ? med['Betadine Antiseptik 30ml'] : complaint.startsWith('Demam') ? med['Paracetamol 500mg'] : complaint.startsWith('Pusing') ? med['Minyak Kayu Putih 60ml'] : null
      if (m) visitMeds.push({ id: uid('sl-visit-med', id), school_id: S, visit_id: id, medicine_id: m, qty: 1 })
    }
    out.push(insert('health_profiles', profiles, { conflict: '(school_id, student_id) DO NOTHING' }), insert('clinic_visits', visits, { conflict: '(school_id, idempotency_key) DO NOTHING' }),
      insert('clinic_visit_medications', visitMeds))

    // ---- discipline & counseling ----
    const records = []
    r = rng('sl-discipline:' + u.key)
    for (let i = 0; i < 70; i++) {
      const st = u.students[r.int(0, u.students.length - 1)]
      const [code, name, k, points] = r.chance(0.75) ? r.pick(DISCIPLINE.filter((d) => d[2] === 'violation')) : r.pick(DISCIPLINE.filter((d) => d[2] === 'merit'))
      records.push({ id: uid('sl-disc-rec', u.key, i), school_id: S, student_id: st.id, academic_year_id: u.yearId, type_id: disc[code], kind: k, points,
        occurred_on: schoolDays[r.int(0, schoolDays.length - 1)], description: k === 'merit' ? `${name}.` : `${name}; sudah diberi teguran lisan.`,
        reporter_user_id: k === 'merit' ? KESISWAAN : u.classes.find((c) => c.id === st.classId).homeroom.id, status: 'active', idempotency_key: `demo:disc:${u.key}:${i}` })
    }
    const cases = []
    r = rng('sl-counseling:' + u.key)
    const caseMembers = []
    const sessions = []
    const sessionStudents = []
    for (let i = 0; i < 8; i++) {
      const st = u.students[r.int(0, u.students.length - 1)]
      const c = COUNSELING[i % COUNSELING.length]
      const caseId = uid('sl-case', u.key, i)
      const opened = schoolDays[r.int(5, schoolDays.length - 20)]
      const closed = i % 3 === 0
      cases.push({ id: caseId, school_id: S, student_id: st.id, category_id: cat[c], opened_by: KONSELOR, status: closed ? 'closed' : i % 3 === 1 ? 'monitoring' : 'open',
        disclose_to_homeroom: i % 2 === 0, opened_at: `${opened}T10:00:00+07:00`, closed_at: closed ? `${schoolDays[schoolDays.length - 5]}T14:00:00+07:00` : null,
        close_reason_enc: closed ? crypto.encrypt('Tujuan konseling tercapai; siswa sudah kembali fokus belajar.', `counseling_cases:${S}:${caseId}`, caseId) : null })
      caseMembers.push({ case_id: caseId, user_id: KONSELOR, member_role: 'counselor', added_by: KONSELOR, added_at: `${opened}T10:00:00+07:00` })
      for (let k = 0; k < 3; k++) {
        const sid = uid('sl-session', caseId, k)
        const when = date(addDays(new Date(`${opened}T00:00:00Z`), k * 7))
        const held = when < '2026-10-15'
        sessions.push({ id: sid, school_id: S, case_id: caseId, kind: 'individual', category_id: cat[c], counselor_user_id: KONSELOR, scheduled_at: `${when}T10:00:00+07:00`,
          held_at: held ? `${when}T10:05:00+07:00` : null, status: held ? 'held' : 'scheduled',
          notes_enc: held ? crypto.encrypt(['Siswa menceritakan kesulitan yang dialami; dibangun rasa percaya.', 'Membahas strategi belajar dan pengaturan waktu.', 'Evaluasi perkembangan; siswa menunjukkan kemajuan.'][k], `counseling_sessions:${S}:${sid}`, sid) : null,
          follow_up_plan_enc: held ? crypto.encrypt('Pertemuan lanjutan minggu depan; koordinasi dengan wali kelas.', `counseling_sessions:${S}:${sid}`, sid + 'f') : null })
        sessionStudents.push({ session_id: sid, student_id: st.id })
      }
    }
    out.push(insert('discipline_records', records), insert('counseling_cases', cases), insert('counseling_case_members', caseMembers, { conflict: 'DO NOTHING' }),
      insert('counseling_sessions', sessions), insert('counseling_session_students', sessionStudents, { conflict: 'DO NOTHING' }))

    // ---- extracurriculars ----
    const members = []
    r = rng('sl-ecc:' + u.key)
    const eccSessions = []
    const eccAttendance = []
    const coaches = []
    // Students an earlier seed already enrolled keep their member row.
    const memberEx = Object.fromEntries(dbRows('teratai_studentlife', `SELECT ecc_id || '|' || student_user_id k, id FROM act_ecc_members WHERE school_id = '${S}' AND period_id = '${periodId}'`).map((x) => [x.k, x.id]))
    ECC[u.level].forEach((name, ei) => {
      const eccId = ecc[name]
      coaches.push({ ecc_id: eccId, user_id: u.teachers[ei % u.teachers.length].id, school_id: S })
      const mine = u.students.filter((_, i) => i % 8 === ei || i % 8 === ei + 4)
      const memberIds = mine.map((st) => {
        if (memberEx[`${eccId}|${st.id}`]) return memberEx[`${eccId}|${st.id}`]
        const id = uid('sl-ecc-member', eccId, st.id)
        members.push({ id, school_id: S, ecc_id: eccId, period_id: periodId, student_user_id: st.id, status: 'active', joined_at: '2026-08-08T10:00:00+07:00' })
        return id
      })
      for (let w = 0; w < 9; w++) {
        const held = date(addDays(new Date('2026-08-10T00:00:00Z'), w * 7 + ei))
        if (held >= '2026-10-15') continue
        const sid = uid('sl-ecc-session', eccId, w)
        eccSessions.push({ id: sid, school_id: S, ecc_id: eccId, held_on: held, note: `Latihan rutin pekan ${w + 1}`, created_by: u.teachers[ei % u.teachers.length].id })
        for (const mid of memberIds) eccAttendance.push({ session_id: sid, member_id: mid, school_id: S, status: r.chance(0.88) ? 'hadir' : r.pick(['izin', 'sakit', 'alpa']) })
      }
    })
    out.push(insert('act_ecc_coaches', coaches, { conflict: 'DO NOTHING' }), insert('act_ecc_members', members, { conflict: '(ecc_id, period_id, student_user_id) DO NOTHING' }),
      insert('act_ecc_sessions', eccSessions), insert('act_ecc_attendance', eccAttendance, { conflict: 'DO NOTHING', chunk: 1500 }))

    // ---- events, registrations, achievements ----
    const events = ex ? dbRows('teratai_studentlife', `SELECT id, name, status, requires_parent_consent rc FROM act_events WHERE school_id = '${S}' AND deleted_at IS NULL`) : []
    if (!ex) {
      const defs = [['Class Meeting: Lomba Antarkelas', 'Lomba', '2026-09-24', 'completed', false], ['Seminar Literasi Digital & Anti-Perundungan', 'Seminar', '2026-10-28', 'published', false],
        [u.level === 'SD' ? 'Kunjungan Edukasi Kebun Binatang Ragunan' : 'Kunjungan Industri ke Kawasan Jababeka', 'Kunjungan Edukasi', '2026-11-20', 'published', true],
        ['Pentas Seni Akhir Tahun', 'Pentas Seni', '2026-12-17', 'draft', false]]
      for (const [name, k, d, status, consent] of defs) {
        const id = uid('sl-event', u.key, name)
        events.push({ id, name, status, rc: consent })
        out.push(insert('act_events', [{ id, school_id: S, name, kind_id: kind[k], description: `${name} untuk siswa ${u.level} Teratai Putih.`, starts_at: `${d}T08:00:00+07:00`,
          ends_at: `${d}T14:00:00+07:00`, location_text: k === 'Kunjungan Edukasi' ? 'Berangkat dari sekolah' : 'Aula Sekolah', quota: k === 'Kunjungan Edukasi' ? 60 : null, fee_idr: 0,
          requires_parent_consent: consent, consent_text: consent ? 'Saya mengizinkan anak saya mengikuti kegiatan ini.' : null, status,
          published_at: status !== 'draft' ? '2026-09-01T09:00:00+07:00' : null, created_by: 'a2600000-0000-0000-0000-00000000000e', idempotency_key: `demo:event:${id}` }],
          { conflict: '(school_id, idempotency_key) DO NOTHING' }))
      }
    }
    const regs = []
    r = rng('sl-events:' + u.key)
    for (const ev of events.filter((e) => ['published', 'completed'].includes(e.status))) {
      for (const st of u.students.filter((_, i) => i % 3 === 0)) {
        const pending = ev.rc && r.chance(0.3)
        regs.push({ id: uid('sl-reg', ev.id, st.id), school_id: S, event_id: ev.id, student_user_id: st.id, registered_by: pending ? st.id : st.parentId,
          registered_via: pending ? 'siswa' : 'orang_tua', status: pending ? 'pending_consent' : 'confirmed',
          consent_text_snapshot: ev.rc && !pending ? 'Saya mengizinkan anak saya mengikuti kegiatan ini.' : null,
          attended_at: ev.status === 'completed' ? '2026-09-24T08:15:00+07:00' : null, attendance_by: ev.status === 'completed' ? KESISWAAN : null })
      }
    }
    const achievements = []
    r = rng('sl-achievements:' + u.key)
    for (let i = 0; i < 10; i++) {
      const st = u.students[r.int(0, u.students.length - 1)]
      achievements.push({ id: uid('sl-achievement', u.key, i), school_id: S, student_user_id: st.id,
        activity_name: r.pick(['Olimpiade Sains Nasional', 'Lomba Pidato Bahasa Inggris', 'Turnamen Futsal Pelajar', 'Lomba Tari Tradisional', 'Kompetisi Robotik', 'Festival Literasi']),
        organizer: r.pick(['Dinas Pendidikan Kota Bekasi', 'Kemendikbudristek', 'Universitas Indonesia', 'PGRI Kota Bekasi']),
        level: r.pick(['kecamatan', 'kabupaten_kota', 'kabupaten_kota', 'provinsi', 'nasional']), rank_label: r.pick(['Juara 1', 'Juara 2', 'Juara 3', 'Harapan 1']),
        achieved_on: schoolDays[r.int(10, schoolDays.length - 1)], submitted_by: st.parentId, submitted_via: 'orang_tua', status: i < 8 ? 'verified' : 'pending',
        verified_by: i < 8 ? KESISWAAN : null, verified_at: i < 8 ? '2026-10-10T10:00:00+07:00' : null })
    }
    out.push(insert('act_event_registrations', regs, { conflict: 'DO NOTHING', chunk: 1000 }), insert('act_achievements', achievements))

    // ---- PKL (SMK grade XII) ----
    if (u.level === 'SMK') {
      const majors = [['TKJ', 'Teknik Komputer dan Jaringan'], ['AKL', 'Akuntansi dan Keuangan Lembaga']].map(([code, name]) => ({ id: uid('sl-major', code), school_id: S, code, name, active: true }))
      const partners = [['PT Telkom Indonesia (Witel Bekasi)', 'Telekomunikasi & Jaringan', 'TKJ'], ['PT Astra Graphia Information Technology', 'Software & IT Infrastructure', 'TKJ'],
        ['KAP Hendra & Rekan', 'Akuntansi & Audit', 'AKL'], ['Bank BJB Cabang Bekasi', 'Perbankan', 'AKL']]
        .map(([name, field, major]) => ({ id: uid('sl-partner', name), school_id: S, name, field, address: 'Bekasi, Jawa Barat', contact_name: 'HRD', contact_phone: '021-8800000', active: true, major }))
      const periodPkl = uid('sl-pkl-period', 'smk-2026-1')
      out.push(insert('majors', majors), insert('pkl_partners', partners.map(({ major, ...p }) => p)))
      out.push(insert('pkl_periods', [{ id: periodPkl, school_id: S, academic_year_id: u.yearId, name: 'PKL Gelombang 1 TA 2026/2027', starts_on: '2026-09-01', ends_on: '2026-12-11', status: 'ongoing' }]))
      out.push(insert('pkl_schemes', [{ id: uid('sl-pkl-scheme', periodPkl), school_id: S, period_id: periodPkl, industry_weight_bp: 6000, school_weight_bp: 4000 }]))
      out.push(insert('classroom_majors', u.classes.filter((c) => c.major).map((c) => ({ classroom_id: c.id, school_id: S, major_id: majors.find((m) => m.code === c.major).id })), { conflict: 'DO NOTHING' }))
      out.push(insert('pkl_quotas', partners.map((p) => ({ id: uid('sl-pkl-quota', p.id), school_id: S, partner_id: p.id, period_id: periodPkl, major_id: majors.find((m) => m.code === p.major).id, quota: 12 }))))
      const placements = []
      r = rng('sl-pkl:' + u.key)
      const pklAtt = []
      const journals = []
      for (const cls of u.classes.filter((c) => c.grade === '12')) {
        const mine = partners.filter((p) => p.major === cls.major)
        cls.students.forEach((st, i) => {
          const partner = mine[i % mine.length]
          const pid = uid('sl-placement', st.id)
          placements.push({ id: pid, school_id: S, period_id: periodPkl, partner_id: partner.id, major_id: majors.find((m) => m.code === cls.major).id, student_id: st.id,
            school_mentor_user_id: 'a2600000-0000-0000-0000-000000000013', starts_on: '2026-09-01', ends_on: '2026-12-11', status: 'active', created_by: 'a2600000-0000-0000-0000-000000000012',
            idempotency_key: `demo:placement:${pid}` })
          for (const d of schoolDays.filter((x) => x >= '2026-09-01')) {
            const status = r.chance(0.93) ? 'hadir' : r.pick(['izin', 'sakit'])
            pklAtt.push({ id: uid('sl-pkl-att', pid, d), school_id: S, placement_id: pid, work_date: d, status, check_in_at: status === 'hadir' ? `${d}T07:5${r.int(0, 9)}:00+07:00` : null,
              check_out_at: status === 'hadir' ? `${d}T16:0${r.int(0, 9)}:00+07:00` : null, confirmation: d < '2026-10-12' ? 'confirmed' : 'pending',
              confirmed_by: d < '2026-10-12' ? 'a2600000-0000-0000-0000-000000000014' : null })
            if (status === 'hadir' && r.chance(0.6)) journals.push({ id: uid('sl-pkl-journal', pid, d), school_id: S, placement_id: pid, entry_date: d,
              activity: cls.major === 'TKJ' ? r.pick(['Membantu instalasi kabel LAN di lantai 2', 'Konfigurasi access point kantor', 'Troubleshooting jaringan klien', 'Mendokumentasikan topologi jaringan', 'Maintenance server dan backup data'])
                : r.pick(['Menginput transaksi harian ke software akuntansi', 'Membantu rekonsiliasi bank', 'Menyusun arsip faktur pajak', 'Membuat laporan kas kecil', 'Mencocokkan buku besar pembantu']),
              photos: [] })
          }
        })
      }
      out.push(insert('pkl_placements', placements, { conflict: '(school_id, idempotency_key) DO NOTHING' }), insert('pkl_attendance', pklAtt, { conflict: '(placement_id, work_date) DO NOTHING', chunk: 1500 }),
        insert('pkl_journals', journals, { conflict: '(placement_id, entry_date) DO NOTHING', chunk: 1500 }))
    }
  }
  return { teratai_studentlife: out.join('\n\n') }
}
