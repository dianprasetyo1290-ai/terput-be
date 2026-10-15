// Phase 22: teacher performance evaluation (supervisi akademik) of SD and
// SMK. The evaluation forms are the service's own defaults: the generator
// asks services/evaluation to bootstrap them (GET /forms, the same call the
// admin screen makes) instead of writing them, because the service rewrites
// any school's forms to DefaultForms on every period save and would drop
// items it does not know — and the answers with them. On top of that:
// - the start-of-term supervision, closed: every task submitted;
// - this term's supervision, open: the principal and some teachers done,
//   the rest still waiting;
// - next term's appraisal, not published yet.
// Tasks follow the service's regenerateTasks (principal, the vice-principal
// for student affairs, self, the peer mesh); student tasks exist only where
// a student submitted, as the service creates them on submission. raw_score
// is the item-weighted mean of the answers, as TaskStore.SubmitAnswers does.
import { createHmac } from 'node:crypto'
import { ACCOUNTS, at, dbRows, envLocal, insert, merge, rng, uid } from './lib.mjs'
import { MODEL } from './model.mjs'

const SMP = '11111111-1111-1111-1111-111111111111'
const { kepala_sekolah: KEPSEK, admin: ADMIN } = ACCOUNTS
const WAKA_KESISWAAN = 'a2600000-0000-0000-0000-00000000000d'
const EVALUATION_URL = process.env.EVALUATION_URL || `http://localhost:${envLocal('EVALUATION_PORT') || 8085}`

function adminToken(schoolId) {
  const b64 = (o) => Buffer.from(JSON.stringify(o)).toString('base64url')
  const now = Math.floor(Date.now() / 1000)
  const head = b64({ alg: 'HS256', typ: 'JWT' })
  const body = b64({ sub: ADMIN, school_id: schoolId, active_role: 'admin', roles: ['admin'], iat: now, exp: now + 300 })
  return `${head}.${body}.${createHmac('sha256', envLocal('SEKOCI_JWT_SECRET')).update(`${head}.${body}`).digest('base64url')}`
}

const NOTES = {
  4: ['Konsisten dan menjadi contoh bagi rekan.', 'Bukti dokumentasi lengkap.', 'Sangat baik, pertahankan.'],
  3: ['Sudah baik, perlu dokumentasi lebih rapi.', 'Berjalan baik; tingkatkan konsistensi.', ''],
  2: ['Perlu pendampingan, dijadwalkan coaching dengan wakasek.', 'Belum konsisten; tindak lanjut di supervisi berikutnya.'],
}

