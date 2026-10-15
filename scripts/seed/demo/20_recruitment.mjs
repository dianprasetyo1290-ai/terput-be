// Phase 20: recruitment (ATS) of SD and SMK — the hiring pipeline and
// interview scorecards, manpower requisitions (approved, filled, one waiting
// for the HR manager and the foundation, a draft), the job postings on the
// careers portal, candidates and their applications at every stage with
// stage history, documents, interviews / microteaching / tests scored by the
// panel, offers (accepted, waiting for approval, draft, declined), and
// onboarding: templates per role and the plans of this year's new hires.
import { createHash } from 'node:crypto'
import { ACCOUNTS, approvals, at, dbRows, envLocal, fieldCrypto, insert, merge, slug, uid } from './lib.mjs'
import { MODEL } from './model.mjs'

const SMP = '11111111-1111-1111-1111-111111111111'
const { admin: ADMIN, kepala_sekolah: KEPSEK, hr_manager: HRM, hr_admin: HRA, recruiter: RECRUITER } = ACCOUNTS
const crypto = fieldCrypto(envLocal('HR_ENC_KEY'), envLocal('HR_BLIND_KEY'))

const STAGES = [['screening', 'Screening Berkas', 'normal'], ['interview', 'Wawancara', 'normal'], ['microteaching', 'Microteaching / Tes Teknis', 'normal'],
  ['offer', 'Penawaran Kerja', 'offer'], ['hired', 'Diterima (Hired)', 'hired'], ['rejected', 'Tidak Lolos', 'rejected']]
const SCORECARDS = [['wawancara', 'Wawancara Umum Calon Guru', null, [['Komunikasi', 3000], ['Motivasi & Integritas', 3000], ['Pemahaman Kurikulum Merdeka', 4000]]],
  ['microteaching', 'Microteaching Calon Guru', 'GURU', [['Penguasaan Materi', 4000], ['Pengelolaan Kelas', 3000], ['Penggunaan Media Pembelajaran', 3000]]],
  ['teknis', 'Tes Teknis Tenaga Kependidikan', null, [['Ketelitian & Akurasi', 6000], ['Penguasaan Aplikasi Kerja', 4000]]]]

