package telemetry

import (
	"context"

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
