# Temuan awal sebelum CEK-W2 (belum diperbaiki, belum diverifikasi build)

Ditemukan saat menyusun file task T9–T26 (2026-10-12). Diperbaiki di langkah CEK-W2 setelah toolchain tersedia.

## 1. Approval cuti/lembur hr tidak pernah diterapkan (T2 ↔ T1) — berat
Alur: `hr/internal/handler/ess.go:300` dan `:458` → `client.CreateApprovalRequest` → core → callback.
- **Callback dikirim ke service yang salah.** `hr/internal/client/client.go:214` mengisi `CallbackURL: c.CoreURL + "/internal/approval-callbacks"`. Route itu milik hr (`hr/internal/handler/router.go:40`), jadi seharusnya `HR_PUBLIC_INTERNAL_URL` (`config.HRPublicInternalURL`).
- **Bentuk payload tidak cocok.** core mengirim `{approval_id, source_id, request_type, decision, decided_by, comment, decided_at}` (`core/internal/jobs/effects.go:18`), sedangkan hr membaca `resource_type`/`resource_id`/`notes` (`hr/internal/handler/callbacks.go:11`) dan menolak 400 karena keduanya kosong.
- **`subject_user_id` diisi user atasan.** `CreateApprovalRequest(..., *sup.UserID, ...)` meneruskan atasan sebagai `SubjectUserID`, padahal core memakai subjek untuk mencari atasan langsung (T1 §4.2). Akibatnya penyetuju menjadi atasan dari atasan. Subjek seharusnya user pemohon.
- Error `CreateApprovalRequest` dibuang (`appID, _ :=`), jadi kegagalan tidak terlihat dan tidak masuk exception queue.

Dampak: cuti/lembur tetap `pending` di hr walaupun sudah disetujui di Approval Center.

## 2. `CreateParentServiceRequest` (T7b, core) — sedang
Belum memverifikasi bahwa orang tua adalah wali siswa yang dipilih, dan belum mengirim notifikasi ke staf. Dicatat oleh penyusun T11.

## 3. Callback koreksi pembayaran billing memakai `localhost` (T6) — berat
`services/billing/internal/handler/gateway_corrections.go:72` mengisi `CallbackURL: http://localhost:<port>/internal/corrections/<id>/callback`. Di docker, core tidak bisa menjangkau billing lewat `localhost`, jadi keputusan approval koreksi pembayaran tidak pernah sampai. Seharusnya memakai URL internal billing dari config (env), seperti `HR_PUBLIC_INTERNAL_URL` di hr. Ditemukan oleh penyusun T23.

## 4. Jurnal gateway T6: akun hardcode + reference harian bentrok — berat
Webhook gateway memakai kode akun literal (`"1101-01"` dst., melanggar GEMINI.md §3) dan memposting jurnal per transaksi dengan reference harian yang sama, sehingga transaksi kedua dst. di hari yang sama dianggap duplikat oleh accounting dan tidak dijurnal. Outbox `gateway_journal_postings` dan `GATEWAY_JOB_INTERVAL_SECONDS` belum dipakai. Perbaikan dijadwalkan di T14 (hapus hardcode) + T12 (worker outbox). Ditemukan oleh penyusun T12/T14.
