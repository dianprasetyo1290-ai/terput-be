# Handoff Task T7b: Permintaan Layanan Orang Tua (Menu 81)

## 1. Ringkasan Eksekutif
Task T7b mengimplementasikan **Menu 81 (Permintaan Layanan Orang Tua)** pada service `core` dan frontend portal orang tua serta meja kerja staf handler:
1. **Service Core (Backend):**
   - Migrasi `0002_service_requests.up.sql`: tabel `service_request_categories`, `service_requests`, dan `service_request_events`.
   - Model & fungsi transisi status tervalidasi: `submitted` → `accepted` / `rejected`, `accepted` → `in_progress` / `rejected`, `in_progress` → `done` / `rejected`. Penolakan wajib menyertakan alasan (`note`).
   - Endpoint CRUD kategori untuk admin (`/service-request-categories`), pengajuan orang tua (`/parent/service-requests`), serta antrean kerja handler (`/service-requests`).
   - Fitur dokumen hasil (`result_attachments`) untuk mengunggah surat keterangan atau berkas administrasi yang dapat diunduh pemohon.
2. **Frontend (Stitch Foundation Design & SEKOCI_DESIGN):**
   - `views/parent/ServiceRequestsView.vue`: Daftar riwayat pengajuan orang tua dengan StatCards dan status filter tabs.
   - `views/parent/ServiceRequestFormView.vue`: Formulir pengajuan layanan dengan SLA badge dan validasi siswa.
   - `views/parent/ServiceRequestDetailView.vue`: Rincian permohonan, status tracking timeline, dan tautan unduh berkas hasil.
   - `views/core/ServiceRequestInboxView.vue`: Meja kerja antrean layanan sekolah sesuai peran penanggung jawab.
   - `views/core/ServiceRequestWorkView.vue`: Meja kerja tindak lanjut staf dengan alur terima/tolak/proses/selesai serta modal alasan penolakan dan dokumen hasil.
   - `views/core/ServiceRequestCategoriesView.vue`: Konfigurasi kategori layanan administrasi sekolah untuk admin.

---

## 2. Bukti Verifikasi
- **Backend Go Vet & Tests:**
  `go vet ./...` -> PASS
  `go test ./internal/model/... ./internal/handler/...` -> PASS (ok 0.483s)
- **Frontend Typecheck & Production Build:**
  `vue-tsc -b && vite build` -> PASS (built in 3.06s)

SELESAI T7b
