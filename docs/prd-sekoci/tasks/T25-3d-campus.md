# T25: 3D Campus di service `ops` + web: peta kampus 3D, peta kantin, status ruang khusus, facility digital twin

**Menu PRD:** 155, 156, 157, 158 (BR-3D-01, NFR-ADV-03, OQ-39, OQ-42) · **Branch:** `feat/T25-campus3d` · **Service:** `ops` (paket `internal/campus3d`) + web · **Migrasi:** `services/ops/migrations/0060_campus3d.*` (nomor 0060–0069 milik T25)
**Prasyarat:** gelombang 4 di-merge: **T9** (master ruang + fungsi baca `facility`, peran `facility_officer`/`asset_officer`/`canteen_admin`), **T12** (tenant & menu kantin di billing). T16 (aset) ada di gelombang 5 yang sama: kode sesuai kontrak §4 dan uji dengan fake. **Feature key:** `campus3d` (identity menolak menyalakannya bila `facility` mati, T9 §2b).

**BR-3D-01 mengikat:** T25 **tidak menyimpan salinan data operasional**. Tabel `campus3d_*` hanya berisi file model, nama node mesh, dan **ID rujukan** (`building_id`, `floor_id`, `room_id`, `tenant_id`). Nama ruang, kapasitas, status, pemesanan, perintah kerja, aset, nama tenant, status buka, menu, dan harga selalu dibaca langsung dari pemiliknya saat request. Tidak ada cache sisi server; respons data operasional memakai `Cache-Control: no-store`.

## 1. Donor / acuan kode
Tidak ada donor. Pola:
- Paket domain & mount: `services/ops/internal/facility` (T9) → `internal/campus3d/{handler,model,store,sources}.go`, `requireFeature("campus3d")`.
- Unggah & sajikan file privat: `services/ops/internal/storage` (disalin T9 dari billing) + pola unduh ber-auth `services/billing` `AdminPaymentProof` (stream, mime dari DB, bukan dari nama file).
- Client internal ke billing: salin pola `services/billing/internal/client/client.go` ke `ops/internal/client` (tambah method, jangan impor lintas modul).
- Frontend: tiga state + `components/ui/*`. **Dependensi baru:** `three` (render glTF/GLB lewat `GLTFLoader`) karena tidak ada renderer 3D di repo; versi dikunci, di-import dinamis hanya di halaman 3D supaya bundle utama tidak bertambah. Tulis alasannya di handoff.

## 2. Migrasi `0060_campus3d.up.sql`
Semua tabel: `id uuid PK`, `school_id uuid NOT NULL`, `created_at/updated_at`.
```
campus3d_models(id, school_id, building_id uuid NOT NULL,            -- rujukan buildings T9
  version int NOT NULL, object_key text NOT NULL, mime CHECK IN('model/gltf-binary'), size_bytes bigint, sha256 text,
  node_names text[] NOT NULL,                                       -- hasil parse GLB saat unggah (struktur model, bukan data operasional)
  status CHECK IN('draft','published','archived'), uploaded_by uuid, published_by NULL, published_at NULL,
  UNIQUE(building_id, version))
UNIQUE INDEX campus3d_one_published ON campus3d_models(building_id) WHERE status='published'
campus3d_floor_nodes(id, school_id, model_id FK, floor_id uuid NOT NULL, node_name text NOT NULL,
  UNIQUE(model_id, floor_id), UNIQUE(model_id, node_name))
campus3d_room_nodes(id, school_id, model_id FK, room_id uuid NOT NULL, node_name text NOT NULL,
  UNIQUE(model_id, room_id), UNIQUE(model_id, node_name))
campus3d_tenant_spots(id, school_id, tenant_id uuid NOT NULL, room_id uuid NOT NULL, UNIQUE(school_id, tenant_id))   -- lokasi tenant = ruang kantin T9
```
Satu model GLB per gedung (OQ-39: unggah glTF/GLB per gedung + pemetaan manual). Tidak ada kolom nama/kapasitas/harga/status operasional. Down: drop terbalik.

