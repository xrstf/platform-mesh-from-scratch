package cmd

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/urfave/cli/v3"

	"go.platform-mesh.io/installer/internal/components"
	"go.platform-mesh.io/installer/internal/generate"
	"go.platform-mesh.io/installer/internal/images"
	"go.platform-mesh.io/installer/internal/ocm"
	"go.platform-mesh.io/installer/internal/tui"
	"go.platform-mesh.io/installer/internal/values"
)

// DeployCommand generates the manifests for a Platform Mesh installation.
func DeployCommand() *cli.Command {
	return &cli.Command{
		Name:  "deploy",
		Usage: "generate the manifests for your Platform Mesh installation",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "version",
				Usage: "Platform Mesh version to install (defaults to the latest released version)",
			},
			&cli.StringFlag{
				Name:  "component",
				Usage: "OCM component to install; either a component name or a full OCM reference",
				Value: ocm.DefaultComponent,
			},
			&cli.StringFlag{
				Name:  "repository",
				Usage: "OCM repository hosting the component",
				Value: ocm.DefaultRepository,
			},
			&cli.StringFlag{
				Name:    "values",
				Aliases: []string{"f"},
				Usage:   "file or directory with your Helm values",
			},
			&cli.StringFlag{
				Name:  "format",
				Usage: "output format (" + strings.Join(generate.Formats(), ", ") + ")",
				Value: generate.FormatFlux,
			},
			&cli.StringFlag{
				Name:    "output",
				Aliases: []string{"o"},
				Usage:   "directory to write the generated manifests to",
				Value:   "generated",
			},
			&cli.StringFlag{
				Name:  "namespace",
				Usage: "namespace the generated Flux objects live in",
				Value: "platform-mesh-system",
			},
			&cli.StringSliceFlag{
				Name:  "enable",
				Usage: "enable a component that is disabled by default (can be given multiple times)",
			},
			&cli.StringSliceFlag{
				Name:  "disable",
				Usage: "skip a component (can be given multiple times)",
			},
			&cli.BoolFlag{
				Name:  "digests",
				Usage: "pin the OCI digest of each chart in addition to its tag",
				Value: true,
			},
			&cli.BoolFlag{
				Name:  "images",
				Usage: "inject the image locations resolved from OCM into the Helm values",
				Value: true,
			},
			&cli.BoolFlag{
				Name:  "show-images",
				Usage: "list every injected image",
			},
			&cli.BoolFlag{
				Name:  "prereleases",
				Usage: "consider prerelease versions when determining the latest version",
			},
		},
		Action: runDeploy,
	}
}

