// Phase 5: tuition billing July-October 2026 for every student of the three
// units: fee schemes (monthly SPP + yearly activity fee), monthly bills with
// their items, scholarships, and the payments behind the paid parts — via
// the payment gateway (VA / QRIS), uploaded transfer proofs (some waiting for
// verification) and the cashier — allocated to bill items, with receipts.
// About 8% of students are in arrears for several months (Tunggakan / Aging),
// with follow-ups and reminders. SD and SMK first get SMP's billing setup.
import { dbRows, insert, q, rng, uid } from './lib.mjs'
import { MODEL } from './model.mjs'

const SMP = '11111111-1111-1111-1111-111111111111'
const ADMIN = '22222222-2222-2222-2222-222222222222'
const MONTHS = [[2026, 7, 'Juli'], [2026, 8, 'Agustus'], [2026, 9, 'September'], [2026, 10, 'Oktober']]
const FEES = {
  sd: { spp: 450000, kegiatan: 1200000 },
  smp: { spp: 750000, kegiatan: 1500000 },
  smk: { spp: 650000, kegiatan: 1800000, praktik: 150000 },
}
// Settings tables keyed by school: SD/SMK get a copy of SMP's rows.
const SETTINGS = ['gateway_settings', 'gateway_accounts', 'billing_reminder_settings', 'billing_journal_settings', 'autodebit_settings',
  'wallet_settings', 'wallet_accounts', 'fee_component_accounts']

const due = (y, m) => `${y}-${String(m).padStart(2, '0')}-10T23:59:59+07:00`
const at = (y, m, d, h = 10, mi = 0) => `${y}-${String(m).padStart(2, '0')}-${String(d).padStart(2, '0')}T${String(h).padStart(2, '0')}:${String(mi).padStart(2, '0')}:00+07:00`

