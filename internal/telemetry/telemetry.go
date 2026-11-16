package telemetry

import (
	"time"

	"go.opentelemetry.io/otel/trace"
)

type tracingContextKey string

const tracingKey tracingContextKey = "tracing"

type tracingData struct {
	span  trace.Span
	start time.Time
}

type Options struct {
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
