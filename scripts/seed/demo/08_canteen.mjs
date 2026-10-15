// Phase 8: canteen & student wallets of the three units — tenants with menus
// and revenue-share schemes (SMP keeps its tenants), allowance and savings
// wallets for about 70% of the students with monthly top-ups, daily canteen
// purchases paid from the wallet (running balance kept consistent) plus cash
// sales, and the monthly tenant settlements (July/August paid, September
// approved).
import { addDays, date, dbRows, insert, q, rng, uid } from './lib.mjs'
import { MODEL } from './model.mjs'

const SMP = '11111111-1111-1111-1111-111111111111'
const ADMIN = '22222222-2222-2222-2222-222222222222'
const OPERATOR = 'a2600000-0000-0000-0000-00000000000a'
const HOLIDAYS = new Set(['2026-08-17', '2026-08-25'])
const TENANTS = [
  ['KNT-01', 'Kantin Bu Sari', 'Kantin Utama Lt. 1', 'Sari Wulandari'],
  ['KNT-02', 'Warung Sehat Pak Darto', 'Selasar Gedung B', 'Sudarto'],
  ['KNT-03', 'Kedai Jus Segar', 'Dekat Lapangan', 'Rina Maharani'],
]
const MENU = {
  'KNT-01': [['Nasi Goreng Ayam', 'Makanan', 15000], ['Mie Ayam', 'Makanan', 12000], ['Nasi Uduk Telur', 'Makanan', 10000], ['Es Teh Manis', 'Minuman', 4000], ['Roti Bakar Cokelat', 'Jajanan', 8000]],
  'KNT-02': [['Nasi Sayur Ayam Bakar', 'Makanan', 16000], ['Gado-Gado', 'Makanan', 12000], ['Air Mineral', 'Minuman', 3000], ['Pisang Rebus', 'Jajanan', 5000], ['Bubur Kacang Hijau', 'Jajanan', 6000]],
  'KNT-03': [['Jus Jeruk', 'Minuman', 8000], ['Jus Alpukat', 'Minuman', 10000], ['Es Buah', 'Minuman', 9000], ['Salad Buah', 'Jajanan', 10000], ['Susu Kedelai', 'Minuman', 5000]],
}
const MONTH_START = ['2026-07-13', '2026-08-01', '2026-09-01', '2026-10-01']

