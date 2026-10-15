// Phase 3: attendance from the first school day (13 Jul 2026) up to
// yesterday, for every class of the three units: a class session per
// timetable lesson (teacher check-in + agenda), the daily student attendance
// on the day's first session, the homeroom teacher's daily submission, and
// the parents' leave requests behind the sick/permission days. Today is left
// empty so a teacher can demo taking attendance live.
import { addDays, date, dbRows, insert, rng, uid } from './lib.mjs'
import { MODEL } from './model.mjs'

const FIRST_DAY = new Date('2026-07-13T00:00:00Z')
const LAST_DAY = new Date('2026-10-14T00:00:00Z')
const HOLIDAYS = new Set(['2026-08-17', '2026-08-25']) // HUT RI, Maulid Nabi

const TOPICS = {
  matematika: ['Bilangan Bulat', 'Operasi Hitung Campuran', 'Pecahan', 'Bentuk Aljabar', 'Persamaan Linear', 'Perbandingan', 'Himpunan', 'Geometri Bangun Datar'],
  indonesia: ['Teks Deskripsi', 'Teks Narasi', 'Membaca Pemahaman', 'Puisi', 'Teks Prosedur', 'Menulis Paragraf', 'Surat Resmi'],
  inggris: ['Greetings', 'Describing People', 'Daily Activities', 'Simple Present Tense', 'Telling Time', 'Descriptive Text'],
  ipa: ['Pengukuran', 'Klasifikasi Makhluk Hidup', 'Zat dan Perubahannya', 'Suhu dan Kalor', 'Energi', 'Sistem Organisasi Kehidupan'],
  ipas: ['Bagian Tubuh Tumbuhan', 'Wujud Benda', 'Gaya dan Gerak', 'Lingkungan Sekitar', 'Keragaman Budaya'],
  ips: ['Interaksi Sosial', 'Kondisi Geografis Indonesia', 'Kegiatan Ekonomi', 'Sejarah Kerajaan Nusantara'],
  pancasila: ['Sejarah Lahirnya Pancasila', 'Norma dan Aturan', 'Hak dan Kewajiban', 'Bhinneka Tunggal Ika'],
  agama: ['Akhlak Terpuji', 'Ibadah Sehari-hari', 'Kisah Teladan', 'Kejujuran dan Tanggung Jawab'],
  pjok: ['Permainan Bola Besar', 'Atletik: Lari Jarak Pendek', 'Senam Lantai', 'Kebugaran Jasmani', 'Renang Gaya Bebas'],
  seni: ['Menggambar Ekspresif', 'Lagu Daerah', 'Tari Kreasi', 'Kerajinan Tangan'],
  informatika: ['Berpikir Komputasional', 'Algoritma dan Pemrograman', 'Jaringan Komputer', 'Analisis Data', 'Etika Digital'],
  jaringan: ['Pengenalan Perangkat Jaringan', 'Topologi Jaringan', 'Pengkabelan UTP', 'Konfigurasi IP Address', 'Subnetting', 'Konfigurasi Router'],
  akuntansi: ['Persamaan Dasar Akuntansi', 'Jurnal Umum', 'Buku Besar', 'Neraca Saldo', 'Siklus Akuntansi Perusahaan Jasa'],
  default: ['Pengantar Materi', 'Diskusi Kelompok', 'Latihan Soal', 'Presentasi', 'Penilaian Harian'],
}
function topicsFor(name) {
  const n = name.toLowerCase()
  for (const [k, v] of Object.entries(TOPICS)) if (n.includes(k)) return v
  if (n.includes('alam')) return TOPICS.ipa
  if (n.includes('sosial')) return TOPICS.ips
  if (n.includes('teknik jaringan') || n.includes('infrastruktur')) return TOPICS.jaringan
  if (n.includes('akuntansi') || n.includes('keuangan')) return TOPICS.akuntansi
  return TOPICS.default
}

function schoolDays() {
  const days = []
  for (let d = FIRST_DAY; d <= LAST_DAY; d = addDays(d, 1)) {
    const dow = d.getUTCDay()
    if (dow === 0 || dow === 6 || HOLIDAYS.has(date(d))) continue
    days.push({ date: date(d), dow })
  }
  return days
}

const iso = (day, hhmm, plusMin = 0) => {
  const [h, m] = hhmm.split(':').map(Number)
  const t = h * 60 + m + plusMin
  return `${day}T${String(Math.floor(t / 60)).padStart(2, '0')}:${String(t % 60).padStart(2, '0')}:00+07:00`
}