func runDeploy(_ context.Context, cmd *cli.Command) error {
	out := tui.NewPrompt()

	if format := cmd.String("format"); format != generate.FormatFlux {
		return fmt.Errorf("unsupported format %q, currently only %q is supported", format, generate.FormatFlux)
	}

	userValues, err := values.Load(cmd.String("values"))
	if err != nil {
		return err
	}

	if userValues.Source == "" {
		out.Print("%s No --values given, generating manifests with chart defaults only.", tui.Yellow("!"))
	}

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

	out.Print("%s Using %s in version %s.", tui.Cyan("→"), tui.Bold(client.Component()), tui.Bold(version))
	out.Print("%s Resolving components …", tui.Cyan("→"))

	resolved, err := client.Components(version)
	if err != nil {
		return err
	}

	config, err := components.Defaults()
	if err != nil {
		return err
	}

	imageMappings, err := components.ImageMappings()
	if err != nil {
		return err
	}

	if !cmd.Bool("images") {
		imageMappings = nil
	}

	byName := map[string]ocm.Component{}
	for _, component := range resolved {
		byName[component.Name] = component
	}

	enabled := cmd.StringSlice("enable")
	disabled := cmd.StringSlice("disable")
	namespace := cmd.String("namespace")

	isEnabled := func(name string) bool {
		if slices.Contains(disabled, name) {
			return false
		}

		if slices.Contains(enabled, name) {
			return true
		}

		return config[name].IsEnabled()
	}

	releases := []generate.Release{}
	skipped := []string{}
	usedValues := map[string]bool{}
	usedImages := map[string]bool{}
	warnings := []string{}
	injected := map[string][]images.Injection{}

	for _, component := range resolved {
		if !isEnabled(component.Name) {
			skipped = append(skipped, component.Name)
			continue
		}

		if len(component.Charts) == 0 {
			continue // components without Helm charts (e.g. image-only components)
		}

		for _, chart := range component.Charts {
			if !isEnabled(chart.Name) {
				skipped = append(skipped, chart.Name)
				continue
			}

			chartConfig := config[chart.Name]

			targetNamespace := chartConfig.TargetNamespace
			if targetNamespace == "" {
				targetNamespace = namespace
			}

			usedValues[chart.Name] = true

			injections, injectionWarnings := imageInjections(component, byName, imageMappings[chart.Name], usedImages)
			warnings = append(warnings, injectionWarnings...)

			chartValues, applyWarnings := images.Inject(userValues.For(chart.Name), injections)
			warnings = append(warnings, applyWarnings...)

			injected[chart.Name] = injections

			releases = append(releases, generate.Release{
				Name:            chart.Name,
				Component:       component.ComponentName,
				ChartURL:        chart.RepositoryURL,
				ChartVersion:    chart.Tag,
				ChartDigest:     chart.Digest,
				Namespace:       namespace,
				TargetNamespace: targetNamespace,
				DependsOn:       chartConfig.DependsOn,
				Values:          chartValues,
			})
		}
	}

	// do not point at HelmReleases that are not generated
	names := map[string]bool{}
	for _, release := range releases {
		names[release.Name] = true
	}

	for i, release := range releases {
		releases[i].DependsOn = slices.DeleteFunc(slices.Clone(release.DependsOn), func(dep string) bool {
			return !names[dep]
		})
	}

	writer := generate.NewFluxWriter()
	writer.Digests = cmd.Bool("digests")

	files, err := writer.Write(cmd.String("output"), releases)
	if err != nil {
		return err
	}

	out.Print("")

	for _, release := range releases {
		suffix := ""
		if count := len(injected[release.Name]); count > 0 {
			suffix = tui.Dim(fmt.Sprintf(" +%d image(s)", count))
		}

		out.Print("  %s %-30s %s%s", tui.Green("✓"), release.Name, tui.Dim(release.ChartURL+":"+release.ChartVersion), suffix)

		if cmd.Bool("show-images") {
			for _, injection := range injected[release.Name] {
				out.Print("      %s %s = %s", tui.Dim("·"), tui.Dim(strings.Join(injection.Path[:len(injection.Path)-1], ".")), tui.Dim(injection.Reference()))
			}
		}
	}

	if len(skipped) > 0 {
		out.Print("")
		out.Print("  %s Skipped: %s", tui.Dim("·"), strings.Join(skipped, ", "))
	}

	// warn about values that nothing consumed, this is usually a typo
	for _, name := range userValues.Components() {
		if !usedValues[name] {
			warnings = append(warnings, fmt.Sprintf("values for %q were ignored: no such component in %s:%s", name, client.Component(), version))
		}
	}

	if cmd.Bool("images") {
		warnings = append(warnings, unmappedImages(resolved, isEnabled, usedImages)...)
	}

	if len(warnings) > 0 {
		out.Print("")

		for _, warning := range warnings {
			out.Print("  %s %s", tui.Yellow("!"), warning)
		}
	}

	out.Print("")
	out.Print("  %s manifests written to %s.", tui.Bold(fmt.Sprintf("%d", len(files))), tui.Cyan(cmd.String("output")))
	out.Print("")
	out.Print("  Apply them to your cluster (Flux must be installed):")
	out.Print("")
	out.Print("     %s", tui.Cyan("kubectl apply --recursive --filename "+cmd.String("output")))
	out.Print("")

	return nil
}

// imageInjections turns the configured image mappings of a release into concrete
// injections, by looking up each image in the resolved component tree.
func imageInjections(
	component ocm.Component,
	byName map[string]ocm.Component,
	mappings []components.ImageMapping,
	used map[string]bool,
) ([]images.Injection, []string) {
	injections := []images.Injection{}
	warnings := []string{}

	for _, mapping := range mappings {
		source := component

		if mapping.Component != "" {
			other, exists := byName[mapping.Component]
			if !exists {
				warnings = append(warnings, fmt.Sprintf(
					"%s: component %q does not exist, image not injected", component.Name, mapping.Component))

				continue
			}

			source = other
		}

		image, exists := source.Image(mapping.ResourceName())
		if !exists {
			warnings = append(warnings, fmt.Sprintf(
				"%s: image %q not found in %s, Helm chart defaults are used instead",
				component.Name, mapping.ResourceName(), source.ComponentName))

			continue
		}

		used[source.Name+"/"+image.Name] = true

		injections = append(injections, images.Injection{
			Path:        mapping.ValuePath(),
			Registry:    image.Registry,
			Repository:  image.Repository,
			Tag:         image.Tag,
			Digest:      image.Digest,
			Combined:    mapping.Combined(),
			WithDigest:  mapping.WithDigest(),
			Description: component.Name + ": " + mapping.String(),
		})
	}

	return injections, warnings
}

// unmappedImages reports images that exist in the component descriptor but are not
// injected into any chart. After a Platform Mesh upgrade this points out new images that
// the installer does not know about yet (and which would keep their original registry in
// a transferred setup).
func unmappedImages(resolved []ocm.Component, isEnabled func(string) bool, used map[string]bool) []string {
	unmapped := []string{}

	for _, component := range resolved {
		if !isEnabled(component.Name) {
			continue
		}

		for _, image := range component.Images {
			if !used[component.Name+"/"+image.Name] {
				unmapped = append(unmapped, fmt.Sprintf("%s/%s (%s)", component.Name, image.Name, image.Reference()))
			}
		}
	}

	sort.Strings(unmapped)

	if len(unmapped) == 0 {
		return nil
	}

	return []string{fmt.Sprintf(
		"%d image(s) are not mapped into any Helm values and keep their original location: %s",
		len(unmapped), strings.Join(unmapped, ", "))}
}
