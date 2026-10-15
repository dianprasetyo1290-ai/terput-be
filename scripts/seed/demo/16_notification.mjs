// Phase 16: notifications & communication of SD and SMK — the notification
// templates (SMP's set), WhatsApp templates, campaign segments and campaigns
// (an in-app announcement to the parents of a grade, a WhatsApp invitation to
// the PPDB open house, a draft and a scheduled one) with their recipients
// and the parents' marketing consents, chat settings and threads between
// homeroom teachers and parents (one message reported to the moderator),
// notification preferences, and the in-app notifications those raise.
import { createHash } from 'node:crypto'
import { ACCOUNTS, at, cloneConfig, insert, merge, pgArray, rng, uid } from './lib.mjs'
import { MODEL } from './model.mjs'

const SMP = '11111111-1111-1111-1111-111111111111'
const { admin: ADMIN, humas: HUMAS, kepala_sekolah: KEPSEK } = ACCOUNTS

const e164 = (p) => '62' + p.replace(/^0/, '')
const hash = (p) => createHash('sha256').update(e164(p)).digest('hex')
const mask = (p) => { const x = e164(p); return `${x.slice(0, 4)}${'*'.repeat(x.length - 6)}${x.slice(-2)}` }

export function generate() {
  const out = []
  for (const u of MODEL.filter((x) => x.schoolId !== SMP)) {
    const r = rng('notification:' + u.key)
    const S = u.schoolId
    const smk = u.level === 'SMK'
    const id = (...k) => uid('notif', u.key, ...k)
    const parentOf = Object.fromEntries(u.parents.map((p) => [p.id, p]))
    out.push(`-- ===== ${u.level} =====`)
    out.push(cloneConfig('notification_templates', S))
    out.push(insert('campaign_settings', [{ school_id: S, monthly_wa_quota: smk ? 1500 : 1000, updated_at: at('2026-08-16') }]))
    out.push(insert('chat_settings', [{ school_id: S, service_start: '07:00', service_end: '16:00', service_days: pgArray([1, 2, 3, 4, 5], 'int4'),
      max_attachment_bytes: 5242880, updated_at: at('2026-08-16') }]))

    // ---- WhatsApp templates ----
    const next = smk ? 'SMK Teratai Putih 2027/2028' : 'SD Teratai Putih 2027/2028'
    const wa = [
      ['open-house', 'Undangan Open House PPDB', `teratai_${u.key}_open_house_2027`, `Halo {{1}}, ${next} mengundang Anda ke Open House PPDB pada {{2}}. Daftar di {{3}}.`, ['nama', 'tanggal', 'tautan'], 'approved'],
      ['daftar-ulang', 'Pengingat Daftar Ulang', `teratai_${u.key}_daftar_ulang`, 'Yth. {{1}}, batas daftar ulang calon siswa {{2}} adalah {{3}}. Abaikan bila sudah menyelesaikan.', ['nama', 'nama_siswa', 'batas'], 'approved'],
      smk
        ? ['job-fair', 'Undangan Job Fair BKK', 'teratai_smk_job_fair', 'Halo {{1}}, BKK SMK Teratai Putih mengadakan Job Fair bersama 12 mitra industri pada {{2}}. Info: {{3}}.', ['nama', 'tanggal', 'tautan'], 'pending']
        : ['pentas', 'Info Pentas Seni Akhir Semester', 'teratai_sd_pentas_seni', 'Halo {{1}}, saksikan Pentas Seni SD Teratai Putih {{2}}. Gratis untuk keluarga siswa.', ['nama', 'tanggal'], 'pending'],
    ]
    out.push(insert('wa_templates', wa.map(([k, name, provider, body, vars, status], i) => ({ id: id('wa', k), school_id: S, name, provider_template_id: provider, language: 'id',
      body_preview: body, variables: vars, approval_status: status, created_at: at(['2026-08-26', '2026-09-05', '2026-10-12'][i]),
      updated_at: at(['2026-08-28', '2026-09-06', '2026-10-12'][i]) }))))

    // ---- segments & campaigns ----
    const g1 = u.classes[0]
    const lowGrades = u.classes.slice(0, smk ? 2 : 3).map((c) => c.grade)
    const segs = [
      ['kelas-awal', `Orang Tua Kelas ${g1.grade}`, 'parents', { grade_levels: [g1.grade] }],
      ['semua', `Orang Tua Seluruh Kelas ${u.level}`, 'parents', { grade_levels: [...new Set(u.classes.map((c) => c.grade))] }],
      ['bawah', smk ? 'Orang Tua Kelas X' : 'Orang Tua Kelas 1-3', 'parents', { grade_levels: [...new Set(lowGrades)] }],
      ['ppdb', 'Pendaftar PPDB Belum Daftar Ulang', 'ppdb_applicants', { statuses: ['offered', 'reregistration'] }],
      ['alumni', 'Orang Tua Alumni 2026', 'alumni_parents', { graduation_year: 2026 }],
    ]
    out.push(insert('campaign_segments', segs.map(([k, name, kind, filter]) => ({ id: id('seg', k), school_id: S, name, kind, filter, created_by: HUMAS, created_at: at('2026-08-31') }))))
    const campaigns = [
      ['literasi', 'kelas-awal', 'in_app', null, {}, smk ? 'Kunjungan Industri Kelas X' : 'Pekan Literasi Kelas 1',
        smk ? 'Kunjungan industri kelas X ke PT Astra Honda Motor Cikarang pada 4 November 2026. Formulir izin orang tua dibagikan wali kelas.'
          : 'Pekan Literasi berlangsung 2-6 November 2026. Ananda diminta membawa satu buku cerita bergambar setiap hari.', 'sent', '2026-10-08T09:00'],
      ['open-house', 'alumni', 'whatsapp', 'open-house', { tanggal: 'Sabtu, 14 November 2026', tautan: `teratai.sch.id/ppdb/${u.key}` }, null, null, 'sent', '2026-09-25T09:00'],
      ['pts', 'semua', 'email', null, {}, 'Jadwal Penilaian Tengah Semester (PTS) Ganjil',
        'Penilaian Tengah Semester Ganjil dilaksanakan 26-30 Oktober 2026. Jadwal lengkap tersedia di Portal Orang Tua menu Ujian.', 'scheduled', '2026-10-19T07:00'],
      ['daftar-ulang', 'ppdb', 'whatsapp', 'daftar-ulang', { batas: '31 Oktober 2026' }, null, null, 'draft', null],
    ]
    out.push(insert('campaigns', campaigns.map(([k, seg, channel, tpl, vars, subject, body, status, when]) => ({ id: id('campaign', k), school_id: S, name: subject ?? wa.find((w) => w[0] === tpl)[1],
      segment_id: id('seg', seg), channel, template_id: tpl ? id('wa', tpl) : null, template_vars: vars, subject, body, scheduled_at: when ? `${when}:00+07:00` : null, status,
      created_by: HUMAS, created_at: at(status === 'draft' ? '2026-10-14' : '2026-09-20'), sent_at: status === 'sent' ? `${when}:00+07:00` : null }))))

    // Recipients: the in-app announcement reached every parent of the first class.
    const recips = []
    const notes = []
    const firstParents = [...new Set(g1.students.map((s) => s.parentId))].map((pid) => parentOf[pid]).filter(Boolean)
    for (const p of firstParents) {
      const read = r.chance(0.7)
      const nid = id('n-campaign', 'literasi', p.id)
      recips.push({ id: id('rcp', 'literasi', p.id), school_id: S, campaign_id: id('campaign', 'literasi'), subject_key: `user:${p.id}`, user_id: p.id, display_name: p.name,
        contact_masked: '', contact_hash: '', status: read ? 'read' : 'delivered', notification_id: nid, created_at: at('2026-10-08'), sent_at: at('2026-10-08', '09:00'),
        delivered_at: at('2026-10-08', '09:00'), read_at: read ? at('2026-10-08', `1${r.int(0, 9)}:${String(r.int(10, 59))}`) : null })
      notes.push({ id: nid, school_id: S, user_id: p.id, type: 'campaign', title: campaigns[0][5], body: campaigns[0][6], data: { campaign_id: id('campaign', 'literasi') },
        idempotency_key: `demo:campaign:literasi:${p.id}`, read_at: read ? at('2026-10-08', '12:00') : null, created_at: at('2026-10-08', '09:00') })
    }
    // WhatsApp invitation: alumni parents, mostly delivered; some without consent.
    const alumni = u.parents.slice(-12)
    alumni.forEach((p, i) => {
      const st = i === 2 ? 'excluded' : i === 5 ? 'failed' : i === 8 ? 'excluded' : i % 3 === 0 ? 'replied' : i % 2 ? 'read' : 'delivered'
      recips.push({ id: id('rcp', 'open-house', p.id), school_id: S, campaign_id: id('campaign', 'open-house'), subject_key: `phone:${hash(p.phone)}`, user_id: null,
        display_name: `${p.gender === 'L' ? 'Bapak' : 'Ibu'} ${p.name}`, contact_masked: mask(p.phone), contact_hash: hash(p.phone), status: st,
        exclude_reason: i === 2 ? 'no_consent' : i === 8 ? 'opted_out' : null, provider_message_id: ['excluded', 'failed'].includes(st) ? null : `wamid.${uid('wa', u.key, i).slice(0, 18)}`,
        last_error: st === 'failed' ? 'Nomor tidak terdaftar di WhatsApp' : null, created_at: at('2026-09-20'),
        sent_at: ['excluded', 'failed'].includes(st) ? null : at('2026-09-25', '09:00'), delivered_at: ['delivered', 'read', 'replied'].includes(st) ? at('2026-09-25', '09:01') : null,
        read_at: ['read', 'replied'].includes(st) ? at('2026-09-25', '11:20') : null, replied_at: st === 'replied' ? at('2026-09-25', '11:35') : null })
    })
    out.push(insert('campaign_recipients', recips, { conflict: '(campaign_id, subject_key) DO NOTHING' }))

    // Marketing consents of the alumni parents (by phone) and of a few parent accounts.
    const consents = []
    const consentEvents = []
    alumni.forEach((p, i) => {
      const status = i === 2 ? null : i === 8 ? 'opted_out' : 'opted_in'
      if (!status) return
      const key = `phone:${hash(p.phone)}`
      consents.push({ id: id('consent', key, 'wa'), school_id: S, subject_key: key, channel: 'whatsapp', status, source: status === 'opted_out' ? 'wa_keyword' : 'ppdb_form',
        changed_at: at(status === 'opted_out' ? '2026-09-10' : '2026-06-20') })
      consentEvents.push({ id: id('consent-ev', key, 1), school_id: S, subject_key: key, channel: 'whatsapp', status: 'opted_in', source: 'ppdb_form', created_at: at('2026-06-20') })
      if (status === 'opted_out') consentEvents.push({ id: id('consent-ev', key, 2), school_id: S, subject_key: key, channel: 'whatsapp', status: 'opted_out', source: 'wa_keyword', created_at: at('2026-09-10') })
    })
    for (const p of firstParents.slice(0, 4)) {
      for (const ch of ['whatsapp', 'email']) {
        const out_ = ch === 'email' && p === firstParents[1]
        consents.push({ id: id('consent', p.id, ch), school_id: S, subject_key: `user:${p.id}`, user_id: p.id, channel: ch, status: out_ ? 'opted_out' : 'opted_in',
          source: out_ ? 'unsubscribe_link' : 'preferences', changed_at: at(out_ ? '2026-10-05' : '2026-08-16') })
        consentEvents.push({ id: id('consent-ev', p.id, ch), school_id: S, subject_key: `user:${p.id}`, channel: ch, status: out_ ? 'opted_out' : 'opted_in',
          source: out_ ? 'unsubscribe_link' : 'preferences', actor_user_id: out_ ? null : p.id, created_at: at(out_ ? '2026-10-05' : '2026-08-16') })
      }
    }
    out.push(insert('marketing_consents', consents, { conflict: '(school_id, subject_key, channel) DO NOTHING' }), insert('marketing_consent_events', consentEvents))

    // ---- chat: homeroom teacher <-> parents ----
    const threads = []
    const parts = []
    const msgs = []
    const reads = []
    const topics = smk
      ? [['Izin PKL dan jadwal kunjungan pembimbing', 'Selamat pagi Pak/Bu, ananda mulai PKL bulan depan. Apakah kunjungan pembimbing sudah dijadwalkan?',
        'Selamat pagi. Kunjungan pembimbing sekolah dijadwalkan minggu kedua PKL, nanti kami kabari lewat portal.'],
      ['Nilai praktik jaringan menurun', 'Bu, nilai praktik jaringan ananda turun. Apa yang bisa kami bantu di rumah?',
        'Terima kasih sudah menghubungi. Ananda perlu latihan konfigurasi router; modul latihan sudah kami unggah di menu Materi.'],
      ['Ananda sakit 2 hari', 'Ananda demam sejak semalam, mohon izin tidak masuk dua hari. Surat dokter menyusul.', 'Baik, semoga lekas sembuh. Izin sudah kami catat di presensi.']]
      : [['Perkembangan membaca ananda', 'Selamat pagi Bu Guru, bagaimana perkembangan membaca ananda di kelas?',
        'Selamat pagi. Ananda sudah lancar membaca suku kata; mohon dibiasakan membaca 15 menit di rumah.'],
      ['Bekal dan alergi kacang', 'Bu, mohon diingatkan ananda alergi kacang. Bekal dari rumah sudah kami siapkan.', 'Baik, sudah kami catat dan sampaikan ke petugas kantin dan UKS.'],
      ['Ananda sakit 2 hari', 'Ananda demam sejak semalam, mohon izin tidak masuk dua hari. Surat dokter menyusul.', 'Baik, semoga lekas sembuh. Izin sudah kami catat di presensi.']]
    u.classes.slice(0, 3).forEach((c, ci) => {
      const st = c.students[ci + 2]
      const parent = parentOf[st.parentId]
      if (!parent) return
      const tid = id('thread', c.id)
      const [subject, ask, answer] = topics[ci]
      const day = `2026-10-${String(6 + ci * 3).padStart(2, '0')}`
      threads.push({ id: tid, school_id: S, subject: `${st.name} - Wali Kelas ${c.name}: ${subject}`, created_by: parent.id, created_at: at(day, '07:15'), kind: 'teacher_parent',
        student_user_id: st.id, teacher_user_id: c.homeroom.id, guardian_user_id: parent.id })
      parts.push({ id: id('part', tid, 'p'), thread_id: tid, user_id: parent.id, role: 'owner', joined_at: at(day, '07:15') },
        { id: id('part', tid, 't'), thread_id: tid, user_id: c.homeroom.id, role: 'member', joined_at: at(day, '07:15') })
      const m1 = id('msg', tid, 1)
      const m2 = id('msg', tid, 2)
      msgs.push({ id: m1, thread_id: tid, sender_user_id: parent.id, body: ask, created_at: at(day, '07:15'), school_id: S },
        { id: m2, thread_id: tid, sender_user_id: c.homeroom.id, body: answer, created_at: at(day, '09:40'), school_id: S })
      reads.push({ id: id('read', m1), message_id: m1, user_id: c.homeroom.id, read_at: at(day, '09:30') }, { id: id('read', m2), message_id: m2, user_id: parent.id, read_at: at(day, '12:05') })
      if (ci === 0) {
        const m3 = id('msg', tid, 3)
        msgs.push({ id: m3, thread_id: tid, sender_user_id: parent.id, body: 'Baik Bu, terima kasih. Kalau ada tugas tambahan mohon dikabari ya.', created_at: at(day, '21:10'),
          outside_service_hours: true, school_id: S })
        notes.push({ id: id('n-chat', m3), school_id: S, user_id: c.homeroom.id, type: 'chat.message', title: `Pesan baru dari ${parent.name}`, body: 'Baik Bu, terima kasih. Kalau ada tugas tambahan mohon dikabari ya.',
          data: { url: `/chat/${tid}`, thread_id: tid }, idempotency_key: `demo:chat:${m3}`, created_at: at(day, '21:10') })
      }
    })
    // Staff coordination thread, archived after the event.
    const staffT = id('thread', 'panitia')
    const staff = [ADMIN, HUMAS, u.teachers[1].id, u.teachers[2].id]
    threads.push({ id: staffT, school_id: S, subject: smk ? 'Koordinasi Panitia Job Fair BKK' : 'Koordinasi Panitia Pentas Seni', created_by: ADMIN, created_at: at('2026-09-15'),
      closed_at: at('2026-10-10'), kind: 'general', archived_at: at('2026-10-10') })
    staff.forEach((x, i) => parts.push({ id: id('part', staffT, i), thread_id: staffT, user_id: x, role: i ? 'member' : 'owner', joined_at: at('2026-09-15') }))
    const staffMsgs = ['Rapat koordinasi panitia Kamis jam 13.00 di ruang guru.', 'Siap. Undangan dan poster sudah saya kirim ke grup orang tua.',
      'Mohon jangan kirim data pribadi siswa lewat grup ini ya, gunakan sistem.', 'Rundown final terlampir. Terima kasih semua!']
    staffMsgs.forEach((b, i) => msgs.push({ id: id('msg', staffT, i), thread_id: staffT, sender_user_id: staff[i], body: b, created_at: at(`2026-09-${15 + i * 4}`, '10:00'), school_id: S }))
    out.push(insert('chat_threads', threads), insert('chat_thread_participants', parts, { conflict: '(thread_id, user_id) DO NOTHING' }), insert('chat_messages', msgs),
      insert('chat_message_reads', reads, { conflict: '(message_id, user_id) DO NOTHING' }))
    out.push(insert('chat_message_reports', [{ id: id('report', 1), school_id: S, message_id: id('msg', staffT, 1), reporter_user_id: u.teachers[2].id,
      reason: 'Poster berisi foto siswa tanpa izin orang tua', status: 'reviewed', reviewed_by: ADMIN, review_note: 'Poster diganti dengan ilustrasi; panitia diingatkan soal izin foto.',
      reviewed_at: at('2026-09-20'), created_at: at('2026-09-19') },
    { id: id('report', 2), school_id: S, message_id: id('msg', id('thread', u.classes[0].id), 3), reporter_user_id: u.classes[0].homeroom.id,
      reason: 'Pesan dikirim di luar jam layanan', status: 'open', created_at: at('2026-10-06', '21:30') }], { conflict: '(message_id, reporter_user_id) DO NOTHING' }))

    // ---- preferences & announcements ----
    out.push(insert('notification_preferences', [
      [KEPSEK, 'email', false], [KEPSEK, 'whatsapp', false], [u.classes[0].homeroom.id, 'in_app', true], [u.classes[0].homeroom.id, 'whatsapp', true],
      [firstParents[0]?.id, 'email', false], [firstParents[0]?.id, 'push', true],
    ].filter(([x]) => x).map(([user, ch, on]) => ({ id: id('pref', user, ch), school_id: S, user_id: user, channel: ch, enabled: on, created_at: at('2026-08-26') })),
    { conflict: '(school_id, user_id, channel) DO NOTHING' }))
    for (const c of u.classes) {
      notes.push({ id: id('n-pts', c.id), school_id: S, user_id: c.homeroom.id, type: 'academic.schedule.published', title: 'Jadwal PTS Ganjil telah terbit',
        body: `Jadwal Penilaian Tengah Semester untuk ${c.name} (26-30 Oktober 2026) sudah dapat dilihat siswa dan orang tua.`, data: { action: '/ujian' },
        idempotency_key: `demo:pts:${c.id}`, read_at: r.chance(0.5) ? at('2026-10-13', '10:00') : null, created_at: at('2026-10-12', '14:00') })
    }
    notes.push({ id: id('n-welcome'), school_id: S, user_id: ADMIN, type: 'system.welcome', title: `Selamat datang di unit ${u.level} Teratai Putih`,
      body: 'Seluruh modul unit ini sudah aktif. Gunakan pemilih unit di sidebar untuk berpindah antar-unit.', data: { action: '/dashboard' }, idempotency_key: `demo:welcome:${u.key}`,
      created_at: at('2026-07-13', '06:00'), read_at: at('2026-07-13', '07:00') })
    out.push(insert('notifications', notes, { conflict: '(school_id, idempotency_key) DO NOTHING' }))
    out.push(insert('notification_deliveries', notes.flatMap((n) => {
      const d = [{ id: id('dlv', n.id, 'in_app'), notification_id: n.id, channel: 'in_app', status: 'sent', attempts: 1, created_at: n.created_at, updated_at: n.created_at }]
      if (n.type === 'academic.schedule.published') d.push({ id: id('dlv', n.id, 'email'), notification_id: n.id, channel: 'email', status: 'sent', attempts: 1,
        provider_message_id: `demo-email-${n.id.slice(0, 8)}`, created_at: n.created_at, updated_at: n.created_at })
      return d
    }), { conflict: '(notification_id, channel) DO NOTHING' }))
  }
  // Consents of every parent account in the three units (SMP included: its
  // base seed only had a handful), so segment previews and campaigns have an
  // eligible audience — only an explicit opt-in per channel is sent
  // (BR-COM-01). In-app at account activation, WhatsApp on the PPDB form,
  // email in the preferences; a few parents opted out.
  for (const u of MODEL) {
    const consents = []
    const events = []
    for (const p of u.parents) {
      const r = rng('consent:' + p.id)
      for (const [ch, share, source, when] of [['in_app', 0.95, 'preferences', '2026-07-13'], ['whatsapp', 0.7, 'ppdb_form', '2026-06-20'], ['email', 0.5, 'preferences', '2026-08-16']]) {
        const x = r.next()
        if (x >= share + 0.05) continue // no answer: stays no_consent
        const status = x < share ? 'opted_in' : 'opted_out'
        const key = `user:${p.id}`
        consents.push({ id: uid('notif-consent', u.key, p.id, ch), school_id: u.schoolId, subject_key: key, user_id: p.id, channel: ch, status,
          source: status === 'opted_out' ? (ch === 'whatsapp' ? 'wa_keyword' : 'unsubscribe_link') : source, changed_at: at(status === 'opted_out' ? '2026-09-12' : when) })
        events.push({ id: uid('notif-consent-ev', u.key, p.id, ch), school_id: u.schoolId, subject_key: key, channel: ch, status,
          source: status === 'opted_out' ? (ch === 'whatsapp' ? 'wa_keyword' : 'unsubscribe_link') : source, actor_user_id: p.id, created_at: at(status === 'opted_out' ? '2026-09-12' : when) })
      }
    }
    out.push(`-- ===== consents ${u.level} =====`, insert('marketing_consents', consents, { conflict: '(school_id, subject_key, channel) DO NOTHING' }),
      insert('marketing_consent_events', events))
  }
  return merge({ teratai_notification: out.join('\n\n') })
}
