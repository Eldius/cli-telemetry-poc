# AGENTS.md

Go POC: a Cobra CLI that scrapes CurseForge Minecraft content metadata into local
SQLite + BadgerDB, instrumented end-to-end with OpenTelemetry (OTLP gRPC -> OpenObserve).
A second `search` command demos a Firecrawl web-search client (`client/firecrawl`,
currently sends no auth header); `fetch` is an empty stub.

## Commands

- Build: `go build ./...` — Test: `go test ./...` (hermetic: httptest + t.TempDir, no external services)
- Makefile targets are only run shortcuts: `make sync` / `make search` = `go run ./cmd/cli ...`
  with `OPENOBSERVE_AUTH_TOKEN` passed through.
- No lint config, no CI, no codegen. Single module, Go 1.27 (uses `sync.WaitGroup.Go`).

## Runtime requirements

- `sync` hard-fails without `CURSEFORGE_API_KEY` (`ErrAPIKeyRequired`).
- Telemetry backends are configured in `config.yaml` (`telemetry.backends.{traces,metrics,logs}`,
  endpoint `192.168.0.152:5081`, auth header `${OPENOBSERVE_AUTH_TOKEN}`). Loaded via viper in
  `pkg/setup` (lookup dirs: `.`, `~/.cli-telemetry-poc/`, `$HOME`; env prefix `cli_telemetry`;
  `${VAR}` values are env-expanded at decode). **A missing config file is fatal** (`InitViper`
  error -> exit 1), while a telemetry backend setup failure only prints a warning and the
  command continues uninstrumented.
- Local data lives in `.data/` (gitignored, can grow to hundreds of MB): `mods.db` (SQLite) + `badger.db`.

## Architecture

- Entrypoint `cmd/cli/main.go`: `os.Exit(run())` — `run()` calls `setup.MainWrapper`
  (viper init -> unmarshal `telemetry.backends` -> `pkg/setup.Setup` for tracer/meter/logger,
  shutdown deferred) and returns `cmd.Execute(ctx)`'s exit code. `Execute` delegates to
  `setup.CobraCmdExecute`, which wraps the whole run in a root span with `process.*` attributes.
- `pkg/*` are thin public facades over `internal/*`: `pkg/setup` -> `internal/telemetry`,
  `pkg/http` -> `internal/http` (otelhttp + request/response body logging RoundTripper).
  Add instrumentation in `internal/`, re-export in `pkg/` if needed. Exception: `pkg/telemetry`
  is not a facade — it's standalone span/metric helpers (`RecordError`, `RecordErrorAndClose`,
  `NewInt64CounterMeter`).
- Per-command spans come from `PersistentPreRunE`/`PersistentPostRunE` on the root command
  (`internal/telemetry/span.go`); command errors should be returned via
  `pkg/telemetry.RecordErrorAndClose`.
- `client/curseforge`: API client (rate limit + retry), `scraper/` (sync orchestration),
  `store/` (storage). Scraper syncs around the CurseForge 10k search-result cap by
  recursively partitioning queries (version -> loader -> category); watermark/sync-state
  lives only in SQLite.

## Gotchas

- **Dual-write storage**: every scraper upsert writes to BOTH `SQLiteStore` and `BadgerStore`.
  Changes to persisted data must update both paths (and both test suites).
- **Never call `os.Exit` below `main`** (commands, `setup.CobraCmdExecute`, etc.) — it
  skips the deferred telemetry flush. Propagate errors/exit codes up to `run()` instead.
  Sole exception: `pkg/setup/viper.go` exits if the home dir lookup fails, which happens
  before telemetry init.
- `RecordErrorAndClose` ends the command span early — don't add spans after it.
- The HTTP user-agent in `client/curseforge` still says `minecraft-mods-db` (copied from
  the parent project); the real module/binary is `cli-telemetry-poc`.
- SQLite store uses CGO-free `modernc.org/sqlite` via `pkg/db` (otelsql-instrumented);
  schema is embedded from `client/curseforge/store/schema.sql` — edit that file, not a migration.
