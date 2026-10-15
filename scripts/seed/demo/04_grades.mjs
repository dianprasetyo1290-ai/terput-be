// Phase 4: grades of the first semester up to the mid-term (STS, 5-9 Oct):
//   exam   — exam types, a question bank per subject and grade, an STS exam
//            per class and subject taken by every student (answers + score),
//            and the end-of-semester (SAS) exams prepared but unpublished;
//   report — assessment types, weights, grade bands, formative / practical
//            grades, the STS grade (= the CBT score) and a finalized
//            mid-semester report card ("rapor bayangan") per student.
// Each student has a base ability, so grades are consistent across subjects.
import { dbRows, insert, q, rng, uid } from './lib.mjs'
import { MODEL } from './model.mjs'
import { questionsFor } from './questions.mjs'

const STS_DAYS = ['2026-10-05', '2026-10-06', '2026-10-07', '2026-10-08', '2026-10-09']
const PRINCIPAL = { sd: 'Dra. Sri Wahyuni, M.Pd.', smp: 'Kepala Sekolah Teratai', smk: 'Ir. Bambang Sutrisno, M.T.' }
const ASSESSMENT_TYPES = [
  ['FORMATIF-1', 'Penilaian Harian Formatif 1', 'formatif', 1],
  ['STS-1', 'Sumatif Tengah Semester Ganjil', 'sts', 2],
  ['SAS-1', 'Sumatif Akhir Semester Ganjil', 'sas', 3],
  ['FORMATIF-2', 'Penilaian Harian Formatif 2', 'formatif', 4],
  ['PRAKTIK-1', 'Penilaian Praktik / Proyek', 'praktik', 5],
]
const BANDS = [[90, 100, 'A', 'Sangat baik: menguasai seluruh tujuan pembelajaran'], [80, 89.99, 'B', 'Baik: menguasai sebagian besar tujuan pembelajaran'],
  [70, 79.99, 'C', 'Cukup: perlu penguatan pada beberapa materi'], [0, 69.99, 'D', 'Perlu bimbingan: belum mencapai KKTP']]
const band = (s) => BANDS.find(([min, max]) => s >= min && s <= max + 0.001)[2]
const EKSKUL = ['Pramuka', 'Futsal', 'Robotik & Coding', 'Paduan Suara', 'Tari Tradisional', 'Basket', 'English Club', 'Karya Ilmiah Remaja']
const code = (name, grade) => `${name.replace(/\(.*?\)/g, '').split(/\s+/).filter((w) => w.length > 2 || /^[A-Z]+$/.test(w)).map((w) => w[0]).join('').toUpperCase().slice(0, 4)}-${grade}`
const round2 = (x) => Math.round(x * 100) / 100
const wib = (d, hh, mm = 0) => `${d}T${String(hh).padStart(2, '0')}:${String(mm).padStart(2, '0')}:00+07:00`

function mid(name, score) {
  const s = name.split(' ')[0]
  if (score >= 90) return `${s} sangat menguasai materi tengah semester dan aktif membantu teman.`
  if (score >= 80) return `${s} menguasai sebagian besar materi; tingkatkan ketelitian saat mengerjakan soal.`
  if (score >= 70) return `${s} cukup memahami materi; perlu latihan rutin pada materi yang belum dikuasai.`
  return `${s} perlu bimbingan dan remedial pada beberapa tujuan pembelajaran.`
}

