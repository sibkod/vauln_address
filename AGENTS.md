# Repository knowledge

## Build & test
- Go 1.22 backend: `go build ./...`, tests `go test ./internal/...` (needs a local Go toolchain; PATH/GOPATH may be unset in this environment).
- Frontend: `cd frontend && npm install && npm run build`. SPA served statically; API expected at `VITE_API_URL` or same-origin `/api`.

## Known pre-existing test failures (not caused by recent changes)
- `internal/validators`: TestValidateTronAddress, TestValidateAddress/Sui_valid fail on clean checkout.
- `internal/handlers`: TestCreateOrder_InvalidRequest/negative_checks expects 400 for negative checks but the handler returns 200 (validation bug in CreateOrder).

## Data conventions
- DB access supports postgres/mysql/sqlite; all queries use `?` placeholders and per-dialect upserts (see UpsertUserNonce).
- `check_history.wallet_address` doubles as report-access key: authenticated wallet address, or `ip:<ip>` for anonymous users (repository.AnonymousRequesterPrefix). Anonymous rows older than 24h are deleted by ReportCleanupService, which implements report expiry (models.AnonymousReportTTL).
- `wallets.reason`/`wallets.source` (migration 004) feed the report endpoint; `leaked_keys` (migration 003) supplies leak metadata — never expose `key_value`.
- Runtime schema (repository.InitSchema) does not create `leaked_keys`; report code tolerates missing table.

## Report endpoint
- `GET /api/report?address&chain` — access only after same requester ran `/api/check`; 403 REPORT_NOT_AVAILABLE / REPORT_EXPIRED, 404 when not in DB. Transaction tree is deterministic (sha256-derived), child statuses: DB status else heuristic unknown/potential_hacker.
- Public sharing without schema changes: share token = UUID of [8-byte check_history.id][8-byte HMAC-SHA256(auth.JWTSecret, "report-share-v1|id")]. `POST /api/report/share` (RequireAuth) mints it, `GET /api/report/shared/:id` serves publicly. Only authenticated users can mint; revoking = deleting the check_history row (never happens for auth rows). See internal/handlers/report_share.go.
- Frontend: `/report/:id` (public) and `/report?address&chain` (owner view) both render ReportView; "Make public & share" only for connected wallets, anon sees a lock notice.
- Tests: `internal/handlers/report_test.go` runs the real handler against real SQLite (TempDir), seeds via repository.CreateWallet + raw SQL for leaked_keys (migration 003 table, not in InitSchema), backdates check_history with UTC "2006-01-02 15:04:05" strings. CheckWallet records history in a goroutine — poll GetLastReportAccess in tests.
- Go toolchain in this sandbox may be missing; install to /tmp/go and export PATH=/tmp/go/bin, GOPATH=$HOME/gopath.
