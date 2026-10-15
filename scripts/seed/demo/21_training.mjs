// Phase 21: staff training (LMS) of SD and SMK — categories and the
// competency framework, courses (online and in-person, free and paid) with
// their materials and sessions, mandatory training per position, and the
// training assignments of every employee of the unit: mandatory ones
// (completed, in progress, not started, overdue), courses the teachers chose
// themselves, and paid certifications waiting for the HR manager's approval.
import { ACCOUNTS, approvals, at, dbRows, insert, merge, rng, uid } from './lib.mjs'
import { MODEL } from './model.mjs'

const SMP = '11111111-1111-1111-1111-111111111111'
const { training_admin: TRAINING } = ACCOUNTS

const CATEGORIES = [['pedagogik', 'Pengembangan Pedagogik & Karakter'], ['kepatuhan', 'Keselamatan & Kepatuhan'], ['layanan', 'Administrasi & Layanan'], ['kejuruan', 'Kompetensi Kejuruan & Sertifikasi']]
const COMPETENCIES = [['PED', 'Kompetensi Pedagogik'], ['PRO', 'Kompetensi Profesional'], ['SOS', 'Kompetensi Sosial'], ['KEP', 'Kompetensi Kepribadian'], ['DIG', 'Literasi Digital'], ['ADM', 'Administrasi Sekolah']]
// [key, title, objective, minutes, provider, kind, category, cost, quota, passing, status, competencies, materials, sessions]
const COURSES = (level) => [
  ['kurmer', 'Workshop Implementasi Kurikulum Merdeka & Pembelajaran Mendalam', 'Guru mampu menyusun RPM berdiferensiasi dan asesmen formatif yang bermakna.', 240,
    'Kemendikdasmen & Yayasan Teratai Putih', 'offline', 'pedagogik', 0, 40, 80, 'published', ['PED', 'PRO'],
    [['file', 'Panduan Pembelajaran dan Asesmen 2026'], ['link', 'Platform Merdeka Mengajar — Pelatihan Mandiri']], [['2026-08-15', 'Aula Serbaguna'], ['2026-11-07', 'Aula Serbaguna']]],
  ['ppksp', 'Perlindungan Anak dan Pencegahan Kekerasan di Satuan Pendidikan (PPKSP)', 'Memahami Permendikbudristek 46/2023 serta alur pelaporan dan penanganan kekerasan.', 120,
    'Kemendikdasmen', 'online', 'kepatuhan', 0, null, 80, 'published', ['SOS', 'KEP'],
    [['video', 'Mengenali Bentuk-bentuk Kekerasan di Sekolah'], ['file', 'Alur Pelaporan TPPK Sekolah']], []],
  ['ai', 'Literasi Digital dan Pemanfaatan AI untuk Pembelajaran', 'Guru mampu memanfaatkan perangkat AI secara etis untuk menyusun bahan ajar dan asesmen.', 180,
    'Internal (Tim TI Teratai)', 'online', 'pedagogik', 0, null, 70, 'published', ['DIG', 'PED'],
    [['video', 'Pengantar AI Generatif di Kelas'], ['file', 'Modul Literasi Digital untuk Guru'], ['link', 'Panduan Etika Penggunaan AI di Sekolah']], []],
  ['data', 'Keamanan Data Pribadi Peserta Didik', 'Tenaga kependidikan memahami UU PDP dan SOP pengelolaan data siswa.', 90,
    'Internal (Tim TI Teratai)', 'online', 'kepatuhan', 0, null, 80, 'published', ['ADM', 'DIG'], [['file', 'SOP Pengelolaan Data Pribadi Sekolah']], []],
  ['p3k', 'Pertolongan Pertama dan Tanggap Darurat Sekolah', 'Warga sekolah mampu melakukan pertolongan pertama dan evakuasi saat darurat.', 240,
    'PMI Kota Bekasi', 'offline', 'kepatuhan', 0, 30, 70, 'published', ['SOS'], [['file', 'Buku Saku P3K']], [['2026-10-24', 'Lapangan Upacara & Aula'], ['2026-07-18', 'Lapangan Upacara & Aula']]],
  ['layanan', 'Pelayanan Prima untuk Tenaga Kependidikan', 'Meningkatkan kualitas layanan administrasi kepada orang tua dan peserta didik.', 120,
    'Internal (Yayasan Teratai Putih)', 'online', 'layanan', 0, null, 70, 'published', ['ADM', 'SOS'], [['video', 'Standar Layanan Tata Usaha']], []],
  ...(level === 'SMK'
    ? [['mtcna', 'Sertifikasi MikroTik Certified Network Associate (MTCNA)', 'Guru produktif TKJ tersertifikasi MTCNA untuk mengajar dan membina LKS.', 1440,
      'MikroTik Academy (mitra)', 'offline', 'kejuruan', 3500000, 12, 60, 'published', ['PRO'], [['file', 'Silabus MTCNA']], [['2026-11-16', 'Bengkel Teknik Jaringan']]],
    ['bnsp', 'Sertifikasi Kompetensi Asesor & Teknisi Akuntansi (BNSP)', 'Guru produktif AKL memperoleh sertifikat kompetensi BNSP.', 960,
      'LSP Akuntansi Indonesia', 'offline', 'kejuruan', 2750000, 10, 70, 'published', ['PRO'], [['file', 'Skema Sertifikasi Teknisi Akuntansi Junior']], [['2026-12-01', 'Laboratorium Akuntansi']]]]
    : [['calistung', 'Strategi Pembelajaran Literasi & Numerasi Kelas Awal', 'Guru kelas rendah mampu menerapkan metode membaca dan berhitung yang menyenangkan.', 480,
      'Tanoto Foundation (PINTAR)', 'offline', 'pedagogik', 1500000, 25, 70, 'published', ['PED'], [['file', 'Modul Literasi Kelas Awal']], [['2026-11-21', 'Aula Serbaguna']]],
    ['abk', 'Pendidikan Inklusif: Mendampingi Anak Berkebutuhan Khusus', 'Guru mampu mengidentifikasi dan menyesuaikan pembelajaran untuk ABK.', 360,
      'Universitas Negeri Jakarta', 'online', 'pedagogik', 950000, 30, 70, 'draft', ['PED', 'SOS'], [['video', 'Mengenal Spektrum Kebutuhan Khusus']], []]]),
]

