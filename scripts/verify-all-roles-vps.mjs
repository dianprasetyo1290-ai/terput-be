import { chromium } from '@playwright/test'

const BASE_URL = 'http://43.157.207.147/teratai'
const PASSWORD = (process.env.DEMO_PASSWORD ?? '')

const ROLES = [
  { role: 'superadmin', username: 'superadmin@terataiputih.sch.id' },
  { role: 'admin', username: 'admin@terataiputih.sch.id' },
  { role: 'kepala_sekolah', username: 'kepala.sekolah@terataiputih.sch.id' },
  { role: 'kepala_yayasan', username: 'kepala.yayasan@terataiputih.sch.id' },
  { role: 'hr_admin', username: 'hr.admin@terataiputih.sch.id' },
  { role: 'hr_manager', username: 'hr.manager@terataiputih.sch.id' },
  { role: 'payroll_officer', username: 'payroll.officer@terataiputih.sch.id' },
  { role: 'recruiter', username: 'recruiter@terataiputih.sch.id' },
  { role: 'training_admin', username: 'training.admin@terataiputih.sch.id' },
  { role: 'auditor', username: 'auditor@terataiputih.sch.id' },
  { role: 'finance_manager', username: 'finance.manager@terataiputih.sch.id' },
  { role: 'pegawai', username: 'pegawai@terataiputih.sch.id' },
  { role: 'guru', username: 'guru@terataiputih.sch.id' },
  { role: 'walikelas', username: 'walikelas@terataiputih.sch.id' },
  { role: 'siswa', username: 'siswa@terataiputih.sch.id' },
  { role: 'orang_tua', username: 'orangtua@terataiputih.sch.id' },
  { role: 'pengawas', username: 'pengawas@terataiputih.sch.id' },
]

async function verifyAllRoles() {
  console.log(`\n======================================================`)
  console.log(`AUDIT & VERIFIKASI 17 ROLE LIVE VPS: ${BASE_URL}`)
  console.log(`======================================================\n`)

  const browser = await chromium.launch({ headless: true })
  const results = []

  for (const { role, username } of ROLES) {
    const context = await browser.newContext()
    const page = await context.newPage()
    let status = 'PASS'
    let notes = ''
    let currentUrl = ''

    try {
      await page.goto(`${BASE_URL}/login`, { waitUntil: 'networkidle', timeout: 15000 })

      // Fill login form
      await page.fill('input[type="text"], input[type="email"], input[name="username"], input#username, input:not([type="password"])', username)
      await page.fill('input[type="password"]', PASSWORD)
      await page.click('button[type="submit"]')

      // Wait for navigation after login
      await page.waitForURL((url) => !url.pathname.endsWith('/login'), { timeout: 10000 })
      await page.waitForLoadState('networkidle')
      await page.waitForTimeout(1000)

      currentUrl = page.url()
      const content = await page.content()

      // Critical Checks
      const hasBelumTersedia = content.includes('belum tersedia') || content.includes('Dashboard untuk role')
      const hasNotFound = content.includes('Halaman tidak ditemukan') || content.includes('Page Not Found') || content.includes('404')
      const hasErrorGagal = content.includes('Gagal memuat data')

      if (hasBelumTersedia) {
        status = 'FAIL'
        notes = 'Menampilkan pesan "belum tersedia"'
      } else if (hasNotFound) {
        status = 'FAIL'
        notes = 'Halaman tidak ditemukan (404)'
      } else if (hasErrorGagal) {
        status = 'FAIL'
        notes = 'Error gagal memuat data'
      } else {
        notes = `Mendarat di: ${currentUrl.replace(BASE_URL, '') || '/'}`
      }

      // Take screenshot for hr_admin, superadmin, siswa, pegawai
      if (['hr_admin', 'superadmin', 'siswa', 'pegawai'].includes(role)) {
        await page.screenshot({ path: `/tmp/screenshot-${role}.png`, fullPage: false })
      }
    } catch (err) {
      status = 'ERROR'
      notes = err.message
    } finally {
      await context.close()
    }

    results.push({ role, username, status, currentUrl, notes })
    console.log(`[${status}] Role: ${role.padEnd(16)} | ${notes}`)
  }

  await browser.close()

  console.log(`\n======================================================`)
  const passed = results.filter((r) => r.status === 'PASS').length
  console.log(`TOTAL HASIL: ${passed} / ${ROLES.length} ROLE LULUS`)
  console.log(`======================================================\n`)

  if (passed !== ROLES.length) {
    process.exit(1)
  }
}

verifyAllRoles().catch((err) => {
  console.error('Fatal test error:', err)
  process.exit(1)
})