export function generate() {
  const out = []
  const smpMethods = dbRows('teratai_billing', `SELECT * FROM payment_methods WHERE school_id = '${SMP}' ORDER BY sort`)
  const existingSchemes = dbRows('teratai_billing', `SELECT id, name, grade_level FROM billing_schemes WHERE school_id = '${SMP}'`)
  const billed = new Set(dbRows('teratai_billing', `SELECT student_user_id FROM student_bills`).map((x) => x.student_user_id))

  for (const u of MODEL) {
    const r = rng('billing:' + u.key)
    const S = u.schoolId
    const fee = FEES[u.key]
    out.push(`-- ===== ${u.level} =====`)

    // ---- setup (SD / SMK copy SMP) ----
    let methods = smpMethods
    if (S !== SMP) {
      for (const t of SETTINGS) {
        out.push(`DO $$ DECLARE cols text; vals text; BEGIN
  SELECT string_agg(quote_ident(column_name), ', ' ORDER BY ordinal_position),
         string_agg(CASE column_name WHEN 'school_id' THEN quote_literal('${S}') || '::uuid' WHEN 'id' THEN 'gen_random_uuid()' ELSE quote_ident(column_name) END, ', ' ORDER BY ordinal_position)
    INTO cols, vals FROM information_schema.columns WHERE table_schema = 'public' AND table_name = '${t}';
  IF NOT EXISTS (SELECT 1 FROM ${t} WHERE school_id = '${S}') THEN
    EXECUTE format('INSERT INTO ${t} (%s) SELECT %s FROM ${t} WHERE school_id = %L ON CONFLICT DO NOTHING', cols, vals, '${SMP}');
  END IF;
END $$;`)
      }
      methods = smpMethods.map((m) => ({ ...m, id: uid('pay-method', u.key, m.code), school_id: S }))
      out.push(insert('payment_methods', methods.map(({ created_at, updated_at, ...m }) => m)))
    }
    const enabled = methods.filter((m) => m.enabled)

    // ---- schemes ----
    const sppScheme = (S === SMP && existingSchemes.find((s) => /^SPP Reguler/.test(s.name))?.id) || uid('scheme', u.key, 'spp')
    const schemes = []
    const components = []
    if (sppScheme === uid('scheme', u.key, 'spp')) {
      schemes.push({ id: sppScheme, school_id: S, academic_year_id: u.yearId, grade_level: null, name: `SPP Reguler ${u.level} 2026/2027`, status: 'active',
        installment: false, due_day: 10, effective_from: '2026-07-01', notes: 'SPP bulanan seluruh kelas, jatuh tempo tanggal 10.' })
      components.push({ id: uid('scheme-comp', sppScheme, 'SPP'), school_id: S, scheme_id: sppScheme, code: 'SPP', name: 'SPP Pokok Bulanan', kind: 'monthly',
        amount_idr: fee.spp, sort_order: 1, is_free: false, installment_terms: 1 })
      if (fee.praktik) components.push({ id: uid('scheme-comp', sppScheme, 'PRAKTIK'), school_id: S, scheme_id: sppScheme, code: 'PRAKTIK',
        name: 'Biaya Praktik Bengkel / Lab', kind: 'monthly', amount_idr: fee.praktik, sort_order: 2, is_free: false, installment_terms: 1 })
    }
    const kegScheme = {}
    for (const grade of [...new Set(u.classes.map((c) => c.grade))]) {
      const existing = S === SMP && existingSchemes.find((s) => s.grade_level === grade && /Kegiatan/.test(s.name))
      const id = existing?.id ?? uid('scheme', u.key, 'kegiatan', grade)
      kegScheme[grade] = { id, amount: existing ? 1500000 : fee.kegiatan }
      if (existing) continue
      schemes.push({ id, school_id: S, academic_year_id: u.yearId, grade_level: grade, name: `Uang Kegiatan Tahunan Kelas ${grade} 2026/2027`, status: 'active',
        installment: true, due_day: 15, effective_from: '2026-07-01', notes: 'Dapat dicicil 3 kali (Juli, Agustus, Oktober).' })
      components.push({ id: uid('scheme-comp', id, 'KEGIATAN'), school_id: S, scheme_id: id, code: 'KEGIATAN', name: 'Uang Kegiatan Tahunan', kind: 'annual',
        amount_idr: fee.kegiatan, sort_order: 1, is_free: false, installment_terms: 3, installment_interval_months: 1 })
    }
    out.push(insert('billing_schemes', schemes), insert('billing_scheme_components', components, { conflict: '(scheme_id, code) DO NOTHING' }))

    // Scholarship: 50% SPP for ~5% of the students.
    const scholars = new Set(u.students.filter((_, i) => i % 19 === 3).map((s) => s.id))
    const discounts = [...scholars].map((sid) => ({ id: uid('discount', sid), school_id: S, name: 'Beasiswa Prestasi 50% SPP', kind: 'percent', value: 5000,
      priority: 10, scope: 'student', student_user_id: sid, scholarship: true, active: true, status: 'active', valid_from: '2026-07-01', valid_to: '2027-06-30',
      reason: 'Peringkat 1-3 paralel tahun lalu', requested_by: ADMIN, decided_by: '77777777-7777-7777-7777-777777777777', decision_note: 'Disetujui' }))
    out.push(insert('bill_discounts', discounts))

    // ---- bills + payments ----
    const bills = []
    const items = []
    const installments = []
    const payments = []
    const allocations = []
    const cashier = []
    const gateway = []
    const gatewayBills = []
    const receipts = []
    const followups = []
    const reminders = []
    let receiptNo = 10000 // generated receipts start above any earlier ones
    let txNo = 0
    const receipt = (source, sourceId, y = 2026) => receipts.push({ id: uid('receipt', source, sourceId), school_id: S, receipt_no: `RCP-${y}-${u.level}-${String(++receiptNo).padStart(5, '0')}`,
      source, source_id: sourceId, sent_at: null })

    for (const st of u.students) {
      if (billed.has(st.id)) continue // the demo student keeps the curated bills
      const cls = u.classes.find((c) => c.id === st.classId)
      const arrears = r.chance(0.08)
      const habit = r.next() // how this family pays: <0.55 gateway, <0.8 upload, else cashier
      const channel = habit < 0.55 ? 'gateway' : habit < 0.8 ? 'upload' : 'cashier'

      // Monthly SPP (+ praktik on SMK).
      MONTHS.forEach(([y, m, label], mi) => {
        const billId = uid('bill', st.id, y, m, 'spp')
        const lines = [{ code: 'SPP', name: `SPP Pokok ${label} ${y}`, amount: fee.spp }]
        if (fee.praktik) lines.push({ code: 'PRAKTIK', name: `Biaya Praktik ${label} ${y}`, amount: fee.praktik })
        const base = lines.reduce((t, l) => t + l.amount, 0)
        const disc = scholars.has(st.id) ? fee.spp / 2 : 0
        const net = base - disc
        // Payment outcome of this month.
        let paid
        if (arrears && mi >= 1) paid = 0
        else if (m === 10) paid = r.chance(0.6) ? net : r.chance(0.25) ? Math.round(net / 2 / 1000) * 1000 : 0
        else if (m === 9) paid = r.chance(0.93) ? net : Math.round(net / 2 / 1000) * 1000
        else paid = net
        const status = paid >= net ? 'paid' : paid > 0 ? 'partial' : 'unpaid'
        bills.push({ id: billId, school_id: S, student_user_id: st.id, scheme_id: sppScheme, period_year: y, period_month: m, base_idr: base, discount_idr: disc,
          net_idr: net, status, due_at: due(y, m),
          discount_snapshot: disc ? [{ id: uid('discount', st.id), label: 'Beasiswa Prestasi 50% SPP', source: 'bill_discount', applied: disc, priority: 10,
            requested: disc, base_before: base, base_after: net }] : [], created_at: at(y, m, 1, 6) })
        const itemIds = lines.map((l, li) => {
          const id = uid('bill-item', billId, l.code)
          const d = li === 0 ? disc : 0
          items.push({ id, school_id: S, bill_id: billId, name: l.name, component_code: l.code, amount_idr: l.amount, discount_idr: d, net_idr: l.amount - d })
          return { id, net: l.amount - d }
        })
        if (paid > 0) pay(st, billId, itemIds, paid, y, m, channel, mi, status === 'partial')
      })

      // Yearly activity fee, 3 instalments.
      const keg = kegScheme[cls.grade]
      const kegId = uid('bill', st.id, 2026, 7, 'kegiatan')
      const third = Math.round(keg.amount / 3 / 1000) * 1000
      const kegPaid = arrears ? 0 : r.chance(0.5) ? keg.amount : r.chance(0.6) ? third : r.chance(0.5) ? third * 2 : 0
      bills.push({ id: kegId, school_id: S, student_user_id: st.id, scheme_id: keg.id, period_year: 2026, period_month: 7, base_idr: keg.amount, discount_idr: 0,
        net_idr: keg.amount, status: kegPaid >= keg.amount ? 'paid' : kegPaid > 0 ? 'partial' : 'unpaid', due_at: '2026-10-15T23:59:59+07:00', discount_snapshot: [],
        created_at: at(2026, 7, 1, 6) })
      const kegItem = uid('bill-item', kegId, 'KEGIATAN')
      items.push({ id: kegItem, school_id: S, bill_id: kegId, name: 'Uang Kegiatan Tahunan 2026/2027', component_code: 'KEGIATAN', amount_idr: keg.amount, discount_idr: 0, net_idr: keg.amount })
      ;[[7, 15], [8, 15], [10, 15]].forEach(([mm, dd], k) => installments.push({ id: uid('bill-inst', kegId, k + 1), school_id: S, bill_id: kegId, seq: k + 1,
        due_at: at(2026, mm, dd, 23, 59), amount_idr: k < 2 ? third : keg.amount - third * 2 }))
      if (kegPaid > 0) pay(st, kegId, [{ id: kegItem, net: keg.amount }], kegPaid, 2026, 7, channel, 9, kegPaid < keg.amount)

      // Collection follow-up for families in arrears.
      if (arrears) {
        const outstanding = MONTHS.slice(1).length * (fee.spp + (fee.praktik ?? 0)) + keg.amount
        followups.push({ id: uid('followup', st.id, 1), school_id: S, student_user_id: st.id, channel: 'whatsapp', outcome: 'Orang tua berjanji membayar tunggakan Agustus-September.',
          promise_date: '2026-10-20', promise_amount_idr: (fee.spp + (fee.praktik ?? 0)) * 2, next_action_on: '2026-10-21', created_by: ADMIN, created_at: at(2026, 10, 2) })
        followups.push({ id: uid('followup', st.id, 2), school_id: S, student_user_id: st.id, channel: 'phone', outcome: 'Telepon tidak diangkat; dijadwalkan panggilan ulang.',
          next_action_on: '2026-10-16', created_by: ADMIN, created_at: at(2026, 10, 12) })
        for (const [kind, d] of [['on_due', 10], ['overdue', 13]]) {
          reminders.push({ id: uid('reminder', st.id, kind), school_id: S, student_user_id: st.id, guardian_user_id: st.parentId, kind, offset_days: kind === 'on_due' ? 0 : 3,
            outstanding_idr: outstanding, status: 'sent', attempts: 1, sent_by: null, idempotency_key: `demo:reminder:${st.id}:${kind}`, created_at: at(2026, 10, d, 8) })
        }
      }
    }

    function pay(st, billId, itemIds, amount, y, m, channel, mi, partial) {
      const day = Math.min(28, 3 + ((mi * 7 + st.id.charCodeAt(0)) % 9))
      const when = at(y, m, day, 8 + (day % 10), (day * 7) % 60)
      const payId = uid('bill-payment', billId)
      // Spread the amount over the bill items in order.
      let left = amount
      const allocs = itemIds.map((it) => {
        const a = Math.min(left, it.net)
        left -= a
        return a > 0 ? { id: uid('bill-alloc', payId, it.id), school_id: S, payment_id: payId, bill_item_id: it.id, amount_idr: a, created_at: when } : null
      }).filter(Boolean)
      // Recent transfers are still waiting for the finance team.
      const pendingUpload = channel === 'upload' && m === 10 && partial === false && st.id.charCodeAt(1) % 3 === 0
      let source = channel, sourceId = null
      if (channel === 'gateway') {
        const method = enabled[(st.id.charCodeAt(2) + mi) % enabled.length]
        const txId = uid('gateway-tx', payId)
        const feeIdr = method.fee_kind === 'flat' ? method.fee_flat_idr : Math.ceil((amount * method.fee_bp) / 10000)
        const parentPays = method.fee_bearer === 'parent'
        gateway.push({ id: txId, school_id: S, tx_no: `TX-${u.level}-${y}${String(m).padStart(2, '0')}-${String(++txNo).padStart(5, '0')}`, payer_user_id: st.parentId,
          student_user_id: st.id, method_id: method.id, bills_amount_idr: amount, fee_idr: feeIdr, total_idr: parentPays ? amount + feeIdr : amount, deposit_idr: 0,
          fee_bearer: method.fee_bearer, status: 'success', provider: 'nicepay', provider_ref: `NP-${txId.slice(0, 18)}`,
          va_number: method.kind === 'va' ? `8848${String(Math.abs(hash(txId)) % 1e12).padStart(12, '0')}` : null, expires_at: when, paid_at: when,
          idempotency_key: `demo:gw:${txId}`, purpose: 'bills', created_at: when })
        gatewayBills.push({ transaction_id: txId, bill_id: billId, position: 1, amount_idr: amount })
        sourceId = txId
        receipt('gateway', txId, y)
      } else if (channel === 'cashier') {
        const cid = uid('cashier', payId)
        cashier.push({ id: cid, school_id: S, student_user_id: st.id, method: st.id.charCodeAt(3) % 2 ? 'cash' : 'transfer', amount_idr: amount, deposit_idr: 0,
          received_by: ADMIN, note: 'Pembayaran di loket TU', status: 'recorded', idempotency_key: `demo:cashier:${cid}`, created_at: when })
        sourceId = cid
        receipt('cashier', cid, y)
      } else {
        source = 'upload'
      }
      payments.push({ id: payId, school_id: S, student_bill_id: billId, amount_idr: amount, transfer_at: when,
        bank_name: channel === 'gateway' ? 'NICEPay' : channel === 'cashier' ? 'Kasir' : ['BCA', 'Bank Mandiri', 'BRI', 'BNI', 'BSI'][st.id.charCodeAt(4) % 5],
        account_name: u.parents.find((p) => p.id === st.parentId)?.name ?? null, reference: `REF${Math.abs(hash(payId)) % 1000000}`,
        object_key: source === 'upload' ? `demo/billing/${payId}.jpg` : null, file_name: source === 'upload' ? 'bukti-transfer.jpg' : null,
        mime: source === 'upload' ? 'image/jpeg' : null, size_bytes: source === 'upload' ? 180224 : null, sha256: source === 'upload' ? payId.replace(/-/g, '').padEnd(64, '0') : null,
        status: pendingUpload ? 'pending' : 'verified', verified_at: pendingUpload ? null : when, idempotency_key: `demo:pay:${payId}`,
        uploader_user_id: source === 'upload' ? st.parentId : ADMIN, source, source_id: sourceId, created_at: when })
      if (!pendingUpload) allocations.push(...allocs)
      else {
        // A bill counted as paid needs a verified payment: keep it open.
        const b = bills.find((x) => x.id === billId)
        if (b) b.status = 'unpaid'
      }
    }

    out.push(insert('student_bills', bills, { conflict: '(school_id, student_user_id, period_year, period_month, scheme_id) DO NOTHING', chunk: 800 }))
    out.push(insert('bill_items', items, { chunk: 1000 }), insert('bill_installments', installments, { conflict: '(bill_id, seq) DO NOTHING', chunk: 1000 }))
    out.push(insert('gateway_transactions', gateway, { conflict: '(school_id, idempotency_key) DO NOTHING', chunk: 800 }), insert('gateway_transaction_bills', gatewayBills, { chunk: 1000 }))
    out.push(insert('cashier_payments', cashier, { conflict: '(school_id, idempotency_key) DO NOTHING' }))
    out.push(insert('bill_payments', payments, { conflict: '(school_id, idempotency_key) DO NOTHING', chunk: 800 }), insert('bill_payment_allocations', allocations, { chunk: 1000 }))
    out.push(insert('receipts', receipts, { conflict: '(source, source_id) DO NOTHING', chunk: 1000 }))
    out.push(`INSERT INTO receipt_sequences (school_id, year, last) VALUES (${q(S)}, 2026, ${receiptNo}) ON CONFLICT (school_id, year) DO UPDATE SET last = GREATEST(receipt_sequences.last, EXCLUDED.last);`)
    out.push(insert('bill_followups', followups), insert('bill_reminder_log', reminders, { conflict: '(school_id, idempotency_key) DO NOTHING' }))
  }
  return { teratai_billing: out.join('\n\n') }
}

function hash(s) {
  let h = 0
  for (let i = 0; i < s.length; i++) h = (Math.imul(31, h) + s.charCodeAt(i)) | 0
  return h
}
