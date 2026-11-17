package setup

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/eldius/cli-telemetry-poc/internal/telemetry"
	"github.com/spf13/cobra"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

type TelemetryOptions struct {
	Service Service  `json:"service" yaml:"service" mapstructure:"service"`
	Backend Backends `json:"backend" yaml:"backend" mapstructure:"backend"`
}

type Backends struct {
	Traces  Backend `json:"traces" yaml:"traces" mapstructure:"traces"`
	Metrics Backend `json:"metrics" yaml:"metrics" mapstructure:"metrics"`
	Logs    Backend `json:"logs" yaml:"logs" mapstructure:"logs"`
}

type Service struct {
	Name    string `json:"name" yaml:"name" mapstructure:"name"`
	Version string `json:"version" yaml:"version" mapstructure:"version"`
}

type Backend struct {
	Endpoint    string            `json:"endpoint" yaml:"endpoint" mapstructure:"endpoint"`
	AuthHeaders map[string]string `json:"auth_headers" yaml:"auth_headers" mapstructure:"auth_headers"`
	Enabled     bool              `json:"enabled" yaml:"enabled" mapstructure:"enabled"`
}

func Setup(ctx context.Context, cfg TelemetryOptions) (ShutdowwnFunc, error) {

	internalOpts := toInternalConfig(cfg)
	tracerShutdown, err := telemetry.SetupTracer(ctx, internalOpts)
	if err != nil {
		return telemetry.EmptyFunc, err
	}

	meterShutdowwn, err := telemetry.SetupMeter(ctx, internalOpts)
	if err != nil {
		return telemetry.EmptyFunc, err
	}

	loggerShutdown, err := telemetry.SetupLogger(ctx, internalOpts)
	if err != nil {
		return telemetry.EmptyFunc, err
	}

	return func() {
		var wg sync.WaitGroup

		wg.Go(tracerShutdown)
		wg.Go(meterShutdowwn)
		wg.Go(loggerShutdown)

		wg.Wait()
	}, nil
}

type ShutdowwnFunc func()

// CobraCmdExecute runs rootCmd wrapped in a root span and returns the
// process exit code. It never calls os.Exit, so the caller can flush
// telemetry before terminating.
func CobraCmdExecute(ctx context.Context, rootCmd *cobra.Command) int {
	tracer := otel.GetTracerProvider().Tracer("cli-telemetry-poc")
	execName := filepath.Base(os.Args[0])
	pid := os.Getpid()
	spanCtx, span := tracer.Start(
		ctx,
		execName,
		trace.WithSpanKind(trace.SpanKindInternal),
		trace.WithAttributes(
			attribute.String("process.executable.name", execName),
			attribute.Int("process.pid", pid),
			attribute.StringSlice("process.command_args", os.Args[1:]),
			attribute.String("process.executable.path", os.Args[0]),
			attribute.String("process.command", strings.Join(os.Args, " ")),
			attribute.String("otel.span.parent.origin", "none"),
			attribute.String("service.name", "cli-telemetry-poc"),
		),
	)
	defer span.End()

	if err := rootCmd.ExecuteContext(spanCtx); err != nil {
		span.SetStatus(codes.Error, err.Error())
		span.SetAttributes(attribute.Int("process.exit.code", 1))
		span.SetAttributes(attribute.String("error.type", "1"))
		return 1
	}

	return 0
}

func toInternalConfig(opts TelemetryOptions) telemetry.Options {
	return telemetry.Options{
		Service: telemetry.Service{
			Name:    opts.Service.Name,
			Version: opts.Service.Version,
		},
		Backend: telemetry.Backends{
			Traces: telemetry.Backend{
				Endpoint:    opts.Backend.Traces.Endpoint,
				AuthHeaders: opts.Backend.Traces.AuthHeaders,
				Enabled:     opts.Backend.Traces.Enabled,
			},
			Metrics: telemetry.Backend{
				Endpoint:    opts.Backend.Metrics.Endpoint,
				AuthHeaders: opts.Backend.Metrics.AuthHeaders,
				Enabled:     opts.Backend.Metrics.Enabled,
			},
			Logs: telemetry.Backend{
				Endpoint:    opts.Backend.Logs.Endpoint,
				AuthHeaders: opts.Backend.Logs.AuthHeaders,
				Enabled:     opts.Backend.Logs.Enabled,
			},
		},
	}
}
