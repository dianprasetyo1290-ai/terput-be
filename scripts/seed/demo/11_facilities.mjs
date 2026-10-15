// Phase 11: facilities of SD and SMK (SMP has its own from the base seeds) —
// a building with its rooms (a classroom per class, labs, library, hall,
// field, UKS, office, canteen, store), room facilities and bookings,
// maintenance schedules and work orders, a store room with consumables and
// their stock movements, and fixed assets (projectors, lab computers, air
// conditioners, furniture).
import { insert, rng, uid } from './lib.mjs'
import { MODEL } from './model.mjs'

const SMP = '11111111-1111-1111-1111-111111111111'
const ADMIN = '22222222-2222-2222-2222-222222222222'
const TEKNISI = 'a2600000-0000-0000-0000-000000000007'
const ITEMS = [['ATK-001', 'Kertas HVS A4 80gr', 'rim', 'ATK', 20, 60], ['ATK-002', 'Spidol Papan Tulis', 'pcs', 'ATK', 50, 200], ['ATK-003', 'Tinta Printer Hitam', 'botol', 'ATK', 5, 20],
  ['ATK-004', 'Map Plastik', 'pcs', 'ATK', 30, 100], ['ATK-005', 'Kapur Tulis', 'kotak', 'ATK', 10, 30], ['KBR-001', 'Cairan Pembersih Lantai', 'liter', 'KBR', 10, 40],
  ['KBR-002', 'Sabun Cuci Tangan', 'liter', 'KBR', 10, 40], ['KBR-003', 'Tisu Gulung', 'pak', 'KBR', 20, 80], ['KBR-004', 'Kantong Sampah Besar', 'pak', 'KBR', 10, 30]]

