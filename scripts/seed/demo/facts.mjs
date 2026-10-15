// Figures of a unit read back from the data the earlier generators produced
// (attendance, billing, achievements), so the quality and BI generators
// report the numbers the modules themselves show. Read once per run.
import { dbRows } from './lib.mjs'
import { MODEL } from './model.mjs'

let cache

function load() {
  const ids = MODEL.map((u) => `'${u.schoolId}'`).join(',')
  const att = dbRows('teratai_attendance', `SELECT school_id, classroom_id, to_char(session_date, 'YYYY-MM') AS month,
      count(*) FILTER (WHERE status IN ('hadir','terlambat')) AS present, count(*) FILTER (WHERE status <> 'belum_presensi') AS marked
    FROM student_attendances WHERE school_id IN (${ids}) GROUP BY 1, 2, 3`)
  const bills = dbRows('teratai_billing', `SELECT b.school_id, b.period_month AS month, sum(b.net_idr)::bigint AS billed, coalesce(sum(p.paid), 0)::bigint AS paid,
      count(*) AS bills, count(*) FILTER (WHERE b.status = 'paid') AS paid_bills
    FROM student_bills b
    LEFT JOIN (SELECT i.bill_id, sum(a.amount_idr) AS paid FROM bill_payment_allocations a JOIN bill_items i ON i.id = a.bill_item_id GROUP BY 1) p ON p.bill_id = b.id
    WHERE b.school_id IN (${ids}) AND b.scheme_id IS NOT NULL AND b.period_year = 2026 AND b.status <> 'void' GROUP BY 1, 2`)
  const ach = dbRows('teratai_studentlife', `SELECT school_id, level, count(*) AS n FROM act_achievements
    WHERE school_id IN (${ids}) AND status = 'verified' AND deleted_at IS NULL GROUP BY 1, 2`)
  cache = { att, bills, ach }
}

/**
 * Facts of a unit: attendance per month (and per grade), billing per month,
 * verified achievements per level. Rates are basis points (9450 = 94,5%).
 */
export function facts(unit) {
  if (!cache) load()
  const S = unit.schoolId
  const gradeOf = Object.fromEntries(unit.classes.map((c) => [c.id, c.grade]))
  const months = {}
  const grades = {}
  for (const r of cache.att.filter((x) => x.school_id === S)) {
    const m = (months[r.month] ??= { present: 0, marked: 0 })
    m.present += Number(r.present)
    m.marked += Number(r.marked)
    const g = ((grades[r.month] ??= {})[gradeOf[r.classroom_id] ?? '?'] ??= { present: 0, marked: 0 })
    g.present += Number(r.present)
    g.marked += Number(r.marked)
  }
  const rate = (x) => (x && x.marked ? Math.round((x.present * 10000) / x.marked) : 0)
  const billing = Object.fromEntries(cache.bills.filter((x) => x.school_id === S).map((b) => [Number(b.month), {
    billed: Number(b.billed), paid: Number(b.paid), bills: Number(b.bills), paidBills: Number(b.paid_bills),
    rate: b.billed ? Math.round((Number(b.paid) * 10000) / Number(b.billed)) : 0 }]))
  const achievements = Object.fromEntries(cache.ach.filter((x) => x.school_id === S).map((a) => [a.level || 'lainnya', Number(a.n)]))
  return {
    attendance: Object.fromEntries(Object.entries(months).map(([m, x]) => [m, { ...x, rate: rate(x) }])),
    attendanceByGrade: Object.fromEntries(Object.entries(grades).map(([m, gs]) => [m, Object.fromEntries(Object.entries(gs).map(([g, x]) => [g, { ...x, rate: rate(x) }]))])),
    billing,
    achievements,
    achievementsTotal: Object.values(achievements).reduce((a, b) => a + b, 0),
    students: unit.students.length,
    teachers: unit.teachers.length,
  }
}
