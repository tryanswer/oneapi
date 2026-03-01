#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")"

echo "fetching Go modules"
go mod download

./web/build.sh

cat <<'EOF'
Dependencies installed.
Run ./start_oneapi.sh to launch the backend.
Build frontends with `npm run build` inside each theme directory when you need the compiled assets.
EOF
