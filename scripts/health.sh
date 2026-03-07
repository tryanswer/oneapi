#!/usr/bin/env bash
set -euo pipefail

source "$(dirname "$0")/common.sh"

HOST="${HOST:-127.0.0.1}"
PORT="${PORT:-3000}"

log "Service status"
systemctl is-active "${SERVICE_NAME}" || true

log "Listening port"
ss -lntup | grep -E ":${PORT}\b" || echo "Port ${PORT} not listening"

log "HTTP health"
if command -v curl >/dev/null 2>&1; then
  curl -fsS "http://${HOST}:${PORT}/api/status" || true
  echo
else
  echo "curl not installed"
fi