export function generate() {
  const days = schoolDays()
  const out = []

  for (const u of MODEL) {
    const r = rng('attendance:' + u.key)
    const classIds = u.classes.map((c) => c.id)
    const list = `'${classIds.join("','")}'`
    // Timetable slots with their subject and teacher (ids come from academic).
    const slots = dbRows('teratai_academic', `
      SELECT s.id schedule_id, s.classroom_id, sp.day_of_week dow, sp.period_no, sp.start_time, sp.end_time,
             cs.subject_id, cs.teacher_user_id, sub.name subject_name
      FROM schedules s
      JOIN schedule_periods sp ON sp.id = s.schedule_period_id
      JOIN classroom_subjects cs ON cs.id = s.classroom_subject_id
      JOIN subjects sub ON sub.id = cs.subject_id
      WHERE s.classroom_id IN (${list})
      ORDER BY s.classroom_id, sp.day_of_week, sp.period_no`)
    // Class-days that already have attendance (earlier seeds) are left alone.
    const taken = new Set(dbRows('teratai_attendance', `SELECT DISTINCT classroom_id::text c, session_date::text d FROM class_sessions WHERE classroom_id IN (${list})`)
      .map((x) => `${x.c}|${x.d}`))
    // The unit's real first-semester id (SMP had one before the generator).
    const termId = dbRows('teratai_academic', `SELECT id FROM terms WHERE academic_year_id = '${u.yearId}' AND "order" = 1`)[0].id

    const sessions = []
    const attendance = []
    const submissions = []
    const leaves = []
    const meetingNo = {}

    for (const cls of u.classes) {
      const mySlots = slots.filter((s) => s.classroom_id === cls.id)
      for (const day of days) {
        if (taken.has(`${cls.id}|${day.date}`)) continue
        const todays = mySlots.filter((s) => s.dow === day.dow)
        if (!todays.length) continue
        let firstSession = null
        for (const s of todays) {
          const k = `${cls.id}|${s.subject_id}`
          meetingNo[k] = (meetingNo[k] ?? 0) + 1
          const topics = topicsFor(s.subject_name)
          const id = uid('session', s.schedule_id, day.date)
          firstSession ??= { id, teacher: s.teacher_user_id }
          sessions.push({ id, school_id: u.schoolId, academic_year_id: u.yearId, term_id: termId, schedule_id: s.schedule_id, classroom_id: cls.id,
            subject_id: s.subject_id, guru_id: s.teacher_user_id, session_date: day.date, checked_in_at: iso(day.date, s.start_time, -r.int(0, 6)),
            checked_out_at: iso(day.date, s.end_time, r.int(0, 3)), slot_start_time: s.start_time, slot_end_time: s.end_time,
            pertemuan: String(meetingNo[k]), pembahasan: topics[Math.floor((meetingNo[k] - 1) / 3) % topics.length],
            materi_pertemuan: `Pertemuan ${meetingNo[k]}: ${topics[Math.floor((meetingNo[k] - 1) / 3) % topics.length]}`,
            bentuk_kegiatan: [r.pick(['teori', 'teori', 'praktikum', 'diskusi', 'formatif'])], agenda_filled_at: iso(day.date, s.end_time, 5),
            tugas_diberikan: r.chance(0.25), tugas_jenis: r.chance(0.5) ? 'individu' : 'kelompok', source: 'checkin' })
        }
        // Daily attendance on the first session of the day.
        for (const st of cls.students) {
          const x = r.next()
          const status = x < 0.92 ? 'hadir' : x < 0.95 ? 'terlambat' : x < 0.97 ? 'sakit' : x < 0.985 ? 'izin' : 'tanpa_keterangan'
          attendance.push({ id: uid('attendance', st.id, day.date), class_session_id: firstSession.id, school_id: u.schoolId, academic_year_id: u.yearId,
            term_id: termId, classroom_id: cls.id, student_id: st.id, session_date: day.date, status,
            catatan: status === 'terlambat' ? `Datang pukul 07.${String(r.int(10, 40))}` : status === 'sakit' ? 'Surat dokter / izin orang tua' : null,
            marked_by: firstSession.teacher, created_at: iso(day.date, '07:20'), updated_at: iso(day.date, '07:20') })
          if (status === 'sakit' || status === 'izin') {
            leaves.push({ id: uid('leave', st.id, day.date), school_id: u.schoolId, academic_year_id: u.yearId, classroom_id: cls.id, student_id: st.id,
              requested_by: st.parentId, type: status, start_date: day.date, end_date: day.date,
              reason: status === 'sakit' ? r.pick(['Demam sejak semalam', 'Batuk pilek', 'Sakit perut', 'Kontrol ke dokter']) : r.pick(['Acara keluarga', 'Menghadiri pernikahan saudara', 'Keperluan keluarga mendesak']),
              status: 'approved', reviewed_by: cls.homeroom.id, reviewed_at: iso(day.date, '09:00'), source: 'parent', created_at: iso(day.date, '06:30') })
          }
        }
        submissions.push({ classroom_id: cls.id, session_date: day.date, school_id: u.schoolId, status: 'submitted', catatan: 'Presensi lengkap',
          submitted_by: cls.homeroom.id, submitted_at: iso(day.date, '08:00'), updated_by: cls.homeroom.id, updated_at: iso(day.date, '08:00') })
      }
      // Upcoming/pending leave requests for the parent portal and the homeroom.
      for (const st of cls.students.filter((_, i) => i % 9 === 4)) {
        leaves.push({ id: uid('leave-pending', st.id), school_id: u.schoolId, academic_year_id: u.yearId, classroom_id: cls.id, student_id: st.id,
          requested_by: st.parentId, type: r.chance(0.5) ? 'izin' : 'sakit', start_date: '2026-10-16', end_date: '2026-10-16',
          reason: r.pick(['Acara keluarga di luar kota', 'Kontrol gigi', 'Demam']), status: 'pending', source: 'parent', created_at: '2026-10-15T06:10:00+07:00' })
      }
    }

    out.push(`-- ${u.level}: ${sessions.length} sessions, ${attendance.length} attendance rows`)
    out.push(insert('attendance_lock_settings', [{ school_id: u.schoolId, enabled: true, cutoff_days: 3, cutoff_time: '15:00', updated_by: '22222222-2222-2222-2222-222222222222' }],
      { conflict: '(school_id) DO NOTHING' }))
    out.push(insert('class_sessions', sessions, { conflict: '(schedule_id, session_date) DO NOTHING', chunk: 400 }))
    out.push(insert('student_attendances', attendance, { conflict: '(classroom_id, session_date, student_id) DO NOTHING', chunk: 800 }))
    out.push(insert('daily_attendance_submissions', submissions, { conflict: '(classroom_id, session_date) DO NOTHING' }))
    out.push(insert('leave_requests', leaves))
  }
  return { teratai_attendance: out.join('\n\n') }
}
