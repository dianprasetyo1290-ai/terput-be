# Sekoci Accounting Service

Layanan akuntansi dan manajemen keuangan untuk platform Sekoci (Sekolah Cerdas Indonesia).

## Overview

Sekoci Accounting menangani:
- Bagan Akun (Chart of Accounts / COA)
- Jurnal Umum & Transaksi Keuangan
- Validasi & Riwayat Jurnal
- Buku Besar (General Ledger)
- Neraca Saldo & Laporan Keuangan (Balance Sheet, P&L, Generator Laporan)
- Penganggaran (Budgeting)
- Manajemen Mata Uang & Periode Akuntansi
- Tutup Buku Akhir Bulan (EOM) & Akhir Tahun (EOY)

## Stack

- **Backend**: Laravel 11 / 12 (PHP 8.4-FPM)
- **Frontend**: Livewire 3 + Tailwind CSS + Limitless UI
- **Database**: PostgreSQL (`skci_accounting`)
- **Web Server**: Nginx + Supervisord
- **Auth**: Laravel Jetstream / Fortify (Sanctum) + Spatie Laravel Permission

## Deployment Guide (Sekoci VM)

Build dan deploy mengikuti konvensi microservices Sekoci:

```bash
SVC=accounting
cd ~/Documents/LMS/skci/services/$SVC
docker buildx build --platform linux/amd64 -t sekoci-src-$SVC:new --load .
docker save sekoci-src-$SVC:new | gzip -1 > /tmp/$SVC.tgz
gcloud compute scp /tmp/$SVC.tgz sekoci:/tmp/ \
  --zone asia-southeast2-a --project sekoci-trial --tunnel-through-iap
```

Di dalam VM `sekoci`:
```bash
cd /opt/apps/sekoci-src
gunzip -c /tmp/accounting.tgz | sudo docker load
sudo docker tag sekoci-src-accounting:new sekoci-src-accounting:latest
sudo docker compose up -d --force-recreate --no-deps accounting
```
