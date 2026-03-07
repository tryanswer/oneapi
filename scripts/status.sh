#!/usr/bin/env bash
set -euo pipefail

source "$(dirname "$0")/common.sh"

require_cmd systemctl
systemctl status "${SERVICE_NAME}" --no-pager
