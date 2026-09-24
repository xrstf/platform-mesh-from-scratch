package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/urfave/cli/v3"

	"ocm.software/ocm/api/ocm/tools/transfer"
	"ocm.software/ocm/api/ocm/tools/transfer/transferhandler/standard"
	common "ocm.software/ocm/api/utils/misc"

	"go.platform-mesh.io/installer/internal/ocm"
	"go.platform-mesh.io/installer/internal/tui"
)

// TransferCommand copies the entire Platform Mesh component tree into another OCI registry.
//
// NB: This command is a sketch. It performs a recursive, by-value transfer using the OCM
// SDK, but has seen little testing and does not yet offer fine-grained control over
// credentials (those are taken from the usual OCM/Docker configuration files).
func TransferCommand() *cli.Command {
	return &cli.Command{
		Name:  "transfer",
		Usage: "transfer Platform Mesh into your own OCI registry (for airgapped installations)",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "version",
				Usage: "Platform Mesh version to transfer (defaults to the latest released version)",
			},
			&cli.StringFlag{
				Name:  "component",
				Usage: "OCM component to transfer; either a component name or a full OCM reference",
				Value: ocm.DefaultComponent,
			},
			&cli.StringFlag{
				Name:  "repository",
				Usage: "OCM repository hosting the component",
				Value: ocm.DefaultRepository,
			},
			&cli.StringFlag{
				Name:     "to",
				Usage:    "target registry, e.g. \"registry.example.com/platform-mesh\"",
				Required: true,
			},
			&cli.BoolFlag{
				Name:  "prereleases",
				Usage: "consider prerelease versions when determining the latest version",
			},
			&cli.BoolFlag{
				Name:  "overwrite",
				Usage: "overwrite component versions that already exist in the target registry",
			},
			&cli.BoolFlag{
				Name:  "dry-run",
				Usage: "only show what would be transferred",
			},
		},
		Action: runTransfer,
	}
}

func runTransfer(_ context.Context, cmd *cli.Command) error {
	out := tui.NewPrompt()

	client, err := ocm.NewClient(cmd.String("repository"), cmd.String("component"))
	if err != nil {
		return err
	}
	defer client.Close() //nolint:errcheck

	version := cmd.String("version")
	if version == "" {
		version = client.PinnedVersion()
	}

	if version == "" {
		out.Print("%s Determining the latest version of %s …", tui.Cyan("→"), client.Component())

		version, err = client.LatestVersion(cmd.Bool("prereleases"))
		if err != nil {
			return err
		}
	}

	target := cmd.String("to")

	out.Print("%s Transferring %s:%s to %s …", tui.Cyan("→"), tui.Bold(client.Component()), tui.Bold(version), tui.Bold(target))

	if cmd.Bool("dry-run") {
		resolved, err := client.Components(version)
		if err != nil {
			return err
		}

		for _, component := range resolved {
			out.Print("  %s %s:%s", tui.Dim("·"), component.ComponentName, component.Version)
		}

		out.Print("")
		out.Print("%s Dry run, nothing was transferred.", tui.Yellow("!"))

		return nil
	}

	cv, err := client.LookupVersion(version)
	if err != nil {
		return err
	}
	defer cv.Close() //nolint:errcheck

	targetRepo, err := ocm.NewClientForRegistry(target)
	if err != nil {
		return fmt.Errorf("cannot open target registry %q: %w", target, err)
	}
	defer targetRepo.Close() //nolint:errcheck

	options := []transfer.TransferOption{
		standard.Recursive(),        // include all referenced components
		standard.ResourcesByValue(), // copy Helm charts and images, do not just link them
		standard.Overwrite(cmd.Bool("overwrite")),
		transfer.WithPrinter(common.NewPrinter(os.Stdout)),
	}

	if err := transfer.Transfer(cv, targetRepo, options...); err != nil {
		return fmt.Errorf("transfer failed: %w", err)
	}

	out.Print("")
	out.Print("  %s Everything was transferred to %s.", tui.Green("✓"), target)
	out.Print("")
	out.Print("  You can now generate manifests from your own registry:")
	out.Print("")
	out.Print("     %s", tui.Cyan(fmt.Sprintf("installer deploy --repository %s --version %s", target, version)))
	out.Print("")

	return nil
}
