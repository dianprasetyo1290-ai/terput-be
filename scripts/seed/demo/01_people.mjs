// Phase 1: the school population of the three units — teachers, students
// and their parents in identity; classes, subjects, teaching assignments,
// enrolments and the weekly timetable in academic.
import { TERM_1, TERM_2, insert, q, uid } from './lib.mjs'
import { MODEL } from './model.mjs'

// New accounts reuse the demo password (copied from the demo admin), so the
// presenter can sign in as any generated person.
function usersSql(rows) {
  if (!rows.length) return ''
  const cols = ['id', 'name', 'email', 'phone', 'birth_date', 'birth_place', 'address', 'religion', 'nisn', 'nik']
  const values = rows.map((u) => `(${cols.map((c) => (c === 'id' ? `${q(u[c])}::uuid` : c === 'birth_date' ? `${q(u[c])}::date` : q(u[c]))).join(', ')})`)
  return `INSERT INTO users (${cols.join(', ')}, password_hash, password_changed, is_superadmin)
SELECT v.*, src.password_hash, true, false FROM (VALUES
${values.join(',\n')}
) AS v(${cols.join(', ')})
CROSS JOIN (SELECT password_hash FROM users WHERE email = 'admin@teratai.sch.id') src
ON CONFLICT DO NOTHING;`
}

