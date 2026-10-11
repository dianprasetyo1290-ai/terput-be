# Aturan Arsitektur & Kolaborasi Multi-Agent Sekoci

Dokumen ini adalah aturan tetap (*immutable rules*) yang wajib ditaati oleh seluruh agen (`sekoci_coordinator`, `sekoci_frontend_agent`, `sekoci_backend_agent`, `sekoci_accounting_agent`, `sekoci_qa_agent`).

---

## 1. Isolasi File & Pencegahan Konflik (Conflict Prevention)
- Jangan pernah mengizinkan dua agen mengedit file yang sama pada giliran kerja paralel yang sama.
- Agen Frontend hanya memodifikasi `web/src/`.
- Agen Backend hanya memodifikasi `services/<service>/`.
- Agen Accounting hanya memodifikasi `services/accounting/` dan `AccountingFrame.vue`.
- Agen QA hanya memodifikasi skrip pengujian di `web/e2e/` atau `web/e2e-teratai-*.mjs`.

## 2. Validasi Lokal Sebelum Deploy (Local Validation First)
- Dilarang keras melakukan deploy ke server atau mengubah konfigurasi produksi sebelum seluruh pemeriksaan lokal lolos.
- Pengujian lokal wajib mencakup:
  1. `web/`: `npx vue-tsc -b` (0 error) dan `npm run build` (build sukses).
  2. `services/*/`: `go test ./...` (seluruh service Go `PASS`).
  3. `e2e`: Test sweep atau login Playwright lolos tanpa gagal.

## 3. Eksekusi Mandiri & Kontinu (Autonomous Execution)
- Jalankan tugas hingga tuntas tanpa berhenti di tengah jalan untuk konfirmasi hal-hal yang sudah jelas dalam PRD atau konvensi kode.
- Jika ada pilihan arsitektur yang ambigu, gunakan tool klarifikasi terstruktur.

## 4. Standar Port & Layanan Docker
- `identity`: 8081
- `academic`: 8082
- `attendance`: 8083
- `exam`: 8084
- `evaluation`: 8085
- `report`: 8086
- `accounting`: 8087
- `admission`: 8088
- `notification`: 8089
- `billing`: 8090
- `core`: 8091 (baru, PRD v1.0 T1)
- `hr`: 8092 (baru, PRD v1.0 T2–T5)
- `web (SPA)`: 5173 (dev) / 8080 (nginx)
- `postgres`: 5432
