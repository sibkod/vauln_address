# Repository knowledge

## Build & test
- Go 1.22 backend: `go build ./...`, tests `go test ./internal/...` (needs a local Go toolchain; PATH/GOPATH may be unset in this environment).
- Frontend: `cd frontend && npm install && npm run build`. SPA served statically; API expected at `VITE_API_URL` or same-origin `/api`.

## Known pre-existing test failures (not caused by recent changes)
- `internal/validators`: TestValidateTronAddress, TestValidateAddress/Sui_valid fail on clean checkout.
- `internal/handlers`: TestCreateOrder_InvalidRequest/negative_checks expects 400 for negative checks but the handler returns 200 (validation bug in CreateOrder).

## Wallet statuses
- Single source of truth: `statusCatalog` in internal/models/models.go (13 statuses: hacked, vulnerable, safe, hacker, drained, phishing, scam, mixer, sanctioned, exchange, suspicious, frozen, unknown) with label, severity (danger/warning/info) and description. `IsValidStatus`/`ValidStatusNames`/`StatusDescription` derive from it; report details and the admin import error use these helpers.
- Public catalog endpoint: `GET /api/statuses` (handlers.GetStatuses). Frontend mirrors the catalog in HomeView/ReportView/ChecksView statusMeta maps and global CSS (`.dot.danger|vulnerable|success`, `.alert-status.<status>` in style.css).
- Scanner ingest: DRAINER → victim `drained` + hacker `hacker`; SUSPICIOUS → counterparty `suspicious`.
- Hacker association (migration 006): `wallets.associated_hacker` + `associated_reason` flag wallets that sent funds INCOMING to a hacker/drainer address. Scanner findings carry `exposed_addresses`; IngestScanFinding calls `repository.MarkAssociatedHacker` — unknown senders are inserted with status `unknown`, existing statuses are never overridden (association is a flag, not a danger verdict). Report response and tx-tree nodes expose the flag; frontend shows a 🕸️ badge (TxTreeNode) and a banner (ReportView).
- Tests: internal/models/models_test.go validates catalog/validation sync; internal/handlers/scanner_test.go TestIngestScanFinding_MarksAssociated covers the association flow.

## Data conventions
- DB access supports postgres/mysql/sqlite; all queries use `?` placeholders and per-dialect upserts (see UpsertUserNonce).
- `check_history.wallet_address` doubles as report-access key: authenticated wallet address, or `ip:<ip>` for anonymous users (repository.AnonymousRequesterPrefix). Anonymous rows older than 24h are deleted by ReportCleanupService, which implements report expiry (models.AnonymousReportTTL).
- `wallets.reason`/`wallets.source` (migration 004) feed the report endpoint; `leaked_keys` (migration 003) supplies leak metadata — never expose `key_value`.
- Runtime schema (repository.InitSchema) does not create `leaked_keys`; report code tolerates missing table.

## Report endpoint
- `GET /api/report?address&chain` — access only after same requester ran `/api/check`; 403 REPORT_NOT_AVAILABLE / REPORT_EXPIRED, 404 when not in DB. Transaction tree comes from indexed scan_findings (deterministic), child statuses: DB status else heuristic unknown/potential_hacker.
- Public sharing without schema changes: share token = UUID of [8-byte check_history.id][8-byte HMAC-SHA256(auth.JWTSecret, "report-share-v1|id")]. `POST /api/report/share` (RequireAuth) mints it, `GET /api/report/shared/:id` serves publicly. Only authenticated users can mint; revoking = deleting the check_history row (never happens for auth rows). See internal/handlers/report_share.go.
- Report transaction tree (buildTxTree in handlers/report.go) is built from indexed scan_findings: each node aggregates that wallet's own findings (tx_count/amount), children = counterparties grouped per address with status from DB or heuristic unknown/potential_hacker; per-node caps reportTreeDepth/reportTreeMaxNodes/reportTreeMaxChildren/reportTreeFindingsCap. Empty when no findings exist.
- Frontend: `/report/:id` (public) and `/report?address&chain` (owner view) both render ReportView; "Make public & share" only for connected wallets, anon sees a lock notice.
- Tests: `internal/handlers/report_test.go` runs the real handler against real SQLite (TempDir), seeds via repository.CreateWallet + raw SQL for leaked_keys (migration 003 table, not in InitSchema), backdates check_history with UTC "2006-01-02 15:04:05" strings. CheckWallet records history in a goroutine — poll GetLastReportAccess in tests.
- Go toolchain in this sandbox may be missing; install to /tmp/go and export PATH=/tmp/go/bin, GOPATH=$HOME/gopath.

