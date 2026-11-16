/*
Copyright © 2026 NAME HERE <EMAIL ADDRESS>
*/
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/eldius/cli-telemetry-poc/cmd/cli/cmd"
	"github.com/eldius/cli-telemetry-poc/pkg/setup"
)

func main() {
	//cmd.Execute()
	ctx := context.Background()

	// Initialize tracing
	otelAuthToken := os.Getenv("OPENOBSERVE_AUTH_TOKEN")
	backend := setup.Backend{
		Endpoint: "192.168.0.152:5081",
		AuthHeaders: map[string]string{
			"Authorization": otelAuthToken,
			"organization":  "default",
			"stream":        "default",
		},
		Enabled: true,
	}
	shutdown, err := setup.Setup(ctx, setup.TelemetryOptions{
		Service: setup.Service{
			Name:    "cli-telemetry-poc",
			Version: "0.0.0a",
		},
		Backend: setup.Backends{
			Traces:  backend,
			Metrics: backend,
			Logs:    backend,
		},
	})
	if err != nil {
		fmt.Printf("setup telemetry failed: %v\n", err)
	}
	defer shutdown()

	cmd.Execute(ctx)
}
