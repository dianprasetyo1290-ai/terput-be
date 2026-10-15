// Phase 17: service desk of SD and SMK — support teams (IT, facilities,
// academic & administrative services) with their agents, ticket categories
// (one fed by the parents' letter requests), SLA policies per priority with
// warning / breach escalation, business hours and holidays, the parents'
// service requests, and tickets from the parent, student and employee
// portals in every state — one breached its SLA and was escalated, one
// became a facility work order (11_facilities), resolved ones rated.
import { ACCOUNTS, at, insert, merge, pgArray, uid } from './lib.mjs'
import { MODEL } from './model.mjs'

const SMP = '11111111-1111-1111-1111-111111111111'
const { admin: ADMIN, servicedesk_agent: AGENT, servicedesk_manager: SDM, teknisi: TEKNISI, facility_officer: FASILITAS } = ACCOUNTS
const H = 3600000

export function generate() {
  const out = []
  for (const u of MODEL.filter((x) => x.schoolId !== SMP)) {
    const S = u.schoolId
    const smk = u.level === 'SMK'
    const id = (...k) => uid('sd', u.key, ...k)
    out.push(`-- ===== ${u.level} =====`)

    // ---- teams ----
    const teams = [['it', 'Tim IT Support & Jaringan', SDM, [SDM, AGENT]], ['sarpras', 'Tim Sarana & Prasarana', FASILITAS, [FASILITAS, TEKNISI]],
      ['layanan', 'Tim Layanan Akademik & Tata Usaha', ADMIN, [ADMIN, AGENT]]]
    out.push(insert('sd_teams', teams.map(([k, name, mgr]) => ({ id: id('team', k), school_id: S, name, manager_user_id: mgr, created_at: at('2026-07-06') }))))
    out.push(insert('sd_team_members', teams.flatMap(([k, , , members]) => members.map((m) => ({ team_id: id('team', k), user_id: m, school_id: S })))))

    // ---- service request categories (parents' letters) ----
    const srCats = [
      ['surat-aktif', 'Permohonan Surat Keterangan Aktif Siswa', 'Surat resmi keterangan siswa aktif untuk tunjangan, BPJS, atau beasiswa', 'admin', 24],
      ['legalisir', 'Legalisir Rapor dan Ijazah', 'Legalisir salinan rapor/ijazah untuk pendaftaran sekolah lanjutan', 'admin', 48],
      ['izin', 'Izin Tidak Masuk Lebih dari 3 Hari', 'Permohonan izin siswa untuk keperluan keluarga lebih dari tiga hari', 'walikelas', 24],
      smk ? ['pkl', 'Surat Pengantar PKL', 'Surat pengantar praktik kerja lapangan ke perusahaan mitra', 'koordinator_pkl', 48]
        : ['mutasi', 'Permohonan Surat Pindah Sekolah', 'Surat keterangan pindah dan rekomendasi untuk mutasi keluar', 'admin', 72],
    ]
    out.push(insert('service_request_categories', srCats.map(([k, name, desc, role, sla]) => ({ id: id('sr-cat', k), school_id: S, name, description: desc, handler_role: role,
      sla_hours: sla, requires_student: true, active: true, created_at: at('2026-07-06') }))))

    // ---- ticket categories ----
    const cats = [
      ['akun', 'Akun & Login Portal', 'Lupa password, akun terkunci, atau tidak bisa masuk portal', 'layanan', false, ['orang_tua', 'siswa', 'pegawai'],
        [{ key: 'portal', type: 'select', label: 'Portal', options: ['Orang Tua', 'Siswa', 'Pegawai'], required: true }], null, false],
      ['jaringan', 'Gangguan Jaringan & Internet', 'Wi-Fi kelas/lab lambat atau terputus', 'it', false, ['pegawai', 'siswa'],
        [{ key: 'lokasi', type: 'text', label: 'Lokasi / ruang', required: true }], null, false],
      ['fasilitas', 'Kerusakan Fasilitas & Kelistrikan', 'AC, proyektor, lampu, atau kelistrikan ruang bermasalah', 'sarpras', true, ['pegawai', 'orang_tua', 'siswa'],
        [{ key: 'lokasi', type: 'text', label: 'Lokasi / ruang', required: true }], null, true],
      ['tagihan', 'Tagihan & Pembayaran', 'Pertanyaan tagihan SPP, virtual account, atau kuitansi', 'layanan', false, ['orang_tua'], [], null, false],
      ['surat', 'Layanan Surat & Administrasi', 'Permohonan surat keterangan aktif dari portal orang tua', 'layanan', false, ['orang_tua', 'siswa'], [], 'surat-aktif', false],
      smk ? ['lab', 'Perangkat Lab & Bengkel', 'PC lab, router, atau alat praktik bengkel bermasalah', 'it', false, ['pegawai', 'siswa'],
        [{ key: 'unit', type: 'text', label: 'Nomor PC / alat', required: false }], null, false]
        : ['elearning', 'Kendala Pembelajaran Daring', 'Tidak bisa membuka materi atau mengumpulkan tugas', 'it', false, ['orang_tua', 'siswa'], [], null, false],
    ]
    out.push(insert('sd_categories', cats.map(([k, name, desc, team, facility, kinds, schema, srCat, fallback]) => ({ id: id('cat', k), school_id: S, name, description: desc,
      team_id: id('team', team), is_facility: facility, form_schema: schema, reporter_kinds: pgArray(kinds), service_request_category_id: srCat ? id('sr-cat', srCat) : null,
      is_fallback: fallback, active: true, created_at: at('2026-07-06') }))))

    // ---- SLA, escalation, hours, holidays ----
    const pol = [['low', 2, 'business_days', 5, 'business_days', 8000], ['normal', 8, 'business_hours', 3, 'business_days', 8000],
      ['high', 4, 'business_hours', 1, 'business_days', 7500], ['urgent', 1, 'business_hours', 6, 'business_hours', 7000]]
    const policies = pol.map(([p, rv, ru, sv, su, warn]) => ({ id: id('sla', p), school_id: S, category_id: null, priority: p, response_value: rv, response_unit: ru,
      resolution_value: sv, resolution_unit: su, warn_bp: warn, effective_from: '2026-07-01', created_at: at('2026-07-06') }))
    const rules = policies.flatMap((p) => [
      { id: id('esc', p.priority, 'warning'), school_id: S, policy_id: p.id, trigger: 'warning', level: 1, target_kind: 'team_manager' },
      ...(p.priority === 'low' ? [] : [{ id: id('esc', p.priority, 'breach'), school_id: S, policy_id: p.id, trigger: 'breach', level: 1, target_kind: 'role', target_role: 'servicedesk_manager' }]),
      ...(p.priority === 'urgent' ? [{ id: id('esc', p.priority, 'breach2'), school_id: S, policy_id: p.id, trigger: 'breach', level: 2, target_kind: 'role', target_role: 'kepala_sekolah' }] : []),
    ])
    out.push(insert('sd_sla_policies', policies), insert('sd_escalation_rules', rules, { conflict: '(policy_id, trigger, level) DO NOTHING' }))
    out.push(insert('sd_business_hours', [1, 2, 3, 4, 5, 6].map((d) => ({ school_id: S, weekday: d, opens_at: '07:00', closes_at: d === 6 ? '12:00' : '15:00' }))))
    out.push(insert('sd_holidays', [['2026-12-24', 'Cuti Bersama Natal'], ['2026-12-25', 'Hari Raya Natal'], ['2027-01-01', 'Tahun Baru Masehi'], ['2027-02-17', 'Tahun Baru Imlek']]
      .map(([d, name]) => ({ id: id('holiday', d), school_id: S, date: d, name })), { conflict: '(school_id, date) DO NOTHING' }))
    const snapshot = (prio) => {
      const p = policies.find((x) => x.priority === prio)
      return { id: p.id, school_id: S, category_id: null, priority: prio, response_value: p.response_value, response_unit: p.response_unit, resolution_value: p.resolution_value,
        resolution_unit: p.resolution_unit, warn_bp: p.warn_bp, effective_from: p.effective_from, effective_to: null,
        escalation_rules: rules.filter((r) => r.policy_id === p.id).map(({ school_id, ...r }) => r) }
    }

    // ---- service requests from parents ----
    const st = (i) => u.students[i]
    const parent = (s) => u.parents.find((p) => p.id === s.parentId) ?? u.parents[0]
    const requests = [
      ['sr1', 'surat-aktif', st(4), 'Surat Keterangan Aktif untuk Tunjangan Kantor', 'Mohon diterbitkan surat keterangan aktif belajar untuk verifikasi tunjangan anak di kantor.', 'in_progress', ADMIN, '2026-10-13T08:20'],
      ['sr2', 'legalisir', st(30), 'Legalisir rapor semester 1-2', 'Mohon legalisir 3 lembar rapor untuk pendaftaran lomba tingkat kota.', 'done', ADMIN, '2026-09-22T10:05'],
      ['sr3', 'izin', st(51), 'Izin acara keluarga di luar kota 4 hari', 'Ananda izin 26-29 Oktober menghadiri pernikahan kakak di Medan.', 'accepted', u.classes[2].homeroom.id, '2026-10-12T19:30'],
      smk ? ['sr4', 'pkl', st(95), 'Surat pengantar PKL ke PT Mitra Data Nusantara', 'Ananda diterima PKL mulai 2 November; perusahaan meminta surat pengantar sekolah.', 'submitted', null, '2026-10-14T13:10']
        : ['sr4', 'mutasi', st(95), 'Surat pindah karena orang tua pindah tugas', 'Kami pindah ke Surabaya akhir November; mohon surat keterangan pindah.', 'submitted', null, '2026-10-14T13:10'],
      ['sr5', 'legalisir', st(70), 'Legalisir ijazah kakak (alumni)', 'Mohon legalisir ijazah kakak ananda yang lulus 2024.', 'rejected', ADMIN, '2026-09-03T09:00'],
    ]
    out.push(insert('service_requests', requests.map(([k, cat, s, subject, desc, status, assignee, t]) => ({ id: id(k), school_id: S, category_id: id('sr-cat', cat),
      requester_user_id: s.parentId, student_user_id: s.id, subject, description: desc, status, assignee_user_id: assignee,
      result_attachments: status === 'done' ? [{ name: 'rapor-legalisir.pdf', key: `servicedesk/${u.key}/${k}/rapor-legalisir.pdf` }] : [],
      created_at: `${t}:00+07:00`, updated_at: `${t}:00+07:00` }))))
    const srEvents = []
    for (const [k, , s, , , status, assignee, t] of requests) {
      const t0 = new Date(`${t}:00+07:00`)
      const ev = (n, from, to, note, actor, hours) => srEvents.push({ id: id('sr-ev', k, n), school_id: S, request_id: id(k), from_status: from, to_status: to, note, actor_user_id: actor,
        created_at: new Date(t0.getTime() + hours * H).toISOString() })
      ev(1, null, 'submitted', 'Permintaan dibuat', s.parentId, 0)
      if (status === 'accepted' || status === 'in_progress' || status === 'done') ev(2, 'submitted', 'accepted', 'Permintaan diterima.', assignee, 13)
      if (status === 'in_progress' || status === 'done') ev(3, 'accepted', 'in_progress', 'Dokumen sedang disiapkan tata usaha.', assignee, 15)
      if (status === 'done') ev(4, 'in_progress', 'done', 'Legalisir selesai, dapat diambil di ruang TU atau diunduh.', assignee, 30)
      if (status === 'rejected') ev(2, 'submitted', 'rejected', `Ijazah alumni dilegalisir langsung oleh alumni yang bersangkutan dengan membawa ijazah asli.`, assignee, 20)
    }
    out.push(insert('service_request_events', srEvents))

    // ---- tickets ----
    const wo = (i) => ({ id: uid('fac-wo', u.key, i), number: `WO-${u.level}-2610-${String(i + 1).padStart(3, '0')}` })
    const teacher = u.teachers[3]
    const T = [
      // key, cat, team, prio, subject, desc, answers, channel, reporter, student, status, assignee, created, extra
      ['t1', 'akun', 'layanan', 'normal', 'Tidak bisa masuk Portal Orang Tua', 'Setelah ganti nomor HP, saya tidak menerima kode OTP saat login.', { portal: 'Orang Tua' },
        'parent_portal', parent(st(8)).id, st(8).id, 'resolved', AGENT, '2026-10-05T19:12', { first: 14, resolved: 20, rating: [5, 'Cepat dan jelas, terima kasih.'] }],
      ['t2', 'jaringan', 'it', 'high', `Wi-Fi ${u.classes[4].name} terputus saat asesmen`, 'Koneksi Wi-Fi putus berulang ketika siswa mengerjakan asesmen daring.', { lokasi: `Ruang ${u.classes[4].name}` },
        'employee_portal', u.classes[4].homeroom.id, null, 'in_progress', SDM, '2026-10-14T08:05', { first: 1 }],
      ['t3', 'fasilitas', 'sarpras', 'high', `Proyektor ${u.classes[1].name} tidak menyala`, 'Lampu indikator proyektor berkedip merah, gambar tidak keluar.', { lokasi: `Ruang ${u.classes[1].name}` },
        'employee_portal', u.classes[1].homeroom.id, null, 'in_progress', TEKNISI, '2026-10-03T07:40', { first: 1, wo: wo(0) }],
      ['t4', 'tagihan', 'layanan', 'normal', 'Virtual account SPP Oktober tidak muncul', 'Di aplikasi bank, nomor VA SPP Oktober tertulis tidak ditemukan.', {},
        'parent_portal', parent(st(17)).id, st(17).id, 'closed', AGENT, '2026-10-02T20:30', { first: 13, resolved: 16, closed: 72, rating: [4, 'Sudah bisa bayar, tapi responsnya baru besok paginya.'] }],
      ['t5', 'akun', 'layanan', 'low', 'Ganti email akun guru', 'Mohon ganti email login ke alamat sekolah yang baru.', { portal: 'Pegawai' },
        'employee_portal', teacher.id, null, 'assigned', AGENT, '2026-10-14T10:15', {}],
      ['t6', smk ? 'lab' : 'elearning', 'it', 'normal', smk ? 'PC lab nomor 7 tidak bisa booting' : 'Tidak bisa membuka materi Bahasa Indonesia',
        smk ? 'Muncul pesan "no bootable device" pada PC nomor 7 lab komputer.' : 'Materi yang diunggah guru menampilkan halaman kosong di HP ananda.',
        smk ? { unit: 'PC-07' } : {}, smk ? 'student_portal' : 'parent_portal', smk ? st(85).id : parent(st(12)).id, smk ? null : st(12).id, 'new', null, '2026-10-15T07:20', {}],
      ['t7', 'fasilitas', 'sarpras', 'urgent', 'Korsleting stopkontak ruang guru', 'Stopkontak dekat dispenser mengeluarkan percikan api, listrik ruang guru dimatikan.', { lokasi: 'Ruang Guru' },
        'employee_portal', ADMIN, null, 'resolved', TEKNISI, '2026-09-29T09:10', { first: 0.5, resolved: 26, breach: true, rating: [3, 'Selesai, tapi baru ditangani esok harinya.'] }],
      ['t8', 'surat', 'layanan', 'normal', 'Surat Keterangan Aktif untuk Tunjangan Kantor', 'Mohon diterbitkan surat keterangan aktif belajar untuk verifikasi tunjangan anak di kantor.', {},
        'parent_portal', st(4).parentId, st(4).id, 'in_progress', ADMIN, '2026-10-13T08:20', { first: 13, sr: id('sr1') }],
      ['t9', 'akun', 'layanan', 'normal', 'Akun siswa terkunci', 'Siswa salah memasukkan password lima kali sehingga akun terkunci.', { portal: 'Siswa' },
        'student_portal', st(60).id, null, 'rejected', AGENT, '2026-09-18T11:00', { first: 2, note: 'Duplikat dari tiket sebelumnya; akun sudah dibuka melalui tiket wali kelas.' }],
    ]
    const tickets = []
    const events = []
    T.forEach(([k, cat, team, prio, subject, desc, answers, channel, reporter, student, status, assignee, t, x], i) => {
      const t0 = new Date(`${t}:00+07:00`)
      const p = policies.find((q) => q.priority === prio)
      const respH = p.response_unit === 'business_hours' ? p.response_value : p.response_value * 8
      const resH = p.resolution_unit === 'business_hours' ? p.resolution_value : p.resolution_value * 8
      const iso = (h) => (h === undefined ? null : new Date(t0.getTime() + h * H).toISOString())
      tickets.push({ id: id('ticket', k), school_id: S, number: `SD-2026-${String(i + 1).padStart(6, '0')}`, category_id: id('cat', cat), team_id: id('team', team), priority: prio,
        subject, description: desc, form_answers: answers, channel, reporter_user_id: reporter, student_user_id: student, service_request_id: x.sr ?? null, status,
        assignee_user_id: assignee, sla_snapshot: snapshot(prio), response_due_at: iso(respH * 1.6), resolution_due_at: iso(resH * 2.4), first_responded_at: iso(x.first),
        resolved_at: iso(x.resolved), closed_at: iso(x.closed), response_breached_at: null, resolution_breached_at: x.breach ? iso(resH * 2.4) : null,
        escalation_level: x.breach ? 1 : 0, work_order_id: x.wo?.id ?? null, work_order_number: x.wo?.number ?? null, satisfaction_score: x.rating?.[0] ?? null,
        satisfaction_comment: x.rating?.[1] ?? null, rated_at: x.rating ? iso((x.closed ?? x.resolved) + 2) : null, idempotency_key: `demo:ticket:${u.key}:${k}`,
        created_at: t0.toISOString(), updated_at: iso(x.closed ?? x.resolved ?? x.first ?? 0) })
      const ev = (n, kind, h, o = {}) => events.push({ id: id('tev', k, n), school_id: S, ticket_id: id('ticket', k), kind, actor_user_id: o.actor ?? assignee ?? ADMIN,
        from_status: o.from ?? null, to_status: o.to ?? null, body: o.body ?? null, internal: o.internal ?? false, dedupe_key: `demo:${k}:${n}`, created_at: iso(h) })
      if (status === 'new') return
      ev(1, 'assign', 0.2, { body: `Ditugaskan ke tim ${teams.find((q) => q[0] === team)[1]}.`, actor: ADMIN })
      ev(2, 'status', 0.2, { from: 'new', to: 'assigned' })
      if (x.first !== undefined && status !== 'assigned') ev(3, 'status', x.first, { from: 'assigned', to: status === 'rejected' ? 'rejected' : 'in_progress', body: x.note ?? null })
      if (status === 'rejected') return
      if (k === 't1') ev(4, 'comment', x.first + 0.1, { body: 'Nomor HP sudah kami perbarui. Silakan coba login ulang dan minta OTP baru.' })
      if (k === 't2') ev(4, 'comment', 1.5, { body: 'Access point kelas kelebihan beban; dipasang AP tambahan sementara.', internal: true })
      if (x.wo) ev(5, 'work_order', x.first + 0.5, { body: `Dibuatkan perintah kerja ${x.wo.number}.` })
      if (x.breach) {
        ev(6, 'sla_breach', resH * 2.4, { body: 'Batas waktu penyelesaian terlewati.', actor: null })
        ev(7, 'escalation', resH * 2.4 + 0.05, { body: 'Dieskalasi ke manajer service desk (level 1).', actor: null })
      }
      if (x.resolved !== undefined) ev(8, 'status', x.resolved, { from: 'in_progress', to: 'resolved', body: k === 't7' ? 'Stopkontak diganti dan jalur kabel diperiksa.' : 'Masalah selesai.' })
      if (x.closed !== undefined) ev(9, 'status', x.closed, { from: 'resolved', to: 'closed', actor: null, body: 'Ditutup otomatis setelah 3 hari tanpa tanggapan.' })
      if (x.rating) ev(10, 'satisfaction', (x.closed ?? x.resolved) + 2, { body: `Rating ${x.rating[0]}/5: ${x.rating[1]}`, actor: reporter })
    })
    out.push(insert('sd_tickets', tickets, { conflict: '(school_id, idempotency_key) DO NOTHING' }), insert('sd_ticket_events', events, { conflict: '(ticket_id, dedupe_key) DO NOTHING' }))
    out.push(`INSERT INTO sd_ticket_sequences (school_id, year, last) VALUES ('${S}', 2026, ${T.length})
  ON CONFLICT (school_id, year) DO UPDATE SET last = GREATEST(sd_ticket_sequences.last, EXCLUDED.last);`)
  }
  return merge({ teratai_core: out.join('\n\n') })
}
