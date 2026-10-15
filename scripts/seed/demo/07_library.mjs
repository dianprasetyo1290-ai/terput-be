// Phase 7: the libraries of the three units — catalogue (textbooks of the
// level, Indonesian fiction, references) with copies, members (every student
// and teacher; SMP's are synced by the ops service already), loans July to
// October (mostly returned, some still out, a few overdue with fines),
// visits and reservations. SD / SMK first get SMP's library setup.
import { addDays, date, dbRows, insert, q, rng, uid } from './lib.mjs'
import { MODEL } from './model.mjs'

const SMP = '11111111-1111-1111-1111-111111111111'
const ADMIN = '22222222-2222-2222-2222-222222222222'
const TODAY = '2026-10-15'

const CATEGORIES = [['CAT-PELAJARAN', 'Buku Teks Pelajaran'], ['CAT-FIKSI', 'Fiksi & Sastra'], ['CAT-REFERENSI', 'Referensi & Kamus'], ['CAT-SAINS', 'Sains & Pengetahuan Umum']]
const FICTION = [
  ['Laskar Pelangi', 'Andrea Hirata', 'Bentang Pustaka', 2005, 89000], ['Sang Pemimpi', 'Andrea Hirata', 'Bentang Pustaka', 2006, 79000],
  ['Bumi', 'Tere Liye', 'Gramedia Pustaka Utama', 2014, 98000], ['Bulan', 'Tere Liye', 'Gramedia Pustaka Utama', 2015, 98000],
  ['Negeri 5 Menara', 'Ahmad Fuadi', 'Gramedia Pustaka Utama', 2009, 92000], ['Ayah', 'Andrea Hirata', 'Bentang Pustaka', 2015, 99000],
  ['Si Kancil dan Kawan-kawan', 'Murti Bunanta', 'Kelompok Pencinta Bacaan Anak', 2018, 55000], ['Seri Kecil-Kecil Punya Karya', 'Berbagai Penulis', 'Mizan', 2019, 45000],
  ['Pulang', 'Tere Liye', 'Republika', 2015, 89000], ['Hujan', 'Tere Liye', 'Gramedia Pustaka Utama', 2016, 95000],
  ['Rindu', 'Tere Liye', 'Republika', 2014, 89000], ['Cerita Rakyat Nusantara', 'Tim Penulis', 'Bhuana Ilmu Populer', 2020, 65000],
]
const REFERENCE = [
  ['Kamus Besar Bahasa Indonesia Edisi Kelima', 'Badan Bahasa', 'Balai Pustaka', 2016, 350000], ['Kamus Inggris-Indonesia', 'John M. Echols & Hassan Shadily', 'Gramedia', 2014, 180000],
  ['Atlas Indonesia dan Dunia', 'Tim Geografi', 'Erlangga', 2021, 120000], ['Ensiklopedia Sains untuk Pelajar', 'Tim Penulis', 'Lentera Abadi', 2019, 250000],
]
const SCIENCE = [
  ['Sapiens: Riwayat Singkat Umat Manusia', 'Yuval Noah Harari', 'Kepustakaan Populer Gramedia', 2017, 125000], ['Seri Sains Anak: Tata Surya', 'Tim Penulis', 'Elex Media', 2020, 70000],
  ['Kosmos', 'Carl Sagan', 'Kepustakaan Populer Gramedia', 2016, 140000], ['Robotika untuk Pemula', 'Budi Raharjo', 'Informatika', 2021, 110000],
]
const TEXTBOOKS = {
  SD: ['Bahasa Indonesia SD Kelas', 'Matematika SD Kelas', 'IPAS SD Kelas', 'Pendidikan Pancasila SD Kelas'],
  SMP: ['Matematika SMP Kelas', 'Bahasa Indonesia SMP Kelas', 'IPA SMP Kelas', 'IPS SMP Kelas', 'English for Nusantara SMP Kelas'],
  SMK: ['Matematika SMK Kelas', 'Bahasa Indonesia SMK Kelas', 'Dasar-dasar Teknik Jaringan Komputer Kelas', 'Dasar-dasar Akuntansi Kelas', 'Bahasa Inggris SMK Kelas'],
}
const ROMAN = { 1: 'I', 2: 'II', 3: 'III', 4: 'IV', 5: 'V', 6: 'VI', 7: 'VII', 8: 'VIII', 9: 'IX', 10: 'X', 11: 'XI', 12: 'XII' }