export async function generate() {
  const units = MODEL.filter((x) => x.schoolId !== SMP)
  for (const u of units) {
    const res = await fetch(`${EVALUATION_URL}/forms`, { headers: { Authorization: `Bearer ${adminToken(u.schoolId)}`, 'X-School-Id': u.schoolId } })
    if (!res.ok) throw new Error(`bootstrap evaluation forms ${u.level}: HTTP ${res.status} ${await res.text()}`)
  }
  const ids = units.map((u) => `'${u.schoolId}'`).join(',')
  const forms = dbRows('teratai_evaluation', `SELECT id, school_id, evaluator_type FROM teacher_evaluation_forms WHERE school_id IN (${ids})`)
  const items = dbRows('teratai_evaluation', `SELECT id, form_id, code, weight FROM teacher_evaluation_items WHERE school_id IN (${ids}) ORDER BY sort_order`)

  const out = []
  for (const u of units) {
    const S = u.schoolId
    const Y = u.yearId
    const id = (...k) => uid('tev', u.key, ...k)
    const form = Object.fromEntries(forms.filter((f) => f.school_id === S).map((f) => [f.evaluator_type, f]))
    const itemsOf = (type) => items.filter((i) => i.form_id === form[type].id)
    const teachers = u.teachers.map((t) => t.id)
    // A teacher's general level (some stronger, some needing coaching), shared by every evaluator.
    const level = Object.fromEntries(u.teachers.map((t, i) => [t.id, [3.6, 3.2, 2.9, 3.4, 3.0, 2.6, 3.5, 3.1, 3.3, 2.8][i % 10]]))
    const classesOf = (tid) => u.classes.filter((c) => u.subjects.some((s) => s.teacher.id === tid && (!s.major || s.major === c.major)))
    out.push(`-- ===== ${u.level} =====`)

    const periods = [
      { key: 'awal', title: 'Supervisi Awal Semester Ganjil 2026/2027', semester: 'Semester 1', start: '2026-08-17', end: '2026-09-12', published: true,
        assessor: 'Dra. Sri Wahyuni, M.Pd', fill: 1, created: '2026-08-10', submittedOn: ['2026-08-24', '2026-09-10'] },
      { key: 'ganjil', title: 'Supervisi Akademik & Kinerja Guru TP 2026/2027 Ganjil', semester: 'Semester 1', start: '2026-10-05', end: '2026-12-14', published: true,
        assessor: 'Dr. H. Ahmad Dahlan, M.Pd', fill: 0.45, created: '2026-10-01', submittedOn: ['2026-10-06', '2026-10-14'] },
      { key: 'genap', title: 'Penilaian Kinerja Guru Semester Genap 2026/2027', semester: 'Semester 2', start: '2027-03-01', end: '2027-04-30', published: false,
        assessor: null, fill: 0, created: '2026-10-12' },
    ]
    const peerEvaluatees = teachers.slice(0, 4)
    const peerEvaluators = teachers.slice(4, 8)
    const rows = { periods: [], tasks: [], answers: [] }
    for (const p of periods) {
      const pid = id('period', p.key)
      rows.periods.push({ id: pid, school_id: S, academic_year_id: Y, title: p.title, semester: p.semester, publish_start_date: p.start, publish_end_date: p.end,
        published: p.published, headmaster_user_id: KEPSEK, curriculum_vp_user_id: null, student_affairs_vp_user_id: WAKA_KESISWAAN,
        target_teacher_ids: teachers, peer_evaluatee_ids: peerEvaluatees, peer_evaluator_ids: peerEvaluators, student_classroom_ids: u.classes.map((c) => c.id),
        lead_assessor_name: p.assessor, created_by: ADMIN, updated_by: ADMIN, created_at: at(p.created), updated_at: at(p.created) })
      const task = (type, target, evaluator, classroom = null) => {
        const r = rng(`tev:${pid}:${type}:${target}:${evaluator}`)
        // Principal and self go first; the open period has them partly done.
        const done = p.fill === 1 || (p.fill > 0 && r.next() < (type === 'kepala_sekolah' ? 0.7 : type === 'siswa' ? 1 : p.fill))
        const tid = id('task', p.key, type, target, evaluator, classroom ?? '')
        let raw = null
        let submittedAt = null
        if (done) {
          let sum = 0
          let wsum = 0
          const base = level[target] + (type === 'self' ? 0.25 : type === 'siswa' ? 0.15 : 0)
          submittedAt = at(r.next() < 0.5 ? p.submittedOn[0] : p.submittedOn[1], `${r.int(8, 15)}:${String(r.int(0, 59)).padStart(2, '0')}`)
          for (const it of itemsOf(type)) {
            const score = Math.max(1, Math.min(4, Math.round(base + (r.next() - 0.5) * 1.2)))
            const w = Number(it.weight)
            sum += w * score
            wsum += w
            const note = type === 'siswa' ? '' : r.pick(NOTES[Math.max(2, score)])
            rows.answers.push({ id: id('answer', tid, it.code), task_id: tid, item_id: it.id, school_id: S, score, notes: note, created_at: submittedAt, updated_at: submittedAt })
          }
          raw = Math.round((sum / wsum) * 100) / 100
        }
        rows.tasks.push({ id: tid, period_id: pid, form_id: form[type].id, school_id: S, academic_year_id: Y, target_teacher_user_id: target, evaluator_user_id: evaluator,
          evaluator_type: type, classroom_id: classroom, status: done ? 'submitted' : 'pending', submitted_at: submittedAt, raw_score: raw,
          created_at: at(p.published ? p.start : p.created, '00:05'), updated_at: submittedAt ?? at(p.published ? p.start : p.created, '00:05') })
      }
      for (const t of teachers) {
        task('kepala_sekolah', t, KEPSEK)
        task('waka_kesiswaan', t, WAKA_KESISWAAN)
        task('self', t, t)
      }
      for (const ev of peerEvaluators) for (const ee of peerEvaluatees) if (ev !== ee) task('peer', ee, ev)
      // Students rate the teachers of their class (only submitted tasks exist).
      if (p.fill > 0) {
        for (const t of teachers) {
          for (const c of classesOf(t).slice(0, 2)) {
            const n = p.fill === 1 ? 5 : 2
            for (const s of c.students.slice(0, n)) task('siswa', t, s.id, c.id)
          }
        }
      }
    }
    out.push(insert('teacher_evaluation_periods', rows.periods), insert('teacher_evaluation_tasks', rows.tasks, { conflict: 'DO NOTHING' }),
      insert('teacher_evaluation_answers', rows.answers, { conflict: '(task_id, item_id) DO NOTHING', chunk: 1000 }))
  }
  return merge({ teratai_evaluation: out.join('\n\n') })
}
