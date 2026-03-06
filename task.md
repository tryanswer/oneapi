# Tasks

## Channel type_name (渠道名称) & UI
- [x] Add `type_name` field to Channel model (persisted) and ensure `/api/channel/` returns it.
- [x] Require `type_name` on create (backend validation).
- [x] Add channel name selector to channel create/edit forms (default/air/berry) with common options and allow custom input.
- [x] Update form validation to require `type_name`.

## Build script
- [x] Fix `web/build.sh` to read `THEMES` and build from its own directory when invoked from repo root.

## Deterministic load balancing (same priority)
- [x] Use token hash to deterministically pick a channel among same-priority candidates.
- [x] Keep random fallback when no token hash is available.
- [x] Apply to retry path while respecting priority tiers.

## Switch DB to MySQL (Aliyun RDS)
- [x] Local dev: migrate SQLite data to MySQL `openclaw` and set `SQL_DSN`.
- [x] Local dev: enable Redis cache via `REDIS_CONN_STRING` + `SYNC_FREQUENCY`.
- [ ] RDS: collect connection info and decide migration approach (fresh vs. migrate from SQLite).
- [ ] RDS: configure `SQL_DSN` (and optional `LOG_SQL_DSN`) for production.
- [ ] RDS: verify schema auto-migration and application startup with MySQL.
