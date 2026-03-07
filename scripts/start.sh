#!/usr/bin/env bash
set -euo pipefail

source "$(dirname "$0")/common.sh"

require_cmd systemctl
log "Starting ${SERVICE_NAME}"
systemctl start "${SERVICE_NAME}"