export function generate() {
  const identity = []
  const academic = []

  for (const u of MODEL) {
    const users = []
    const roles = []
    const links = []
    for (const t of u.teachers.filter((t) => !t.existing)) {
      users.push({ id: t.id, name: t.name, email: t.email, phone: t.phone, birth_date: t.birthDate, birth_place: t.birthPlace,
        address: t.address, religion: t.religion, nisn: null, nik: t.nik })
    }
    for (const t of u.teachers) roles.push({ user_id: t.id, school_id: u.schoolId, role: 'guru', status: 'active' })
    for (const c of u.classes) roles.push({ user_id: c.homeroom.id, school_id: u.schoolId, role: 'walikelas', status: 'active' })
    for (const p of u.parents) {
      users.push({ id: p.id, name: p.name, email: p.email, phone: p.phone, birth_date: null, birth_place: null,
        address: p.address, religion: p.religion, nisn: null, nik: p.nik })
      roles.push({ user_id: p.id, school_id: u.schoolId, role: 'orang_tua', status: 'active' })
    }
    for (const s of u.students) {
      if (!s.existing) {
        users.push({ id: s.id, name: s.name, email: s.email, phone: null, birth_date: s.birthDate, birth_place: s.birthPlace,
          address: s.address, religion: s.religion, nisn: s.nisn, nik: null })
      }
      roles.push({ user_id: s.id, school_id: u.schoolId, role: 'siswa', status: 'active', student_status: 'aktif', academic_year_id: u.yearId })
      const parent = u.parents.find((p) => p.id === s.parentId)
      links.push({ id: uid('guardian-link', s.id), school_id: u.schoolId, guardian_user_id: s.parentId, student_user_id: s.id,
        relationship: parent?.relationship ?? 'wali', is_primary: true, status: 'active', verified_at: '2026-07-10T08:00:00Z' })
    }
    identity.push(`-- ${u.level}`, usersSql(users),
      insert('user_school_roles', roles.map((r) => ({ student_status: null, academic_year_id: null, ...r })), { conflict: '(user_id, school_id, role) DO NOTHING' }),
      insert('guardian_links', links, { conflict: '(school_id, guardian_user_id, student_user_id) DO NOTHING' }))

    // ---- academic ----
    const sql = [`-- ${u.level}`]
    sql.push(insert('terms', [
      { id: uid('term', u.key, 1), school_id: u.schoolId, academic_year_id: u.yearId, name: 'Semester 1 (Ganjil)', order: 1, start_date: TERM_1.start, end_date: TERM_1.end },
      { id: uid('term', u.key, 2), school_id: u.schoolId, academic_year_id: u.yearId, name: 'Semester 2 (Genap)', order: 2, start_date: TERM_2.start, end_date: TERM_2.end },
    ], { conflict: '(academic_year_id, "order") DO NOTHING' }))
    sql.push(insert('subjects', u.subjects.filter((s) => !s.existing).map((s) => ({
      id: s.id, school_id: u.schoolId, name: s.name, kelompok: s.kelompok, subject_group: s.kelompok === 'kejuruan' ? 'Kejuruan' : 'Umum' }))))
    sql.push(insert('classrooms', u.classes.filter((c) => !c.existing).map((c) => ({
      id: c.id, school_id: u.schoolId, academic_year_id: u.yearId, name: c.name, grade_level: c.grade, homeroom_teacher_user_id: c.homeroom.id, capacity: 32 }))))
    // Existing classes take the model's homeroom teacher too (the old seed
    // made the yayasan head the homeroom of Kelas VII A, which left the demo
    // Wali Kelas account without a class).
    for (const c of u.classes.filter((c) => c.existing)) {
      sql.push(`UPDATE classrooms SET homeroom_teacher_user_id = ${q(c.homeroom.id)} WHERE id = ${q(c.id)};`)
    }

    // Teaching assignments: every class gets the general subjects plus the
    // vocational ones of its major (SMK). Matched on (classroom, subject) so a
    // class that already had some keeps them.
    const cs = []
    for (const c of u.classes) {
      for (const s of u.subjects.filter((s) => !s.major || s.major === c.major)) {
        cs.push({ id: uid('class-subject', c.id, s.id), school_id: u.schoolId, academic_year_id: u.yearId, classroom_id: c.id,
          subject_id: s.id, teacher_user_id: s.teacher.id, is_active: true, hours_per_week: s.hours })
      }
    }
    sql.push(insert('classroom_subjects', cs, { conflict: '(classroom_id, subject_id) DO NOTHING' }))

    sql.push(insert('class_enrollments', u.students.map((s) => ({
      id: uid('enrollment', s.id), school_id: u.schoolId, academic_year_id: u.yearId, classroom_id: s.classId,
      student_user_id: s.id, status: 'active', enrolled_at: '2026-07-13T07:00:00+07:00' })), { conflict: '(academic_year_id, student_user_id) DO NOTHING' }))

    // Weekly timetable: Monday-Friday, 7 periods (8 on SMK), subjects spread
    // round-robin by their weekly hours.
    const perDay = u.level === 'SMK' ? 8 : 7
    const minutes = u.level === 'SD' ? 35 : u.level === 'SMP' ? 40 : 45
    const periods = []
    const slots = []
    for (const c of u.classes) {
      const subs = cs.filter((x) => x.classroom_id === c.id)
      const bag = subs.flatMap((x) => Array(Math.max(1, Math.round(x.hours_per_week / 2))).fill(x.subject_id))
      let k = 0
      for (let day = 1; day <= 5; day++) {
        let t = 7 * 60
        for (let p = 1; p <= perDay; p++) {
          if (p === 4) t += 15 // istirahat
          const start = `${String(Math.floor(t / 60)).padStart(2, '0')}:${String(t % 60).padStart(2, '0')}`
          t += minutes
          const end = `${String(Math.floor(t / 60)).padStart(2, '0')}:${String(t % 60).padStart(2, '0')}`
          periods.push({ id: uid('period', c.id, day, p), school_id: u.schoolId, academic_year_id: u.yearId, classroom_id: c.id,
            day_of_week: day, period_no: p, start_time: start, end_time: end })
          slots.push({ classroom: c.id, day, p, subject: bag[k++ % bag.length] })
        }
      }
    }
    // Periods an earlier seed created keep their id but take the standard times,
    // so a class never shows two overlapping timetables.
    sql.push(insert('schedule_periods', periods, { conflict: '(classroom_id, day_of_week, period_no) DO UPDATE SET start_time = EXCLUDED.start_time, end_time = EXCLUDED.end_time' }))
    // Schedules by natural keys, so periods/assignments that already existed
    // under other ids are reused. A period that already has a lesson is kept.
    for (let i = 0; i < slots.length; i += 400) {
      const vals = slots.slice(i, i + 400).map((s) => `(${q(s.classroom)}::uuid, ${s.day}, ${s.p}, ${q(s.subject)}::uuid)`).join(',\n')
      sql.push(`INSERT INTO schedules (school_id, academic_year_id, classroom_id, schedule_period_id, classroom_subject_id)
SELECT ${q(u.schoolId)}, ${q(u.yearId)}, v.c, sp.id, cs.id FROM (VALUES
${vals}
) AS v(c, d, p, s)
JOIN schedule_periods sp ON sp.classroom_id = v.c AND sp.day_of_week = v.d AND sp.period_no = v.p
JOIN classroom_subjects cs ON cs.classroom_id = v.c AND cs.subject_id = v.s
WHERE NOT EXISTS (SELECT 1 FROM schedules x WHERE x.schedule_period_id = sp.id)
ON CONFLICT DO NOTHING;`)
    }
    academic.push(...sql)
  }
  return { teratai_identity: identity.join('\n\n'), teratai_academic: academic.join('\n\n') }
}
