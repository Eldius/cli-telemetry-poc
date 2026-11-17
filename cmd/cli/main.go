package main

import (
	"context"
	"fmt"
	"os"

	"github.com/eldius/cli-telemetry-poc/cmd/cli/cmd"
	"github.com/eldius/cli-telemetry-poc/pkg/setup"
)

func main() {
	os.Exit(run())
}

// run wires telemetry and executes the root command, returning the process
// exit code. Defers (including the telemetry flush) run before os.Exit.
func run() int {
	ctx := context.Background()
	svc := setup.Service{
		Name:    "cli-telemetry-poc",
		Version: "0.0.0a",
	}
	ini := func() error {
		cmd.Execute(ctx)
		return nil
	}
	if err := setup.MainWrapper(
		ctx,
		svc,
		ini,
		setup.WithConfigFileName("config"),
		setup.WithConfigFileType("yaml"),
		setup.WithConfigFileLookupDirectories("."),
		setup.WithEnvPrefix("cli_telemetry"),
	); err != nil {
		fmt.Printf("setup telemetry failed: %v\n", err)
		return 1
	}
	return 0
}
