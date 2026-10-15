// Phase 18: procurement & budget of SD and SMK — the school budget (RAPBS
// 2026/2027 approved, last year's closed, a mid-year revision waiting for the
// principal and the foundation) with its lines, realisation and overspend
// alerts; vendors with contacts, legal documents, bank accounts (NPWP and
// account numbers encrypted like services/ops) and the accounting sync; and
// the purchasing cycle: purchase requests → purchase orders → goods receipts
// (one partial, one return) → vendor invoices matched three ways, with the
// approvals each step needs in the Approval Center.
import { ACCOUNTS, approvals, at, envLocal, fieldCrypto, insert, merge, pgArray, uid } from './lib.mjs'
import { MODEL } from './model.mjs'

const SMP = '11111111-1111-1111-1111-111111111111'
const { admin: ADMIN, kepala_sekolah: KEPSEK, procurement_officer: PENGADAAN, finance_manager: FINANCE, inventory_officer: GUDANG } = ACCOUNTS
const crypto = fieldCrypto(envLocal('OPS_ENC_KEY'), envLocal('OPS_BLIND_KEY'))
const VAT_BP = 1100

// [key, code, name, category, acc, div, planned]
const LINES = {
  SD: [['atk', '5.1.01', 'ATK & Bahan Habis Pakai Kantor', 'operasional', '5101', 'TU', 36000000],
    ['peraga', '5.1.02', 'Alat Peraga & Media Pembelajaran', 'operasional', '5102', 'KUR', 24000000],
    ['buku', '5.1.03', 'Buku Bacaan Perpustakaan & Pojok Baca', 'operasional', '5103', 'KUR', 15000000],
    ['tik', '5.2.01', 'Pengadaan Peralatan TIK & Mebel', 'investasi', '1305', 'SARPRAS', 55000000],
    ['rawat', '5.3.01', 'Pemeliharaan Gedung & Sarana', 'pemeliharaan', '5301', 'SARPRAS', 30000000],
    ['kegiatan', '5.4.01', 'Kegiatan Kesiswaan & Lomba', 'operasional', '5401', 'KES', 20000000],
    ['latih', '5.5.01', 'Pengembangan Profesi Guru', 'personalia', '5501', 'SDM', 18000000]],
  SMK: [['atk', '5.1.01', 'ATK & Bahan Habis Pakai Kantor', 'operasional', '5101', 'TU', 40000000],
    ['praktik', '5.1.02', 'Bahan Praktik Bengkel TKJ & Lab AKL', 'operasional', '5102', 'KUR', 45000000],
    ['buku', '5.1.03', 'Buku & Lisensi Software Kejuruan', 'operasional', '5103', 'KUR', 25000000],
    ['tik', '5.2.01', 'Peralatan Jaringan, TIK & Mebel', 'investasi', '1305', 'SARPRAS', 90000000],
    ['rawat', '5.3.01', 'Pemeliharaan Gedung & Sarana', 'pemeliharaan', '5301', 'SARPRAS', 35000000],
    ['kegiatan', '5.4.01', 'PKL, Kunjungan Industri & Lomba LKS', 'operasional', '5401', 'KES', 30000000],
    ['latih', '5.5.01', 'Sertifikasi Kompetensi & Pelatihan Guru', 'personalia', '5501', 'SDM', 25000000]],
}
// [key, code, name, address, categories, terms, contact, phone, email, bank]
const VENDORS = {
  SD: [['sarana', 'VND-001', 'CV Sarana Edukasi Nusantara', 'Jl. Ahmad Yani No. 45, Bekasi', ['ATK', 'Buku', 'Alat Peraga'], 30, 'Ibu Ratna Sari', 'penjualan@saranaedukasi.example', 'Bank Mandiri'],
    ['gramedia', 'VND-002', 'PT Gramedia Asri Media', 'Jl. Ir. H. Juanda No. 88, Bekasi', ['ATK', 'Buku'], 30, 'Bapak Hendra Wijaya', 'korporat@gramedia-asri.example', 'Bank BCA'],
    ['kreasi', 'VND-003', 'CV Kreasi Mainan Edukatif', 'Jl. Pramuka No. 12, Bekasi Selatan', ['Alat Peraga', 'APE'], 14, 'Ibu Dewi Anggraini', 'order@kreasiedukatif.example', 'Bank BRI'],
    ['komputindo', 'VND-004', 'CV Mitra Komputindo', 'Ruko Galaxy Blok C-12, Bekasi Selatan', ['Elektronik', 'TIK'], 14, 'Bapak Rudi Hartono', 'sales@mitrakomputindo.example', 'Bank BNI'],
    ['bangun', 'VND-005', 'UD Bangun Jaya Teknik', 'Jl. Raya Jatiasih No. 77, Bekasi', ['Material', 'Jasa Perbaikan'], 7, 'Bapak Slamet Riyadi', 'bangunjaya@mail.example', 'Bank BRI']],
  SMK: [['sarana', 'VND-001', 'CV Sarana Edukasi Nusantara', 'Jl. Ahmad Yani No. 45, Bekasi', ['ATK', 'Buku'], 30, 'Ibu Ratna Sari', 'penjualan@saranaedukasi.example', 'Bank Mandiri'],
    ['netindo', 'VND-002', 'PT Netindo Solusi Jaringan', 'Kawasan Industri Jababeka Blok F-3, Cikarang', ['Jaringan', 'TIK', 'Jasa Jaringan'], 30, 'Bapak Andika Pratama', 'b2b@netindo.example', 'Bank BCA'],
    ['komputindo', 'VND-003', 'CV Mitra Komputindo', 'Ruko Galaxy Blok C-12, Bekasi Selatan', ['Elektronik', 'TIK'], 14, 'Bapak Rudi Hartono', 'sales@mitrakomputindo.example', 'Bank BNI'],
    ['akuntansi', 'VND-004', 'PT Solusi Akuntansi Indonesia', 'Jl. Sudirman Kav. 21, Jakarta', ['Lisensi Software'], 30, 'Ibu Meilani Kurnia', 'edu@solusiakuntansi.example', 'Bank Mandiri'],
    ['bangun', 'VND-005', 'UD Bangun Jaya Teknik', 'Jl. Raya Jatiasih No. 77, Bekasi', ['Material', 'Jasa Perbaikan'], 7, 'Bapak Slamet Riyadi', 'bangunjaya@mail.example', 'Bank BRI']],
}
// PR → PO scenarios: [key, requester, reason, needed, line, vendor, poStatus|null, prStatus, items: [desc, qty, unit, est, price, item?, received?, rejected?]]
const PURCHASES = {
  SD: [
    ['atk', ADMIN, 'Kebutuhan ATK Tata Usaha semester ganjil 2026/2027', '2026-08-15', 'atk', 'gramedia', 'received', 'ordered',
      [['Kertas HVS A4 80gr', 40, 'rim', 52000, 50500, 'ATK-001'], ['Tinta Printer Hitam', 12, 'botol', 95000, 92000, 'ATK-003'], ['Spidol Papan Tulis', 120, 'pcs', 9000, 8500, 'ATK-002']]],
    ['peraga', 'T2', 'Alat peraga matematika & IPAS kelas 1-6 (Kurikulum Merdeka)', '2026-09-05', 'peraga', 'kreasi', 'received', 'ordered',
      [['Set Balok Pecahan & Blok Dienes', 6, 'set', 650000, 625000], ['Torso Organ Tubuh Manusia 45 cm', 2, 'unit', 1250000, 1200000], ['Globe & Peta Indonesia Timbul', 6, 'set', 400000, 385000, null, 6, 1]]],
    ['buku', 'T5', 'Buku cerita bergambar untuk pojok baca kelas 1-3', '2026-10-20', 'buku', 'sarana', 'partially_received', 'ordered',
      [['Paket Buku Cerita Bergambar Jenjang A-B (30 judul)', 6, 'paket', 1450000, 1400000, null, 4]]],
    ['laptop', 'T1', 'Laptop guru untuk asesmen digital (ANBK & rapor)', '2026-11-15', 'tik', 'komputindo', 'pending_approval', 'ordered',
      [['Laptop Guru Lenovo IdeaPad Slim 3 (Core i5, 16 GB)', 4, 'unit', 8900000, 8750000]]],
    ['lampu', ADMIN, 'Penggantian lampu lapangan dan talang bocor perpustakaan (WO bulan Oktober)', '2026-10-25', 'rawat', 'bangun', null, 'pending_approval',
      [['Lampu Sorot LED 100W', 4, 'unit', 450000], ['Talang Air PVC 4 inci + pemasangan', 1, 'paket', 1800000]]],
    ['proyektor', 'T3', 'Proyektor tambahan untuk kelas 4-6', '2026-11-30', 'tik', 'komputindo', null, 'rejected',
      [['Proyektor Epson EB-X51', 3, 'unit', 6500000]]],
    ['lomba', 'T4', 'Perlengkapan lomba FLS2N & O2SN tingkat kota', '2026-11-10', 'kegiatan', null, null, 'draft',
      [['Kostum Tari Daerah (anak)', 8, 'set', 350000], ['Bola Futsal Size 4', 6, 'pcs', 275000]]],
  ],
  SMK: [
    ['atk', ADMIN, 'Kebutuhan ATK Tata Usaha semester ganjil 2026/2027', '2026-08-15', 'atk', 'sarana', 'received', 'ordered',
      [['Kertas HVS A4 80gr', 50, 'rim', 52000, 50500, 'ATK-001'], ['Tinta Printer Hitam', 16, 'botol', 95000, 92000, 'ATK-003'], ['Map Plastik', 300, 'pcs', 3500, 3400, 'ATK-004', 300, 10]]],
    ['jaringan', 'T7', 'Bahan praktik bengkel TKJ semester ganjil', '2026-08-25', 'praktik', 'netindo', 'received', 'ordered',
      [['Kabel UTP Cat6 (1 roll 305 m)', 6, 'roll', 1650000, 1600000], ['Konektor RJ45 Cat6 (isi 100)', 10, 'pak', 125000, 120000], ['Router MikroTik hAP ac2', 8, 'unit', 1150000, 1120000]]],
    ['lisensi', 'T9', 'Lisensi software akuntansi lab AKL (36 PC, 1 tahun)', '2026-09-15', 'buku', 'akuntansi', 'received', 'ordered',
      [['Lisensi Software Akuntansi Edisi Pendidikan (per PC/tahun)', 36, 'lisensi', 450000, 425000]]],
    ['switch', 'T8', 'Switch managed & rak server untuk ruang praktik XI TKJ', '2026-10-31', 'tik', 'netindo', 'partially_received', 'ordered',
      [['Switch Managed 24 Port Gigabit', 3, 'unit', 4800000, 4650000, null, 2], ['Rak Server 20U + PDU', 1, 'unit', 5200000, 5100000, null, 0]]],
    ['pc', 'T1', 'Penggantian 10 PC lab komputer (rusak & lambat)', '2026-12-01', 'tik', 'komputindo', 'pending_approval', 'ordered',
      [['PC Lenovo ThinkCentre Neo 50s (i5, 16 GB, SSD 512)', 10, 'unit', 8700000, 8500000]]],
    ['atap', ADMIN, 'Perbaikan kebocoran atap bengkel TKJ', '2026-10-28', 'rawat', 'bangun', null, 'pending_approval',
      [['Seng Galvalum + rangka (perbaikan 24 m²)', 1, 'paket', 4200000], ['Jasa Tukang 3 hari', 1, 'paket', 1500000]]],
    ['lks', 'T6', 'Perlengkapan lomba LKS bidang IT Network Systems Administration', '2026-11-15', 'kegiatan', null, null, 'draft',
      [['Kit Latihan Fiber Optic (splicer sewa 1 bulan)', 1, 'paket', 3500000]]],
  ],
}

