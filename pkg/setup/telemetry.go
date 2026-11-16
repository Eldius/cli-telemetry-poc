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
	Service Service
	Backend Backends
}

type Backends struct {
	Traces  Backend
	Metrics Backend
	Logs    Backend
}

type Service struct {
	Name    string
	Version string
}

type Backend struct {
	Endpoint    string
	AuthHeaders map[string]string
	Enabled     bool
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

func CobraCmdExecute(ctx context.Context, rootCmd *cobra.Command) {
	if tracer := otel.GetTracerProvider().Tracer("cli-telemetry-poc"); tracer != nil {
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

		if err := rootCmd.ExecuteContext(spanCtx); err != nil {
			span.SetStatus(codes.Error, err.Error())
			span.SetAttributes(attribute.Int("process.exit.code", 1))
			span.SetAttributes(attribute.String("error.type", "1"))
			span.End()
			os.Exit(1)
		}

		span.End()
	}
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
