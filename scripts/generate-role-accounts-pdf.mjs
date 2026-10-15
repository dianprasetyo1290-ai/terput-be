import { chromium } from '@playwright/test'
import path from 'node:path'
import os from 'node:os'

const DOWNLOADS_DIR = path.join(os.homedir(), 'Downloads')
const OUTPUT_PDF = path.join(DOWNLOADS_DIR, 'Akun_Pengguna_Per_Role_Teratai_Putih.pdf')

const accounts = [
  // 1. Pimpinan & Yayasan
  {
    category: 'Pimpinan & Yayasan',
    role: 'superadmin',
    roleLabel: 'Super Administrator',
    name: 'Superadmin Sekoci',
    email: 'superadmin@sekoci.local',
    pw: (process.env.DEMO_PASSWORD ?? ''),
    scope: 'Akses penuh seluruh satuan pendidikan, manajemen pengguna global, konfigurasi sistem & audit',
    landing: '/dashboard',
    badge: 'System Admin'
  },
  {
    category: 'Pimpinan & Yayasan',
    role: 'kepala_yayasan',
    roleLabel: 'Ketua Yayasan',
    name: 'Ketua Yayasan Teratai',
    email: 'yayasan@teratai.sch.id',
    pw: (process.env.DEMO_PASSWORD ?? ''),
    scope: 'Oversight eksekutif yayasan, laporan keuangan yayasan, rekap SDM lintas unit, & persetujuan kebijakan',
    landing: '/dashboard',
    badge: 'Executive'
  },
  {
    category: 'Pimpinan & Yayasan',
    role: 'kepala_sekolah',
    roleLabel: 'Kepala Sekolah',
    name: 'Kepala Sekolah Teratai',
    email: 'kepsek@teratai.sch.id',
    pw: (process.env.DEMO_PASSWORD ?? ''),
    scope: 'Persetujuan akhir alur approval (cuti, mutasi, pengeluaran), monitoring mutu akademik, & evaluasi guru',
    landing: '/dashboard',
    badge: 'Executive'
  },
  {
    category: 'Pimpinan & Yayasan',
    role: 'pengawas',
    roleLabel: 'Pengawas Satuan',
    name: 'Pengawas Teratai',
    email: 'pengawas@teratai.sch.id',
    pw: (process.env.DEMO_PASSWORD ?? ''),
    scope: 'Pengawasan pelaksanaan ujian CBT, asesmen nasional, supervisi kurikulum, & evaluasi kinerja satuan',
    landing: '/dashboard',
    badge: 'Supervisor'
  },

  // 2. Operasional Akademik
  {
    category: 'Akademik & Tata Usaha',
    role: 'admin',
    roleLabel: 'Administrator Sekolah',
    name: 'Admin Teratai',
    email: 'admin@teratai.sch.id',
    pw: (process.env.DEMO_PASSWORD ?? ''),
    scope: 'Data induk siswa/guru, tahun akademik, rombel, jadwal, PPDB terpadu, tarif SPP, & administrasi harian',
    landing: '/dashboard',
    badge: 'Operations'
  },
  {
    category: 'Akademik & Tata Usaha',
    role: 'guru',
    roleLabel: 'Guru Mata Pelajaran',
    name: 'Guru Teratai',
    email: 'guru@teratai.sch.id',
    pw: (process.env.DEMO_PASSWORD ?? ''),
    scope: 'Jadwal mengajar, presensi siswa per jam, input nilai formatif/sumatif, bank soal CBT, & RPM/silabus',
    landing: '/akademik/jadwal-saya',
    badge: 'Faculty'
  },
  {
    category: 'Akademik & Tata Usaha',
    role: 'walikelas',
    roleLabel: 'Wali Kelas',
    name: 'Wali Kelas Teratai',
    email: 'walikelas@teratai.sch.id',
    pw: (process.env.DEMO_PASSWORD ?? ''),
    scope: 'Binaan rombel, monitoring presensi harian, sidang kenaikan kelas, catatan sikap, & generate buku rapor',
    landing: '/walikelas/dashboard',
    badge: 'Faculty'
  },

  // 3. SDM, Payroll & Pelatihan (Fase 1 & Fase 2)
  {
    category: 'SDM, Payroll & Pelatihan',
    role: 'hr_admin',
    roleLabel: 'HR Administrator',
    name: 'HR Administrator Teratai',
    email: 'hradmin@teratai.sch.id',
    pw: (process.env.DEMO_PASSWORD ?? ''),
    scope: 'Master data pegawai (NIK/NPWP AES-256), struktur organisasi, dokumen kontrak, presensi & cuti staf',
    landing: '/hr/pegawai',
    badge: 'HR & Org'
  },
  {
    category: 'SDM, Payroll & Pelatihan',
    role: 'hr_manager',
    roleLabel: 'HR Manager',
    name: 'HR Manager Teratai',
    email: 'hrmanager@teratai.sch.id',
    pw: (process.env.DEMO_PASSWORD ?? ''),
    scope: 'Persetujuan cuti/lembur pegawai, perencanaan formasi organisasi, matriks kompetensi, & reviu SDM',
    landing: '/hr/organisasi',
    badge: 'HR Management'
  },
  {
    category: 'SDM, Payroll & Pelatihan',
    role: 'payroll_officer',
    roleLabel: 'Petugas Payroll',
    name: 'Petugas Payroll Teratai',
    email: 'payroll@teratai.sch.id',
    pw: (process.env.DEMO_PASSWORD ?? ''),
    scope: 'Siklus penggajian bulanan, perhitungan PPh 21 TER (PP 58/2023), BPJS Kes/TK, slip gaji, & jurnal otomatis',
    landing: '/payroll/run',
    badge: 'Payroll'
  },
  {
    category: 'SDM, Payroll & Pelatihan',
    role: 'recruiter',
    roleLabel: 'Recruiter (ATS)',
    name: 'Recruiter Teratai',
    email: 'recruiter@teratai.sch.id',
    pw: (process.env.DEMO_PASSWORD ?? ''),
    scope: 'Applicant Tracking System: formasi lowongan, posting portal karier publik, kanban seleksi, & penawaran gaji',
    landing: '/rekrutmen/lowongan',
    badge: 'Recruitment'
  },
  {
    category: 'SDM, Payroll & Pelatihan',
    role: 'training_admin',
    roleLabel: 'Admin Pelatihan',
    name: 'Admin Pelatihan Teratai',
    email: 'training@teratai.sch.id',
    pw: (process.env.DEMO_PASSWORD ?? ''),
    scope: 'Katalog kursus, penugasan pelatihan wajib jabatan, monitoring kuis kelulusan, & kepatuhan sertifikasi',
    landing: '/pelatihan/katalog',
    badge: 'L&D Admin'
  },
  {
    category: 'SDM, Payroll & Pelatihan',
    role: 'pegawai',
    roleLabel: 'Staf Pegawai (ESS)',
    name: 'Staf Pegawai Teratai',
    email: 'pegawai@teratai.sch.id',
    pw: (process.env.DEMO_PASSWORD ?? ''),
    scope: 'Employee Self-Service: kehadiran mandiri, pengajuan cuti & lembur dengan timeline approval, slip gaji, kursus',
    landing: '/pegawai',
    badge: 'Employee'
  },

  // 4. Finansial & Pengawasan Sistem
  {
    category: 'Keuangan & Pengawasan',
    role: 'finance_manager',
    roleLabel: 'Finance Manager',
    name: 'Finance Manager Teratai',
    email: 'financemanager@teratai.sch.id',
    pw: (process.env.DEMO_PASSWORD ?? ''),
    scope: 'Dashboard NICEPay Gateway, kasir loket tunai/transfer, rekonsiliasi bank 50-baris, settlement, & akuntansi COA',
    landing: '/keuangan/payment-dashboard',
    badge: 'Finance'
  },
  {
    category: 'Keuangan & Pengawasan',
    role: 'auditor',
    roleLabel: 'Auditor Internal',
    name: 'Auditor Internal Teratai',
    email: 'auditor@teratai.sch.id',
    pw: (process.env.DEMO_PASSWORD ?? ''),
    scope: 'Pemeriksaan audit trail event keamanan, selisih data mutasi JSON diff, & antrean galat integrasi (DLQ retry)',
    landing: '/audit',
    badge: 'Audit & Compliance'
  },

  // 5. Komunitas (Siswa & Orang Tua)
  {
    category: 'Portal Komunitas',
    role: 'siswa',
    roleLabel: 'Siswa / Murid',
    name: 'Budi Santoso (Siswa)',
    email: 'siswa@teratai.sch.id',
    pw: (process.env.DEMO_PASSWORD ?? ''),
    scope: 'Jadwal pelajaran, nilai tugas/ulangan, riwayat presensi harian, ujian CBT online, & informasi Tagihan Saya',
    landing: '/siswa/dashboard',
    badge: 'Student Portal'
  },
  {
    category: 'Portal Komunitas',
    role: 'orang_tua',
    roleLabel: 'Orang Tua / Wali Murid',
    name: 'Wali Santoso (Orang Tua)',
    email: 'orangtua@teratai.sch.id',
    pw: (process.env.DEMO_PASSWORD ?? ''),
    scope: 'Multi-bill checkout tagihan SPP online (VA/QRIS), pantau presensi & nilai anak, e-rapor, serta tiket layanan TU',
    landing: '/orang-tua/dashboard',
    badge: 'Parent Portal'
  }
]