export function generate() {
  const out = []
  const smpCats = Object.fromEntries(dbRows('teratai_ops', `SELECT code, id FROM lib_collection_categories WHERE school_id = '${SMP}' AND deleted_at IS NULL`).map((x) => [x.code, x.id]))
  const smpMemberCats = dbRows('teratai_ops', `SELECT * FROM lib_member_categories WHERE school_id = '${SMP}' AND deleted_at IS NULL`)
  const smpMembers = Object.fromEntries(dbRows('teratai_ops', `SELECT user_id, id FROM lib_members WHERE school_id = '${SMP}'`).map((x) => [x.user_id, x.id]))
  const smpTitles = new Set(dbRows('teratai_ops', `SELECT lower(title) t FROM lib_titles WHERE school_id = '${SMP}' AND deleted_at IS NULL`).map((x) => x.t))

  for (const u of MODEL) {
    const r = rng('library:' + u.key)
    const S = u.schoolId
    out.push(`-- ===== ${u.level} =====`)

    // ---- setup ----
    let cat = smpCats
    let memberCats = smpMemberCats
    if (S !== SMP) {
      cat = Object.fromEntries(CATEGORIES.map(([c]) => [c, uid('lib-cat', u.key, c)]))
      out.push(insert('lib_collection_categories', CATEGORIES.map(([c, name]) => ({ id: cat[c], school_id: S, code: c, name }))))
      memberCats = smpMemberCats.map((m) => ({ ...m, id: uid('lib-member-cat', u.key, m.name), school_id: S }))
      out.push(insert('lib_member_categories', memberCats.map(({ created_at, updated_at, deleted_at, ...m }) => m)))
      out.push(insert('lib_fine_rates', memberCats.filter((m) => m.is_default).map((m) => ({ id: uid('lib-fine-rate', u.key, m.applies_to), school_id: S,
        member_category_id: m.id, late_fee_per_day_idr: m.applies_to === 'student' ? 500 : 1000, lost_fee_idr: null, effective_from: '2026-07-01' }))))
      out.push(insert('lib_settings', [{ school_id: S, fine_policy: 'pay_at_library', reminder_days_before: 2 }], { conflict: '(school_id) DO NOTHING' }))
    }
    const defaultCat = (kind) => memberCats.find((m) => m.applies_to === kind && m.is_default) ?? memberCats.find((m) => m.applies_to === kind)

    // ---- members ----
    const members = []
    const memberOf = {}
    for (const [kind, list] of [['student', u.students], ['employee', u.teachers]]) {
      for (const p of list) {
        const existing = S === SMP ? smpMembers[p.id] : null
        const id = existing ?? uid('lib-member', S, p.id)
        memberOf[p.id] = { id, kind, classId: p.classId }
        if (!existing) members.push({ id, school_id: S, kind, user_id: p.id, category_id: defaultCat(kind).id, member_code: `LIB-${kind === 'student' ? 'S' : 'E'}-${p.id.slice(0, 8)}`,
          status: 'active', synced_at: '2026-07-13T07:00:00+07:00' })
      }
    }
    out.push(insert('lib_members', members, { conflict: '(school_id, user_id) DO NOTHING' }))

    // ---- catalogue ----
    const grades = [...new Set(u.classes.map((c) => Number(c.grade)))]
    const titles = []
    for (const g of grades) for (const tb of TEXTBOOKS[u.level]) titles.push([`${tb} ${ROMAN[g]} Kurikulum Merdeka`, 'Kemendikbudristek', 'Pusat Perbukuan', 2026, 80000, 'CAT-PELAJARAN', 6])
    const pick = (list, catCode, copies) => list.filter((_, i) => u.level !== 'SD' || catCode !== 'CAT-SAINS' || i % 2 === 1)
      .forEach(([t, a, p, y, price]) => titles.push([t, a, p, y, price, catCode, copies]))
    pick(FICTION, 'CAT-FIKSI', 3); pick(REFERENCE, 'CAT-REFERENSI', 2); pick(SCIENCE, 'CAT-SAINS', 2)
    const titleRows = []
    const copies = []
    titles.forEach(([title, author, publisher, year, price, catCode, n], ti) => {
      if (S === SMP && smpTitles.has(title.toLowerCase())) return
      const id = uid('lib-title', u.key, title)
      titleRows.push({ id, school_id: S, title, author, publisher, isbn: `978-623-${String(100 + ti).padStart(3, '0')}-${String(r.int(100, 999))}-${r.int(0, 9)}`,
        publish_year: year, category_id: cat[catCode], replacement_price_idr: price })
      for (let k = 1; k <= n; k++) copies.push({ id: uid('lib-copy', id, k), school_id: S, title_id: id, barcode: `BC-${u.level}-${String(ti + 1).padStart(3, '0')}-${String(k).padStart(2, '0')}`,
        shelf_location: `RAK-${'ABCDEF'[ti % 6]}-${String(1 + (ti % 4)).padStart(2, '0')}`, status: 'available', titleId: id })
    })

    // ---- loans, fines, visits ----
    const loans = []
    const fines = []
    const visits = []
    const reservations = []
    const busy = new Set()
    const borrowers = [...u.students.filter((_, i) => i % 3 !== 1), ...u.teachers.slice(0, 5)]
    for (const p of borrowers) {
      const m = memberOf[p.id]
      if (!m) continue
      const n = r.int(1, 4)
      for (let k = 0; k < n; k++) {
        const copy = copies[r.int(0, copies.length - 1)]
        if (!copy) break
        const loanDay = addDays(new Date('2026-07-20T00:00:00Z'), r.int(0, 85))
        const loaned = date(loanDay)
        const days = m.kind === 'student' ? 14 : 21
        const dueDate = date(addDays(loanDay, days))
        // Recent loans are still out; one in eight of those is overdue.
        const stillOut = loaned >= '2026-09-25' && !busy.has(copy.id)
        const overdue = stillOut && r.chance(0.35) && dueDate < TODAY
        const lateReturn = !stillOut && r.chance(0.12)
        const returned = stillOut ? null : date(addDays(loanDay, lateReturn ? days + r.int(1, 6) : r.int(3, days)))
        if (returned && returned > TODAY) continue
        if (stillOut) { busy.add(copy.id); copy.status = 'on_loan' }
        const cls = u.classes.find((c) => c.id === m.classId)
        const loanId = uid('lib-loan', p.id, k)
        loans.push({ id: loanId, school_id: S, copy_id: copy.id, member_id: m.id, classroom_id: m.classId ?? null, classroom_name: cls?.name?.replace('Kelas ', '') ?? null,
          loaned_at: `${loaned}T09:${String(r.int(10, 50))}:00+07:00`, due_date: dueDate, returned_at: returned ? `${returned}T13:${String(r.int(10, 50))}:00+07:00` : null,
          renew_count: 0, status: returned ? 'returned' : 'active', loaned_by: ADMIN, returned_by: returned ? ADMIN : null, idempotency_key: `demo:loan:${loanId}` })
        const lateDays = returned ? Math.round((new Date(returned) - new Date(dueDate)) / 86400000) : overdue ? Math.round((new Date(TODAY) - new Date(dueDate)) / 86400000) : 0
        if (lateDays > 0 && (lateReturn || overdue)) {
          const rate = m.kind === 'student' ? 500 : 1000
          fines.push({ id: uid('lib-fine', loanId), school_id: S, loan_id: loanId, member_id: m.id, kind: 'late', late_days: lateDays, amount_idr: lateDays * rate,
            rate_snapshot: { late_fee_per_day_idr: rate }, settlement: 'pay_at_library', status: returned ? 'paid' : 'open',
            paid_at: returned ? `${returned}T13:30:00+07:00` : null, received_by: returned ? ADMIN : null })
        }
        visits.push({ id: uid('lib-visit', m.id, loaned), school_id: S, member_id: m.id, visited_on: loaned, visited_at: `${loaned}T09:05:00+07:00` })
      }
      if (r.chance(0.05)) {
        const t = titleRows[r.int(0, titleRows.length - 1)]
        if (t) reservations.push({ id: uid('lib-reservation', m.id, t.id), school_id: S, title_id: t.id, member_id: m.id, status: 'waiting', created_at: '2026-10-13T10:00:00+07:00' })
      }
    }

    out.push(insert('lib_titles', titleRows), insert('lib_copies', copies.map(({ titleId, ...c }) => c)))
    out.push(insert('lib_loans', loans, { chunk: 800 }), insert('lib_fines', fines))
    out.push(insert('lib_visits', visits, { conflict: '(member_id, visited_on) DO NOTHING', chunk: 1000 }), insert('lib_reservations', reservations))
  }
  return { teratai_ops: out.join('\n\n') }
}