## 3. Fungsi murni (`internal/campus3d/model.go`) + vektor
| Fungsi | Vektor |
|---|---|
| `ParseGLBNodeNames(r io.Reader, max int64) ([]string, error)` (cek magic `glTF` v2, chunk JSON, ambil `nodes[].name`) | GLB contoh 3 node → 3 nama; file bukan GLB / versi 1 / melebihi batas → error "File harus GLB (glTF 2.0)" |
| `ValidateMappings(nodeNames, floors, rooms, mappings) []Issue` | node tidak ada di model → error; ruang beda sekolah/terhapus → error; satu node dipakai dua ruang → error; ruang aktif yang belum dipetakan → peringatan (boleh publish) |
| `FocusTarget(mappings, roomID) (Target{building_id, floor_id, node_name}, ok)` | **AC-155-01** "Lab Kimia" → gedung & lantai lab kimia + node-nya; ruang belum dipetakan → `ok=false` ("Belum ada di model 3D") |
| `SearchRooms(rooms, mappings, q) []Hit` (case & spasi dinormalisasi; awalan kata > mengandung; nama dan kode) | **AC-155-01** "lab kimia" → Lab Kimia urutan pertama, `mapped=true`; "kim" → tetap ketemu |
| `RoomStatusAt(busy []Busy, at time.Time) (status, current *Busy, next *Busy)` | **AC-157-01** dipesan 10.00–12.00, dibuka 10.30 → `terpakai` + pemesan; 12.00 → `kosong` + next; jadwal tetap 07–09 pukul 08 → `terpakai` (label jadwal) |
| `TodaySchedule(busy, day, loc)` (Asia/Jakarta) | urut waktu, hanya hari itu (FR-157-002) |
| `TwinOverlay(rooms, workOrders, assets) []RoomOverlay` (prioritas status tertinggi per ruang; jumlah aset per ruang) | **AC-158-01** ruang 8A dengan WO "Perbaikan AC" status `diproses` → disorot, label "Perbaikan AC – Diproses"; ruang tanpa WO aktif → tidak disorot; aset 3 buah di 8A → `asset_count=3` (FR-158-001) |
| `CanteenView(spots, tenants)` (gabung lokasi dengan data live tenant) | tenant `is_open=false` → label "Tutup"; tenant tanpa spot → tidak tampil di peta, tampil di daftar "Belum dipetakan" (khusus pengelola) |
Test skema BR-3D-01: test yang membaca `information_schema.columns` untuk tabel `campus3d_%` dan gagal bila ada kolom selain daftar di §2.

## 4. Integrasi (kontrak yang **dibaca**)
Antarmuka di `internal/campus3d/sources.go`; implementasi didaftarkan di `main.go`; nil-safe.
| Sumber | Kontrak | Pemilik | Dipakai untuk |
|---|---|---|---|
| Ruang & gedung | in-process `facility.ListBuildings(ctx, schoolID)`, `facility.ListRooms(ctx, schoolID, filter)`, `facility.GetRoom(ctx, schoolID, id)` (bentuk = `GET /internal/facility/buildings`, `/rooms`, `/rooms/{id}` di T9 §4.2) | T9 | FR-155-001/002/003, validasi pemetaan |
| Pemesanan & jadwal tetap | in-process `facility.BusyIntervals(ctx, schoolID, roomIDs, from, to)` (= `GET /internal/facility/availability`) | T9 | FR-157-001/002, AC-157-01 |
| Perintah kerja aktif | in-process `facility.ActiveWorkOrders(ctx, schoolID, roomIDs)` (= `GET /internal/facility/work-orders?status=active`) | T9 | FR-158-002, AC-158-01 |
| Aset per ruang | `type AssetSource interface { ListByRooms(ctx, schoolID string, roomIDs []string) ([]AssetSummary, error) }`, `AssetSummary{ID, Code, Name, Category, Condition, RoomID}`; didaftarkan T16 (kontrak final di **T16 §5**, bila berbeda ikuti T16 dan sesuaikan adapter saja) | T16 | FR-158-001. Nil → panel "Modul aset belum aktif", overlay WO tetap jalan |
| Tenant kantin | HTTP billing (X-Internal-Key): `GET /internal/canteen/tenants?school_id=` → `[{id, name, status:'active'\|'inactive', is_open bool, open_hours?}]` | T12 | FR-156-001 |
| Menu tenant | HTTP billing: `GET /internal/canteen/tenants/{id}/menu?school_id=` → `[{id, name, category?, price_idr, available bool}]` | T12 | FR-156-002, AC-156-01 |
Jika T12 sudah punya endpoint baca yang setara dengan path lain, pakai itu dan tulis pemetaannya di handoff. Jika belum ada, T25 menambahkan dua handler read-only itu di `services/billing/internal/canteen` (kode saja, tanpa migrasi, hanya tambah baris di router billing). Billing mati/timeout → 502 dengan pesan "Data kantin tidak tersedia", peta tetap tampil tanpa status tenant.
- Sensor (FR-158-003, OQ-42): **tidak dikerjakan** sesuai default PARALLEL-v2. Panel digital twin menampilkan "Data sensor belum tersedia". Dicatat di tabel cakupan sebagai "tidak dikerjakan (OQ-42)".
- Tidak ada approval, posting akuntansi, maupun job berkala di task ini. Unggah, publish, ubah pemetaan, dan ubah spot tenant ditulis ke `audit_events` (T9).
- NFR-ADV-03: batas ukuran file lewat env `CAMPUS3D_MAX_MODEL_BYTES` (limit teknis, bukan aturan bisnis); file disajikan dengan `ETag` = sha256 + `Cache-Control: private, max-age` (file model boleh di-cache browser karena berversi; data operasional tidak); model dimuat per gedung, bukan seluruh kampus sekaligus; indikator progres unduh.