export function generate() {
  const ap = approvals()
  const out = []
  for (const u of MODEL.filter((x) => x.schoolId !== SMP)) {
    const S = u.schoolId
    const id = (...k) => uid('proc', u.key, ...k)
    const who = (x) => (x.startsWith('T') ? u.teachers[Number(x.slice(1))].id : x)
    out.push(`-- ===== ${u.level} =====`)
    out.push(insert('budget_settings', [{ school_id: S, fiscal_year_start_month: 7, lock_rule: 'approval_required', tolerance_bp: 500, warn_threshold_bp: 8000, updated_at: at('2026-07-01') }]))

    // ---- budgets ----
    const lines = LINES[u.level]
    const lineId = (k) => id('line', 'rapbs26', k)
    const total = lines.reduce((t, l) => t + l[6], 0)
    const rapbsApp = ap.add({ school: S, key: `budget:${u.key}:rapbs26`, type: 'budget', service: 'ops', sourceId: id('budget', 'rapbs26'),
      title: `RAPBS ${u.level} Tahun Ajaran 2026/2027`, summary: { fiscal_year: '2026/2027', lines: lines.length, total_planned_idr: total }, amount: total, maker: ADMIN,
      steps: [['kepala_sekolah', 72], ['kepala_yayasan', 120]], decisions: [{ decision: 'approved', at: at('2026-06-10', '10:00'), comment: 'Sesuai rencana kerja sekolah.' },
        { decision: 'approved', at: at('2026-06-19', '14:00'), comment: 'Disahkan rapat pleno yayasan.' }], createdAt: at('2026-06-01') })
    const revTotal = Math.round(total * 0.12 / 1000000) * 1000000
    const revApp = ap.add({ school: S, key: `budget:${u.key}:rapbs26p`, type: 'budget', service: 'ops', sourceId: id('budget', 'rapbs26p'),
      title: `Perubahan RAPBS ${u.level} 2026/2027 (Semester Genap)`, summary: { fiscal_year: '2026/2027', reason: 'Penambahan pos TIK dan pemeliharaan', total_planned_idr: revTotal },
      amount: revTotal, maker: ADMIN, steps: [['kepala_sekolah', 72], ['kepala_yayasan', 120]],
      decisions: [{ decision: 'approved', at: at('2026-10-12', '11:00'), comment: 'Lanjutkan ke yayasan.' }], createdAt: at('2026-10-09') })
    out.push(insert('budgets', [
      { id: id('budget', 'rapbs26'), school_id: S, code: 'RAPBS-2026-2027', name: `RAPBS ${u.level} Tahun Ajaran 2026/2027`, fiscal_year: '2026/2027', period_start: '2026-07-01',
        period_end: '2027-06-30', status: 'approved', total_planned_idr: total, approval_id: rapbsApp, created_by: ADMIN, created_at: at('2026-05-15'), updated_at: at('2026-06-19') },
      { id: id('budget', 'rapbs25'), school_id: S, code: 'RAPBS-2025-2026', name: `RAPBS ${u.level} Tahun Ajaran 2025/2026`, fiscal_year: '2025/2026', period_start: '2025-07-01',
        period_end: '2026-06-30', status: 'closed', total_planned_idr: Math.round(total * 0.9), created_by: ADMIN, created_at: at('2025-05-15'), updated_at: at('2026-07-05') },
      { id: id('budget', 'rapbs26p'), school_id: S, code: 'RAPBS-2026-2027-P', name: `Perubahan RAPBS ${u.level} 2026/2027 (Semester Genap)`, fiscal_year: '2026/2027',
        period_start: '2027-01-01', period_end: '2027-06-30', status: 'pending_approval', total_planned_idr: revTotal, approval_id: revApp, created_by: ADMIN,
        created_at: at('2026-10-09'), updated_at: at('2026-10-12') }], { conflict: '(school_id, code) DO NOTHING' }))
    out.push(insert('budget_lines', [
      ...lines.map(([k, code, name, cat, acc, div, amt]) => ({ id: lineId(k), school_id: S, budget_id: id('budget', 'rapbs26'), code, name, category: cat, acc_code: acc, div_code: div,
        amount_planned_idr: amt, created_at: at('2026-05-15') })),
      ...lines.map(([k, code, name, cat, acc, div, amt]) => ({ id: id('line', 'rapbs25', k), school_id: S, budget_id: id('budget', 'rapbs25'), code, name, category: cat, acc_code: acc,
        div_code: div, amount_planned_idr: Math.round(amt * 0.9), created_at: at('2025-05-15') })),
      { id: id('line', 'rapbs26p', 'tik'), school_id: S, budget_id: id('budget', 'rapbs26p'), code: '5.2.01', name: lines[3][2] + ' (tambahan)', category: 'investasi', acc_code: '1305',
        div_code: 'SARPRAS', amount_planned_idr: Math.round(revTotal * 0.7), created_at: at('2026-10-09') },
      { id: id('line', 'rapbs26p', 'rawat'), school_id: S, budget_id: id('budget', 'rapbs26p'), code: '5.3.01', name: 'Pemeliharaan Gedung & Sarana (tambahan)', category: 'pemeliharaan',
        acc_code: '5301', div_code: 'SARPRAS', amount_planned_idr: revTotal - Math.round(revTotal * 0.7), created_at: at('2026-10-09') },
    ], { conflict: '(budget_id, code) DO NOTHING' }))

    // ---- vendors ----
    const vendors = VENDORS[u.level]
    const vid = (k) => id('vendor', k)
    out.push(insert('vendors', vendors.map(([k, code, name, addr, cats, terms], i) => {
      const v = vid(k)
      const npwp = `0${i + 1}.${300 + i * 17}.${410 + i}.${i + 2}-${430 + i}.000`
      return { id: v, school_id: S, code, name, npwp_enc: crypto.encrypt(npwp, `vendors:${S}:${v}`, v + 'npwp'), npwp_bidx: crypto.blindIndex(npwp, `vendors:${S}:${v}`), address: addr,
        categories: pgArray(cats), payment_terms_days: terms, status: 'active', created_at: at('2026-07-15') }
    })))
    out.push(insert('vendor_contacts', vendors.flatMap(([k, , , , , , contact, email], i) => [
      { id: id('contact', k), school_id: S, vendor_id: vid(k), name: contact, phone: `0812-${1000 + i * 7}-${2000 + i * 13}`, email, is_primary: true, created_at: at('2026-07-15') },
      ...(i < 2 ? [{ id: id('contact', k, 'tagihan'), school_id: S, vendor_id: vid(k), name: 'Bagian Penagihan', phone: `021-8${800 + i}-77${10 + i}`, email: email.replace(/^[^@]+/, 'tagihan'),
        is_primary: false, created_at: at('2026-07-15') }] : [])])))
    out.push(insert('vendor_documents', vendors.flatMap(([k, code], i) => [['nib', null], ['npwp', null], ...(i % 2 ? [['siup', '2027-03-31']] : [])].map(([kind, exp]) => ({
      id: id('doc', k, kind), school_id: S, vendor_id: vid(k), kind, object_key: `ops/vendors/${u.key}/${code}/${kind}.pdf`, expires_on: exp, uploaded_by: PENGADAAN, created_at: at('2026-07-16') })))))
    const banks = vendors.map(([k, , name, , , , , , bank], i) => {
      const bid = id('bank', k)
      const acc = String(1230000000 + i * 1111111 + (u.key === 'smk' ? 7 : 3))
      const aad = `vendor_bank_accounts:${S}:${bid}`
      return { id: bid, school_id: S, vendor_id: vid(k), bank_name: bank, account_no_enc: crypto.encrypt(acc, aad, bid), account_no_bidx: crypto.blindIndex(acc, aad), account_holder: name,
        status: 'active', requested_by: PENGADAAN, activated_at: at('2026-07-20'), created_at: at('2026-07-16') }
    })
    // The first vendor asks to move its payments to a new account.
    const nb = id('bank', vendors[0][0], 'baru')
    const nbAcc = '7001234567'
    const nbApp = ap.add({ school: S, key: `vendor-bank:${u.key}:${vendors[0][0]}`, type: 'vendor_bank_change', service: 'ops', sourceId: nb,
      title: `Perubahan Rekening Vendor ${vendors[0][2]}`, summary: { vendor: vendors[0][2], bank_name: 'Bank Syariah Indonesia', account_holder: vendors[0][2] }, maker: PENGADAAN,
      steps: [['finance_manager', 48]], createdAt: at('2026-10-11', '10:00'), body: 'Vendor mengajukan perubahan rekening; cocokkan dengan surat resmi bermeterai sebelum menyetujui.' })
    banks.push({ id: nb, school_id: S, vendor_id: vid(vendors[0][0]), bank_name: 'Bank Syariah Indonesia', account_no_enc: crypto.encrypt(nbAcc, `vendor_bank_accounts:${S}:${nb}`, nb),
      account_no_bidx: crypto.blindIndex(nbAcc, `vendor_bank_accounts:${S}:${nb}`), account_holder: vendors[0][2], status: 'pending_approval', approval_id: nbApp, requested_by: PENGADAAN,
      created_at: at('2026-10-11') })
    out.push(insert('vendor_bank_accounts', banks))
    out.push(insert('vendor_accounting_sync', vendors.map(([k], i) => ({ vendor_id: vid(k), school_id: S, payload_hash: `demo-${u.key}-${k}`, status: i === 4 ? 'failed' : 'synced',
      attempts: i === 4 ? 3 : 1, last_error: i === 4 ? 'Akun hutang 2101 belum dipetakan di modul akuntansi' : null, synced_at: i === 4 ? null : at('2026-07-21'), updated_at: at('2026-07-21') }))))

    // ---- purchasing cycle ----
    const items = (code) => (code ? uid('fac-item', u.key, code) : null)
    const warehouse = uid('fac-warehouse', u.key)
    const prs = []
    const prLines = []
    const pos = []
    const poLines = []
    const grs = []
    const grLines = []
    const returns = []
    const evals = []
    const invoices = []
    const invLines = []
    const actual = {}
    let prSeq = 0
    let poSeq = 0
    let grSeq = 0
    PURCHASES[u.level].forEach(([k, requester, reason, needed, line, vendor, poStatus, prStatus, its], pi) => {
      const prId = id('pr', k)
      const number = `PR-2026-${String(++prSeq).padStart(4, '0')}`
      const est = its.reduce((t, [, qty, , e]) => t + qty * e, 0)
      const created = at(['2026-07-28', '2026-08-18', '2026-09-24', '2026-10-05', '2026-10-08', '2026-09-30', '2026-10-14'][pi], '09:00')
      const t0 = new Date(created)
      const day = (n, h = '10:00') => at(new Date(t0.getTime() + n * 86400000).toISOString().slice(0, 10), h)
      const overBudget = prStatus === 'rejected'
      let prApp = null
      if (prStatus !== 'draft') {
        prApp = ap.add({ school: S, key: `pr:${u.key}:${k}`, type: 'purchase_request', service: 'ops', sourceId: prId, title: `Permintaan Pembelian ${number} (Rp${est.toLocaleString('id-ID')})`,
          summary: { number, reason, total_est_idr: est, over_budget: overBudget }, amount: est, maker: who(requester), steps: [['kepala_sekolah', 48]], createdAt: day(0, '09:05'),
          decisions: prStatus === 'pending_approval' ? [] : [{ decision: overBudget ? 'rejected' : 'approved', at: day(1, '13:00'),
            comment: overBudget ? 'Pos TIK sudah terserap untuk laptop guru; ajukan kembali di perubahan RAPBS.' : 'Setuju, sesuai RAPBS.' }] })
      }
      prs.push({ id: prId, school_id: S, number, requester_user_id: who(requester), reason, needed_by: needed, status: prStatus, total_est_idr: est, over_budget: overBudget,
        budget_check: overBudget ? { status: 'over_budget', line: lines.find((l) => l[0] === line)[1] } : { status: 'ok' }, approval_id: prApp,
        idempotency_key: `demo:pr:${u.key}:${k}`, created_at: created, updated_at: prStatus === 'pending_approval' || prStatus === 'draft' ? created : day(3) })
      its.forEach(([desc, qty, unit, e, , code], li) => prLines.push({ id: id('pr-line', k, li), school_id: S, pr_id: prId, item_id: items(code), description: desc, qty, unit,
        est_unit_price_idr: e, budget_line_key: lineId(line), qty_ordered: poStatus ? qty : 0 }))
      if (!poStatus) return

      // Purchase order.
      const poId = id('po', k)
      const poNo = `PO-2026-${String(++poSeq).padStart(4, '0')}`
      const poTotal = its.reduce((t, [, qty, , , price]) => t + qty * price, 0)
      const poDecisions = poStatus === 'pending_approval' ? [{ decision: 'approved', at: day(6, '10:00'), comment: 'Harga sesuai 3 pembanding.' }]
        : [{ decision: 'approved', at: day(3, '10:00'), comment: 'Setuju.' }, ...(poTotal >= 25000000 ? [{ decision: 'approved', at: day(4, '09:00'), comment: 'Disetujui yayasan.' }] : [])]
      const poApp = ap.add({ school: S, key: `po:${u.key}:${k}`, type: 'purchase_order', service: 'ops', sourceId: poId, title: `Pesanan Pembelian ${poNo} — ${vendors.find((v) => v[0] === vendor)[2]}`,
        summary: { number: poNo, vendor: vendors.find((v) => v[0] === vendor)[2], total_idr: poTotal }, amount: poTotal, maker: PENGADAAN,
        steps: poTotal >= 25000000 ? [['finance_manager', 48], ['kepala_yayasan', 72]] : [['finance_manager', 48]], decisions: poDecisions, createdAt: day(2, '14:00') })
      pos.push({ id: poId, school_id: S, number: poNo, vendor_id: vid(vendor), payment_terms_days: vendors.find((v) => v[0] === vendor)[5], expected_date: needed, status: poStatus,
        total_idr: poTotal, approval_id: poApp, pdf_object_key: poStatus === 'pending_approval' ? null : `ops/po/${u.key}/${poNo}.pdf`,
        sent_at: poStatus === 'pending_approval' ? null : day(4, '15:00'), idempotency_key: `demo:po:${u.key}:${k}`, created_at: day(2, '14:00'),
        updated_at: poStatus === 'pending_approval' ? day(6) : day(12) })
      its.forEach(([desc, qty, , , price, code], li) => {
        poLines.push({ id: id('po-line', k, li), school_id: S, po_id: poId, pr_line_id: id('pr-line', k, li), item_id: items(code), description: desc, qty, unit_price_idr: price,
          budget_line_key: lineId(line), warehouse_id: code ? warehouse : null })
        if (poStatus !== 'pending_approval') actual[line] = (actual[line] ?? 0) + qty * price
      })
      if (poStatus === 'pending_approval' || poStatus === 'sent') return

      // Goods receipt (and a return of rejected units).
      const grId = id('gr', k)
      const grNo = `GR-2026-${String(++grSeq).padStart(4, '0')}`
      const received = day(11)
      grs.push({ id: grId, school_id: S, number: grNo, po_id: poId, received_on: received.slice(0, 10), received_by: GUDANG,
        note: poStatus === 'partially_received' ? 'Sebagian barang belum dikirim vendor (inden).' : 'Diterima di gudang sarpras.', idempotency_key: `demo:gr:${u.key}:${k}`, created_at: received })
      const accepted = []
      its.forEach(([desc, qty, , , price, , got = qty, bad = 0], li) => {
        if (!got) return
        const glId = id('gr-line', k, li)
        grLines.push({ id: glId, school_id: S, gr_id: grId, po_line_id: id('po-line', k, li), qty_received: got, qty_accepted: got - bad, qty_rejected: bad,
          reject_reason: bad ? 'Barang cacat/pecah saat pengiriman' : null, asset_handoff_at: line === 'tik' ? day(12) : null, created_at: received })
        if (bad) returns.push({ id: id('return', k, li), school_id: S, gr_line_id: glId, qty: bad, reason: `${desc}: ${bad} unit cacat, dikembalikan untuk diganti.`, returned_on: day(13).slice(0, 10),
          created_by: GUDANG, created_at: day(13) })
        accepted.push([li, desc, got - bad, price, glId])
      })
      const late = pi % 3 === 1
      evals.push({ id: id('eval', k), school_id: S, vendor_id: vid(vendor), goods_receipt_id: grId, on_time: !late, quality_score: its.some((x) => x[7]) ? 3 : 4 + (pi % 2),
        price_score: 4, note: late ? 'Terlambat beberapa hari dari jadwal, kualitas baik.' : 'Tepat waktu, barang sesuai spesifikasi.', created_by: PENGADAAN, created_at: day(14) })

      // Vendor invoice, matched against the PO and the receipt.
      const invId = id('invoice', k)
      const sub = accepted.reduce((t, [, , q, p]) => t + q * p, 0)
      const billedQtyBump = poStatus === 'partially_received' ? 0 : pi === 2 ? 1 : 0 // one vendor bills a unit more than received
      const subtotal = sub + (billedQtyBump ? accepted[0][3] : 0)
      const vat = Math.round((subtotal * VAT_BP) / 10000)
      const status = billedQtyBump ? 'discrepancy' : poStatus === 'partially_received' ? 'pending_match' : pi === 0 ? 'paid' : 'partially_paid'
      const invTotal = subtotal + vat
      invoices.push({ id: invId, school_id: S, number: `INV/${vendors.find((v) => v[0] === vendor)[1]}/${u.level}/2026/${String(pi + 1).padStart(3, '0')}`, vendor_id: vid(vendor),
        po_id: poId, invoice_date: day(13).slice(0, 10), due_date: day(13 + vendors.find((v) => v[0] === vendor)[5]).slice(0, 10), subtotal_idr: subtotal, vat_idr: vat,
        total_idr: invTotal, paid_amount_idr: status === 'paid' ? invTotal : status === 'partially_paid' ? Math.round(invTotal / 2 / 1000) * 1000 : 0, status,
        match_status: billedQtyBump ? 'qty_mismatch' : status === 'pending_match' ? 'pending' : 'matched_ok',
        match_discrepancy_note: billedQtyBump ? `Ditagih ${accepted[0][2] + 1} ${its[0][2]}, diterima ${accepted[0][2]}.` : null, pdf_object_key: `ops/ap/${u.key}/${invId}.pdf`,
        journal_trans_no: ['paid', 'partially_paid'].includes(status) ? `AP-INV-${u.level}-2026-${String(pi + 1).padStart(4, '0')}` : null, idempotency_key: `demo:inv:${u.key}:${k}`,
        created_by: ACCOUNTS.finance_manager, created_at: day(13), updated_at: day(20) })
      accepted.forEach(([li, desc, q, p, glId], n) => invLines.push({ id: id('inv-line', k, li), school_id: S, invoice_id: invId, po_line_id: id('po-line', k, li), gr_line_id: glId,
        description: desc, qty: q + (billedQtyBump && n === 0 ? 1 : 0), unit_price_idr: p, budget_line_key: lineId(line), created_at: day(13) }))
    })
    out.push(insert('purchase_requests', prs, { conflict: '(school_id, idempotency_key) DO NOTHING' }), insert('purchase_request_lines', prLines),
      insert('purchase_orders', pos, { conflict: '(school_id, idempotency_key) DO NOTHING' }), insert('purchase_order_lines', poLines),
      insert('goods_receipts', grs, { conflict: '(school_id, idempotency_key) DO NOTHING' }), insert('goods_receipt_lines', grLines), insert('goods_returns', returns),
      insert('vendor_evaluations', evals, { conflict: '(goods_receipt_id) DO NOTHING' }),
      insert('vendor_invoices', invoices, { conflict: '(school_id, idempotency_key) DO NOTHING' }), insert('vendor_invoice_lines', invLines))

    // ---- realisation & alerts ----
    const planned = Object.fromEntries(lines.map(([k, , , , , , amt]) => [k, amt]))
    // Running costs paid outside purchasing (contracts, events) on top of the POs.
    const other = { atk: 0.22, peraga: 0.18, praktik: 0.35, buku: 0.1, tik: 0, rawat: 0.88, kegiatan: 0.31, latih: 0.27 }
    const cache = lines.map(([k]) => ({ budget_line_id: lineId(k), school_id: S, actual_idr: Math.round((actual[k] ?? 0) + planned[k] * (other[k] ?? 0)), last_synced_at: at('2026-10-15', '01:00') }))
    out.push(insert('budget_actuals_cache', cache, { conflict: '(budget_line_id) DO UPDATE SET actual_idr = EXCLUDED.actual_idr, last_synced_at = EXCLUDED.last_synced_at' }))
    const alerts = []
    for (const c of cache) {
      const k = lines.find((l) => lineId(l[0]) === c.budget_line_id)
      const bp = Math.round((c.actual_idr * 10000) / k[6])
      const pct = (bp / 100).toFixed(1).replace('.', ',')
      if (bp >= 8000) alerts.push({ id: id('alert', k[0], 80), school_id: S, budget_line_id: c.budget_line_id, threshold_bp: 8000, current_bp: bp,
        message: `Serapan pos ${k[1]} ${k[2]} mencapai ${pct}%`, acknowledged_at: k[0] === 'rawat' ? at('2026-10-02') : null, acknowledged_by: k[0] === 'rawat' ? KEPSEK : null,
        created_at: at('2026-09-30', '01:00') })
      if (bp >= 10000) alerts.push({ id: id('alert', k[0], 100), school_id: S, budget_line_id: c.budget_line_id, threshold_bp: 10000, current_bp: bp,
        message: `Pos ${k[1]} ${k[2]} melebihi anggaran (${pct}%)`, created_at: at('2026-10-14', '01:00') })
    }
    out.push(insert('budget_alerts', alerts))
  }
  return merge({ teratai_ops: out.join('\n\n') }, ap.sql())
}
