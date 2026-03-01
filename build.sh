#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")"

go build -ldflags "-s -w" -o one-api