// [key, position, dept, qty, reason, title, slug, description, qualifications, budget, status, filled, type, closes]
const REQS = {
  SD: [
    ['seni', 'GURU', 'AKAD', 1, 'addition', 'Guru Seni Budaya SD Teratai Putih', 'guru-seni-budaya-sd-2026', 'Mengajar Seni Budaya kelas 1-6 dan membina ekskul tari & paduan suara.',
      'S1 Pendidikan Seni/PGSD, portofolio karya, siap microteaching.', 5000000, 'filled', 1, 'full_time', '2026-06-20'],
    ['kelas', 'GURU', 'AKAD', 2, 'addition', 'Guru Kelas SD (Kelas Rendah)', 'guru-kelas-sd-2026', 'Wali kelas sekaligus guru kelas 1-3 dengan pendekatan pembelajaran berdiferensiasi.',
      'S1 PGSD, IPK min. 3,00, sabar dan menyukai anak-anak, sertifikat pendidik menjadi nilai tambah.', 5200000, 'approved', 0, 'full_time', '2026-11-20'],
    ['pjok', 'GURU', 'AKAD', 1, 'replacement', 'Guru PJOK SD Teratai Putih', 'guru-pjok-sd-2026', 'Mengajar PJOK kelas 1-6 dan mendampingi O2SN.',
      'S1 PJKR/PGSD Penjas, sehat jasmani, pengalaman melatih cabang olahraga.', 4800000, 'approved', 0, 'contract', '2026-10-31'],
    ['tu', 'STAF-TU', 'TU', 1, 'addition', 'Staf Tata Usaha SD', 'staf-tu-sd-2026', 'Administrasi persuratan, Dapodik, dan layanan orang tua.',
      'D3/S1 semua jurusan, menguasai Excel, pengalaman Dapodik menjadi nilai tambah.', 4300000, 'pending_approval', 0, 'contract', null],
    ['inggris', 'GURU', 'AKAD', 1, 'addition', 'Guru Bahasa Inggris SD', 'guru-bahasa-inggris-sd-2027', 'Mengajar Bahasa Inggris kelas 1-6 (muatan lokal).',
      'S1 Pendidikan Bahasa Inggris, TOEFL ITP min. 500.', 4800000, 'draft', 0, 'part_time', null],
  ],
  SMK: [
    ['akl', 'GURU', 'AKAD', 1, 'addition', 'Guru Produktif Akuntansi (AKL)', 'guru-produktif-akl-2026', 'Mengajar mapel kejuruan AKL dan membina kelas industri bersama mitra.',
      'S1 Akuntansi/Pendidikan Ekonomi Akuntansi, menguasai aplikasi akuntansi, sertifikat kompetensi BNSP menjadi nilai tambah.', 6000000, 'filled', 1, 'full_time', '2026-09-30'],
    ['tkj', 'GURU', 'AKAD', 2, 'addition', 'Guru Produktif Teknik Jaringan (TKJ)', 'guru-produktif-tkj-2026', 'Mengajar praktik jaringan di bengkel TKJ dan menyiapkan siswa LKS.',
      'S1 Teknik Informatika/Pendidikan TI, sertifikasi MikroTik (MTCNA) atau Cisco (CCNA).', 6500000, 'approved', 0, 'full_time', '2026-11-30'],
    ['teknisi', 'TEKNISI', 'UMUM', 1, 'addition', 'Teknisi Laboratorium Komputer', 'teknisi-lab-smk-2026', 'Merawat 72 PC lab, jaringan sekolah, dan membantu praktik.',
      'D3/S1 Teknik Komputer, pengalaman troubleshooting jaringan min. 1 tahun.', 4800000, 'pending_approval', 0, 'contract', null],
    ['bk', 'GURU-BK', 'BK', 1, 'replacement', 'Guru Bimbingan Konseling SMK', 'guru-bk-smk-2027', 'Layanan BK, bimbingan karier, dan tracer study lulusan.',
      'S1 Bimbingan Konseling, pengalaman bimbingan karier.', 5000000, 'draft', 0, 'full_time', null],
  ],
}
// Applicants per requisition: [name, source, stage, status, extra]
const APPLICANTS = {
  SD: {
    seni: [['@hire', 'careers_portal', 'hired', 'hired'], ['Wulan Permata Sari', 'careers_portal', 'rejected', 'rejected']],
    kelas: [['Nur Aini Rahmawati', 'careers_portal', 'offer', 'active', 'offer_pending'], ['Intan Puspitasari', 'jobfair', 'microteaching', 'active', 'micro_scheduled'],
      ['Rizky Ramadhan', 'careers_portal', 'interview', 'active', 'interview_done'], ['Laras Wening', 'referral', 'screening', 'active'],
      ['Fajar Nugraha', 'careers_portal', 'rejected', 'rejected'], ['Citra Ayuningtyas', 'careers_portal', 'screening', 'withdrawn']],
    pjok: [['Bagus Prakoso', 'careers_portal', 'offer', 'active', 'offer_draft'], ['Dimas Saputro', 'careers_portal', 'interview', 'active', 'interview_scheduled'],
      ['Yoga Pratama', 'referral', 'offer', 'rejected', 'offer_declined']],
  },
  SMK: {
    akl: [['Meilani Kusumawati', 'careers_portal', 'hired', 'hired', 'offer_accepted'], ['Andre Setiawan', 'jobfair', 'rejected', 'rejected']],
    tkj: [['Hendra Kurniawan', 'careers_portal', 'offer', 'active', 'offer_pending'], ['Galih Saputra', 'careers_portal', 'microteaching', 'active', 'micro_scheduled'],
      ['Reza Mahendra', 'jobfair', 'interview', 'active', 'interview_done'], ['Ilham Firdaus', 'careers_portal', 'screening', 'active'],
      ['Aldi Nugroho', 'referral', 'interview', 'active', 'interview_scheduled'], ['Kevin Wijaya', 'careers_portal', 'rejected', 'rejected'],
      ['Farhan Akbar', 'careers_portal', 'offer', 'rejected', 'offer_declined']],
    teknisi: [],
  },
}