## 5. Endpoint (JWT + `requireFeature("campus3d")`; `admin`, `superadmin` selalu boleh)
| Method & path | Peran |
|---|---|
| `POST /campus3d/models` (multipart `building_id`, file GLB) → draft + `node_names` · `GET /campus3d/models?building_id=` · `DELETE /campus3d/models/{id}` (hanya draft) | facility_officer (FR-155-004) |
| `GET /campus3d/models/{id}/mapping` · `PUT /campus3d/models/{id}/mapping` `{floors:[{floor_id,node_name}], rooms:[{room_id,node_name}]}` → `{issues}` · `POST /campus3d/models/{id}/publish` (422 bila ada issue error; model lama → `archived`) | facility_officer |
| `GET /campus3d/tenant-spots` · `PUT /campus3d/tenant-spots` `[{tenant_id, room_id}]` (ruang wajib `kind='kantin'`, tenant wajib ada di T12) | canteen_admin, facility_officer |
| `GET /campus3d/scene` → `{buildings:[{id, name, floors:[{id, level, name, node_name?}], model:{id, version, file_url, sha256}?}]}` (nama dibaca live dari T9) | semua peran login (FR-155-001) |
| `GET /campus3d/models/{id}/file` (stream ber-auth, hanya `published` untuk non-pengelola) | semua peran login |
| `GET /campus3d/rooms/search?q=` → `[{room_id, name, code, kind, building_id, floor_id, node_name?, mapped}]` | semua peran login (FR-155-003, AC-155-01) |
| `GET /campus3d/rooms/{roomId}` → info live T9 (nama, jenis, kapasitas, fasilitas, status) + target fokus | semua peran login (FR-155-002) |
| `GET /campus3d/canteen` → `[{tenant_id, name, is_open, room_id, node_name, building_id, floor_id}]` · `GET /campus3d/canteen/{tenantId}/menu` (live T12) | orang_tua, siswa, semua staf, canteen_admin (FR-156-001/002) |
| `GET /campus3d/availability?at=` → ruang `kind IN('lab','perpustakaan','ruang_serbaguna','aula')`: `{room_id, status, current:{label, booked_by_name, starts_at, ends_at}?, today:[…], booking_url}` | guru, siswa, semua staf (FR-157-001/002). `booked_by_name` hanya untuk staf; siswa melihat label/tujuan tanpa nama pemesan |
| `GET /campus3d/twin` → per ruang: WO aktif (code, judul, status, prioritas), aset (dari T16), `sensor: null` | facility_officer, asset_officer, kepala_sekolah (FR-158-001/002/003) |
Semua endpoint baca data operasional memakai `school_id` dari claims dan meneruskannya ke sumber; tidak ada parameter yang bisa membaca sekolah lain (superadmin lewat `?school_id=`).

