---
name: sekoci-docker-flow
description: "Runbook for managing, spinning up, and checking local Docker microservices stack for SEKOCI LMS Teratai Putih."
---

# SEKOCI Local Docker Flow

Gunakan panduan ini untuk mengoperasikan environment Docker lokal untuk testing integrasi dan pengujian Playwright.

---

## 1. Menjalankan Seluruh Stack Lokal

Pastikan Docker daemon berjalan, lalu jalankan:

```bash
cd /Users/tia-nurul/Documents/LMS/skci-teratai
docker compose -f docker-compose.local.yml up -d
```

## 2. Memeriksa Status Container

```bash
docker compose -f docker-compose.local.yml ps
```

Daftar port yang harus aktif:
- `postgres`: 5432
- `identity`: 8081
- `academic`: 8082
- `attendance`: 8083
- `exam`: 8084
- `evaluation`: 8085
- `report`: 8086
- `admission`: 8087
- `notification`: 8088
- `billing`: 8089
- `accounting`: 8090
- `web`: 8080 (atau `npm run dev` pada 5173)

## 3. Membersihkan Docker Cache

Sesuai preferensi operasional pengguna, jika diminta membersihkan build cache:

```bash
docker builder prune -f
```
