#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")"

SERVICE_NAME="openclaw-oneapi.service"
BIN="$(pwd)/one-api"

if [ ! -x "$BIN" ]; then
  echo "one-api binary not found or not executable: $BIN" >&2
  exit 1
fi

cat > "/etc/systemd/system/${SERVICE_NAME}" <<EOF
[Unit]
Description=OpenClaw OneAPI Gateway
After=network.target

[Service]
Type=simple
WorkingDirectory=$(pwd)
ExecStart=${BIN}
Restart=always
RestartSec=3
LimitNOFILE=1048576

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload
systemctl enable "${SERVICE_NAME}"

echo "Installed ${SERVICE_NAME}"
