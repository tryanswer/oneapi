#!/usr/bin/env bash
set -euo pipefail

source "$(dirname "$0")/common.sh"

require_cmd systemctl
log "Restarting ${SERVICE_NAME}"
systemctl restart "${SERVICE_NAME}"
