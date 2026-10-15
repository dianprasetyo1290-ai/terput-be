// Shared helpers of the demo data generators (scripts/seed/demo/*.mjs).
// Everything is deterministic: the same key always gives the same uuid and the
// same seed the same "random" choices, so a re-run upserts the same rows
// (ON CONFLICT DO NOTHING) instead of piling up new ones, and ids line up
// across databases (a student's id in identity is the one billing uses).
import { createCipheriv, createHash, createHmac } from 'node:crypto'
import { execFileSync } from 'node:child_process'
import { readFileSync } from 'node:fs'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

const BE_ROOT = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '../../..')

/**
 * Rows of a query on the local stack's database, as objects. For ids the
 * generators must reuse but cannot derive (rows created by earlier seeds with
 * random ids, e.g. timetable slots).
 */
export function dbRows(db, sql) {
  const out = execFileSync('docker', ['compose', '-f', 'docker-compose.yml', '-f', 'docker-compose.local.yml', '--env-file', '.env.local',
    'exec', '-T', 'postgres', 'psql', '-U', process.env.POSTGRES_USER || 'teratai', '-d', db, '-At', '-c',
    `SELECT coalesce(json_agg(t), '[]') FROM (${sql}) t`], { cwd: BE_ROOT, encoding: 'utf8', maxBuffer: 256 * 1024 * 1024 })
  return JSON.parse(out.trim() || '[]')
}

/** Stable uuid (v5-shaped) for a key, e.g. uid('student', 'smp', 17). */
export function uid(...parts) {
  const h = createHash('sha1').update('sekoci-demo:' + parts.join(':')).digest('hex')
  return `${h.slice(0, 8)}-${h.slice(8, 12)}-5${h.slice(13, 16)}-${((parseInt(h[16], 16) & 3) | 8).toString(16)}${h.slice(17, 20)}-${h.slice(20, 32)}`
}

/** A variable of the local stack's .env.local. */
export function envLocal(key) {
  const text = readFileSync(path.join(BE_ROOT, '.env.local'), 'utf8')
  let value
  for (const line of text.split(/\r?\n/)) {
    const m = line.match(/^([A-Z0-9_]+)=(.*)$/)
    if (m && m[1] === key) value = m[2].trim() // the last definition wins, like docker compose
  }
  return value
}

/**
 * Field encryption of a service that stores sensitive columns as AES-256-GCM
 * (nonce || ciphertext || tag) with the school id as AAD, plus an HMAC-SHA256
 * blind index — the scheme of services/hr/internal/crypto. The nonce is
 * derived from the value's key so the generated SQL is stable.
 */
export function fieldCrypto(encKeyB64, blindKey) {
  const key = Buffer.from(encKeyB64, 'base64')
  if (key.length !== 32) throw new Error('encryption key must be 32 bytes (base64)')
  return {
    encrypt(plaintext, aad, nonceKey) {
      if (plaintext === null || plaintext === undefined || plaintext === '') return null
      const nonce = createHash('sha256').update(`nonce:${nonceKey}:${aad}`).digest().subarray(0, 12)
      const c = createCipheriv('aes-256-gcm', key, nonce)
      c.setAAD(Buffer.from(aad))
      const body = Buffer.concat([c.update(String(plaintext), 'utf8'), c.final()])
      return Buffer.concat([nonce, body, c.getAuthTag()])
    },
    blindIndex(value, aad) {
      if (!value) return null
      return createHmac('sha256', blindKey).update(aad).update(Buffer.from([0])).update(value).digest('hex')
    },
  }
}

/** Seeded PRNG (mulberry32). */
export function rng(seed) {
  let a = typeof seed === 'number' ? seed : parseInt(createHash('sha1').update(String(seed)).digest('hex').slice(0, 8), 16)
  const next = () => {
    a |= 0
    a = (a + 0x6d2b79f5) | 0
    let t = Math.imul(a ^ (a >>> 15), 1 | a)
    t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296
  }
  return {
    next,
    int: (min, max) => min + Math.floor(next() * (max - min + 1)),
    pick: (arr) => arr[Math.floor(next() * arr.length)],
    chance: (p) => next() < p,
  }
}

