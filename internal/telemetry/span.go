package telemetry

import (
	"context"
	"log/slog"
	"os"
	"time"

	"github.com/spf13/cobra"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.43.0"
	"go.opentelemetry.io/otel/trace"
)

func PersistentPreRunE(cmd *cobra.Command, args []string) error {
	start := time.Now()

	ctx := cmd.Context()

	if tracer := otel.GetTracerProvider(); tracer != nil {
		tracer := otel.GetTracerProvider().Tracer("main")
		tracer.Start(cmd.Context(), cmd.Name(), trace.WithSpanKind(trace.SpanKindClient))
		spanCtx, span := tracer.Start(ctx, cmd.Name(), trace.WithSpanKind(trace.SpanKindClient))
		span.SetAttributes(
			attribute.String("process.executable.name", cmd.Root().Name()),
			attribute.Int("process.pid", os.Getpid()),
			attribute.String("cmd_name", cmd.Name()),
			attribute.StringSlice("process.command_args", args),
			attribute.String("process.executable.path", cmd.CommandPath()),
			attribute.StringSlice("aliases", cmd.Aliases),
			attribute.String("called_as", cmd.CalledAs()),
		)
		ctx = spanCtx
		cmd.SetContext(context.WithValue(ctx, tracingKey, &tracingData{span: span, start: start}))
	}

	return nil
}

func PersistentPostRunE(cmd *cobra.Command, args []string) error {
	var isTracing bool
	var runningTime time.Duration

	if data, ok := cmd.Context().Value(tracingKey).(*tracingData); ok && data != nil {
		if data.span != nil {
			isTracing = true
			data.span.SetStatus(codes.Ok, "")
			data.span.SetAttributes(
				attribute.Int("process.exit.code", 0),
			)
		}
		defer func() {
			if data.span != nil {
				data.span.End()
			}
		}()
		runningTime = time.Since(data.start)
		slog.With("cmd", map[string]any{
			"cmd_name":     cmd.Name(),
			"cmd_args":     args,
			"is_recording": isTracing,
			"running_time": runningTime.String(),
		}).Debug("stopping trace")

	}
	return nil
}

var defaultResources *resource.Resource

func getDefaultResources(cfg Options) *resource.Resource {
	if defaultResources == nil {
		defaultResources = resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceNameKey.String(cfg.Service.Name),
			semconv.ServiceVersionKey.String(cfg.Service.Version),
		)
	}
	return defaultResources
}
