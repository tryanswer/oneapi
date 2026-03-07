#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

APP_NAME="openclaw-oneapi"
SERVICE_NAME="${APP_NAME}.service"

REMOTE_HOST="${REMOTE_HOST:-aliyun}"
REMOTE_ROOT="${REMOTE_ROOT:-/root/openclaw/oneapi}"
REMOTE_CONFIG_DIR="${REMOTE_CONFIG_DIR:-/root/openclaw/config/oneapi}"
REMOTE_BRANCH="${REMOTE_BRANCH:-}"
REMOTE_BIN_NAME="${REMOTE_BIN_NAME:-one-api}"
REMOTE_BIN_PATH="${REMOTE_ROOT}/${REMOTE_BIN_NAME}"
REMOTE_ENV_FILE="${REMOTE_CONFIG_DIR}/.env"

LOCAL_BIN_PATH="${PROJECT_ROOT}/one-api"
LOCAL_ENV_FILE="${PROJECT_ROOT}/.env"
LOCAL_ENV_EXAMPLE="${PROJECT_ROOT}/.env.example"

log() {
  printf "[%s] %s\n" "$(date '+%Y-%m-%d %H:%M:%S')" "$*"
}

require_cmd() {
  command -v "$1" >/dev/null 2>&1 || {
    echo "Missing command: $1" >&2
    exit 1
  }
}
