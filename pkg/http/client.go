package http

import (
	"net/http"

	"github.com/eldius/cli-telemetry-poc/internal/client"
)

// NewHTTPClient returns a new instrumented
// HTTP curseforge
func NewHTTPClient() *http.Client {
	return client.NewHTTPClient()
}