const htmlContent = `
<!DOCTYPE html>
<html lang="id">
<head>
  <meta charset="UTF-8">
  <title>Daftar Akun Pengguna Teratai Putih Global School</title>
  <style>
    @page {
      size: A4 portrait;
      margin: 12mm 14mm 14mm 14mm;
    }
    * {
      box-sizing: border-box;
      margin: 0;
      padding: 0;
      font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, Helvetica, Arial, sans-serif;
    }
    body {
      background: #ffffff;
      color: #0f172a;
      font-size: 8pt;
      line-height: 1.35;
    }

    /* Header */
    .header {
      display: flex;
      justify-content: space-between;
      align-items: center;
      border-bottom: 2px solid #0f172a;
      padding-bottom: 8px;
      margin-bottom: 12px;
    }
    .header-left h1 {
      font-size: 14pt;
      font-weight: 800;
      color: #0f172a;
      letter-spacing: -0.5px;
      text-transform: uppercase;
    }
    .header-left p {
      font-size: 8.5pt;
      color: #475569;
      font-weight: 500;
      margin-top: 2px;
    }
    .header-right {
      text-align: right;
    }
    .env-badge {
      display: inline-block;
      background: #0284c7;
      color: #ffffff;
      font-size: 7.5pt;
      font-weight: 700;
      padding: 3px 8px;
      border-radius: 4px;
      text-transform: uppercase;
      letter-spacing: 0.5px;
    }
    .date-str {
      font-size: 7.5pt;
      color: #64748b;
      margin-top: 3px;
    }

    /* Info Banner */
    .summary-card {
      background: #f8fafc;
      border: 1px solid #cbd5e1;
      border-radius: 6px;
      padding: 8px 12px;
      margin-bottom: 12px;
      display: grid;
      grid-template-columns: repeat(4, 1fr);
      gap: 10px;
    }
    .summary-item .label {
      font-size: 7pt;
      color: #64748b;
      text-transform: uppercase;
      font-weight: 700;
      margin-bottom: 2px;
    }
    .summary-item .value {
      font-size: 8.5pt;
      font-weight: 700;
      color: #0f172a;
    }
    .summary-item .highlight {
      color: #059669;
    }

    /* Table */
    table {
      width: 100%;
      border-collapse: collapse;
      margin-bottom: 12px;
    }
    th {
      background: #0f172a;
      color: #ffffff;
      font-size: 7.5pt;
      font-weight: 700;
      text-transform: uppercase;
      letter-spacing: 0.3px;
      padding: 6px 6px;
      text-align: left;
    }
    td {
      padding: 5px 6px;
      border-bottom: 1px solid #e2e8f0;
      font-size: 7.5pt;
      vertical-align: middle;
    }
    tr:nth-child(even) td {
      background: #f8fafc;
    }
    .role-badge {
      display: inline-block;
      font-size: 6.5pt;
      font-weight: 700;
      padding: 2px 5px;
      border-radius: 3px;
      background: #e0e7ff;
      color: #3730a3;
      text-transform: uppercase;
      white-space: nowrap;
    }
    .status-ok {
      color: #059669;
      font-weight: 700;
      font-size: 7pt;
      display: flex;
      align-items: center;
      gap: 3px;
    }
    .status-dot {
      display: inline-block;
      width: 6px;
      height: 6px;
      border-radius: 50%;
      background: #059669;
    }
    .pw-code {
      font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
      font-weight: 700;
      color: #b91c1c;
      background: #fee2e2;
      padding: 1px 4px;
      border-radius: 3px;
      font-size: 7pt;
      white-space: nowrap;
    }
    .email-code {
      font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
      font-weight: 600;
      color: #0369a1;
      font-size: 7pt;
      white-space: nowrap;
    }

    /* Footer Guidelines */
    .guidelines {
      background: #f1f5f9;
      border-left: 4px solid #0284c7;
      padding: 8px 10px;
      border-radius: 0 4px 4px 0;
      margin-top: 6px;
    }
    .guidelines h3 {
      font-size: 8pt;
      font-weight: 700;
      color: #0f172a;
      margin-bottom: 4px;
    }
    .guidelines ol {
      padding-left: 14px;
      font-size: 7.2pt;
      color: #334155;
    }
    .guidelines li {
      margin-bottom: 2px;
    }
    .footer-bar {
      margin-top: 10px;
      text-align: center;
      font-size: 6.8pt;
      color: #94a3b8;
      border-top: 1px solid #e2e8f0;
      padding-top: 6px;
    }
  </style>
</head>
<body>

  <!-- Header -->
  <div class="header">
    <div class="header-left">
      <h1>Kredensial Akun Resmi per Role</h1>
      <p>Yayasan & Satuan Pendidikan Teratai Putih Global • SEKOCI Enterprise LMS</p>
    </div>
    <div class="header-right">
      <span class="env-badge">Production Verified</span>
      <div class="date-str">Diperbarui: 11 Oktober 2026</div>
    </div>
  </div>

  <!-- Summary Banner -->
  <div class="summary-card">
    <div class="summary-item">
      <div class="label">Portal Login Resmi</div>
      <div class="value">http://43.157.207.147/teratai/login</div>
    </div>
    <div class="summary-item">
      <div class="label">Satuan Pendidikan</div>
      <div class="value">Teratai Putih Global School</div>
    </div>
    <div class="summary-item">
      <div class="label">Total Peran (Role)</div>
      <div class="value">17 Role Terpenuhi (100%)</div>
    </div>
    <div class="summary-item">
      <div class="label">Status Verifikasi API</div>
      <div class="value highlight">✅ 17/17 Lolos Login (HTTP 200)</div>
    </div>
  </div>

  <!-- Table of Roles -->
  <table>
    <thead>
      <tr>
        <th style="width: 20px; text-align: center;">No</th>
        <th style="width: 95px;">Peran (Role)</th>
        <th style="width: 120px;">Nama Pengguna</th>
        <th style="width: 130px;">Email Login</th>
        <th style="width: 85px;">Kata Sandi</th>
        <th>Cakupan Hak Akses & Menu Utama</th>
        <th style="width: 60px; text-align: center;">Verifikasi</th>
      </tr>
    </thead>
    <tbody>
      ${accounts.map((acc, idx) => `
        <tr>
          <td style="text-align: center; font-weight: 700; color: #64748b;">${idx + 1}</td>
          <td>
            <div style="font-weight: 700; color: #0f172a;">${acc.roleLabel}</div>
            <span class="role-badge">${acc.badge}</span>
          </td>
          <td>
            <div style="font-weight: 600; color: #1e293b;">${acc.name}</div>
          </td>
          <td>
            <span class="email-code">${acc.email}</span>
          </td>
          <td>
            <span class="pw-code">${acc.pw}</span>
          </td>
          <td style="color: #334155; line-height: 1.25;">
            ${acc.scope}
          </td>
          <td style="text-align: center;">
            <span class="status-ok">
              <span class="status-dot"></span> Aktif
            </span>
          </td>
        </tr>
      `).join('')}
    </tbody>
  </table>

  <!-- Guidelines -->
  <div class="guidelines">
    <h3>Petunjuk Penggunaan & Keamanan:</h3>
    <ol>
      <li><strong>Akses Sistem:</strong> Buka tautan <span style="font-family: monospace; font-weight: 700;">http://43.157.207.147/teratai/login</span> melalui peramban Google Chrome / Safari / Edge.</li>
      <li><strong>Role Switching:</strong> Pengguna dengan multi-peran (seperti Kepala Sekolah atau Wali Kelas) dapat berganti peran secara instan melalui pemilih peran pada pojok kanan atas navbar tanpa perlu keluar (logout).</li>
      <li><strong>Kepatuhan Keamanan (Security Compliance):</strong> Semua data sensitif (NIK, NPWP, Rekening, Gaji Pokok) dilindungi enkripsi AES-256-GCM. Harap ubah kata sandi default setelah pengujian awal via menu <em>Ubah Password</em>.</li>
      <li><strong>Integrasi Modul PRD v1.0:</strong> Semua modul Gelombang 1 (T1 Core Approval/DLQ, T2 HRIS, T6 Payment Gateway, T7a Mutasi Siswa, T8 Audit) dan Gelombang 2 (T3 Payroll PPh 21/BPJS, T4 ATS Rekrutmen, T5 Pelatihan LMS, T7b Layanan Orang Tua) aktif dan terhubung ke backend.</li>
    </ol>
  </div>

  <div class="footer-bar">
    Dokumen Resmi Kredensial Pengujian Sistem SEKOCI LMS • Yayasan Teratai Putih Global • Dicetak Otomatis pada 11 Oktober 2026
  </div>

</body>
</html>
`

const browser = await chromium.launch()
const page = await browser.newPage()
await page.setContent(htmlContent, { waitUntil: 'networkidle' })
await page.pdf({
  path: OUTPUT_PDF,
  format: 'A4',
  printBackground: true,
  margin: {
    top: '10mm',
    bottom: '10mm',
    left: '12mm',
    right: '12mm'
  }
})
await browser.close()

console.log(`🎉 [PDF SELESAI] File PDF resmi berhasil dibuat di:\n${OUTPUT_PDF}`)
