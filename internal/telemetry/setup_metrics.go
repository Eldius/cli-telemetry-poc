package telemetry

import (
	"context"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/runtime"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.43.0"
	"google.golang.org/grpc/encoding/gzip"
)

var (
	ErrMeterInitialization     = errors.New("meter initialization")
	ErrMeterConnectionShutdown = errors.New("meter connection shutdown")
)

func EmptyFunc() {}

func SetupMeter(ctx context.Context, cfg Options) (func(), error) {
	exporter, err := newMeterExporter(ctx, cfg)
	if err != nil {
		return EmptyFunc, fmt.Errorf("%w:%w", ErrMeterInitialization, err)
	}
	provider := newMeterProvider(ctx, exporter, cfg)
	otel.SetMeterProvider(provider)
	// Start go runtime metric collection.
	if err := runtime.Start(runtime.WithMinimumReadMemStatsInterval(3 * time.Second)); err != nil {
		log.Fatal(err)
	}
	return func() {
		slog.Debug("Meter exporter shutting down")
		if err := exporter.ForceFlush(ctx); err != nil {
			err = fmt.Errorf("%w:%w", ErrMeterConnectionShutdown, err)
			slog.With("error", err).Warn("Meter exporter shutting down")
		}
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

func newMeterExporter(_ context.Context, cfg Options) (*otlpmetricgrpc.Exporter, error) {
	exporter, err := otlpmetricgrpc.New(context.TODO(),
		otlpmetricgrpc.WithInsecure(), // use http & not https
		otlpmetricgrpc.WithEndpoint(cfg.Backend.Traces.Endpoint),
		otlpmetricgrpc.WithCompressor(gzip.Name),
		otlpmetricgrpc.WithTimeout(10*time.Second),
		otlpmetricgrpc.WithHeaders(cfg.Backend.Traces.AuthHeaders),
	)
	if err != nil {
		return nil, fmt.Errorf("%w:%w", ErrMeterInitialization, err)
	}

	return exporter, nil
}

func newMeterProvider(_ context.Context, exp metric.Exporter, cfg Options) *metric.MeterProvider {
	res := resource.NewWithAttributes(
		semconv.SchemaURL,
		semconv.ServiceNameKey.String(cfg.Service.Name),
		semconv.ServiceVersionKey.String(cfg.Service.Version),
	)

	tp := metric.NewMeterProvider(
		metric.WithResource(res),
		metric.WithReader(metric.NewPeriodicReader(
			exp,
			metric.WithInterval(3*time.Second))),
	)

	return tp
}