class PgArray {
  constructor(items, type) { this.items = items; this.type = type }
}
/** A Postgres array value for insert(), e.g. pgArray(['ATK', 'Buku']) or pgArray(ids, 'uuid'). */
export const pgArray = (items, type = 'text') => new PgArray(items, type)

/** SQL literal. */
export function q(v) {
  if (v === null || v === undefined) return 'NULL'
  if (typeof v === 'number') return String(v)
  if (typeof v === 'boolean') return v ? 'true' : 'false'
  if (v instanceof Date) return `'${v.toISOString()}'`
  if (Buffer.isBuffer(v)) return `'\\x${v.toString('hex')}'::bytea`
  if (v instanceof PgArray) return `ARRAY[${v.items.map(q).join(', ')}]::${v.type}[]`
  if (typeof v === 'object') return `'${JSON.stringify(v).replace(/'/g, "''")}'::jsonb`
  return `'${String(v).replace(/'/g, "''")}'`
}

/** Multi-row INSERT ... ON CONFLICT DO NOTHING, chunked. */
export function insert(table, rows, { conflict = 'DO NOTHING', chunk = 500 } = {}) {
  if (!rows.length) return ''
  // Columns of every row; a column a row leaves out takes the table default.
  const cols = [...new Set(rows.flatMap((r) => Object.keys(r)))]
  const out = []
  for (let i = 0; i < rows.length; i += chunk) {
    const values = rows.slice(i, i + chunk).map((r) => `(${cols.map((c) => (c in r ? q(r[c]) : 'DEFAULT')).join(', ')})`)
    out.push(`INSERT INTO ${table} (${cols.map((c) => (/^[a-z_][a-z0-9_]*$/.test(c) && c !== 'order' ? c : `"${c}"`)).join(', ')}) VALUES\n${values.join(',\n')}\nON CONFLICT ${conflict};`)
  }
  return out.join('\n')
}

export const date = (d) => d.toISOString().slice(0, 10)
export const addDays = (d, n) => new Date(d.getTime() + n * 86400000)

// ---- the demo school -------------------------------------------------------

export const FOUNDATION_ID = 'a2700000-0000-0000-0000-000000000001'

/** The three units of Yayasan Teratai Putih and their 2026/2027 year. */
export const UNITS = [
  { key: 'sd', level: 'SD', schoolId: 'a2700000-0000-0000-0000-0000000000d1', yearId: 'a2700000-0000-0000-0000-0000000000d2',
    classes: [['Kelas 1A', '1'], ['Kelas 2A', '2'], ['Kelas 3A', '3'], ['Kelas 4A', '4'], ['Kelas 5A', '5'], ['Kelas 6A', '6']] },
  { key: 'smp', level: 'SMP', schoolId: '11111111-1111-1111-1111-111111111111', yearId: '12121212-1212-1212-1212-121212121212',
    // The first four already exist (seeded before); the generator fills them.
    classes: [['Kelas VII A', '7', 'c1000000-0000-0000-0000-000000000001'], ['Kelas VII B', '7', '5e300200-0000-0000-0000-000000000001'],
      ['Kelas VIII A', '8', '5e300200-0000-0000-0000-000000000002'], ['Kelas VIII B', '8'],
      ['Kelas IX A', '9', '9ee5def5-e06d-443f-9a98-85846de9695d'], ['Kelas IX B', '9']] },
  { key: 'smk', level: 'SMK', schoolId: 'a2700000-0000-0000-0000-0000000000e1', yearId: 'a2700000-0000-0000-0000-0000000000e2',
    classes: [['X TKJ', '10'], ['X AKL', '10'], ['XI TKJ', '11'], ['XI AKL', '11'], ['XII TKJ', '12'], ['XII AKL', '12']] },
]

