// Phase 10: school transport — SMP's existing routes get about 30 riders;
// SD and SMK get zones, fares, two routes with stops, vehicles, crew and
// about 25 riders each. For every unit: trips of the last ten school days
// with boarding / alighting events, parents' absence notices, and the
// monthly transport charges August-October.
import { addDays, date, dbRows, insert, rng, uid } from './lib.mjs'
import { MODEL } from './model.mjs'

const SMP = '11111111-1111-1111-1111-111111111111'
const TRANSPORT = 'a2600000-0000-0000-0000-000000000008'
const ROUTES = {
  sd: [['RT-SD-01', 'Rute Harapan Indah - Sekolah', [['Bundaran Harapan Indah', '06:00'], ['Perumahan Taman Harapan Baru', '06:15'], ['Pasar Kranji', '06:30']]],
    ['RT-SD-02', 'Rute Jatiasih - Sekolah', [['Gerbang Tol Jatiasih', '06:05'], ['Perumahan Jatiasih Indah', '06:20'], ['Kantor Kecamatan Jatiasih', '06:35']]]],
  smk: [['RT-SMK-01', 'Rute Summarecon - Sekolah', [['Halte Summarecon Mall Bekasi', '06:10'], ['Gerbang Perumahan Galaxy', '06:25'], ['Taman Kota Bekasi', '06:40']]],
    ['RT-SMK-02', 'Rute Pondok Gede - Sekolah', [['Terminal Pondok Gede', '05:55'], ['Pasar Pondok Gede', '06:10'], ['Perumahan Jatibening', '06:30']]]],
}

