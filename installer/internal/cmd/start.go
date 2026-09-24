package cmd

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/urfave/cli/v3"

	"go.platform-mesh.io/installer/internal/starter"
	"go.platform-mesh.io/installer/internal/tui"
)

// StartCommand bootstraps a new Platform Mesh configuration.
func StartCommand() *cli.Command {
	return &cli.Command{
		Name:  "start",
		Usage: "interactively create your initial Platform Mesh configuration",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "output",
				Usage: "directory to write the configuration files to",
				Value: "manifests",
			},
			&cli.StringFlag{
				Name:  "base-domain",
				Usage: "base domain of the installation (skips the interactive question)",
			},
			&cli.StringFlag{
				Name:  "traefik-ip",
				Usage: "Service IP for Traefik (skips the interactive question)",
			},
			&cli.StringFlag{
				Name:  "front-proxy-ip",
				Usage: "Service IP for the kcp front-proxy (skips the interactive question)",
			},
		},
		Action: runStart,
	}
}

func runStart(_ context.Context, cmd *cli.Command) error {
	prompt := tui.NewPrompt()

	prompt.Print("")
	prompt.Print("  %s", tui.Bold("Welcome to Platform Mesh!"))
	prompt.Print("")
	prompt.Print("  This wizard asks you a few questions and then writes the initial")
	prompt.Print("  configuration for your installation. Everything it creates is yours:")
	prompt.Print("  keep it in git, edit it, add comments – the installer will never")
	prompt.Print("  rewrite these files.")
	prompt.Print("")

	input := starter.Input{
		BaseDomain:   cmd.String("base-domain"),
		TraefikIP:    cmd.String("traefik-ip"),
		FrontProxyIP: cmd.String("front-proxy-ip"),
		Directory:    cmd.String("output"),
	}

	var err error

	if input.BaseDomain == "" {
		prompt.Print("%s", tui.Dim("  Platform Mesh will be reachable below a base domain of your choosing."))
		prompt.Print("%s", tui.Dim("  Each organization gets its own subdomain, so two levels of wildcards"))
		prompt.Print("%s", tui.Dim("  must resolve to your cluster: for \"mesh.example.com\" you will need"))
		prompt.Print("%s", tui.Dim("  \"*.mesh.example.com\" and \"*.*.mesh.example.com\" in DNS."))
		prompt.Print("%s", tui.Dim("  How you set up DNS is entirely up to you (external-dns works nicely)."))
		prompt.Print("")

		input.BaseDomain, err = prompt.Ask("Base domain", "", tui.ValidateDomain)
		if err != nil {
			return err
		}

		prompt.Print("")
	}

	if input.TraefikIP == "" || input.FrontProxyIP == "" {
		prompt.Print("%s", tui.Dim("  Two Services require a fixed ClusterIP. Both IPs must be inside your"))
		prompt.Print("%s", tui.Dim("  cluster's Service CIDR and must not be in use yet."))
		prompt.Print("")
	}

	if input.TraefikIP == "" {
		input.TraefikIP, err = prompt.Ask("Service IP for Traefik (the LoadBalancer)", "", tui.ValidateIP)
		if err != nil {
			return err
		}
	}

	if input.FrontProxyIP == "" {
		input.FrontProxyIP, err = prompt.Ask("Service IP for the kcp front-proxy", "", tui.ValidateIP)
		if err != nil {
			return err
		}
	}

	prompt.Print("")

	if !cmd.IsSet("output") {
		input.Directory, err = prompt.Ask("Where should the configuration be written to?", input.Directory, tui.ValidateNotEmpty)
		if err != nil {
			return err
		}
	}

	files, err := starter.Write(input)
	if err != nil {
		return err
	}

	prompt.Print("")
	for _, file := range files {
		prompt.Print("  %s %s", tui.Green("✓"), file)
	}

	valuesFile := filepath.Join(input.Directory, starter.HelmValuesFilename)

	prompt.Print("")
	prompt.Print("  %s", tui.Bold("You are all set. What now?"))
	prompt.Print("")
	prompt.Print("  1. Review %s and adjust it to your needs.", tui.Cyan(valuesFile))
	prompt.Print("  2. Generate the manifests for your cluster:")
	prompt.Print("")
	prompt.Print("       %s", tui.Cyan(fmt.Sprintf("installer deploy --values %s", valuesFile)))
	prompt.Print("")

	return nil
}