export const STUDENTS_PER_CLASS = 20
export const TERM_1 = { start: '2026-07-13', end: '2026-12-19' }
export const TERM_2 = { start: '2027-01-04', end: '2027-06-19' }

// ---- Indonesian names ------------------------------------------------------

const MALE = ['Ahmad', 'Muhammad', 'Rizky', 'Fajar', 'Dimas', 'Bagus', 'Arya', 'Rafi', 'Daffa', 'Naufal', 'Raka', 'Farhan', 'Hafiz', 'Ilham',
  'Kevin', 'Yoga', 'Aditya', 'Galih', 'Bima', 'Satria', 'Reza', 'Andi', 'Wahyu', 'Fikri', 'Zaki', 'Alif', 'Haikal', 'Rendi', 'Aldi', 'Gilang']
const FEMALE = ['Siti', 'Nur', 'Aisyah', 'Putri', 'Ayu', 'Dewi', 'Rina', 'Salsabila', 'Nabila', 'Zahra', 'Alya', 'Kirana', 'Anisa', 'Laras',
  'Maya', 'Citra', 'Intan', 'Fitri', 'Tiara', 'Nadia', 'Aulia', 'Keisha', 'Amelia', 'Sekar', 'Wulan', 'Indah', 'Rahma', 'Shafa', 'Dinda', 'Najwa']
const LAST = ['Pratama', 'Saputra', 'Wijaya', 'Nugroho', 'Hidayat', 'Santoso', 'Kusuma', 'Ramadhan', 'Permana', 'Setiawan', 'Lestari', 'Rahayu',
  'Hartono', 'Gunawan', 'Siregar', 'Nasution', 'Harahap', 'Simanjuntak', 'Wibowo', 'Purnomo', 'Susanto', 'Firmansyah', 'Maulana', 'Kurniawan',
  'Utami', 'Anggraini', 'Putra', 'Hakim', 'Syahputra', 'Rangkuti', 'Lubis', 'Tanjung', 'Halim', 'Suryadi', 'Prasetyo', 'Fauzi']
const PLACES = ['Jakarta', 'Bekasi', 'Bogor', 'Depok', 'Tangerang', 'Bandung', 'Semarang', 'Surabaya', 'Yogyakarta', 'Medan', 'Palembang', 'Cirebon']
const STREETS = ['Jl. Melati', 'Jl. Kenanga', 'Jl. Mawar', 'Jl. Anggrek', 'Jl. Flamboyan', 'Jl. Cempaka', 'Jl. Teratai', 'Jl. Dahlia', 'Jl. Kamboja', 'Jl. Seroja']
const RELIGIONS = ['Islam', 'Islam', 'Islam', 'Islam', 'Islam', 'Islam', 'Kristen', 'Katolik', 'Hindu', 'Buddha']

export function person(r, { gender = r.chance(0.5) ? 'L' : 'P', last = r.pick(LAST) } = {}) {
  const first = r.pick(gender === 'L' ? MALE : FEMALE)
  return { first, last, gender, name: `${first} ${last}` }
}
export const place = (r) => r.pick(PLACES)
export const address = (r) => `${r.pick(STREETS)} No. ${r.int(1, 120)}, ${r.pick(['Bekasi Timur', 'Bekasi Barat', 'Jatiasih', 'Pondok Gede', 'Cibubur', 'Duren Sawit'])}`
export const religion = (r) => r.pick(RELIGIONS)
export const phone = (r) => `08${r.pick(['12', '13', '21', '22', '52', '57', '77', '78', '95', '96'])}${String(r.int(10000000, 99999999))}`
export const slug = (s) => s.toLowerCase().replace(/[^a-z0-9]+/g, '.').replace(/^\.|\.$/g, '')