export function generate() {
  const out = []
  for (const u of MODEL.filter((x) => x.schoolId !== SMP)) {
    const r = rng('facilities:' + u.key)
    const S = u.schoolId
    out.push(`-- ===== ${u.level} =====`)
    for (const t of ['inventory_settings', 'asset_settings']) {
      out.push(`DO $$ DECLARE cols text; vals text; BEGIN
  SELECT string_agg(quote_ident(column_name), ', ' ORDER BY ordinal_position),
         string_agg(CASE column_name WHEN 'school_id' THEN quote_literal('${S}') || '::uuid' ELSE quote_ident(column_name) END, ', ' ORDER BY ordinal_position)
    INTO cols, vals FROM information_schema.columns WHERE table_schema = 'public' AND table_name = '${t}';
  IF NOT EXISTS (SELECT 1 FROM ${t} WHERE school_id = '${S}') THEN
    EXECUTE format('INSERT INTO ${t} (%s) SELECT %s FROM ${t} WHERE school_id = %L ON CONFLICT DO NOTHING', cols, vals, '${SMP}');
  END IF;
END $$;`)
    }

    // ---- building, floors, rooms ----
    const building = uid('fac-building', u.key)
    const floors = [1, 2].map((level) => ({ id: uid('fac-floor', u.key, level), school_id: S, building_id: building, level, name: `Lantai ${level}` }))
    out.push(insert('buildings', [{ id: building, school_id: S, code: 'GDG-A', name: `Gedung Utama ${u.level}`, address: 'Jl. Teratai Putih No. 1, Bekasi', sort: 1 }]), insert('floors', floors))
    const rooms = []
    const room = (code, name, kind, capacity, usage, floor, approval = false) => {
      const id = uid('fac-room', u.key, code)
      rooms.push({ id, school_id: S, floor_id: floors[floor - 1].id, code, name, kind, capacity, usage, requires_approval: usage === 'bookable' && approval, status: 'active' })
      return id
    }
    const classRooms = u.classes.map((c, i) => ({ cls: c, id: room(`R-${String(101 + i + (i >= 3 ? 97 : 0))}`, `Ruang ${c.name}`, 'kelas', 32, 'fixed', i < 3 ? 1 : 2) }))
    const lab = room('LAB-KOM', 'Laboratorium Komputer', 'lab', 36, 'bookable', 2)
    const perpus = room('PERPUS', 'Perpustakaan', 'perpustakaan', 40, 'fixed', 1)
    const aula = room('AULA', 'Aula Serbaguna', 'aula', 200, 'bookable', 1, true)
    const lapangan = room('LAP', 'Lapangan Olahraga', 'lapangan', 300, 'bookable', 1)
    room('UKS', 'Ruang UKS', 'uks', 6, 'fixed', 1)
    const kantor = room('TU', 'Kantor Tata Usaha', 'kantor', 10, 'fixed', 1)
    room('KANTIN', 'Kantin Sekolah', 'kantin', 80, 'fixed', 1)
    const gudang = room('GUDANG', 'Gudang Sarpras', 'gudang', 0, 'fixed', 1)
    const extra = u.level === 'SMK' ? [room('BENGKEL-TKJ', 'Bengkel Teknik Jaringan', 'lab', 36, 'bookable', 2), room('LAB-AKL', 'Laboratorium Akuntansi', 'lab', 36, 'bookable', 2)] : []
    out.push(insert('rooms', rooms))
    const facilities = []
    for (const { id } of classRooms) [['Proyektor', 1], ['AC', 1], ['Papan Tulis', 1], ['Meja & Kursi Siswa', 32]].forEach(([name, qty]) => facilities.push({ id: uid('fac-facility', id, name), school_id: S, room_id: id, name, qty }))
    for (const id of [lab, ...extra]) [['Komputer', 30], ['Proyektor', 1], ['AC', 2]].forEach(([name, qty]) => facilities.push({ id: uid('fac-facility', id, name), school_id: S, room_id: id, name, qty }))
    ;[['Sound System', 1], ['Kursi Lipat', 200], ['Panggung', 1]].forEach(([name, qty]) => facilities.push({ id: uid('fac-facility', aula, name), school_id: S, room_id: aula, name, qty }))
    out.push(insert('room_facilities', facilities))
    const bookings = [
      [aula, '2026-09-24T07:30:00+07:00', '2026-09-24T12:00:00+07:00', 'Class Meeting', 180, 'confirmed'],
      [aula, '2026-10-28T08:00:00+07:00', '2026-10-28T12:00:00+07:00', 'Seminar Literasi Digital', 150, 'confirmed'],
      [aula, '2026-11-14T08:00:00+07:00', '2026-11-14T13:00:00+07:00', 'Rapat Pleno Komite Sekolah', 120, 'pending_approval'],
      [lab, '2026-10-16T09:00:00+07:00', '2026-10-16T11:00:00+07:00', 'Simulasi ANBK', 30, 'confirmed'],
      [lapangan, '2026-10-17T07:00:00+07:00', '2026-10-17T10:00:00+07:00', 'Latihan Upacara HUT Sekolah', 250, 'confirmed'],
      [lab, '2026-10-21T13:00:00+07:00', '2026-10-21T15:00:00+07:00', 'Pelatihan Guru: Asesmen Digital', 25, 'confirmed'],
    ].map(([roomId, s, e, purpose, participants, status], i) => ({ id: uid('fac-booking', u.key, i), school_id: S, room_id: roomId, starts_at: s, ends_at: e, purpose, participants,
      requester_user_id: ADMIN, requester_name: 'Admin Teratai', source: 'internal', status, idempotency_key: `demo:booking:${u.key}:${i}` }))
    out.push(insert('room_bookings', bookings, { conflict: '(school_id, idempotency_key) DO NOTHING' }))

    // ---- store room ----
    const warehouse = uid('fac-warehouse', u.key)
    out.push(insert('warehouses', [{ id: warehouse, school_id: S, code: 'WH-01', name: 'Gudang ATK & Kebersihan', room_id: gudang, status: 'active' }]))
    const cats = { ATK: uid('fac-item-cat', u.key, 'ATK'), KBR: uid('fac-item-cat', u.key, 'KBR'), ELK: uid('fac-item-cat', u.key, 'ELK'), MBL: uid('fac-item-cat', u.key, 'MBL') }
    const assetCats = { ELK: uid('fac-asset-cat', u.key, 'ELK'), MBL: uid('fac-asset-cat', u.key, 'MBL') }
    out.push(insert('asset_categories', [
      { id: assetCats.ELK, school_id: S, code: 'AST-ELK', name: 'Peralatan Elektronik', useful_life_months: 48, dep_start: 'next_month', asset_acc_code: '1301', accum_dep_acc_code: '1391', dep_expense_acc_code: '6301' },
      { id: assetCats.MBL, school_id: S, code: 'AST-MBL', name: 'Mebel & Perlengkapan', useful_life_months: 96, dep_start: 'next_month', asset_acc_code: '1302', accum_dep_acc_code: '1392', dep_expense_acc_code: '6302' }]))
    out.push(insert('item_categories', [
      { id: cats.ATK, school_id: S, code: 'ATK', name: 'Alat Tulis Kantor', tracking: 'stock', purchase_acc_code: '5101' },
      { id: cats.KBR, school_id: S, code: 'KBR', name: 'Perlengkapan Kebersihan', tracking: 'stock', purchase_acc_code: '5102' },
      { id: cats.ELK, school_id: S, code: 'ELK', name: 'Elektronik', tracking: 'asset', asset_category_id: assetCats.ELK },
      { id: cats.MBL, school_id: S, code: 'MBL', name: 'Mebel', tracking: 'asset', asset_category_id: assetCats.MBL }]))
    const items = []
    const balances = []
    const moves = []
    for (const [code, name, unit, cat, min, received] of ITEMS) {
      const id = uid('fac-item', u.key, code)
      items.push({ id, school_id: S, code, name, category_id: cats[cat], unit, min_stock: min, status: 'active' })
      let bal = 0
      const mv = (direction, qty, kind, d, ref) => {
        bal += direction === 'in' ? qty : -qty
        moves.push({ id: uid('fac-move', id, d, kind), school_id: S, warehouse_id: warehouse, item_id: id, direction, qty, kind, ref_type: kind === 'receipt' ? 'goods_receipt' : 'issue',
          ref_id: uid('fac-ref', u.key, ref), ref_no: ref, balance_after: bal, note: kind === 'issue' ? 'Pemakaian rutin bulanan' : 'Penerimaan awal tahun ajaran', created_by: ADMIN,
          created_at: `${d}T10:00:00+07:00`, idempotency_key: `demo:stock:${id}:${d}:${kind}` })
      }
      mv('in', received, 'receipt', '2026-07-06', `GR-${u.level}-2607-001`)
      for (const [m, d] of [['07', '2026-07-31'], ['08', '2026-08-31'], ['09', '2026-09-30']]) mv('out', Math.max(1, Math.round(received * (0.18 + r.next() * 0.12))), 'issue', d, `ISS-${u.level}-26${m}`)
      // Two items run low, to show the minimum-stock warning.
      if (code === 'ATK-002' || code === 'KBR-003') mv('out', Math.max(1, bal - Math.floor(min / 2)), 'issue', '2026-10-12', `ISS-${u.level}-2610`)
      balances.push({ school_id: S, warehouse_id: warehouse, item_id: id, qty: bal })
    }
    out.push(insert('items', items), insert('stock_movements', moves, { conflict: '(school_id, idempotency_key) DO NOTHING' }),
      insert('stock_balances', balances, { conflict: 'DO NOTHING' }))

    // ---- fixed assets ----
    const assets = []
    let seq = 0
    const asset = (name, cat, cost, roomId, acquired, condition = 'baik') => {
      const id = uid('fac-asset', u.key, ++seq)
      assets.push({ id, school_id: S, code: `AST/${u.level}/${cat}/${String(seq).padStart(4, '0')}`, name, category_id: assetCats[cat], status: 'active', condition, acquisition_cost_idr: cost,
        acquired_on: acquired, residual_idr: Math.round(cost * 0.05), room_id: roomId, custodian_user_id: ADMIN, serial_no: `SN${u.level}${String(seq).padStart(5, '0')}`,
        qr_token: uid('fac-asset-qr', id), source: 'manual', activated_at: `${acquired}T09:00:00+07:00`, activated_by: ADMIN })
      return id
    }
    const firstProjector = []
    for (const { id } of classRooms) {
      firstProjector.push(asset('Proyektor Epson EB-X51', 'ELK', 6500000, id, '2025-07-10', r.chance(0.15) ? 'rusak_ringan' : 'baik'))
      asset('AC Daikin 1 PK', 'ELK', 5200000, id, '2024-07-15')
      asset('Set Meja & Kursi Siswa (32 set)', 'MBL', 24000000, id, '2023-07-01')
    }
    for (const id of [lab, ...extra]) for (let k = 1; k <= 30; k++) asset(`PC Lenovo ThinkCentre Neo 50s #${k}`, 'ELK', 8500000, id, '2025-01-20', k === 7 ? 'rusak_ringan' : 'baik')
    asset('Sound System Yamaha', 'ELK', 15000000, aula, '2024-11-01')
    asset('Laptop Operasional TU', 'ELK', 9800000, kantor, '2026-02-01')
    out.push(insert('assets', assets, { chunk: 500 }))

    // ---- maintenance ----
    const schedules = [
      { id: uid('fac-sched', u.key, 'ac'), school_id: S, room_id: classRooms[0].id, title: 'Servis rutin AC ruang kelas', priority: 'sedang', interval_unit: 'month', interval_count: 3,
        next_due_date: '2026-11-01', default_assignee_user_id: TEKNISI, default_assignee_name: 'Teknisi Teratai', active: true, created_by: ADMIN },
      { id: uid('fac-sched', u.key, 'lab'), school_id: S, room_id: lab, title: 'Pengecekan jaringan & PC laboratorium', priority: 'sedang', interval_unit: 'month', interval_count: 1,
        next_due_date: '2026-10-20', default_assignee_user_id: TEKNISI, default_assignee_name: 'Teknisi Teratai', active: true, created_by: ADMIN }]
    const orders = [
      ['Proyektor tidak menyala', classRooms[1].id, firstProjector[1], 'tinggi', 'diproses', 'report'],
      ['Kebocoran atap di depan perpustakaan', perpus, null, 'tinggi', 'ditugaskan', 'report'],
      ['Servis rutin AC ruang kelas', classRooms[0].id, null, 'sedang', 'selesai', 'preventive'],
      ['Lampu lapangan mati 2 titik', lapangan, null, 'rendah', 'dilaporkan', 'report'],
      ['PC laboratorium nomor 7 tidak bisa booting', lab, null, 'sedang', 'menunggu_material', 'report'],
    ].map(([title, roomId, assetId, priority, status, source], i) => ({ id: uid('fac-wo', u.key, i), school_id: S, code: `WO-${u.level}-2610-${String(i + 1).padStart(3, '0')}`, source,
      room_id: roomId, asset_id: assetId, title, description: title + '.', priority, status, reporter_user_id: ADMIN, reporter_name: 'Admin Teratai',
      assignee_user_id: status === 'dilaporkan' ? null : TEKNISI, assignee_name: status === 'dilaporkan' ? null : 'Teknisi Teratai',
      completion_note: status === 'selesai' ? 'Filter dibersihkan, freon dicek normal.' : null, completed_at: status === 'selesai' ? '2026-10-05T15:00:00+07:00' : null,
      schedule_id: source === 'preventive' ? schedules[0].id : null, scheduled_for: source === 'preventive' ? '2026-10-01' : null, idempotency_key: `demo:wo:${u.key}:${i}`,
      created_at: `2026-10-${String(1 + i * 2).padStart(2, '0')}T09:00:00+07:00` }))
    out.push(insert('maintenance_schedules', schedules), insert('work_orders', orders, { conflict: '(school_id, code) DO NOTHING' }))
  }
  return { teratai_ops: out.join('\n\n') }
}