export function generate() {
  const out = []
  const days = []
  for (let d = new Date('2026-09-30T00:00:00Z'); d <= new Date('2026-10-14T00:00:00Z'); d = addDays(d, 1)) if (d.getUTCDay() > 0 && d.getUTCDay() < 6) days.push(date(d))
  const allocated = new Set(dbRows('teratai_ops', `SELECT student_user_id FROM transport_allocations WHERE end_date IS NULL`).map((x) => x.student_user_id))

  for (const u of MODEL) {
    const r = rng('transport:' + u.key)
    const S = u.schoolId
    out.push(`-- ===== ${u.level} =====`)
    let routes // [{ id, stops: [{id}], vehicleId, driverId, attendantId, fare }]
    if (S === SMP) {
      const rs = dbRows('teratai_ops', `SELECT r.id, (SELECT json_agg(s.id ORDER BY s.seq) FROM transport_stops s WHERE s.route_id = r.id AND s.deleted_at IS NULL) stops,
          a.vehicle_id, a.driver_crew_id, a.attendant_crew_id
        FROM transport_routes r JOIN transport_route_assignments a ON a.route_id = r.id AND a.valid_to IS NULL
        WHERE r.school_id = '${S}' AND r.status = 'active' AND r.deleted_at IS NULL`)
      routes = rs.filter((x) => x.stops?.length).map((x) => ({ id: x.id, stops: x.stops.map((id) => ({ id })), vehicleId: x.vehicle_id, driverId: x.driver_crew_id, attendantId: x.attendant_crew_id, fare: 450000 }))
    } else {
      const zones = [['Zona 1 (0-5 km)', 350000], ['Zona 2 (5-10 km)', 450000], ['Zona 3 (lebih dari 10 km)', 550000]].map(([name, fare], i) => ({ id: uid('tr-zone', u.key, i), name, fare }))
      out.push(insert('transport_settings', [{ school_id: S, prorata_policy: 'full_month', doc_reminder_days: 30, notify_parent_on_trip: true, component_code: 'transport', bill_due_day: 10 }],
        { conflict: '(school_id) DO NOTHING' }))
      out.push(insert('transport_zones', zones.map((z) => ({ id: z.id, school_id: S, name: z.name }))))
      out.push(insert('transport_fares', zones.map((z) => ({ id: uid('tr-fare', z.id), school_id: S, zone_id: z.id, monthly_amount_idr: z.fare, effective_from: '2026-07-01' }))))
      routes = []
      ROUTES[u.key].forEach(([code, name, stops], ri) => {
        const id = uid('tr-route', u.key, code)
        const vehicleId = uid('tr-vehicle', u.key, ri)
        const driverId = uid('tr-crew', u.key, ri, 'driver')
        const attendantId = uid('tr-crew', u.key, ri, 'attendant')
        out.push(insert('transport_routes', [{ id, school_id: S, code, name, status: 'active' }]))
        out.push(insert('transport_vehicles', [{ id: vehicleId, school_id: S, plate_no: `B ${7300 + ri * 11 + (u.key === 'sd' ? 0 : 100)} ${u.key === 'sd' ? 'TPA' : 'TPK'}`,
          name: `Bus Sekolah ${u.level} ${ri + 1}`, capacity: 19, status: 'active' }]))
        out.push(insert('transport_crew', [
          { id: driverId, school_id: S, kind: 'driver', external_name: r.pick(['Pak Slamet', 'Pak Joko', 'Pak Udin', 'Pak Rahmat']), phone: `0813${r.int(10000000, 99999999)}`,
            license_no: `SIM-B1-${r.int(100000, 999999)}`, license_valid_until: '2028-03-31' },
          { id: attendantId, school_id: S, kind: 'attendant', external_name: r.pick(['Bu Ani', 'Bu Tuti', 'Bu Wati', 'Bu Lilis']), phone: `0857${r.int(10000000, 99999999)}` }]))
        out.push(insert('transport_route_assignments', [{ id: uid('tr-assign', id), school_id: S, route_id: id, vehicle_id: vehicleId, driver_crew_id: driverId,
          attendant_crew_id: attendantId, valid_from: '2026-07-01' }]))
        const stopRows = stops.map(([sname, pickup], si) => ({ id: uid('tr-stop', id, si), school_id: S, route_id: id, seq: si + 1, name: sname, address: `${sname}, Bekasi`,
          zone_id: zones[si].id, pickup_time: pickup, dropoff_time: `${String(Math.floor((870 + si * 15) / 60)).padStart(2, '0')}:${String((870 + si * 15) % 60).padStart(2, '0')}` }))
        out.push(insert('transport_stops', stopRows))
        routes.push({ id, stops: stopRows, vehicleId, driverId, attendantId, fare: 450000 })
      })
    }
    if (!routes.length) continue

    // Riders.
    const riders = u.students.filter((s, i) => i % (S === SMP ? 4 : 5) === 1 && !allocated.has(s.id))
    const allocations = []
    const charges = []
    const notices = []
    riders.forEach((st, i) => {
      const route = routes[i % routes.length]
      const stop = route.stops[i % route.stops.length]
      st.route = route
      st.stop = stop
      allocations.push({ id: uid('tr-alloc', st.id), school_id: S, student_user_id: st.id, route_id: route.id, stop_id: stop.id, start_date: '2026-07-13', created_by: TRANSPORT })
      for (const m of [8, 9, 10]) charges.push({ id: uid('tr-charge', st.id, m), school_id: S, student_user_id: st.id, period_year: 2026, period_month: m,
        items: [{ label: `Antar-jemput ${['', '', '', '', '', '', '', 'Juli', 'Agustus', 'September', 'Oktober'][m]} 2026`, amount_idr: route.fare }], amount_idr: route.fare, version: 1, status: 'issued' })
      if (i % 6 === 0) notices.push({ id: uid('tr-notice', st.id), school_id: S, student_user_id: st.id, notice_date: '2026-10-16', direction: 'jemput',
        reason: 'Diantar orang tua', created_by: st.parentId })
    })

    // Trips of the last ten school days.
    const trips = []
    // Trips an earlier seed recorded keep their id.
    const tripEx = Object.fromEntries(dbRows('teratai_ops', `SELECT route_id || '|' || trip_date || '|' || direction k, id FROM transport_trips WHERE school_id = '${S}'`).map((x) => [x.k, x.id]))
    const events = []
    for (const route of routes) {
      const mine = riders.filter((s) => s.route === route)
      for (const d of days) {
        for (const direction of ['jemput', 'antar']) {
          const tripId = tripEx[`${route.id}|${d}|${direction}`] ?? uid('tr-trip', route.id, d, direction)
          if (!tripEx[`${route.id}|${d}|${direction}`]) trips.push({ id: tripId, school_id: S, route_id: route.id, trip_date: d, direction, vehicle_id: route.vehicleId, driver_crew_id: route.driverId,
            attendant_crew_id: route.attendantId, status: 'done', started_at: `${d}T${direction === 'jemput' ? '05:50' : '14:20'}:00+07:00`,
            finished_at: `${d}T${direction === 'jemput' ? '06:55' : '15:30'}:00+07:00` })
          for (const st of mine) {
            const absent = r.chance(0.05)
            const evs = absent ? ['absent'] : ['board', 'alight']
            evs.forEach((ev, k) => events.push({ id: uid('tr-event', tripId, st.id, ev), school_id: S, trip_id: tripId, student_user_id: st.id, stop_id: st.stop.id, event: ev,
              source: 'scan', recorded_by: TRANSPORT, recorded_at: `${d}T${direction === 'jemput' ? '06' : '14'}:${String(25 + k * 25).padStart(2, '0')}:00+07:00`,
              client_event_id: uid('tr-client', tripId, st.id, ev) }))
          }
        }
      }
    }
    out.push(insert('transport_allocations', allocations), insert('transport_charges', charges, { conflict: '(school_id, student_user_id, period_year, period_month) DO NOTHING' }))
    out.push(insert('transport_absence_notices', notices, { conflict: 'DO NOTHING' }))
    out.push(insert('transport_trips', trips, { conflict: '(route_id, trip_date, direction) DO NOTHING' }), insert('transport_trip_events', events, { conflict: 'DO NOTHING', chunk: 1500 }))
  }
  return { teratai_ops: out.join('\n\n') }
}