// ---- demo accounts and the Approval Center ---------------------------------

/** Accounts that act across the three units (roles granted by 27_identity_units.sql). */
export const ACCOUNTS = {
  admin: '22222222-2222-2222-2222-222222222222',
  kepala_sekolah: '77777777-7777-7777-7777-777777777777',
  kepala_yayasan: '88888888-8888-8888-8888-888888888888',
  finance_manager: '6adbb1f2-dc7c-449a-879f-a7460d72c4af',
  hr_manager: '73be9b1a-a65b-45dc-8ba6-ab9d15c2632d',
  hr_admin: '1baffb7f-3a79-4e47-9994-e4005c6dffbf',
  recruiter: '47846f7f-51e7-4601-a980-8a936b684759',
  training_admin: 'ba18877e-21cf-4bd1-9aa5-0f41fe7acf94',
  procurement_officer: 'a2600000-0000-0000-0000-000000000003',
  inventory_officer: 'a2600000-0000-0000-0000-000000000002',
  facility_officer: 'a2600000-0000-0000-0000-000000000006',
  teknisi: 'a2600000-0000-0000-0000-000000000007',
  servicedesk_agent: 'a2600000-0000-0000-0000-000000000010',
  servicedesk_manager: 'a2600000-0000-0000-0000-000000000011',
  qa_coordinator: 'a2600000-0000-0000-0000-000000000015',
  operator_dapodik: 'a2600000-0000-0000-0000-000000000016',
  humas: 'a2600000-0000-0000-0000-000000000017',
  bi_analyst: 'a2600000-0000-0000-0000-000000000018',
}

const CALLBACKS = {
  ops: 'http://ops:8093/internal/approval-callbacks',
  hr: 'http://hr:8092/internal/approval-callbacks',
  quality: 'http://quality:8095/internal/approval-callbacks',
}

/**
 * Collects Approval Center requests (teratai_core) with their steps, and the
 * in-app notifications they raise (teratai_notification): the approver of a
 * pending step is told it waits for them, the maker of a decided request
 * gets the outcome. `decisions` holds one entry per decided step, in order;
 * a rejected one ends the request.
 *
 *   const ap = approvals()
 *   const id = ap.add({ school, key, type: 'purchase_request', service: 'ops', sourceId, title, maker,
 *     steps: [['kepala_sekolah', 48]], decisions: [{ decision: 'approved', at, comment }], createdAt })
 *   return merge(ownSql, ap.sql())
 */
