#!/usr/bin/env bash
set -euo pipefail

source "$(dirname "$0")/common.sh"

require_cmd systemctl
log "Stopping ${SERVICE_NAME}"
systemctl stop "${SERVICE_NAME}"
