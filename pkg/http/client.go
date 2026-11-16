package http

import (
	"net/http"

	inthttp "github.com/eldius/cli-telemetry-poc/internal/http"
)

// NewHTTPClient returns a new instrumented
// HTTP curseforge
func NewHTTPClient() *http.Client {
	return inthttp.NewHTTPClient()
}
