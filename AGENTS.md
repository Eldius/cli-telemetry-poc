# AGENTS.md

Go POC: a Cobra CLI that scrapes CurseForge Minecraft content metadata into local
SQLite + BadgerDB, instrumented end-to-end with OpenTelemetry (OTLP gRPC -> OpenObserve).

## Commands

- Build: `go build ./...` — Test: `go test ./...` (hermetic: httptest + t.TempDir, no external services)
- Makefile targets are only run shortcuts: `make sync` / `make search` = `go run ./cmd/cli ...`
- No lint config, no CI, no codegen. Single module, Go 1.27 (uses `sync.WaitGroup.Go`).

## Runtime requirements

- `sync` hard-fails without `CURSEFORGE_API_KEY`.
- Telemetry endpoint is hardcoded (`192.168.0.152:5081`) in `cmd/cli/main.go`;
  auth comes from `OPENOBSERVE_AUTH_TOKEN`. There is no config file mechanism —
  change backend settings in main.go.
- Local data lives in `.data/` (gitignored, can grow to hundreds of MB): `mods.db` (SQLite) + `badger.db`.

## Architecture

- Entrypoint `cmd/cli/main.go`: `os.Exit(run())` — `run()` calls `pkg/setup.Setup`
  (OTel tracer/meter/logger), defers the flush, and returns `cmd.Execute(ctx)`'s exit code.
- `pkg/*` are thin public facades over `internal/*`: `pkg/setup` -> `internal/telemetry`,
  `pkg/http` -> `internal/http` (otelhttp + request/response body logging RoundTripper).
  Add instrumentation in `internal/`, re-export in `pkg/` if needed.
- Per-command spans come from `PersistentPreRunE`/`PersistentPostRunE` on the root command
  (`internal/telemetry/span.go`); errors should be returned via `pkg/telemetry.RecordErrorAndClose`.
- `client/curseforge`: API client (rate limit + retry), `scraper/` (sync orchestration),
  `store/` (storage). Scraper syncs around the CurseForge 10k search-result cap by
  recursively partitioning queries (version -> loader -> category); watermark/sync-state
  lives only in SQLite.

## Gotchas

- **Dual-write storage**: every scraper upsert writes to BOTH `SQLiteStore` and `BadgerStore`.
  Changes to persisted data must update both paths (and both test suites).
- **Never call `os.Exit` below `main`** (commands, `setup.CobraCmdExecute`, etc.) — it
  skips the deferred telemetry flush. Propagate errors/exit codes up to `run()` instead.
  Note `RecordErrorAndClose` also ends the command span early — don't add spans after it.
- The HTTP user-agent in `client/curseforge` still says `minecraft-mods-db` (copied from
  the parent project); the real module/binary is `cli-telemetry-poc`.
- viper in go.mod is only a transitive dep of `github.com/eldius/initial-config-go`;
  this repo's own code does not use it — don't assume config loading exists.
- `README.md` contains a live-looking OpenObserve Basic auth token — treat as a secret, don't copy it around.
- SQLite store uses CGO-free `modernc.org/sqlite` via `pkg/db` (otelsql-instrumented);
  schema is embedded from `client/curseforge/store/schema.sql` — edit that file, not a migration.
