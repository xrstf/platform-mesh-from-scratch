// Package main is the entrypoint of the Platform Mesh installer CLI.
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/urfave/cli/v3"

	"go.platform-mesh.io/installer/internal/cmd"
)

// version is overridable at build time via -ldflags.
var version = "dev"

func main() {
	app := &cli.Command{
		Name:    "installer",
		Usage:   "prepare Platform Mesh manifests for a Kubernetes cluster",
		Version: version,
		Commands: []*cli.Command{
			cmd.StartCommand(),
			cmd.DeployCommand(),
			cmd.TransferCommand(),
		},
	}

	if err := app.Run(context.Background(), os.Args); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