const ONBOARDING = [['guru', 'Onboarding Guru Baru', 'AKAD', [['Serahkan salinan KTP, NPWP, ijazah, dan sertifikat pendidik', 'dokumen', 'employee', null, 0],
  ['Pembuatan akun SEKOCI dan email sekolah', 'akun', 'role', 'servicedesk_agent', 1], ['Penyerahan laptop, kunci loker, dan kartu akses', 'perangkat', 'role', 'asset_officer', 2],
  ['Orientasi visi-misi, tata tertib, dan budaya sekolah', 'orientasi', 'supervisor', null, 3], ['Pendampingan guru senior (buddy) minggu pertama', 'orientasi', 'supervisor', null, 5],
  ['Kursus wajib: Perlindungan Anak (PPKSP)', 'pelatihan', 'employee', null, 14], ['Menyusun RPM pertama bersama wakasek kurikulum', 'pelatihan', 'supervisor', null, 21]]],
['staf', 'Onboarding Tenaga Kependidikan', 'TU', [['Serahkan salinan KTP, NPWP, ijazah', 'dokumen', 'employee', null, 0], ['Pembuatan akun SEKOCI dan email sekolah', 'akun', 'role', 'servicedesk_agent', 1],
  ['Penyerahan perangkat kerja', 'perangkat', 'role', 'asset_officer', 2], ['Orientasi SOP layanan dan keamanan data', 'orientasi', 'supervisor', null, 3],
  ['Kursus wajib: Keamanan Data Pribadi', 'pelatihan', 'employee', null, 14]]]]

const sha = (s) => createHash('sha256').update(s).digest('hex')

