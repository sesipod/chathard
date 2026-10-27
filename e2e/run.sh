#!/bin/sh
# Browser end-to-end test. Builds nothing: build the web app (cd web && npx vite build)
# and the server binary first:  (cd server && CGO_ENABLED=0 go build -o $E2E_DIR/tc .)
# Registration is limited to 3/hour/IP, so the server is restarted with a fresh DB each run.
set -e
E2E_DIR=${E2E_DIR:-/tmp/tc-e2e}; export E2E_DIR
REPO=$(cd "$(dirname "$0")/.." && pwd)
pkill -x tc 2>/dev/null || true; sleep 1
rm -rf "$E2E_DIR/data"; mkdir -p "$E2E_DIR/data"
printf 'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg==' | base64 -d > "$E2E_DIR/red.png"
echo 'TOP-SECRET-FILE-CONTENT-7731' > "$E2E_DIR/secret-plans.txt"
(cd "$REPO" && DB_PATH=$E2E_DIR/data/t.db BLOB_DIR=$E2E_DIR/data/blobs CHAT_ADDR=127.0.0.1:3100 CHAT_DOMAIN=chat.test \
  VPN_CIDRS=127.0.0.1/32 EXTRA_ORIGINS=http://127.0.0.1:3100 "$E2E_DIR/tc" > "$E2E_DIR/server.log" 2>&1 &)
sleep 2
cd "$E2E_DIR" && cp "$REPO/e2e/e2e.mjs" . && node e2e.mjs
