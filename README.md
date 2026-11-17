# cli-telemetry-poc

It's a sample code to study OTEL telemetry
configuration patterns.

## Configuration

Runtime configuration lives in a YAML file named `config` (see `./config.yaml`),
loaded with [viper](https://github.com/spf13/viper) by `pkg/setup`. Lookup
directories, in order: `.` (current dir), `~/.cli-telemetry-poc/`, and `$HOME`.
A missing config file aborts startup.

Environment variables:

- `OPENOBSERVE_AUTH_TOKEN` — referenced from `config.yaml` as
  `${OPENOBSERVE_AUTH_TOKEN}`; `${VAR}` values are env-expanded when the config
  is decoded.
- `CURSEFORGE_API_KEY` — required by the `sync` command (get one at
  <https://console.curseforge.com>).
- Any config key can be overridden via env vars prefixed with `cli_telemetry`
  (dots become underscores, e.g. `CLI_TELEMETRY_TELEMETRY_BACKENDS_TRACES_ENDPOINT`).

## Telemetry

Traces, metrics and logs are exported over OTLP gRPC to an OpenObserve backend,
configured per-signal under `telemetry.backends` in `config.yaml`:

```yaml
telemetry:
  backends:
    traces:
      endpoint: 192.168.0.152:5081
      enabled: true
      auth_headers:
        authorization: "${OPENOBSERVE_AUTH_TOKEN}"
        organization: default
        stream: default
    metrics: # same shape
    logs:    # same shape
```

Each backend can be toggled independently via `enabled` and carries its own
auth headers. If a backend is unreachable at startup, a warning is printed and
the command runs uninstrumented.

How it's wired:

- `cmd/cli/main.go` calls `setup.MainWrapper`, which loads the config, sets up
  the tracer/meter/logger providers (`pkg/setup` over `internal/telemetry`),
  and defers the flush before exiting.
- The whole CLI run is wrapped in a root span (`setup.CobraCmdExecute`) carrying
  `process.*` attributes; each Cobra subcommand gets its own child span via
  `PersistentPreRunE`/`PersistentPostRunE`.
- `pkg/telemetry` offers helpers for command code: `RecordError`,
  `RecordErrorAndClose`, `NewInt64CounterMeter`.
- Outbound HTTP (`pkg/http`, otelhttp + body-logging RoundTripper) and SQL
  (`pkg/db`, otelsql) are auto-instrumented.

## reference links

- [Get Started - Introduction - Firecrawl](https://docs.firecrawl.dev/introduction)
