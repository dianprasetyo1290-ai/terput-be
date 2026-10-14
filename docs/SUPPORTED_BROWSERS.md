# Browser dan Ukuran Layar yang Didukung (NFR-COMP-01, OQ-11)

Berlaku untuk SPA SEKOCI (`web/`). Perbarui tabel ini setiap rilis bila
dependensi build berubah, lalu cantumkan di catatan rilis.

## Browser

| Browser | Versi minimum | Catatan |
|---|---|---|
| Google Chrome (desktop & Android) | 111 | |
| Microsoft Edge | 111 | |
| Mozilla Firefox | 128 | |
| Safari (macOS) | 16.4 | |
| Safari / browser apa pun di iOS & iPadOS | iOS 16.4 | semua browser iOS memakai mesin WebKit |
| Samsung Internet | 22 | berbasis Chromium 111 |

Tidak didukung: Internet Explorer, Edge Legacy (EdgeHTML), dan versi di bawah
tabel di atas. JavaScript dan cookie/localStorage wajib aktif.

Dasar angka:
- Vite 8 (`build.target` bawaan `baseline-widely-available`): Chrome/Edge 111,
  Firefox 114, Safari/iOS 16.4.
- Tailwind CSS v4 (cascade layers, `@property`, `color-mix()`): Chrome 111,
  Firefox 128, Safari 16.4. Batas tertinggi dari keduanya yang dipakai.

## Ukuran layar

Diuji tanpa scroll horizontal pada lebar 375, 768, 1024, dan 1440 px
(GEMINI.md §5). Lebar minimum yang didukung 360 px. Tabel lebar di-scroll di
dalam wrapper-nya sendiri.

## Cara verifikasi per rilis

1. Cek versi `vite` dan `tailwindcss` di `web/package-lock.json`; bila mayor
   berubah, sesuaikan tabel dengan dokumentasi resminya.
2. Jalankan Playwright (`web/playwright.config.ts`) untuk Chromium, Firefox,
   dan WebKit pada keempat lebar di atas.