export function generate() {
  const out = []
  const smpTenants = Object.fromEntries(dbRows('teratai_billing', `SELECT code, id FROM canteen_tenants WHERE school_id = '${SMP}'`).map((x) => [x.code, x.id]))
  const smpMenu = dbRows('teratai_billing', `SELECT id, tenant_id, name, price_idr FROM canteen_menu_items WHERE school_id = '${SMP}' AND deleted_at IS NULL AND active`)
  const haveWallet = new Set(dbRows('teratai_billing', `SELECT student_user_id || '|' || kind k FROM wallets`).map((x) => x.k))

  for (const u of MODEL) {
    const r = rng('canteen:' + u.key)
    const S = u.schoolId
    out.push(`-- ===== ${u.level} =====`)

    // ---- tenants & menus ----
    const tenants = TENANTS.map(([code, name, loc, owner]) => ({ code, id: (S === SMP && smpTenants[code]) || uid('canteen-tenant', u.key, code), name, loc, owner }))
    out.push(insert('canteen_tenants', tenants.filter((t) => !(S === SMP && smpTenants[t.code])).map((t) => ({ id: t.id, school_id: S, code: t.code, name: t.name,
      location_label: t.loc, owner_name: t.owner, owner_phone: `0812${r.int(10000000, 99999999)}`, bank_name: 'BCA', bank_account_no: String(r.int(1000000000, 9999999999)),
      bank_account_name: t.owner, status: 'active', is_open: true })), { conflict: '(school_id, code) DO NOTHING' }))
    if (S === SMP) out.push(`UPDATE canteen_tenants SET status = 'active', is_open = true WHERE school_id = '${S}';`)
    out.push(insert('canteen_tenant_schemes', tenants.filter((t) => !(S === SMP && smpTenants[t.code])).map((t) => ({ id: uid('canteen-scheme', t.id), school_id: S, tenant_id: t.id,
      kind: 'revenue_share', share_bp: 1000, share_base: 'all', effective_from: '2026-07-01', created_by: ADMIN }))))
    // The admin can serve every tenant; the demo canteen operator works at the
    // first one (an operator without a tenant gets 403 on POS and reports).
    out.push(insert('canteen_tenant_operators', [
      ...tenants.map((t) => ({ tenant_id: t.id, user_id: ADMIN, school_id: S, email: 'admin@teratai.sch.id', full_name: 'Admin Teratai', active: true })),
      { tenant_id: tenants[0].id, user_id: OPERATOR, school_id: S, email: 'operator.kantin@teratai.sch.id', full_name: 'Operator Kantin Teratai', active: true },
    ], { conflict: 'DO NOTHING' }))
    const menu = {}
    const menuRows = []
    for (const t of tenants) {
      const existing = S === SMP ? smpMenu.filter((m) => m.tenant_id === t.id) : []
      menu[t.id] = existing.map((m) => ({ id: m.id, name: m.name, price: Number(m.price_idr) }))
      for (const [name, category, price] of MENU[t.code]) {
        if (menu[t.id].some((m) => m.name === name)) continue
        const id = uid('canteen-item', t.id, name)
        menu[t.id].push({ id, name, price })
        menuRows.push({ id, school_id: S, tenant_id: t.id, name, category, price_idr: price, active: true })
      }
    }
    out.push(insert('canteen_menu_items', menuRows))

    // ---- wallets, top-ups, purchases ----
    const days = []
    for (let d = new Date('2026-07-13T00:00:00Z'); d <= new Date('2026-10-14T00:00:00Z'); d = addDays(d, 1)) {
      const ds = date(d)
      if (d.getUTCDay() === 0 || d.getUTCDay() === 6 || HOLIDAYS.has(ds)) continue
      days.push(ds)
    }
    const wallets = []
    const movements = []
    const topups = []
    const sales = []
    const saleItems = []
    // Daily receipt numbers continue after the ones an earlier seed used.
    const seqs = Object.fromEntries(dbRows('teratai_billing', `SELECT tenant_id || '|' || sale_date k, last FROM canteen_sale_sequences WHERE tenant_id IN (${tenants.map((t) => q(t.id)).join(',')})`).map((x) => [x.k, Number(x.last)]))
    const totals = {} // tenant|month -> { wallet, cash }
    const saleNo = (t, ds) => {
      const k = `${t.id}|${ds}`
      seqs[k] = (seqs[k] ?? 0) + 1
      return `${t.code}-${ds.replace(/-/g, '')}-${String(seqs[k]).padStart(4, '0')}`
    }
    const addSale = (t, ds, hhmm, payMethod, studentId, movementId, items) => {
      const total = items.reduce((s, it) => s + it.price * it.qty, 0)
      const id = uid('canteen-sale', t.id, ds, payMethod, studentId ?? hhmm, items.map((i) => i.id).join())
      sales.push({ id, school_id: S, tenant_id: t.id, sale_no: saleNo(t, ds), pay_method: payMethod, total_idr: total, student_user_id: studentId, wallet_movement_id: movementId,
        operator_user_id: ADMIN, client_txn_id: uid('canteen-client', id), recorded_at: `${ds}T${hhmm}:00+07:00`, synced_at: `${ds}T${hhmm}:05+07:00`, offline: false, status: 'completed' })
      items.forEach((it, i) => saleItems.push({ id: uid('canteen-sale-item', id, i), sale_id: id, item_id: it.id, name_snapshot: it.name, qty: it.qty, unit_price_idr: it.price, line_total_idr: it.price * it.qty }))
      const mk = `${t.id}|${ds.slice(0, 7)}`
      totals[mk] ??= { wallet: 0, cash: 0 }
      totals[mk][payMethod] += total
      return total
    }

    for (const st of u.students.filter((_, i) => i % 10 < 7)) {
      if (haveWallet.has(`${st.id}|allowance`)) continue
      // One generator per student: skipping one never shifts the others.
      const r = rng('canteen-student:' + st.id)
      const walletId = uid('wallet', S, st.id, 'allowance')
      const limit = u.level === 'SD' ? 20000 : u.level === 'SMP' ? 30000 : 40000
      let bal = 0
      let mseq = 0
      const move = (kind, amount, ds, refType, refId, hhmm = '06:30') => {
        bal += amount
        const id = uid('wallet-move', walletId, ++mseq)
        movements.push({ id, school_id: S, wallet_id: walletId, kind, amount_idr: amount, balance_after_idr: bal, ref_type: refType, ref_id: refId, created_by: amount > 0 ? st.parentId : ADMIN,
          idempotency_key: `demo:wallet:${id}`, created_at: `${ds}T${hhmm}:00+07:00` })
        return id
      }
      for (const day of days) {
        const monthIdx = MONTH_START.findLastIndex((m) => day >= m)
        if (day === days.find((d) => d >= MONTH_START[monthIdx])) {
          const amount = r.pick([150000, 200000, 250000, 300000])
          const topupId = uid('wallet-topup', walletId, monthIdx)
          const channel = r.chance(0.6) ? 'gateway' : 'cashier'
          topups.push({ id: topupId, school_id: S, student_user_id: st.id, wallet_kind: 'allowance', amount_idr: amount, channel, status: 'success', requested_by: st.parentId,
            idempotency_key: `demo:topup:${topupId}`, notified_at: `${day}T06:10:00+07:00`, created_at: `${day}T06:00:00+07:00` })
          move(channel === 'gateway' ? 'topup_gateway' : 'topup_cashier', amount, day, 'wallet_topup', topupId, '06:00')
        }
        if (!r.chance(0.45)) continue
        const t = tenants[r.int(0, tenants.length - 1)]
        const items = [menu[t.id][r.int(0, menu[t.id].length - 1)]].map((m) => ({ ...m, qty: 1 }))
        if (r.chance(0.5)) { const m = menu[t.id][r.int(0, menu[t.id].length - 1)]; items.push({ ...m, qty: 1 }) }
        const total = items.reduce((s, it) => s + it.price, 0)
        if (total > bal || total > limit) continue
        const hhmm = r.pick(['09:35', '09:40', '09:45', '12:05', '12:15'])
        const moveId = uid('wallet-move', walletId, mseq + 1)
        move('pos_sale', -total, day, 'canteen_sale', null, hhmm)
        const saleTotal = addSale(t, day, hhmm, 'wallet', st.id, moveId, items)
        movements[movements.length - 1].ref_id = sales[sales.length - 1].id
        if (saleTotal !== total) throw new Error('total mismatch')
      }
      wallets.push({ id: walletId, school_id: S, student_user_id: st.id, kind: 'allowance', balance_idr: bal, daily_limit_idr: limit, status: 'active' })
      // Savings for some.
      if (r.chance(0.4) && !haveWallet.has(`${st.id}|savings`)) {
        const sid = uid('wallet', S, st.id, 'savings')
        let sbal = 0
        ;[0, 1, 2, 3].forEach((mi) => {
          const amount = r.pick([50000, 100000, 150000])
          sbal += amount
          const mid = uid('wallet-move', sid, mi + 1)
          const tid = uid('wallet-topup', sid, mi)
          topups.push({ id: tid, school_id: S, student_user_id: st.id, wallet_kind: 'savings', amount_idr: amount, channel: 'cashier', status: 'success', requested_by: st.parentId,
            idempotency_key: `demo:topup:${tid}`, created_at: `${MONTH_START[mi]}T07:30:00+07:00` })
          movements.push({ id: mid, school_id: S, wallet_id: sid, kind: 'topup_cashier', amount_idr: amount, balance_after_idr: sbal, ref_type: 'wallet_topup', ref_id: tid, created_by: ADMIN,
            idempotency_key: `demo:wallet:${mid}`, created_at: `${MONTH_START[mi]}T07:30:00+07:00` })
        })
        wallets.push({ id: sid, school_id: S, student_user_id: st.id, kind: 'savings', balance_idr: sbal, daily_limit_idr: null, status: 'active' })
      }
    }
    // Cash sales (teachers, guests, students without a wallet).
    const rc = rng('canteen-cash:' + u.key)
    for (const day of days) {
      for (const t of tenants) {
        for (let k = 0; k < rc.int(4, 9); k++) {
          const m = menu[t.id][rc.int(0, menu[t.id].length - 1)]
          addSale(t, day, `${rc.pick(['07', '09', '12'])}:${String(rc.int(10, 55))}`, 'cash', null, null, [{ ...m, qty: rc.int(1, 2) }])
        }
      }
    }

    // ---- monthly settlements ----
    const settlements = []
    for (const [mk, tot] of Object.entries(totals)) {
      const [tenantId, ym] = mk.split('|')
      if (ym === '2026-10') continue
      const end = new Date(Date.UTC(Number(ym.slice(0, 4)), Number(ym.slice(5)), 0))
      const share = Math.round(((tot.wallet + tot.cash) * 1000) / 10000)
      const status = ym === '2026-09' ? 'approved' : 'paid'
      const scheme = S === SMP ? null : uid('canteen-scheme', tenantId)
      settlements.push({ id: uid('canteen-settlement', tenantId, ym), school_id: S, tenant_id: tenantId, period_from: `${ym}-01`, period_to: date(end),
        scheme_id: scheme ?? `(SELECT id FROM canteen_tenant_schemes WHERE tenant_id = '${tenantId}' ORDER BY effective_from LIMIT 1)`, scheme_kind: 'revenue_share',
        wallet_sales_idr: tot.wallet, cash_sales_idr: tot.cash, school_share_idr: share, tenant_payout_idr: tot.wallet - share, status, decided_by: ADMIN,
        paid_at: status === 'paid' ? `${date(addDays(end, 5))}T10:00:00+07:00` : null, paid_ref: status === 'paid' ? `TRF-${ym}-${tenantId.slice(0, 4)}` : null,
        paid_by: status === 'paid' ? ADMIN : null, created_by: ADMIN, created_at: `${date(addDays(end, 2))}T09:00:00+07:00` })
    }

    out.push(insert('wallets', wallets, { conflict: '(school_id, student_user_id, kind) DO NOTHING' }))
    out.push(insert('wallet_topups', topups, { conflict: '(school_id, idempotency_key) DO NOTHING', chunk: 1000 }))
    // Movements first: a wallet sale points at its movement.
    out.push(insert('wallet_movements', movements, { conflict: '(school_id, idempotency_key) DO NOTHING', chunk: 1500 }))
    out.push(insert('canteen_sales', sales, { conflict: 'DO NOTHING', chunk: 1000 }), insert('canteen_sale_items', saleItems, { chunk: 1500 }))
    out.push(insert('canteen_sale_sequences', Object.entries(seqs).map(([k, last]) => ({ tenant_id: k.split('|')[0], sale_date: k.split('|')[1], last })),
      { conflict: '(tenant_id, sale_date) DO UPDATE SET last = GREATEST(canteen_sale_sequences.last, EXCLUDED.last)', chunk: 1500 }))
    // scheme_id may be a lookup for SMP's own tenants.
    out.push(...settlements.map((s) => insert('canteen_settlements', [s]).replace(q(s.scheme_id), s.scheme_id.startsWith('(') ? s.scheme_id : q(s.scheme_id))))
  }
  return { teratai_billing: out.join('\n\n') }
}