export function generate() {
  const exam = []
  const report = []

  for (const u of MODEL) {
    const r = rng('grades:' + u.key)
    const S = u.schoolId
    const termId = dbRows('teratai_academic', `SELECT id FROM terms WHERE academic_year_id = '${u.yearId}' AND "order" = 1`)[0].id
    const classIds = `'${u.classes.map((c) => c.id).join("','")}'`
    const assignments = dbRows('teratai_academic', `SELECT cs.id, cs.classroom_id, cs.subject_id, cs.teacher_user_id, s.name subject_name
      FROM classroom_subjects cs JOIN subjects s ON s.id = cs.subject_id WHERE cs.classroom_id IN (${classIds}) AND cs.deleted_at IS NULL ORDER BY s.name`)
    // Attendance totals up to the mid-term for the report cards.
    const att = Object.fromEntries(dbRows('teratai_attendance', `SELECT student_id, count(*) FILTER (WHERE status IN ('hadir','terlambat')) h,
        count(*) FILTER (WHERE status = 'sakit') s, count(*) FILTER (WHERE status = 'izin') i, count(*) FILTER (WHERE status = 'tanpa_keterangan') a
      FROM student_attendances WHERE classroom_id IN (${classIds}) AND session_date <= '2026-10-09' GROUP BY 1`).map((x) => [x.student_id, x]))
    const haveCard = new Set(dbRows('teratai_report', `SELECT student_id FROM report_cards WHERE classroom_id IN (${classIds})
      AND report_type = 'bayangan' AND period_label = 'Tengah Semester Ganjil 2026/2027'`).map((x) => x.student_id))
    const haveAttempt = new Set(dbRows('teratai_exam', `SELECT e.classroom_id || '|' || e.subject_id k FROM exams e WHERE e.classroom_id IN (${classIds})
      AND e.exam_type_id IN (SELECT id FROM exam_types WHERE mapping_rapor = 'sts') AND e.deleted_at IS NULL`).map((x) => x.k))

    // ---- exam: types, banks, STS and SAS exams ----
    let typeIds
    if (u.key === 'smp') {
      typeIds = Object.fromEntries(dbRows('teratai_exam', `SELECT mapping_rapor, id FROM exam_types WHERE school_id = '${S}' AND deleted_at IS NULL`).map((x) => [x.mapping_rapor, x.id]))
    } else {
      typeIds = { sts: uid('exam-type', u.key, 'sts'), sas: uid('exam-type', u.key, 'sas'), formatif: uid('exam-type', u.key, 'formatif') }
      exam.push(insert('exam_types', [
        { id: typeIds.sts, school_id: S, academic_year_id: u.yearId, name: 'Sumatif Tengah Semester (STS)', mapping_rapor: 'sts', created_by: '22222222-2222-2222-2222-222222222222' },
        { id: typeIds.sas, school_id: S, academic_year_id: u.yearId, name: 'Sumatif Akhir Semester (SAS)', mapping_rapor: 'sas', created_by: '22222222-2222-2222-2222-222222222222' },
        { id: typeIds.formatif, school_id: S, academic_year_id: u.yearId, name: 'Ulangan Harian Formatif', mapping_rapor: 'formatif', created_by: '22222222-2222-2222-2222-222222222222' },
      ]))
    }

    const folders = []
    const questions = []
    const options = []
    const bank = {} // `${subject}|${grade}` -> [{ id, correctId, optionIds }]
    for (const s of u.subjects) {
      for (const grade of [...new Set(u.classes.map((c) => c.grade))]) {
        if (!u.classes.some((c) => c.grade === grade && (!s.major || s.major === c.major))) continue
        const folderId = uid('q-folder', u.key, s.id, grade)
        folders.push({ id: folderId, school_id: S, academic_year_id: u.yearId, name: `Bank Soal ${s.name} Kelas ${grade} Ganjil`,
          description: `Soal pilihan ganda STS dan latihan ${s.name} kelas ${grade}.`, created_by: s.teacher.id, is_hidden: false })
        bank[`${s.id}|${grade}`] = questionsFor(s.name, grade).map((qq, qi) => {
          const qid = uid('question', folderId, qi)
          questions.push({ id: qid, school_id: S, academic_year_id: u.yearId, folder_id: folderId, created_by: s.teacher.id, question_type: 'pilihan_ganda',
            content: qq.text, poin: 12.5, level_kognitif: ['C1', 'C2', 'C2', 'C3', 'C3', 'C4', 'C2', 'C3'][qi], is_published: true })
          const optIds = qq.options.map((o, oi) => {
            const oid = uid('q-option', qid, oi)
            options.push({ id: oid, question_id: qid, content: o, is_correct: oi === qq.correct, order_index: oi })
            return oid
          })
          return { id: qid, correctId: optIds[qq.correct], optionIds: optIds }
        })
      }
    }
    exam.push(insert('question_folders', folders), insert('questions', questions), insert('answer_options', options, { conflict: '(question_id, order_index) DO NOTHING' }))

    // ---- report: settings ----
    let typeByCode
    if (u.key === 'smp') {
      typeByCode = Object.fromEntries(dbRows('teratai_report', `SELECT code, id FROM report_assessment_types WHERE school_id = '${S}'`).map((x) => [x.code, x.id]))
    } else {
      typeByCode = Object.fromEntries(ASSESSMENT_TYPES.map(([c]) => [c, uid('assess-type', u.key, c)]))
      report.push(insert('report_assessment_types', ASSESSMENT_TYPES.map(([c, name, bucket, sort]) => ({ id: typeByCode[c], school_id: S, code: c, name, bucket,
        weight_percent: 100, exam_linked: bucket === 'sts' || bucket === 'sas', sort_order: sort })), { conflict: '(school_id, code) DO NOTHING' }))
      report.push(insert('report_weight_settings', [{ school_id: S, akhir_formatif_weight: 30, akhir_sts_weight: 30, akhir_sas_weight: 40,
        sementara_formatif_weight: 30, sementara_praktik_weight: 40, sementara_sts_weight: 30, updated_by: '22222222-2222-2222-2222-222222222222' }], { conflict: '(school_id) DO NOTHING' }))
      report.push(insert('report_grade_bands', BANDS.map(([min, max, label, description]) => ({ id: uid('grade-band', u.key, label), school_id: S, min_score: min, max_score: max, label, description }))))
      report.push(insert('report_note_templates', [{ school_id: S,
        mid_template: 'Ananda {nama} menunjukkan perkembangan yang baik pada tengah semester ini. Tingkatkan terus semangat belajarnya.',
        final_template: 'Ananda {nama} telah menyelesaikan semester dengan baik. Pertahankan prestasi dan tingkatkan kedisiplinan.',
        updated_by: '22222222-2222-2222-2222-222222222222' }], { conflict: '(school_id) DO NOTHING' }))
    }

    const ability = Object.fromEntries(u.students.map((s) => [s.id, r.int(68, 97)]))
    const score = (st, spread = 8) => Math.max(45, Math.min(100, ability[st] + r.int(-spread, spread)))

    const exams = []
    const examClasses = []
    const examQuestions = []
    const attempts = []
    const answers = []
    const grades = []
    const cards = []
    const cardSubjects = []
    const perStudent = {} // student -> [{ subject, sts, formatif, praktik, computed }]

    for (const cls of u.classes) {
      const mine = assignments.filter((a) => a.classroom_id === cls.id)
      mine.forEach((a, ai) => {
        const qs = bank[`${a.subject_id}|${cls.grade}`]
        const day = STS_DAYS[ai % STS_DAYS.length]
        const hour = 7 + (ai % 3) * 2
        const stsScores = {}
        if (qs && !haveAttempt.has(`${cls.id}|${a.subject_id}`)) {
          const examId = uid('exam-sts', cls.id, a.subject_id)
          exams.push({ id: examId, school_id: S, academic_year_id: u.yearId, term_id: termId, exam_type_id: typeIds.sts,
            title: `STS ${a.subject_name} ${cls.name}`, description: 'Sumatif Tengah Semester Ganjil 2026/2027. Kerjakan dengan jujur.',
            classroom_id: cls.id, subject_id: a.subject_id, teacher_id: a.teacher_user_id, start_at: wib(day, hour), end_at: wib(day, hour + 1, 30),
            is_shuffled: true, is_published: true, published_at: wib('2026-10-01', 9), created_by: a.teacher_user_id, audience: 'academic' })
          examClasses.push({ exam_id: examId, classroom_id: cls.id })
          const eqIds = qs.map((qq, qi) => {
            const eqId = uid('exam-question', examId, qi)
            examQuestions.push({ id: eqId, exam_id: examId, question_id: qq.id, order_index: qi, points: 12.5 })
            return eqId
          })
          for (const st of cls.students) {
            // Number of correct answers follows the student's ability.
            const target = Math.max(2, Math.min(8, Math.round((score(st.id, 10) / 100) * 8)))
            const correctSet = new Set([...Array(8).keys()].sort(() => r.next() - 0.5).slice(0, target))
            const attemptId = uid('attempt', examId, st.id)
            const start = new Date(wib(day, hour, r.int(0, 5))).getTime()
            const finalScore = target * 12.5
            stsScores[st.id] = finalScore
            attempts.push({ id: attemptId, school_id: S, academic_year_id: u.yearId, exam_id: examId, user_id: st.id, classroom_id: cls.id, status: 'selesai',
              current_position: 7, focus_lost_count: r.chance(0.1) ? 1 : 0, started_at: new Date(start).toISOString(),
              submitted_at: new Date(start + r.int(35, 80) * 60000).toISOString(), score: finalScore, correct_count: target,
              graded_at: new Date(start + 90 * 60000).toISOString(), final_score: Math.round(finalScore), participant_type: 'student', participant_id: st.id })
            qs.forEach((qq, qi) => {
              const ok = correctSet.has(qi)
              const wrong = qq.optionIds.filter((o) => o !== qq.correctId)
              answers.push({ id: uid('answer', attemptId, qi), attempt_id: attemptId, exam_question_id: eqIds[qi],
                selected_option_id: ok ? qq.correctId : wrong[qi % wrong.length], is_correct: ok, points_awarded: ok ? 12.5 : 0,
                answered_at: new Date(start + (qi + 1) * 4 * 60000).toISOString() })
            })
          }
          // End-of-semester exam, prepared but not published yet.
          exams.push({ id: uid('exam-sas', cls.id, a.subject_id), school_id: S, academic_year_id: u.yearId, term_id: termId, exam_type_id: typeIds.sas,
            title: `SAS ${a.subject_name} ${cls.name}`, description: 'Sumatif Akhir Semester Ganjil 2026/2027.', classroom_id: cls.id, subject_id: a.subject_id,
            teacher_id: a.teacher_user_id, start_at: wib(`2026-12-0${1 + (ai % 5)}`, hour), end_at: wib(`2026-12-0${1 + (ai % 5)}`, hour + 1, 30),
            is_shuffled: true, is_published: false, created_by: a.teacher_user_id, audience: 'academic' })
        }

        // Formative, practical and STS grades of the report service.
        for (const st of cls.students) {
          const f1 = score(st.id), f2 = score(st.id), pr = score(st.id, 6)
          const sts = stsScores[st.id] ?? score(st.id, 10)
          const rows = [['FORMATIF-1', f1], ['FORMATIF-2', f2], ['PRAKTIK-1', pr], ['STS-1', sts]]
          for (const [c, v] of rows) {
            grades.push({ id: uid('grade', a.id, st.id, c), school_id: S, term_id: termId, classroom_subject_id: a.id, student_id: st.id,
              assessment_type_id: typeByCode[c], score: v, created_by: a.teacher_user_id })
          }
          const formatif = (f1 + f2) / 2
          ;(perStudent[st.id] ??= []).push({ subjectId: a.subject_id, name: a.subject_name, sts, formatif, praktik: pr,
            computed: round2(formatif * 0.3 + pr * 0.4 + sts * 0.3) })
        }
      })

      // Mid-semester report cards (rapor bayangan), finalized by the homeroom teacher.
      for (const st of cls.students) {
        if (haveCard.has(st.id)) continue
        const subjects = perStudent[st.id] ?? []
        const a = att[st.id] ?? { h: 0, s: 0, i: 0, a: 0 }
        const cardId = uid('report-card', st.id, 'sts-ganjil')
        const avg = subjects.reduce((t, x) => t + x.computed, 0) / Math.max(1, subjects.length)
        cards.push({ id: cardId, school_id: S, academic_year_id: u.yearId, term_id: termId, classroom_id: cls.id, student_id: st.id, status: 'finalized',
          finalized_at: wib('2026-10-12', 14), finalized_by: cls.homeroom.id, report_type: 'bayangan', period_label: 'Tengah Semester Ganjil 2026/2027',
          student_name: st.name, student_nisn: st.nisn ?? null, classroom_name: cls.name, term_name: 'Semester 1 (Ganjil)',
          homeroom_teacher_name: cls.homeroom.name, principal_name: PRINCIPAL[u.key],
          attendance_hadir: Number(a.h), attendance_sakit: Number(a.s), attendance_izin: Number(a.i), attendance_tanpa_keterangan: Number(a.a),
          attendance_synced_at: wib('2026-10-12', 13), header_synced_at: wib('2026-10-12', 13),
          class_teacher_note: avg >= 85 ? `Ananda ${st.name.split(' ')[0]} berprestasi sangat baik di tengah semester ini. Pertahankan!`
            : avg >= 75 ? `Ananda ${st.name.split(' ')[0]} menunjukkan perkembangan yang baik. Tingkatkan terus semangat belajarnya.`
            : `Ananda ${st.name.split(' ')[0]} perlu lebih rajin belajar dan mengikuti program remedial yang disiapkan sekolah.`,
          ekstrakurikuler: [{ kegiatan: r.pick(EKSKUL), predikat: avg >= 80 ? 'A' : 'B', keterangan: 'Aktif mengikuti latihan rutin' }], prestasi: [] })
        for (const x of subjects) {
          cardSubjects.push({ id: uid('report-card-subject', cardId, x.subjectId), report_card_id: cardId, school_id: S, subject_id: x.subjectId,
            subject_name: x.name, subject_code: code(x.name, cls.grade), computed_score: x.computed, grade_label: band(x.computed),
            sts_score: x.sts, formatif_score: x.formatif, praktik_score: x.praktik, mid_note: mid(st.name, x.computed), source_exam_count: 1,
            last_synced_at: wib('2026-10-12', 13) })
        }
      }
    }

    exam.push(`-- ${u.level}: ${exams.length} exams, ${attempts.length} attempts`)
    exam.push(insert('exams', exams), insert('exam_classrooms', examClasses, { conflict: '(exam_id, classroom_id) DO NOTHING' }),
      insert('exam_questions', examQuestions, { conflict: '(exam_id, question_id) DO NOTHING' }),
      insert('exam_attempts', attempts, { conflict: '(exam_id, participant_type, participant_id) DO NOTHING', chunk: 800 }),
      insert('exam_attempt_answers', answers, { conflict: '(attempt_id, exam_question_id) DO NOTHING', chunk: 1500 }))
    report.push(`-- ${u.level}: ${grades.length} grades, ${cards.length} report cards`)
    report.push(insert('formative_grades', grades, { conflict: '(term_id, classroom_subject_id, student_id, assessment_type_id) DO NOTHING', chunk: 1500 }),
      insert('report_cards', cards, { conflict: '(student_id, classroom_id, term_id, report_type, period_label) DO NOTHING' }),
      insert('report_card_subjects', cardSubjects, { conflict: '(report_card_id, subject_id) DO NOTHING', chunk: 1500 }))
  }
  return { teratai_exam: exam.join('\n\n'), teratai_report: report.join('\n\n') }
}