export function approvals() {
  const requests = []
  const steps = []
  const notes = []
  return {
    add({ school, key, type, service, sourceId, title, summary = {}, amount = null, maker, subject = null, steps: flow, decisions = [], createdAt, body }) {
      const id = uid('approval', key)
      const rejected = decisions.findIndex((d) => d.decision === 'rejected')
      const status = rejected >= 0 ? 'rejected' : decisions.length >= flow.length ? 'approved' : 'pending'
      const last = decisions[decisions.length - 1]
      const t0 = new Date(createdAt)
      requests.push({ id, school_id: school, request_type: type, source_service: service, source_id: sourceId, title, summary, amount_idr: amount,
        maker_user_id: maker, subject_user_id: subject, status, current_seq: status === 'pending' ? decisions.length + 1 : null,
        callback_url: CALLBACKS[service], correlation_id: `demo:${key}`, idempotency_key: `demo:${key}`,
        flow_snapshot: flow.map(([role, sla], i) => ({ seq: i + 1, approver_kind: 'role', approver_role: role, sla_hours: sla })),
        decided_at: status === 'pending' ? null : last.at, callback_status: status === 'pending' ? 'pending' : 'done',
        callback_attempts: status === 'pending' ? 0 : 1, last_callback_at: status === 'pending' ? null : last.at, created_at: createdAt, updated_at: last?.at ?? createdAt })
      let prevAt = t0
      flow.forEach(([role, sla], i) => {
        const d = decisions[i]
        const st = d ? d.decision : rejected >= 0 && i > rejected ? 'skipped' : status === 'pending' && i === decisions.length ? 'pending' : 'waiting'
        steps.push({ id: uid('approval-step', key, i + 1), school_id: school, request_id: id, seq: i + 1, approver_kind: 'role', approver_role: role, status: st,
          sla_hours: sla, due_at: st === 'pending' || d ? new Date(prevAt.getTime() + sla * 3600000) : null, decided_by: d ? (d.by ?? ACCOUNTS[role]) : null,
          comment: d?.comment ?? null, decided_at: d?.at ?? null, created_at: createdAt })
        if (d) prevAt = new Date(d.at)
      })
      const url = `/approvals/${id}`
      if (status === 'pending') {
        const role = flow[decisions.length][0]
        notes.push({ id: uid('approval-note', key), school_id: school, user_id: ACCOUNTS[role], type: 'approval', title: `Menunggu persetujuan Anda: ${title}`,
          body: body ?? (decisions.length ? `Tahap ${decisions.length} sudah disetujui, menunggu keputusan Anda.` : 'Pengajuan baru masuk ke kotak persetujuan Anda.'),
          data: { url, approval_id: id, source_service: service }, idempotency_key: `demo:approval:${key}`, created_at: prevAt.toISOString() })
      } else {
        notes.push({ id: uid('approval-note', key), school_id: school, user_id: maker, type: 'approval',
          title: `Pengajuan ${status === 'approved' ? 'disetujui' : 'ditolak'}: ${title}`, body: last.comment ?? '', data: { url, approval_id: id, source_service: service },
          idempotency_key: `demo:approval:${key}`, read_at: last.at, created_at: last.at })
      }
      return id
    },
    sql() {
      return {
        teratai_core: [insert('approval_requests', requests, { conflict: '(school_id, idempotency_key) DO NOTHING' }),
          insert('approval_steps', steps, { conflict: '(id) DO NOTHING' })].join('\n'),
        teratai_notification: insert('notifications', notes, { conflict: '(school_id, idempotency_key) DO NOTHING' }),
      }
    },
  }
}

/** Joins the per-database SQL of several parts of a generator. */
export function merge(...parts) {
  const out = {}
  for (const p of parts) for (const [db, sql] of Object.entries(p)) if (sql && sql.trim()) out[db] = out[db] ? `${out[db]}\n\n${sql}` : sql
  return out
}

/** Copies the SMP rows of a config table to another unit when it has none. */
export function cloneConfig(table, schoolId, { from = '11111111-1111-1111-1111-111111111111', nullCols = [] } = {}) {
  return `DO $$ DECLARE cols text; vals text; BEGIN
  SELECT string_agg(quote_ident(column_name), ', ' ORDER BY ordinal_position),
         string_agg(CASE WHEN column_name = 'school_id' THEN quote_literal('${schoolId}') || '::uuid'
           WHEN column_name = 'id' AND data_type = 'uuid' THEN 'gen_random_uuid()'
           WHEN column_name = ANY(ARRAY[${nullCols.map((c) => `'${c}'`).join(',') || "''"}]) THEN 'NULL'
           ELSE quote_ident(column_name) END, ', ' ORDER BY ordinal_position)
    INTO cols, vals FROM information_schema.columns WHERE table_schema = 'public' AND table_name = '${table}';
  IF NOT EXISTS (SELECT 1 FROM ${table} WHERE school_id = '${schoolId}') THEN
    EXECUTE format('INSERT INTO ${table} (%s) SELECT %s FROM ${table} WHERE school_id = %L ON CONFLICT DO NOTHING', cols, vals, '${from}');
  END IF;
END $$;`
}

/** ISO timestamp in Jakarta time, e.g. at('2026-10-14', '09:30'). */
export const at = (d, t = '09:00') => `${d}T${t}:00+07:00`
