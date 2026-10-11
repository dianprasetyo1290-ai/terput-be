#!/usr/bin/env bash
# Jalankan beberapa Gemini headless paralel, satu git worktree per task.
#   scripts/gemini-parallel.sh all      SEMUA otomatis: gelombang 1 → gabung → gelombang 2 → gabung → uji integrasi
#                                       (jalan di background; aman tutup terminal). Review Claude di akhir.
#                                       Aman diulang: task yang sudah SELESAI dilewati (resume).
#   scripts/gemini-parallel.sh status   lihat progres
#   scripts/gemini-parallel.sh 1|2      jalankan satu gelombang saja (manual)
#   scripts/gemini-parallel.sh clean    hapus worktree (branch tetap ada)
# Aturan & prompt: docs/prd-sekoci/PARALLEL.md, GEMINI.md §7b.
set -euo pipefail

ROOT=$(git rev-parse --show-toplevel)
WT="$(dirname "$ROOT")/skci-teratai-wt"
LOGS="$WT/logs"
INT="$WT/INT"                    # worktree branch integrasi
INT_BRANCH="integrasi/prd-v1"
POLL=${POLL:-60}                 # detik antar cek status

# TASK|branch|file|bagian
WAVE1=(
  "T1|feat/T1-core|tasks/T1-core-approval-exception-audit.md|semua"
  "T2|feat/T2-hris|tasks/T2-hris.md|semua"
  "T6|feat/T6-payment|tasks/T6-payment-gateway.md|semua"
  "T7a|feat/T7a-transfer|tasks/T7-transfer-layanan-tagihan-saya.md|§1 dan §3 saja"
  "T8|feat/T8-audit|tasks/T8-audit-gap-01-77.md|semua"
)
WAVE2=(
  "T3|feat/T3-payroll|tasks/T3-payroll.md|semua"
  "T4|feat/T4-ats|tasks/T4-ats.md|semua"
  "T5|feat/T5-training|tasks/T5-pelatihan-onboarding.md|semua"
  "T7b|feat/T7b-layanan|tasks/T7-transfer-layanan-tagihan-saya.md|§2 saja"
)

say() { echo "[$(date '+%H:%M:%S')] $*"; }
phase() { mkdir -p "$LOGS"; echo "$*" >"$LOGS/PHASE"; say "== $*"; }

prompt() { # $1 task  $2 file  $3 bagian
  cat <<EOF
Kamu adalah engineer implementasi untuk repo skci-teratai. Kamu bekerja dalam MODE PARALEL: ada beberapa agen Gemini lain yang mengerjakan task berbeda di worktree lain pada saat yang sama.

Task kamu: $1. File spesifikasi: docs/prd-sekoci/$2 (bagian: $3).

Langkah wajib:
1. Baca seluruh GEMINI.md (terutama §7b Mode paralel). Aturannya mengikat dan mengalahkan kebiasaanmu sendiri.
2. Baca docs/prd-sekoci/README.md, docs/prd-sekoci/PARALLEL.md (nomor migrasimu), docs/prd-sekoci/DESIGN.md (desain sistem & UI Teratai), lalu file spesifikasi task-mu sampai habis, lalu bagian PRD di docs/prd-sekoci/source/ yang dirujuk.
3. Sebelum menulis kode, baca kode acuan yang disebut di GEMINI.md §1 dan file donor yang disebut di task. Ikuti pola yang ada, jangan menciptakan pola baru.
4. Kerjakan backend dulu (migrasi → fungsi murni + test tabel → store → handler + test → integrasi), lalu frontend (types → service → route → view mengikuti desain di docs/prd-sekoci/DESIGN.md), sesuai urutan di file task.
5. Verifikasi sesuai GEMINI.md §7b: go vet + go test setiap service yang disentuh, migrasi up/down/up di DB Postgres lokal khusus task-mu, npx vue-tsc -b && npm run build. Semua harus hijau. Jangan pakai docker compose, jangan jalankan server.
6. Tulis docs/prd-sekoci/handoff/$1.md (format GEMINI.md §7 langkah 4) dengan tabel FR/AC → bukti dan output asli perintah verifikasi.
7. Commit semua perubahan di branch saat ini (lokal, jangan push, jangan deploy). Baris terakhir output: "SELESAI $1" atau "BLOCKED $1".

Larangan: jangan bertanya ke pengguna (tidak ada yang menjawab, putuskan dengan default README §5 lalu catat). Jangan hardcode nilai bisnis. Jangan menyentuh file di luar cakupan task kecuali penambahan baris di file bersama. Jangan menyalin kredensial dari repo donor. Jangan menghapus atau melemahkan test yang ada.
EOF
}