export function generate() {
  const ap = approvals()
  const out = []
  const employees = dbRows('teratai_hr', `SELECT e.school_id, e.id, e.full_name, e.user_id, e.hire_date, p.code AS position FROM employees e
    JOIN employment_assignments a ON a.employee_id = e.id JOIN positions p ON p.id = a.position_id
    WHERE e.school_id <> '${SMP}' AND e.status = 'active' ORDER BY e.employee_no`)
  for (const u of MODEL.filter((x) => x.schoolId !== SMP)) {
    const S = u.schoolId
    const id = (...k) => uid('lms', u.key, ...k)
    const pos = (c) => uid('hr-pos', u.key, c)
    out.push(`-- ===== ${u.level} =====`)
    out.push(insert('training_categories', CATEGORIES.map(([k, name]) => ({ id: id('cat', k), school_id: S, name, created_at: at('2026-06-01') }))))
    out.push(insert('competencies', COMPETENCIES.map(([code, name]) => ({ id: id('comp', code), school_id: S, code, name, created_at: at('2026-06-01') })), { conflict: '(school_id, code) DO NOTHING' }))
    const courses = COURSES(u.level)
    out.push(insert('courses', courses.map(([k, title, obj, min, provider, kind, cat, cost, quota, pass, status]) => ({ id: id('course', k), school_id: S, title, objective: obj,
      duration_minutes: min, provider, kind, category_id: id('cat', cat), cost_idr: cost, quota, passing_score: pass, status, created_at: at('2026-06-15') }))))
    out.push(insert('course_competencies', courses.flatMap(([k, , , , , , , , , , , comps]) => comps.map((c) => ({ course_id: id('course', k), competency_id: id('comp', c) })))))
    out.push(insert('course_materials', courses.flatMap(([k, , , , , , , , , , , , mats]) => mats.map(([kind, title], i) => ({ id: id('material', k, i), school_id: S, course_id: id('course', k),
      sort: i + 1, kind, title, object_key: kind === 'file' ? `training/materials/${u.key}/${k}/${i + 1}.pdf` : null,
      url: kind === 'file' ? null : kind === 'video' ? `https://www.youtube.com/watch?v=teratai${k}${i}` : 'https://guru.kemendikdasmen.go.id/', created_at: at('2026-06-15') })))))
    const sessions = courses.flatMap(([k, , , min, , , , , quota, , , , , ss]) => ss.map(([d, loc], i) => ({ id: id('session', k, i), school_id: S, course_id: id('course', k),
      starts_at: at(d, '08:00'), ends_at: new Date(new Date(at(d, '08:00')).getTime() + Math.min(min, 480) * 60000).toISOString(), location: loc, quota: quota ?? 40, created_at: at('2026-06-15') })))
    out.push(insert('course_sessions', sessions))
    const mandatory = [['GURU', 'ppksp', 30], ['GURU', 'kurmer', 60], ['GURU-BK', 'ppksp', 14], ['STAF-TU', 'data', 30], ['STAF-KEU', 'data', 30], ['STAF-TU', 'layanan', 60],
      ['PUSTAKAWAN', 'layanan', 60], ['STAF-UMUM', 'p3k', 60], ['TEKNISI', 'p3k', 60]]
    out.push(insert('mandatory_trainings', mandatory.map(([p, c, days]) => ({ id: id('mandatory', p, c), school_id: S, position_id: pos(p), course_id: id('course', c), due_days: days,
      created_at: at('2026-06-20') })), { conflict: '(position_id, course_id) DO NOTHING' }))

    // ---- assignments ----
    const rows = []
    const add = (e, c, o) => rows.push({ id: id('assign', e.id, c), school_id: S, employee_id: e.id, course_id: id('course', c), created_at: at('2026-07-15'), updated_at: at('2026-10-10'), ...o })
    for (const [i, e] of employees.filter((x) => x.school_id === S).entries()) {
      const r = rng('lms:' + e.id)
      for (const [p, c] of mandatory.filter(([p]) => p === e.position)) {
        const due = c === 'kurmer' ? '2026-11-07' : '2026-08-31'
        const x = r.next()
        const st = c === 'kurmer' ? (x < 0.55 ? 'completed' : 'not_started') : x < 0.7 ? 'completed' : x < 0.85 ? 'in_progress' : 'not_started'
        const session = c === 'kurmer' ? id('session', 'kurmer', st === 'completed' ? 0 : 1) : c === 'p3k' ? id('session', 'p3k', st === 'completed' ? 1 : 0) : null
        add(e, c, { session_id: session, source: 'position', due_date: c === 'p3k' ? '2026-10-24' : due, status: st,
          started_at: st !== 'not_started' ? at(c === 'kurmer' ? '2026-08-15' : `2026-08-${String(3 + (i % 20)).padStart(2, '0')}`, '19:00') : null,
          completed_at: st === 'completed' ? at(c === 'kurmer' ? '2026-08-15' : `2026-08-${String(4 + (i % 20)).padStart(2, '0')}`, c === 'kurmer' ? '12:00' : '20:15') : null,
          score: st === 'completed' ? r.int(80, 100) : null })
      }
      if (e.position.startsWith('GURU') && r.chance(0.6)) {
        const done = r.chance(0.4)
        add(e, 'ai', { source: 'self', due_date: '2026-11-30', status: done ? 'completed' : 'in_progress', started_at: at('2026-09-20', '19:30'),
          completed_at: done ? at('2026-10-04', '21:00') : null, score: done ? r.int(70, 98) : null })
      }
    }
    // Paid courses: teachers of the matching subjects ask to join.
    const subjectTeacher = (name) => u.subjects.find((s) => s.name === name)?.teacher.id
    const paid = u.level === 'SMK'
      ? [['mtcna', subjectTeacher('Administrasi Infrastruktur Jaringan'), 'pending_approval'], ['mtcna', subjectTeacher('Dasar-dasar Teknik Jaringan Komputer'), 'not_started'],
        ['bnsp', subjectTeacher('Dasar-dasar Akuntansi dan Keuangan Lembaga'), 'pending_approval'], ['bnsp', subjectTeacher('Akuntansi Keuangan'), 'rejected']]
      : [['calistung', u.classes[0].homeroom.id, 'pending_approval'], ['calistung', u.classes[1].homeroom.id, 'not_started'], ['calistung', u.classes[2].homeroom.id, 'rejected']]
    const course = Object.fromEntries(courses.map((c) => [c[0], c]))
    for (const [c, userId, status] of paid) {
      const e = employees.find((x) => x.school_id === S && x.user_id === userId)
      if (!e || rows.some((x) => x.employee_id === e.id && x.course_id === id('course', c))) continue
      const aid = id('assign', e.id, c)
      const appId = ap.add({ school: S, key: `training:${u.key}:${e.id}:${c}`, type: 'training_paid', service: 'hr', sourceId: aid, title: `Pelatihan Berbayar: ${course[c][1]} — ${e.full_name}`,
        summary: { employee: e.full_name, course: course[c][1], provider: course[c][4], cost_idr: course[c][7] }, amount: course[c][7], maker: userId, subject: userId,
        steps: [['hr_manager', 48]], createdAt: at(status === 'pending_approval' ? '2026-10-13' : '2026-10-02', '10:00'),
        decisions: status === 'pending_approval' ? [] : [{ decision: status === 'rejected' ? 'rejected' : 'approved', at: at('2026-10-05', '11:00'),
          comment: status === 'rejected' ? 'Kuota anggaran pelatihan semester ini sudah terpakai; ajukan lagi semester genap.' : 'Disetujui, sesuai rencana pengembangan guru.' }] })
      add(e, c, { id: aid, session_id: id('session', c, 0), source: 'self', due_date: course[c][13][0]?.[0] ?? '2026-12-15', status, approval_id: appId,
        created_at: at(status === 'pending_approval' ? '2026-10-13' : '2026-10-02'), updated_at: at('2026-10-13') })
    }
    // A training admin assigns first aid to the unit's homeroom teachers.
    for (const c of u.classes.slice(0, 3)) {
      const e = employees.find((x) => x.school_id === S && x.user_id === c.homeroom.id)
      if (!e || rows.some((x) => x.employee_id === e.id && x.course_id === id('course', 'p3k'))) continue
      add(e, 'p3k', { session_id: id('session', 'p3k', 0), source: 'manual', due_date: '2026-10-24', status: 'not_started', created_at: at('2026-10-01'), updated_at: at('2026-10-01') })
    }
    out.push(insert('training_assignments', rows))
  }
  return merge({ teratai_hr: out.join('\n\n') }, ap.sql())
}
