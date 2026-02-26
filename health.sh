#!/usr/bin/env bash
set -euo pipefail

SERVICE_NAME="openclaw-oneapi.service"
HOST="127.0.0.1"
PORT="3000"

log() { printf "[%s] %s\n" "$(date +%H:%M:%S)" "$*"; }

log "Service status"
systemctl is-active "$SERVICE_NAME" || true

log "Listening port"
ss -lntup | grep -E ":${PORT}\b" || echo "Port $PORT not listening"

log "HTTP health"
if command -v curl >/dev/null 2>&1; then
  curl -s -o /dev/null -w "HTTP %{http_code}\n" "http://${HOST}:${PORT}/health" || true
else
  echo "curl not installed"
fi
