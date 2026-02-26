#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")"

echo "fetching Go modules"
go mod download

for theme in default berry air; do
  theme_dir="web/${theme}"
  if [ -d "${theme_dir}" ]; then
    echo "installing npm deps for ${theme_dir}"
    (cd "${theme_dir}" && npm ci)
  fi
done

cat <<'EOF'
Dependencies installed.
Run ./start_oneapi.sh to launch the backend.
Build frontends with `npm run build` inside each theme directory when you need the compiled assets.
EOF
