#!/usr/bin/env bash
set -euo pipefail

source "$(dirname "$0")/common.sh"

require_cmd go

cd "${PROJECT_ROOT}"
log "Building ${APP_NAME}"
go build -o "${LOCAL_BIN_PATH}" .

if [ ! -x "${LOCAL_BIN_PATH}" ]; then
  echo "Build finished but binary not found: ${LOCAL_BIN_PATH}" >&2
  exit 1
fi

log "Build artifact: ${LOCAL_BIN_PATH}"
