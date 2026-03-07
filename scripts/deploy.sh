#!/usr/bin/env bash
set -euo pipefail

source "$(dirname "$0")/common.sh"

require_cmd ssh
log "Deploying ${APP_NAME} on ${REMOTE_HOST} via git sync"
ssh "${REMOTE_HOST}" \
  REMOTE_ROOT="${REMOTE_ROOT}" \
  REMOTE_CONFIG_DIR="${REMOTE_CONFIG_DIR}" \
  REMOTE_BIN_PATH="${REMOTE_BIN_PATH}" \
  REMOTE_ENV_FILE="${REMOTE_ENV_FILE}" \
  SERVICE_NAME="${SERVICE_NAME}" \
  REMOTE_BRANCH="${REMOTE_BRANCH}" \
  'bash -s' <<'REMOTE_SCRIPT'
set -euo pipefail

if [ ! -d "${REMOTE_ROOT}/.git" ]; then
  echo "Missing git repo on remote: ${REMOTE_ROOT}" >&2
  exit 1
fi

mkdir -p "${REMOTE_CONFIG_DIR}" "${REMOTE_ROOT}/logs"

if [ -n "${REMOTE_BRANCH}" ]; then
  git -C "${REMOTE_ROOT}" fetch --all --prune
  git -C "${REMOTE_ROOT}" checkout "${REMOTE_BRANCH}"
  git -C "${REMOTE_ROOT}" pull --ff-only origin "${REMOTE_BRANCH}"
else
  CURRENT_BRANCH="$(git -C "${REMOTE_ROOT}" rev-parse --abbrev-ref HEAD)"
  git -C "${REMOTE_ROOT}" pull --ff-only origin "${CURRENT_BRANCH}"
fi

if [ ! -f "${REMOTE_ENV_FILE}" ]; then
  echo "Missing remote env file: ${REMOTE_ENV_FILE}" >&2
  echo "Create it manually or sync it separately before starting the service." >&2
  exit 1
fi

cd "${REMOTE_ROOT}"
go build -o "${REMOTE_BIN_PATH}" .
chmod +x \
  "${REMOTE_ROOT}/scripts/common.sh" \
  "${REMOTE_ROOT}/scripts/start.sh" \
  "${REMOTE_ROOT}/scripts/stop.sh" \
  "${REMOTE_ROOT}/scripts/restart.sh" \
  "${REMOTE_ROOT}/scripts/status.sh" \
  "${REMOTE_ROOT}/scripts/health.sh" \
  "${REMOTE_ROOT}/scripts/install_service.sh" \
  "${REMOTE_ROOT}/scripts/deploy.sh"

"${REMOTE_ROOT}/scripts/install_service.sh"
"${REMOTE_ROOT}/scripts/restart.sh"
REMOTE_SCRIPT

log "Deployment complete"