## 6. Frontend
`web/src/services/ops/{campus3d.ts,types.ts}`, `web/src/router/domains/campus3d.routes.ts`. Semua route `meta.feature: 'campus3d'`, navGroup **Kampus 3D** (ortu: **Portal**).
Komponen bersama `components/campus3d/CampusViewer.vue`: props `modelUrl`, `floors`, `highlights: {node_name, tone: 'info'|'warning'|'danger'|'success', label}[]`, `focus?: {floor_id, node_name}`; emit `select(node_name)`, `ready`, `error`. `three` di-import dinamis. Kontrol: putar/zoom/geser, pilih lantai (tombol berteks), reset kamera, semua bisa dengan keyboard. Warna sorotan dari token `style.css`, selalu disertai label teks di panel samping (warna bukan satu-satunya penanda). WebGL tidak tersedia atau model gagal dimuat → tampilan daftar gedung/lantai/ruang yang setara (fallback aksesibel), bukan halaman kosong.
| Path | View (`views/campus3d/…`) | Peran | navLabel |
|---|---|---|---|
| `kampus-3d` | `CampusMapView.vue` (pilih gedung, pencarian ruang dengan hasil yang memindah kamera ke gedung+lantai dan menyorot ruang, panel info ruang live) | semua peran login | Peta Kampus 3D |
| `kampus-3d/kantin` | `CanteenMapView.vue` (tenant disorot dengan badge "Buka"/"Tutup"; pilih tenant → daftar menu + harga rupiah, "Diperbarui: <waktu>") | orang_tua, siswa, semua staf, canteen_admin | Peta Kantin |
| `kampus-3d/ruang-khusus` | `RoomAvailabilityMapView.vue` (lab/perpustakaan/serbaguna: badge "Terpakai"/"Kosong", pemesan (staf), jadwal hari ini, tombol "Pesan ruang" → `/fasilitas/booking?room_id=` bila pengguna punya akses booking; muat ulang tiap 60 detik saat tab terlihat) | guru, siswa, semua staf | Status Ruang |
| `kampus-3d/digital-twin` | `DigitalTwinView.vue` (sorotan ruang dengan WO aktif + status, daftar aset per ruang, panel sensor "belum tersedia", filter prioritas) | facility_officer, asset_officer, kepala_sekolah | Digital Twin |
| `kampus-3d/kelola` · `/:modelId` | `ModelManagerView.vue` (unggah GLB per gedung, versi, status), `MappingEditorView.vue` (klik node di viewer ↔ pilih lantai/ruang dari dropdown T9, daftar issue, publish dengan dialog konfirmasi) + tab "Spot Tenant" | facility_officer (tab tenant: canteen_admin juga) | Kelola Model 3D |

## 7. Bukti handoff
Vektor §3 + test handler: **AC-155-01** (search → target fokus benar), **AC-156-01** (fake T12 harga diubah antar dua request → respons kedua harga baru, tanpa salinan), **AC-157-01** (fake waktu 10.30), **AC-158-01** (fake WO 8A `diproses`), test skema BR-3D-01, nama ruang diubah di facility → `/campus3d/scene` langsung memakai nama baru, unggah non-GLB → 422, publish dengan issue → 422, siswa tidak melihat `booked_by_name`, siswa/ortu ditolak di `/campus3d/twin` dan endpoint kelola (403), `campus3d` mati → 403, `AssetSource` nil → twin tetap 200 dengan penanda modul aset belum aktif. Tabel cakupan memuat FR-155-001..004, FR-156-001..002, FR-157-001..002, FR-158-001..003 (003 = tidak dikerjakan, OQ-42). Playwright: facility_officer mengunggah GLB contoh kecil (fixture uji di `web/e2e-fixtures/`, bukan data klien) untuk satu gedung → memetakan lantai & "Lab Kimia" → publish → siswa membuka Peta Kampus 3D, mencari "Lab Kimia" → ruang tersorot + panel info; guru memesan lab di menu 117 untuk jam sekarang → Status Ruang menampilkan "Terpakai"; facility_officer membuat WO di ruang itu → Digital Twin menyorot ruang dengan status. Ukur waktu buka model di profil throttling "Fast 4G" + CPU 4x dan laporkan angkanya (NFR-ADV-03, target ≤ 8 detik).
