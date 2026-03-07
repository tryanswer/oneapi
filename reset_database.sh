#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$ROOT_DIR"

if [[ -f ".env" ]]; then
  set -a
  # shellcheck disable=SC1091
  source ".env"
  set +a
fi

TABLES=(
  abilities
  channels
  logs
  options
  redemptions
  tokens
  users
  video_generation_tasks
)

YES=0

usage() {
  cat <<'EOF'
Usage:
  ./reset_database.sh [--yes]

Behavior:
  - Preserve schema, clear OneAPI business tables only
  - Support MySQL (via SQL_DSN) and SQLite (via SQLITE_PATH / default one-api.db)
  - Does not modify .env
  - After reset, restart one-api to recreate the initial root account (root / 123456)

Options:
  --yes    Skip confirmation prompt
EOF
}

confirm() {
  if [[ "$YES" -eq 1 ]]; then
    return 0
  fi
  printf "This will DELETE all OneAPI runtime data and restore initial state. Continue? [y/N] "
  read -r answer
  case "${answer:-}" in
    y|Y|yes|YES) ;;
    *) echo "Aborted."; exit 1 ;;
  esac
}

require_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "Missing required command: $1" >&2
    exit 1
  fi
}

parse_args() {
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --yes)
        YES=1
        shift
        ;;
      -h|--help)
        usage
        exit 0
        ;;
      *)
        echo "Unknown argument: $1" >&2
        usage
        exit 1
        ;;
    esac
  done
}

reset_sqlite() {
  require_cmd sqlite3
  local db_path="${SQLITE_PATH:-one-api.db}"
  if [[ ! -f "$db_path" ]]; then
    echo "SQLite database not found: $db_path" >&2
    exit 1
  fi

  local joined_tables=""
  local quoted_names=""
  local table
  for table in "${TABLES[@]}"; do
    joined_tables+="DELETE FROM ${table};"
    quoted_names+="'${table}',"
  done
  quoted_names="${quoted_names%,}"

  sqlite3 "$db_path" <<EOF
PRAGMA foreign_keys = OFF;
BEGIN;
${joined_tables}
DELETE FROM sqlite_sequence WHERE name IN (${quoted_names});
COMMIT;
PRAGMA foreign_keys = ON;
EOF

  echo "SQLite reset completed: $db_path"
}

reset_mysql() {
  require_cmd mysql
  local dsn="$SQL_DSN"
  if [[ -z "$dsn" ]]; then
    echo "SQL_DSN is empty, cannot use MySQL reset." >&2
    exit 1
  fi

  local parsed
  parsed="$(python3 - "$dsn" <<'PY'
import re
import sys

dsn = sys.argv[1]
pattern = re.compile(r'^(?P<user>[^:]+):(?P<password>[^@]*)@tcp\((?P<host>[^:)]+)(?::(?P<port>\d+))?\)/(?P<db>[^?]+)')
match = pattern.match(dsn)
if not match:
    sys.exit(1)
print(match.group('user'))
print(match.group('password'))
print(match.group('host'))
print(match.group('port') or '3306')
print(match.group('db'))
PY
)" || {
    echo "Failed to parse SQL_DSN. Expected format: user:password@tcp(host:port)/dbname?..." >&2
    exit 1
  }

  local mysql_user mysql_password mysql_host mysql_port mysql_db
  mysql_user="$(printf '%s\n' "$parsed" | sed -n '1p')"
  mysql_password="$(printf '%s\n' "$parsed" | sed -n '2p')"
  mysql_host="$(printf '%s\n' "$parsed" | sed -n '3p')"
  mysql_port="$(printf '%s\n' "$parsed" | sed -n '4p')"
  mysql_db="$(printf '%s\n' "$parsed" | sed -n '5p')"

  local sql="SET FOREIGN_KEY_CHECKS=0;"
  local table
  for table in "${TABLES[@]}"; do
    sql+="TRUNCATE TABLE \`${table}\`;"
  done
  sql+="SET FOREIGN_KEY_CHECKS=1;"

  MYSQL_PWD="$mysql_password" mysql \
    -h "$mysql_host" \
    -P "$mysql_port" \
    -u "$mysql_user" \
    "$mysql_db" \
    -e "$sql"

  echo "MySQL reset completed: ${mysql_db}@${mysql_host}:${mysql_port}"
}

main() {
  parse_args "$@"
  confirm

  if [[ -n "${SQL_DSN:-}" ]]; then
    reset_mysql
  else
    reset_sqlite
  fi

  cat <<'EOF'

Next steps:
1. Restart one-api
2. Login with the regenerated initial root account:
   username: root
   password: 123456

Note:
- Redis cache is not cleared by this script.
- After restart, OneAPI will rebuild cache and recreate default root data automatically.
EOF
}

main "$@"