## Drainer scanner (solana_scan.py) integration
- Scanner posts findings to `POST /api/admin/scanner/findings` (X-Admin-Key = ADMIN_API_KEY, or flags `--api-url/--api-key`, env `VAULN_API_URL`/`ADMIN_API_KEY`). Findings stored in `scan_findings` (unique `signature`); DRAINER findings auto-register victim as `drained` and hacker as `hacker` in wallets (source `solana_scan`), and flag every `exposed_addresses` entry (senders funding the operator) as `associated_hacker` via MarkAssociatedHacker.
- Live monitoring: public `GET /api/monitor/findings?limit&after_id&before_id` (after_id = incremental ascending polling, before_id = older pages for "load more") and `GET /api/monitor/stats`; frontend page `/monitor` (MonitorView.vue) polls every 4s.
- User drainer reports: `POST /api/drainer-reports` requires a one-time SVG captcha from `GET /api/captcha` (services.CaptchaService, in-memory, 10min TTL; `Answer(id)` is a test hook). Stored in `drainer_reports`, forwarded to Telegram via services.TelegramService (TELEGRAM_BOT_TOKEN/TELEGRAM_CHAT_ID); delivery flag `telegram_sent`. Frontend page `/report-drainer` (ReportDrainerView.vue).
- Report endpoint includes `evidence` chain (models.StatusEvidence): registry listing, key leaks, scanner indicators P1..P6 with tx/counterparty/amount. Meta for P-codes lives in handlers/scanner.go (scanIndicatorMeta) — mirror of detect_patterns in solana_scan.py.
- scan-wallet mode resolves the real operator per transaction via extract_parties (sweep destination / takeover new owner / non-victim signer); the scanned wallet is the hacker only when it actually received a sweep transfer. The API ingest also drops findings where hacker == victim (malformed scanner output). The wallet report JSON lists all operator wallets in `operator_wallets` (sweep destinations + takeover owners with swept SOL).
- solana_scan.py whitelist: `solana_programs.json` (~290 known program IDs, sources in file header) auto-loaded at startup (`--programs-file` / env `SOLANA_PROGRAMS_FILE`); embedded KNOWN_PROGRAMS is the fallback. Detectors: P1 account takeover, P2 ≥90% SOL sweep, P3 unknown program, P4 control account, P5 drainer watchlist, P6 signer token sweep (preTokenBalances→0). Verdict DRAINER = P1 | P5+(P2|P6); (P2|P6)+P3 is SUSPICIOUS. P6 requires >=2 mints swept with no SOL compensation (single-token full swaps are legit). Never whitelist addresses from KNOWN_BAD_PROGRAMS.
- Tests: internal/handlers/scanner_test.go reuses setupReportTest (real SQLite); scanner.go holds all new handlers.

## Build variants
- `./build.sh local|dev|prod [--env-file F] [--skip-frontend] [--skip-backend]`: sources values from build/env/<variant>, inlines VITE_API_URL/VITE_SOLANA_CLUSTER into the frontend bundle, cross-compiles cmd/server (GOOS/GOARCH), and assembles build/dist/<variant>/ (frontend, server, generated .env, solana_scan.py + solana_programs.json, migrations). build/dist/ is gitignored; build/env files hold empty secret placeholders — fill before building.
