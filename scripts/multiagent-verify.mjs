#!/usr/bin/env node
import { exec } from 'node:child_process'
import { promisify } from 'node:util'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

const execAsync = promisify(exec)
const __dirname = path.dirname(fileURLToPath(import.meta.url))
const rootDir = path.resolve(__dirname, '..')

console.log('🚀 [SEKOCI MULTI-AGENT] Memulai Verifikasi Paralel Microservices & Web...\n')

const tasks = [
  {
    name: 'Frontend Web Typecheck (vue-tsc)',
    cwd: path.join(rootDir, 'web'),
    cmd: 'npx vue-tsc -b',
  },
  {
    name: 'Frontend Production Build (vite)',
    cwd: path.join(rootDir, 'web'),
    cmd: 'npm run build',
  },
  {
    name: 'Identity Service Tests',
    cwd: path.join(rootDir, 'services/identity'),
    cmd: 'go test ./...',
  },
  {
    name: 'Academic Service Tests',
    cwd: path.join(rootDir, 'services/academic'),
    cmd: 'go test ./...',
  },
  {
    name: 'Attendance Service Tests',
    cwd: path.join(rootDir, 'services/attendance'),
    cmd: 'go test ./...',
  },
  {
    name: 'Exam CBT Service Tests',
    cwd: path.join(rootDir, 'services/exam'),
    cmd: 'go test ./...',
  },
  {
    name: 'Report E-Rapor Service Tests',
    cwd: path.join(rootDir, 'services/report'),
    cmd: 'go test ./...',
  },
  {
    name: 'PPDB Admission Service Tests',
    cwd: path.join(rootDir, 'services/admission'),
    cmd: 'go test ./...',
  },
  {
    name: 'Billing & Keuangan Service Tests',
    cwd: path.join(rootDir, 'services/billing'),
    cmd: 'go test ./...',
  },
  {
    name: 'Core Service Tests',
    cwd: path.join(rootDir, 'services/core'),
    cmd: 'go test ./internal/model/... ./internal/handler/...',
  },
  {
    name: 'HR Service Tests',
    cwd: path.join(rootDir, 'services/hr'),
    cmd: 'go test ./...',
  },
]

const startTime = Date.now()

const results = await Promise.all(
  tasks.map(async (task) => {
    const t0 = Date.now()
    try {
      await execAsync(task.cmd, { cwd: task.cwd })
      const dur = ((Date.now() - t0) / 1000).toFixed(2)
      return { ...task, ok: true, dur }
    } catch (err) {
      const dur = ((Date.now() - t0) / 1000).toFixed(2)
      return { ...task, ok: false, dur, error: err.message }
    }
  })
)

const totalDuration = ((Date.now() - startTime) / 1000).toFixed(2)
console.log('='.repeat(70))
console.log('📊 HASIL VERIFIKASI MULTI-AGENT PARALEL')
console.log('='.repeat(70))

let allSuccess = true
for (const res of results) {
  if (res.ok) {
    console.log(`✅ [PASS] (${res.dur}s) ${res.name}`)
  } else {
    allSuccess = false
    console.log(`❌ [FAIL] (${res.dur}s) ${res.name}`)
    console.log(`   Error: ${res.error?.split('\n')[0] || 'Unknown error'}`)
  }
}

console.log('='.repeat(70))
console.log(`Waktu Total Eksekusi Paralel: ${totalDuration}s`)
if (allSuccess) {
  console.log('🎉 SEMUA LAYANAN DAN KOMPONEN 100% SIAP & LOLOS UJI!\n')
  process.exit(0)
} else {
  console.log('⚠️ Terdapat komponen yang gagal. Periksa log di atas.\n')
  process.exit(1)
}
