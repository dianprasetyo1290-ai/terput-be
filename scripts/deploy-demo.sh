#!/usr/bin/env bash
# Deploys the demo stack to a dedicated VPS that serves the app at its bare IP
# (docker-compose.demo.yml). Images are built HERE and streamed to the box, so
# a 2 GB VPS never has to build. Run from sekoci-be, with sekoci-fe next to it.
#
#   ./scripts/deploy-demo.sh <host> [ssh-key] [--with-data <dump-dir>]
#
#   host        e.g. root@43.156.89.152
#   ssh-key     default ~/.ssh/terput_vps
#   --with-data restore the per-database pg_dump files (<db>.dump) in
#               <dump-dir> — only on the first deploy, into empty databases.
#
# Needs .env.demo next to docker-compose.yml (see docker-compose.demo.yml).
# Step by step: prepare the box (Docker, swap, firewall), copy the repo files
# the stack mounts (migrations, db-init, compose files), build and load the
# images, start Postgres, optionally restore data, then start everything.
set -euo pipefail
cd "$(dirname "$0")/.."

HOST="${1:?usage: deploy-demo.sh <user@host> [ssh-key] [--with-data <dir>]}"
KEY="${2:-$HOME/.ssh/terput_vps}"
DATA=""
if [ "${3:-}" = "--with-data" ]; then DATA="${4:?--with-data needs a directory}"; fi
[ -f .env.demo ] || { echo ".env.demo missing"; exit 1; }

APP=/opt/terput
SSH=(ssh -i "$KEY" -o IdentitiesOnly=yes -o ServerAliveInterval=30 "$HOST")
COMPOSE="docker compose -p terput -f docker-compose.yml -f docker-compose.local.yml -f docker-compose.demo.yml --env-file .env.demo"
STAGE="$(mktemp -d)"
trap 'rm -rf "$STAGE"' EXIT

echo "==> 1/6 prepare the box"
"${SSH[@]}" 'bash -s' <<'REMOTE'
set -e
if ! command -v docker >/dev/null; then curl -fsSL https://get.docker.com | sh; fi
if ! swapon --show | grep -q /swapfile; then
  fallocate -l 4G /swapfile && chmod 600 /swapfile && mkswap /swapfile >/dev/null && swapon /swapfile
  grep -q '^/swapfile' /etc/fstab || echo '/swapfile none swap sw 0 0' >> /etc/fstab
fi
if command -v ufw >/dev/null; then ufw allow 22/tcp >/dev/null; ufw allow 80/tcp >/dev/null; fi
mkdir -p /opt/terput
REMOTE

echo "==> 2/6 copy repo files"
git archive HEAD | tar -x -C "$STAGE"
cp docker-compose.demo.yml .env.demo "$STAGE/"
tar -C "$STAGE" -czf - . | "${SSH[@]}" "tar -xzf - -C $APP && chmod 600 $APP/.env.demo"

echo "==> 3/6 build images (sekoci-fe HEAD as ./web)"
mkdir -p "$STAGE/web"
git -C ../sekoci-fe archive HEAD | tar -x -C "$STAGE/web"
(cd "$STAGE" && $COMPOSE build)

echo "==> 4/6 load images on the box"
# One file per image, each copied with retries and checked by size: a single
# ~1 GB stream over one SSH connection gets reset on slow uplinks.
IMAGES=$(cd "$STAGE" && $COMPOSE config --images | grep '^terput-' | sort -u)
"${SSH[@]}" "mkdir -p /opt/terput-images"
for img in $IMAGES; do
  f="$STAGE/$img.tar.gz"
  docker save "$img" | gzip -1 > "$f"
  size=$(wc -c < "$f" | tr -d ' ')
  for try in 1 2 3 4 5; do
    scp -q -i "$KEY" -o IdentitiesOnly=yes -o ServerAliveInterval=15 "$f" "$HOST:/opt/terput-images/$img.tar.gz" || true
    [ "$("${SSH[@]}" "wc -c < /opt/terput-images/$img.tar.gz 2>/dev/null" | tr -d ' ')" = "$size" ] && break
    [ "$try" = 5 ] && { echo "upload of $img failed"; exit 1; }
    echo "    retry $img ($try)"
  done
  "${SSH[@]}" "gunzip -c /opt/terput-images/$img.tar.gz | docker load -q && rm /opt/terput-images/$img.tar.gz"
  rm "$f"
done

echo "==> 5/6 start postgres"
"${SSH[@]}" "cd $APP && $COMPOSE pull -q postgres redis && $COMPOSE up -d postgres && for i in \$(seq 60); do $COMPOSE exec -T postgres pg_isready -U \$(grep ^POSTGRES_USER= .env.demo | tail -1 | cut -d= -f2) >/dev/null 2>&1 && break; sleep 2; done"
if [ -n "$DATA" ]; then
  for f in "$DATA"/*.dump; do
    db=$(basename "$f" .dump)
    echo "    restore $db"
    "${SSH[@]}" "cd $APP && $COMPOSE exec -T postgres sh -c 'pg_restore -U \$POSTGRES_USER -d $db --no-owner --exit-on-error'" < "$f"
  done
fi

echo "==> 6/6 start the stack"
"${SSH[@]}" "cd $APP && $COMPOSE up -d --no-build && docker image prune -f >/dev/null && $COMPOSE ps --format '{{.Service}} {{.State}}'"
echo "done: http://${HOST#*@}/"