export function generate() {
  const ap = approvals()
  const out = []
  const hires = dbRows('teratai_hr', `SELECT e.school_id, e.id, e.full_name, e.hire_date, p.code FROM employees e
    JOIN employment_assignments a ON a.employee_id = e.id JOIN positions p ON p.id = a.position_id
    WHERE e.school_id <> '${SMP}' AND e.hire_date >= '2026-07-01' ORDER BY e.hire_date, e.employee_no`)
  for (const u of MODEL.filter((x) => x.schoolId !== SMP)) {
    const S = u.schoolId
    const id = (...k) => uid('ats', u.key, ...k)
    const pos = (c) => uid('hr-pos', u.key, c)
    const dept = (c) => uid('hr-dept', u.key, c)
    out.push(`-- ===== ${u.level} =====`)
    out.push(insert('pipeline_stages', STAGES.map(([code, name, kind], i) => ({ id: id('stage', code), school_id: S, code, name, sort: i + 1, kind, created_at: at('2026-06-01') }))))
    out.push(insert('scorecard_templates', SCORECARDS.map(([k, name, p]) => ({ id: id('scorecard', k), school_id: S, position_id: p ? pos(p) : null, name, created_at: at('2026-06-01') }))))
    out.push(insert('scorecard_criteria', SCORECARDS.flatMap(([k, , , cs]) => cs.map(([name, w], i) => ({ id: id('criterion', k, i), school_id: S, template_id: id('scorecard', k),
      name, weight_bp: w, created_at: at('2026-06-01') })))))
    const unitHires = hires.filter((h) => h.school_id === S)
    const hiredTeacher = unitHires.find((h) => h.code === 'GURU')

    // ---- requisitions & postings ----
    const reqs = REQS[u.level]
    out.push(insert('requisitions', reqs.map(([k, p, d, qty, reason, , , , qual, budget, status, filled], i) => {
      const rid = id('req', k)
      let approvalId = null
      if (status !== 'draft') {
        approvalId = ap.add({ school: S, key: `requisition:${u.key}:${k}`, type: 'requisition', service: 'hr', sourceId: rid, title: `Permintaan Tenaga Kerja: ${reqs[i][5]} (${qty} orang)`,
          summary: { position: reqs[i][5], quantity: qty, reason, budget_idr: budget }, amount: budget * qty * 12, maker: HRA, steps: [['hr_manager', 48], ['kepala_yayasan', 72]],
          decisions: status === 'pending_approval' ? [{ decision: 'approved', at: at('2026-10-13', '10:00'), comment: 'Sesuai kebutuhan beban kerja.' }]
            : [{ decision: 'approved', at: at(i === 0 ? '2026-05-12' : '2026-08-12', '10:00'), comment: 'Setuju.' }, { decision: 'approved', at: at(i === 0 ? '2026-05-14' : '2026-08-14', '14:00'), comment: 'Disetujui yayasan.' }],
          createdAt: at(status === 'pending_approval' ? '2026-10-12' : i === 0 ? '2026-05-10' : '2026-08-10', '09:00') })
      }
      return { id: rid, school_id: S, position_id: pos(p), department_id: dept(d), quantity: qty, reason, qualifications: qual, budget_idr: budget,
        replaces_employee_id: reason === 'replacement' ? null : null, exceeds_headcount: false, status, filled_count: filled, approval_id: status === 'pending_approval' ? approvalId : null,
        requested_by: HRA, idempotency_key: `demo:req:${u.key}:${k}`, created_at: at(i === 0 ? '2026-05-10' : status === 'pending_approval' ? '2026-10-12' : '2026-08-10'),
        updated_at: at('2026-10-12') }
    }), { conflict: '(school_id, idempotency_key) DO NOTHING' }))
    const postings = reqs.filter((x) => ['approved', 'filled'].includes(x[10])).map(([k, , , , , title, sl, desc, qual, , status, , type, closes]) => ({
      id: id('posting', k), school_id: S, requisition_id: id('req', k), slug: sl, title, description: desc, qualifications: qual, employment_type: type, closes_on: closes,
      status: status === 'filled' ? 'closed' : 'open', closed_reason: status === 'filled' ? 'Posisi telah terisi' : null,
      published_at: at(status === 'filled' ? '2026-05-15' : '2026-08-20', '08:00'), created_at: at(status === 'filled' ? '2026-05-14' : '2026-08-18') }))
    out.push(insert('job_postings', postings, { conflict: '(school_id, slug) DO NOTHING' }))

    // ---- candidates, applications, interviews, offers ----
    const candidates = []
    const apps = []
    const events = []
    const docs = []
    const interviews = []
    const panel = []
    const scores = []
    const offers = []
    const stageId = (c) => id('stage', c)
    for (const [rk, list] of Object.entries(APPLICANTS[u.level])) {
      const req = reqs.find((x) => x[0] === rk)
      const isTeacher = req[1].startsWith('GURU')
      list.forEach(([rawName, source, stage, status, extra], i) => {
        const name = rawName === '@hire' ? hiredTeacher?.full_name ?? 'Dinda Utami' : rawName
        const key = `${rk}:${i}`
        const cid = id('candidate', key)
        const email = `${slug(name)}@mail.contoh.id`
        const applied = req[10] === 'filled' ? `2026-05-${String(18 + i).padStart(2, '0')}` : `2026-0${8 + (i % 2)}-${String(22 + i).padStart(2, '0')}`
        candidates.push({ id: cid, school_id: S, full_name: name, email, email_norm: email, phone_enc: crypto.encrypt(`0812${String(5550000 + i * 731 + rk.length)}`, S, cid + 'phone'),
          consent_processing_at: at(applied, '20:00'), consent_talent_pool: i % 3 !== 2, created_at: at(applied, '20:00') })
        const aid = id('application', key)
        apps.push({ id: aid, school_id: S, posting_id: id('posting', rk), candidate_id: cid, stage_id: stageId(stage), status, cv_object_key: `ats/cv/${aid}.pdf`,
          answers: isTeacher ? { pengalaman_tahun: String(1 + (i % 5)), sertifikat_pendidik: i % 2 ? 'Ya' : 'Belum' } : { pendidikan: 'D3/S1', pengalaman_tahun: String(1 + i) },
          source, access_token_hash: sha(`demo:${u.key}:${aid}`), created_at: at(applied, '20:00'), updated_at: at('2026-10-12') })
        docs.push({ id: id('doc', key, 'cv'), school_id: S, application_id: aid, kind: 'cv', object_key: `ats/documents/${aid}/CV_${slug(name)}.pdf`, file_name: `CV_${name.replace(/ /g, '_')}.pdf`, created_at: at(applied, '20:00') },
          { id: id('doc', key, 'ijazah'), school_id: S, application_id: aid, kind: 'ijazah', object_key: `ats/documents/${aid}/Ijazah_${slug(name)}.pdf`, file_name: `Ijazah_${name.replace(/ /g, '_')}.pdf`, created_at: at(applied, '20:01') })
        // Stage history up to the current stage.
        const order = ['screening', 'interview', 'microteaching', 'offer', 'hired']
        const reached = stage === 'rejected' ? (extra === 'offer_declined' ? 3 : i % 2 ? 1 : 0) : order.indexOf(stage)
        let t = new Date(at(applied, '20:00'))
        const step = (from, to, note) => {
          t = new Date(t.getTime() + 6 * 86400000)
          events.push({ id: id('event', key, to, events.length), school_id: S, application_id: aid, from_stage: from ? stageId(from) : null, to_stage: stageId(to), note,
            actor_user_id: RECRUITER, created_at: t.toISOString() })
        }
        events.push({ id: id('event', key, 'apply'), school_id: S, application_id: aid, from_stage: null, to_stage: stageId('screening'), note: 'Lamaran masuk', actor_user_id: null, created_at: at(applied, '20:00') })
        for (let s = 1; s <= reached; s++) step(order[s - 1], order[s], ['', 'Lolos screening berkas', isTeacher ? 'Wawancara baik, jadwalkan microteaching' : 'Wawancara baik, jadwalkan tes teknis',
          'Rekomendasi panel: terima', 'Penawaran diterima kandidat'][s])
        if (stage === 'rejected') step(order[reached], 'rejected', extra === 'offer_declined' ? 'Kandidat menolak penawaran' : reached ? 'Skor wawancara di bawah ambang' : 'Kualifikasi tidak sesuai')
        if (status === 'withdrawn') events.push({ id: id('event', key, 'withdrawn'), school_id: S, application_id: aid, from_stage: stageId('screening'), to_stage: stageId('screening'),
          note: 'Kandidat mengundurkan diri (diterima di sekolah lain)', actor_user_id: RECRUITER, created_at: at('2026-09-20') })

        // Interviews and scores.
        const ivs = []
        if (reached >= 1 || extra === 'interview_scheduled' || extra === 'interview_done') ivs.push(['interview', 'wawancara', extra === 'interview_scheduled' ? 'scheduled' : 'done'])
        if (reached >= 2 || extra === 'micro_scheduled') ivs.push([isTeacher ? 'microteaching' : 'test', isTeacher ? 'microteaching' : 'teknis', extra === 'micro_scheduled' ? 'scheduled' : 'done'])
        ivs.forEach(([kind, card, st], n) => {
          const iid = id('interview', key, kind)
          const when = st === 'scheduled' ? `2026-10-${String(19 + n + i).padStart(2, '0')}` : new Date(new Date(at(applied)).getTime() + (8 + n * 6) * 86400000).toISOString().slice(0, 10)
          interviews.push({ id: iid, school_id: S, application_id: aid, kind, starts_at: at(when, n ? '08:00' : '10:00'), ends_at: at(when, n ? '09:00' : '11:00'),
            location_or_link: kind === 'microteaching' ? `Ruang ${u.classes[1].name}` : kind === 'test' ? 'Lab Komputer' : 'Ruang Rapat Kepala Sekolah', template_id: id('scorecard', card),
            status: st, created_at: at(applied, '21:00') })
          for (const p of [RECRUITER, KEPSEK]) {
            panel.push({ id: id('panel', iid, p), school_id: S, interview_id: iid, user_id: p, created_at: at(applied, '21:00') })
            if (st !== 'done') continue
            SCORECARDS.find((x) => x[0] === card)[3].forEach((_, ci) => {
              const base = stage === 'rejected' && extra !== 'offer_declined' ? 62 : 80
              scores.push({ id: id('score', iid, p, ci), school_id: S, interview_id: iid, panel_user_id: p, criterion_id: id('criterion', card, ci),
                score: base + ((i * 7 + ci * 5 + (p === KEPSEK ? 3 : 0)) % 15), recommendation: base > 70 ? 'Direkomendasikan' : 'Tidak direkomendasikan',
                submitted_at: at(when, '12:00'), created_at: at(when, '12:00') })
            })
          }
        })

        // Offers.
        const offerKind = extra?.startsWith('offer_') ? extra : stage === 'hired' ? 'offer_accepted' : null
        if (offerKind) {
          const oid = id('offer', key)
          const salary = req[9] + (i % 3) * 100000
          const statusMap = { offer_accepted: 'accepted', offer_pending: 'pending_approval', offer_draft: 'draft', offer_declined: 'declined' }
          let approvalId = null
          if (offerKind !== 'offer_draft') {
            approvalId = ap.add({ school: S, key: `offer:${u.key}:${key}`, type: 'offer', service: 'hr', sourceId: oid, title: `Penawaran Kerja: ${name} — ${req[5]}`,
              summary: { candidate: name, position: req[5], base_salary_idr: salary, start_date: '2026-11-02' }, amount: salary, maker: RECRUITER, steps: [['hr_manager', 48], ['kepala_sekolah', 48]],
              decisions: offerKind === 'offer_pending' ? [{ decision: 'approved', at: at('2026-10-13', '15:00'), comment: 'Gaji dalam rentang posisi.' }]
                : [{ decision: 'approved', at: at(stage === 'hired' && req[10] === 'filled' ? '2026-06-08' : '2026-09-25', '10:00'), comment: 'Setuju.' },
                  { decision: 'approved', at: at(stage === 'hired' && req[10] === 'filled' ? '2026-06-09' : '2026-09-26', '09:00'), comment: 'Setuju, segera kirim surat penawaran.' }],
              createdAt: at(offerKind === 'offer_pending' ? '2026-10-12' : stage === 'hired' && req[10] === 'filled' ? '2026-06-06' : '2026-09-24', '09:00') })
          }
          const hiredEmp = stage === 'hired' && rawName === '@hire' ? hiredTeacher : null
          offers.push({ id: oid, school_id: S, application_id: aid, position_id: pos(req[1]), start_date: hiredEmp?.hire_date ?? (stage === 'hired' ? '2026-10-01' : '2026-11-02'),
            employment_type: req[12], base_salary_enc: crypto.encrypt(String(salary), S, oid + 'salary'), out_of_range: false, status: statusMap[offerKind],
            approval_id: offerKind === 'offer_pending' ? approvalId : null, document_object_key: offerKind === 'offer_draft' ? '' : `ats/offers/${oid}.pdf`,
            responded_at: ['offer_accepted', 'offer_declined'].includes(offerKind) ? at(stage === 'hired' && req[10] === 'filled' ? '2026-06-12' : '2026-09-29', '12:00') : null,
            response_note: offerKind === 'offer_accepted' ? 'Terima kasih, saya bersedia bergabung.' : offerKind === 'offer_declined' ? 'Mohon maaf, saya menerima tawaran di tempat lain.' : null,
            employee_id: hiredEmp?.id ?? null, created_at: at('2026-09-24'), updated_at: at('2026-10-13') })
        }
      })
    }
    out.push(insert('candidates', candidates, { conflict: '(school_id, email_norm) DO NOTHING' }), insert('applications', apps, { conflict: '(posting_id, candidate_id) DO NOTHING' }),
      insert('application_events', events), insert('application_documents', docs), insert('interviews', interviews),
      insert('interview_panel', panel, { conflict: '(interview_id, user_id) DO NOTHING' }), insert('interview_scores', scores, { conflict: '(interview_id, panel_user_id, criterion_id) DO NOTHING' }),
      insert('offers', offers))

    // ---- onboarding ----
    out.push(insert('onboarding_templates', ONBOARDING.map(([k, name, d]) => ({ id: id('onb-tpl', k), school_id: S, name, department_id: dept(d), created_at: at('2026-06-01') }))))
    out.push(insert('onboarding_template_tasks', ONBOARDING.flatMap(([k, , , tasks]) => tasks.map(([title, cat, kind, role, off], i) => ({ id: id('onb-tpl-task', k, i), school_id: S,
      template_id: id('onb-tpl', k), sort: i + 1, title, category: cat, owner_kind: kind, owner_role: role, due_offset_days: off, created_at: at('2026-06-01') })))))
    const plans = []
    const tasks = []
    for (const h of unitHires) {
      const tk = h.code === 'GURU' ? 'guru' : 'staf'
      const pid = id('onb-plan', h.id)
      const tpl = ONBOARDING.find((x) => x[0] === tk)
      const start = new Date(`${h.hire_date}T00:00:00+07:00`)
      // July hires finished everything except the last training step of one.
      const allDone = h !== hiredTeacher
      plans.push({ id: pid, school_id: S, employee_id: h.id, template_id: id('onb-tpl', tk), start_date: h.hire_date, status: allDone ? 'completed' : 'active',
        created_at: at('2026-06-25'), updated_at: at('2026-08-01') })
      tpl[3].forEach(([title, cat, kind, role, off], i) => {
        const due = new Date(start.getTime() + off * 86400000).toISOString().slice(0, 10)
        const done = allDone || i < tpl[3].length - 1
        tasks.push({ id: id('onb-task', pid, i), school_id: S, plan_id: pid, title, category: cat, owner_user_id: kind === 'supervisor' ? KEPSEK : null,
          owner_role: kind === 'role' ? role : null, due_date: due, done_at: done ? at(due, '14:00') : null, done_by: done ? (kind === 'role' ? ADMIN : kind === 'supervisor' ? KEPSEK : null) : null,
          created_at: at('2026-06-25') })
      })
    }
    out.push(insert('onboarding_plans', plans, { conflict: '(employee_id) DO NOTHING' }), insert('onboarding_tasks', tasks))
  }
  return merge({ teratai_hr: out.join('\n\n') }, ap.sql())
}
