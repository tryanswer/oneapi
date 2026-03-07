#!/usr/bin/env bash
set -euo pipefail

source "$(dirname "$0")/common.sh"

require_cmd systemctl

mkdir -p "${REMOTE_ROOT}" "${REMOTE_CONFIG_DIR}" "${REMOTE_ROOT}/logs"

cat > "/etc/systemd/system/${SERVICE_NAME}" <<EOF
[Unit]
Description=OpenClaw OneAPI Gateway
After=network.target

[Service]
Type=simple
WorkingDirectory=${REMOTE_ROOT}
EnvironmentFile=-${REMOTE_ENV_FILE}
ExecStart=${REMOTE_BIN_PATH} --log-dir ${REMOTE_ROOT}/logs
Restart=always
RestartSec=3
LimitNOFILE=1048576
ExecStartPre=/usr/bin/test -x ${REMOTE_BIN_PATH}

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload
systemctl enable "${SERVICE_NAME}"

echo "Installed ${SERVICE_NAME}"
