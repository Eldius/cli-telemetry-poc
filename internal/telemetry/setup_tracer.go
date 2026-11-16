package telemetry

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"google.golang.org/grpc/encoding/gzip"
)

var (
	ErrTracesInitialization           = errors.New("traces initialization")
	ErrTracesConnectionInitialization = errors.New("traces connection initialization")
)

func SetupTracer(ctx context.Context, cfg Options) (func(), error) {
	exporter, err := newSpanExporter(ctx, cfg)
	if err != nil {
		return EmptyFunc, fmt.Errorf("%w: %w: %w", ErrTracesInitialization, ErrTracesConnectionInitialization, err)
	}

	provider := newTracerProvider(ctx, exporter, cfg)
	otel.SetTracerProvider(provider)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	return func() {
		slog.Debug("Meter exporter shutting down")
		if err := provider.ForceFlush(ctx); err != nil {
			err = fmt.Errorf("%w:%w", ErrMeterConnectionShutdown, err)
			slog.With("error", err).Warn("meter provider shutting down")
		}
		if err := exporter.Shutdown(ctx); err != nil {
			err = fmt.Errorf("%w:%w", ErrMeterConnectionShutdown, err)
			slog.With("error", err).Warn("shutdown metric exporter")
		}
		if err := provider.Shutdown(ctx); err != nil {
			err = fmt.Errorf("%w:%w", ErrMeterConnectionShutdown, err)
			slog.With("error", err).Warn("meter provider shutting down")
		}
	}, nil

}

func newSpanExporter(_ context.Context, cfg Options) (sdktrace.SpanExporter, error) {
	l := slog.Default()

	spanExporter, err := otlptracegrpc.New(context.TODO(),
		otlptracegrpc.WithInsecure(), // use http & not https
		otlptracegrpc.WithEndpoint(cfg.Backend.Traces.Endpoint),
		otlptracegrpc.WithCompressor(gzip.Name),
		otlptracegrpc.WithTimeout(10*time.Second),
		otlptracegrpc.WithHeaders(cfg.Backend.Traces.AuthHeaders),
	)
	if err != nil {
		err = fmt.Errorf("%w: %w: %w", ErrTracesInitialization, ErrTracesConnectionInitialization, err)
		l.With("error", err).Error("failed to create gRPC trace exporter")
		return nil, err
	}

	return spanExporter, nil
}

func newTracerProvider(_ context.Context, exp sdktrace.SpanExporter, cfg Options) *sdktrace.TracerProvider {
	res := getDefaultResources(cfg)

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
		sdktrace.WithResource(res),
		sdktrace.WithBatcher(exp),
	)
	return tp
}
