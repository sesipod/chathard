#!/usr/bin/env bash
set -euo pipefail

# ──────────────────────────────────────────────
# TailChat — Deploy update script
# Usage: sudo /opt/tailchat/scripts/update.sh [--from-dir <path>]
# ──────────────────────────────────────────────

TAILCHAT_DIR="/opt/tailchat"
DATA_DIR="${TAILCHAT_DIR}/data"
SERVER_SRC_DIR="${TAILCHAT_DIR}/server-src"
SERVER_BIN="${TAILCHAT_DIR}/tailchat-server"
NEW_BIN="${TAILCHAT_DIR}/tailchat-server.new"
PREV_BIN="${TAILCHAT_DIR}/tailchat-server.prev"
MIGRATIONS_DIR="${TAILCHAT_DIR}/server/db/migrations"
LOG_FILE="${DATA_DIR}/updates.log"

mkdir -p "$(dirname "$LOG_FILE")"

timestamp() { date -u '+%Y-%m-%dT%H:%M:%SZ'; }

log() {
  echo "[$(timestamp)] $*"
  echo "[$(timestamp)] $*" >> "$LOG_FILE"
}

# ── Step 1: Acquire new code ───────────────────
if [[ "${1:-}" == "--from-dir" && -n "${2:-}" ]]; then
  SRC_DIR="$2"
  log "Acquiring code from directory: ${SRC_DIR}"
  rsync -a "${SRC_DIR}/" "${SERVER_SRC_DIR}/"
else
  log "Pulling latest code from git"
  cd "${SERVER_SRC_DIR}"
  git pull origin main
fi

COMMIT_HASH=$(cd "${SERVER_SRC_DIR}" && git log -1 --oneline 2>/dev/null || echo "unknown")
log "New commit: ${COMMIT_HASH}"

# Sync source to working directories
rsync -a "${SERVER_SRC_DIR}/" "${TAILCHAT_DIR}/" --exclude=data --exclude='.git' --exclude=config.yaml

# ── Step 2: Check DB migrations ────────────────
log "Checking for pending DB migrations"

if [[ ! -f "${DATA_DIR}/tailchat.db" ]]; then
  log "Database not found — will be created on first server start"
else
  # Ensure schema_migrations table exists
  sqlite3 "${DATA_DIR}/tailchat.db" \
    "CREATE TABLE IF NOT EXISTS schema_migrations (
      version INTEGER PRIMARY KEY,
      name TEXT NOT NULL,
      applied_at TEXT NOT NULL DEFAULT (datetime('now'))
    );"

  if [[ -d "${MIGRATIONS_DIR}" ]]; then
    for migration in $(ls "${MIGRATIONS_DIR}"/*.sql 2>/dev/null | sort); do
      filename=$(basename "${migration}")
      version="${filename%%_*}"
      # Check if already applied
      applied=$(sqlite3 "${DATA_DIR}/tailchat.db" \
        "SELECT COUNT(*) FROM schema_migrations WHERE version = ${version};")
      if [[ "$applied" -eq 0 ]]; then
        log "Applying migration: ${filename}"
        sqlite3 "${DATA_DIR}/tailchat.db" < "${migration}"
        sqlite3 "${DATA_DIR}/tailchat.db" \
          "INSERT INTO schema_migrations (version, name) VALUES (${version}, '${filename}');"
        log "Migration applied: ${filename}"
      else
        log "Migration already applied: ${filename} (skipping)"
      fi
    done
  else
    log "No migrations directory found at ${MIGRATIONS_DIR}"
  fi
fi

# ── Step 3: Build frontend ─────────────────────
log "Building frontend"
cd "${TAILCHAT_DIR}/web"
if [[ -f package-lock.json ]]; then
  npm ci
else
  npm install
fi
npm run build

if [[ ! -f "${TAILCHAT_DIR}/web/dist/index.html" ]]; then
  log "ERROR: Frontend build failed — dist/index.html not found"
  exit 1
fi
log "Frontend build complete"
chown -R tailchat:tailchat "${TAILCHAT_DIR}/web/dist"

# ── Step 4: Build backend ──────────────────────
log "Building backend"
cd "${TAILCHAT_DIR}/server"
export PATH="$PATH:/usr/local/go/bin"
go build -buildvcs=false -o /tmp/tailchat-server.new .
mv /tmp/tailchat-server.new "${NEW_BIN}"
chown tailchat:tailchat "${NEW_BIN}"

if ! "${NEW_BIN}" --version &>/dev/null; then
  log "ERROR: Backend binary verification failed"
  rm -f "${NEW_BIN}"
  exit 1
fi
log "Backend build complete"

# ── Step 5: Atomic swap & restart ──────────────
log "Performing atomic swap"
# Preserve current binary as rollback target
if [[ -f "${SERVER_BIN}" ]]; then
  cp "${SERVER_BIN}" "${PREV_BIN}"
fi

systemctl stop tailchat
mv "${NEW_BIN}" "${SERVER_BIN}"
systemctl start tailchat
log "Atomic swap complete — service restarted"

# ── Step 6: Health check ───────────────────────
log "Running health check"
if curl -s --retry 5 --retry-delay 2 http://localhost:3000/api/health; then
  echo ""
  log "Health check passed"
else
  echo ""
  log "ERROR: Health check failed — rolling back"
  systemctl stop tailchat
  mv "${PREV_BIN}" "${SERVER_BIN}"
  systemctl start tailchat
  log "Rolled back to previous binary"
  exit 1
fi

# Clean up prev binary on success
rm -f "${PREV_BIN}"

# ── Step 7: Log update ─────────────────────────
log "Update complete: ${COMMIT_HASH}"
echo "[$(timestamp)] UPDATE SUCCESS: ${COMMIT_HASH}" >> "${LOG_FILE}"
echo "=== Update completed successfully ==="