prompt_conflict() { # $1 task $2 branch
  cat <<EOF
MODE GABUNG. Kamu berada di worktree branch $INT_BRANCH yang sedang di-merge dengan $2 (task $1) dan ada konflik.
1. Lihat konflik: git diff --name-only --diff-filter=U.
2. Selesaikan SEMUA konflik dengan mempertahankan perubahan KEDUA sisi. File bersama (docker-compose*.yml, .env.example, web/src/api/client.ts, web/src/router/index.ts, web/.env*) hampir selalu cukup digabung barisnya. Jangan membuang fitur task mana pun. Baca docs/prd-sekoci/handoff/*.md bila perlu konteks.
3. git add file yang diselesaikan, lalu git commit --no-edit.
Jangan jalankan docker, jangan push, jangan bertanya ke pengguna. Baris terakhir output: "SELESAI MRG-$1" atau "BLOCKED MRG-$1".
EOF
}

prompt_check() { # $1 gelombang
  cat <<EOF
MODE CEK GABUNGAN gelombang $1. Kamu berada di worktree branch $INT_BRANCH yang baru saja menggabungkan semua branch gelombang $1. Ikuti GEMINI.md §7b (tanpa docker).
1. Untuk setiap service di services/* yang punya go.mod: go vet ./... && go test ./... . Untuk web: npm ci (bila perlu) lalu npx vue-tsc -b && npm run build.
2. Migrasi semua service Go dari nol di DB Postgres lokal baru (createdb skci_int_w$1_<svc>, migrate up, down 1, up). DB lama dengan nama yang sama boleh di-drop dulu.
3. Perbaiki kerusakan akibat penggabungan (import ganda, route ganda, tipe bentrok, kontrak antar-task yang tidak cocok, catatan "setelah rebase" di handoff gelombang ini seperti menyambungkan ApprovalTimeline). Perubahan minimal, jangan menulis ulang fitur.
4. Tulis docs/prd-sekoci/handoff/W$1-gabung.md: apa yang rusak, apa yang diperbaiki, output asli perintah verifikasi. Commit.
Jangan push, jangan deploy, jangan bertanya ke pengguna. Baris terakhir output: "SELESAI CEK-W$1" atau "BLOCKED CEK-W$1".
EOF
}

prompt_final() {
  cat <<EOF
MODE INTEGRASI AKHIR. Kamu berada di worktree branch $INT_BRANCH yang berisi semua task T1–T8. Semua agen lain sudah selesai, jadi docker boleh dipakai sekarang.
1. docker compose -f docker-compose.local.yml -p skci-int up -d --build (semua service termasuk *-migrate). Jika port host sudah dipakai stack lain, JANGAN mematikan stack itu: tulis di laporan dan akhiri dengan BLOCKED.
2. Pastikan semua *-migrate exit 0 dan semua service healthy.
3. Aktifkan feature flag hris, ats, payroll, training, payment_gateway untuk sekolah uji, buat akun uji per peran baru, konfigurasikan alur approval yang dibutuhkan (scripts/seed-*.sql dari task bila ada).
4. Jalankan uji Playwright end-to-end yang disebut di bagian "Bukti handoff" SETIAP task T1–T8 (pola web/e2e-teratai-*.mjs), headed, screenshot ke docs/prd-sekoci/handoff/screens/.
5. Perbaiki bug kecil yang ditemukan (commit terpisah per bug), catat bug besar.
6. Tulis docs/prd-sekoci/handoff/FINAL.md: tabel skenario → lulus/gagal + screenshot, bug diperbaiki, bug terbuka, status tiap task (termasuk yang BLOCKED). Commit. Matikan stack: docker compose -p skci-int down.
Jangan push, jangan deploy, jangan bertanya ke pengguna. Baris terakhir output: "SELESAI FINAL" atau "BLOCKED FINAL".
EOF
}

state() { # $1 nama log → SELESAI|BLOCKED|BERHENTI|berjalan
  local log="$LOGS/$1.log" mark
  [ -f "$log" ] || { echo berjalan; return; }
  # penanda = baris utuh "SELESAI X"/"BLOCKED X" (boleh dibungkus ** atau `); ambil yang terakhir
  mark=$(grep -E "^[*\`[:space:]]*(SELESAI|BLOCKED) $1[*\`[:space:]]*$" "$log" | tail -1 | grep -oE 'SELESAI|BLOCKED' || true)
  if [ -n "$mark" ]; then echo "$mark"
  elif grep -q '^\[exit' "$log"; then echo BERHENTI
  else echo berjalan; fi
}

run_gemini_bg() { # $1 nama log  $2 dir  $3 prompt
  # nohup: tetap jalan walau terminal ditutup. -m hanya dikirim kalau GEMINI_MODEL diset.
  ( cd "$2" && nohup sh -c 'gemini --yolo $0 -p "$1"; echo "[exit $?]"' \
      "${GEMINI_MODEL:+-m $GEMINI_MODEL}" "$3" >"$LOGS/$1.log" 2>&1 & )
}

run_gemini_fg() { # $1 nama log  $2 dir  $3 prompt → tunggu selesai
  run_gemini_bg "$@"
  while [ "$(state "$1")" = berjalan ]; do sleep "$POLL"; done
  say "$1: $(state "$1")"
}

start_wave() { # $1 base commit, sisanya baris task
  local base=$1; shift
  mkdir -p "$LOGS"
  for row in "$@"; do
    IFS='|' read -r task branch file part <<<"$row"
    local dir="$WT/$task"
    # lanjut (resume): task yang sudah SELESAI tidak diulang; hapus log-nya untuk memaksa ulang
    [ "$(state "$task")" = SELESAI ] && { say "✓ $task sudah SELESAI, dilewati"; continue; }
    if [ ! -d "$dir" ]; then
      if git -C "$ROOT" show-ref --quiet "refs/heads/$branch"; then
        git -C "$ROOT" worktree add "$dir" "$branch"
      else
        git -C "$ROOT" worktree add -b "$branch" "$dir" "$base"
      fi
    fi
    say "▶ $task ($branch) log: $LOGS/$task.log"
    run_gemini_bg "$task" "$dir" "$(prompt "$task" "$file" "$part")"
  done
}

wait_wave() { # baris task
  local row task pending
  while :; do
    pending=0
    for row in "$@"; do task=${row%%|*}; [ "$(state "$task")" = berjalan ] && pending=$((pending+1)); done
    [ $pending -eq 0 ] && break
    sleep "$POLL"
  done
  for row in "$@"; do task=${row%%|*}; say "$task: $(state "$task")"; done
}

merge_wave() { # $1 nomor gelombang, sisanya baris task
  local n=$1; shift
  local row task branch
  for row in "$@"; do
    IFS='|' read -r task branch _ <<<"$row"
    if [ "$(state "$task")" != SELESAI ]; then
      say "lewati $task ($(state "$task")): tidak digabung"; echo "$task" >>"$LOGS/SKIPPED"; continue
    fi
    say "gabung $branch"
    if ! git -C "$INT" merge --no-ff --no-edit "$branch" >>"$LOGS/merge.log" 2>&1; then
      run_gemini_fg "MRG-$task" "$INT" "$(prompt_conflict "$task" "$branch")"
      if [ -n "$(git -C "$INT" diff --name-only --diff-filter=U)" ] || git -C "$INT" rev-parse -q --verify MERGE_HEAD >/dev/null; then
        git -C "$INT" merge --abort || true
        say "konflik $task tidak terselesaikan: dilewati"; echo "$task" >>"$LOGS/SKIPPED"
      fi
    fi
  done
  run_gemini_fg "CEK-W$n" "$INT" "$(prompt_check "$n")"
}

orchestrate() {
  local base; base=$(git -C "$ROOT" rev-parse HEAD)
  mkdir -p "$LOGS"; : >"$LOGS/SKIPPED"
  if [ ! -d "$INT" ]; then
    if git -C "$ROOT" show-ref --quiet "refs/heads/$INT_BRANCH"; then git -C "$ROOT" worktree add "$INT" "$INT_BRANCH"
    else git -C "$ROOT" worktree add -b "$INT_BRANCH" "$INT" "$base"; fi
  fi

  phase "gelombang 1 berjalan"
  start_wave "$base" "${WAVE1[@]}"; wait_wave "${WAVE1[@]}"
  for t in T1 T2; do
    [ "$(state "$t")" = SELESAI ] || { phase "BERHENTI: $t $(state "$t"), gelombang 2 butuh $t. Panggil Claude."; exit 1; }
  done
  phase "menggabungkan gelombang 1"
  merge_wave 1 "${WAVE1[@]}"

  phase "gelombang 2 berjalan"
  start_wave "$(git -C "$INT" rev-parse HEAD)" "${WAVE2[@]}"; wait_wave "${WAVE2[@]}"
  phase "menggabungkan gelombang 2"
  merge_wave 2 "${WAVE2[@]}"

  phase "uji integrasi akhir (docker + Playwright)"
  run_gemini_fg FINAL "$INT" "$(prompt_final)"

  phase "SELESAI: semua di branch $INT_BRANCH ($(state FINAL)). Minta Claude: review semua."
}

status() {
  [ -d "$LOGS" ] || { echo "Belum ada yang dijalankan."; return; }
  [ -f "$LOGS/PHASE" ] && echo "Fase: $(cat "$LOGS/PHASE")"
  for log in "$LOGS"/*.log; do
    local name; name=$(basename "$log" .log)
    case $name in orchestrator|merge) continue ;; esac
    printf '%-7s %-10s %s\n' "$name" "$(state "$name")" "$(git -C "$WT/$name" log --oneline -1 2>/dev/null || true)"
  done
  [ -s "$LOGS/SKIPPED" ] && echo "Tidak digabung: $(tr '\n' ' ' <"$LOGS/SKIPPED")"
  return 0
}

need_clean() {
  [ -z "$(git -C "$ROOT" status --porcelain)" ] || {
    echo "Working tree belum bersih. Commit baseline dulu: git add -A && git commit -m baseline" >&2; exit 1; }
}

case "${1:-}" in
  all)
    need_clean
    if [ "${_ORCH:-}" = 1 ]; then orchestrate
    else
      mkdir -p "$LOGS"
      _ORCH=1 nohup "$0" all >"$LOGS/orchestrator.log" 2>&1 &
      echo "Berjalan di background (aman tutup terminal). Pantau: $0 status   |   log: $LOGS/orchestrator.log"
    fi ;;
  1) need_clean; start_wave "$(git -C "$ROOT" rev-parse HEAD)" "${WAVE1[@]}" ;;
  2) need_clean; start_wave "$(git -C "$ROOT" rev-parse HEAD)" "${WAVE2[@]}" ;;
  status) status ;;
  clean) for d in "$WT"/T* "$INT"; do [ -d "$d" ] && git -C "$ROOT" worktree remove --force "$d"; done; true ;;
  *) echo "pakai: $0 all|status|1|2|clean" >&2; exit 1 ;;
esac
