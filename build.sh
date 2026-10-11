#!/bin/bash
cd /opt/apps/teratai
C="docker compose -f docker-compose.yml -f docker-compose.server.yml"
for s in identity academic attendance exam evaluation report admission notification billing core hr web accounting; do
  echo "=== $s $(date +%T)"; $C build $s || { echo "FAIL $s"; exit 1; }
done
echo "ALL_BUILT $(date +%T)"
