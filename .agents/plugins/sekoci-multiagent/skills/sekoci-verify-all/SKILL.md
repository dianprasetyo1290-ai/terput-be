---
name: sekoci-verify-all
description: "Run comprehensive verification across all SEKOCI LMS microservices and frontend web (vue-tsc, vite build, and go tests). Use before committing or marking any large multi-agent task as done."
---

# SEKOCI Full Verification Procedure

Gunakan skill ini untuk memverifikasi integritas seluruh komponen sistem Sekoci secara paralel dan menyeluruh.

---

## Langkah 1: Verifikasi Frontend Web
Masuk ke direktori `web/` dan jalankan pemeriksaan tipe TypeScript serta build bundle:

```bash
cd /Users/tia-nurul/Documents/LMS/skci-teratai/web
npx vue-tsc -b
npm run build
```

Kriteria Sukses:
- `vue-tsc -b`: Exit code 0, 0 errors.
- `npm run build`: Berhasil mengompilasi semua chunks tanpa warning fatal.

---

## Langkah 2: Verifikasi Seluruh Microservices Go
Jalankan unit test pada masing-masing service backend:

```bash
# Identity
cd /Users/tia-nurul/Documents/LMS/skci-teratai/services/identity && go test ./...

# Academic
cd /Users/tia-nurul/Documents/LMS/skci-teratai/services/academic && go test ./...

# Attendance
cd /Users/tia-nurul/Documents/LMS/skci-teratai/services/attendance && go test ./...

# Exam
cd /Users/tia-nurul/Documents/LMS/skci-teratai/services/exam && go test ./...

# Report
cd /Users/tia-nurul/Documents/LMS/skci-teratai/services/report && go test ./...

# Admission (PPDB)
cd /Users/tia-nurul/Documents/LMS/skci-teratai/services/admission && go test ./...

# Billing
cd /Users/tia-nurul/Documents/LMS/skci-teratai/services/billing && go test ./...
```

Kriteria Sukses:
- Semua unit test paket menampilkan status `ok` atau `[no test files]`.
- Exit code 0.
