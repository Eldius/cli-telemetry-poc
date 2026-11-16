package telemetry

import (
	"context"
	"log/slog"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

// RecordError records an error in the current span associated with the provided context.
func RecordError(ctx context.Context, err error) {
	span := trace.SpanFromContext(ctx)
	span.RecordError(err)
}

// RecordErrorAndClose records an error in the current span associated with the provided context and closes the span.
func RecordErrorAndClose(ctx context.Context, err error, options ...trace.SpanEndOption) error {
	span := trace.SpanFromContext(ctx)
	span.RecordError(err)
	span.End(options...)
	return err
}

// NewInt64CounterMeter returns a new Int64 counter meter
func NewInt64CounterMeter(metricName string) metric.Int64Counter {
	meterProvider := otel.GetMeterProvider().Meter("cli-telemetry-poc")
	counter, err := meterProvider.Int64Counter(metricName)
	if err != nil {
		slog.With("error", err).Warn("Failed to create meter")
	}

	return counter
}
