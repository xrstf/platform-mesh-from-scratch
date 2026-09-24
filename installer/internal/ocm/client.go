// Package ocm wraps the OCM SDK with the few operations the installer needs:
// resolving the latest version of a component, walking the component tree and
// extracting the Helm charts from it.
package ocm

import (
	"fmt"
	"sort"
	"strings"

	"github.com/Masterminds/semver/v3"

	"ocm.software/ocm/api/oci"
	ocmapi "ocm.software/ocm/api/ocm"
	"ocm.software/ocm/api/ocm/compdesc"
	metav1 "ocm.software/ocm/api/ocm/compdesc/meta/v1"
	"ocm.software/ocm/api/ocm/extensions/accessmethods/ociartifact"
	"ocm.software/ocm/api/ocm/extensions/repositories/ocireg"
)

const (
	// DefaultRepository is the OCM repository that hosts the Platform Mesh components.
	DefaultRepository = "ghcr.io/platform-mesh"
	// DefaultComponent is the root component of a Platform Mesh installation.
	DefaultComponent = "github.com/platform-mesh/platform-mesh"

	resourceTypeHelmChart = "helmChart"
)

// Chart describes a single Helm chart that is part of a component.
type Chart struct {
	// Name is the name the chart is known as inside Platform Mesh, e.g. "traefik-crds".
	Name string
	// Component is the OCM component the chart originates from.
	Component string
	// Version is the chart version.
	Version string
	// RepositoryURL is the OCI repository of the chart, without scheme, e.g. "ghcr.io/foo/charts/bar".
	RepositoryURL string
	// Tag is the OCI tag of the chart (can be empty if only a digest is known).
	Tag string
	// Digest is the OCI digest of the chart (can be empty).
	Digest string
}

// Component is one of the direct children of the Platform Mesh root component.
type Component struct {
	// Name is the reference name inside the root component, e.g. "account-operator".
	Name string
	// ComponentName is the full OCM component name.
	ComponentName string
	// Version is the OCM component version.
	Version string
	// Charts are all Helm charts found in this component (usually exactly one).
	Charts []Chart
}

// Client talks to an OCM repository.
type Client struct {
	ctx       ocmapi.Context
	repo      ocmapi.Repository
	component string
	version   string
}

// NewClient opens the OCM repository for the given component reference. The reference can either
// be a full OCM uniform reference ("ghcr.io/platform-mesh//github.com/platform-mesh/platform-mesh"),
// or a plain component name, in which case the given repository is used.
func NewClient(repository, component string) (*Client, error) {
	ctx := ocmapi.DefaultContext()

	ref := component
	if !strings.Contains(component, "//") {
		ref = strings.TrimSuffix(repository, "/") + "//" + normalizeComponent(component)
	}

	spec, err := ocmapi.ParseRef(ref)
	if err != nil {
		return nil, fmt.Errorf("invalid component reference %q: %w", ref, err)
	}

	repoSpec, err := ctx.MapUniformRepositorySpec(&spec.UniformRepositorySpec)
	if err != nil {
		return nil, fmt.Errorf("cannot determine OCM repository for %q: %w", ref, err)
	}

	repo, err := ctx.RepositoryForSpec(repoSpec)
	if err != nil {
		return nil, fmt.Errorf("cannot open OCM repository for %q: %w", ref, err)
	}

	version := ""
	if spec.Version != nil {
		version = *spec.Version
	}

	return &Client{
		ctx:       ctx,
		repo:      repo,
		component: spec.Component,
		version:   version,
	}, nil
}

// NewClientForRegistry opens an OCM repository (without pinning a component), used as
// mirror target.
func NewClientForRegistry(registry string) (ocmapi.Repository, error) {
	ctx := ocmapi.DefaultContext()

	host, subPath, _ := strings.Cut(strings.TrimPrefix(strings.TrimPrefix(registry, "oci://"), "https://"), "/")

	return ctx.RepositoryForSpec(ocireg.NewRepositorySpec(host, &ocireg.ComponentRepositoryMeta{
		SubPath: subPath,
	}))
}

// Close releases the repository.
func (c *Client) Close() error {
	return c.repo.Close()
}

// Component returns the (normalized) component name this client works on.
func (c *Client) Component() string {
	return c.component
}

// PinnedVersion returns the version that was part of the component reference (can be empty).
func (c *Client) PinnedVersion() string {
	return c.version
}

// LatestVersion determines the most recent version of the root component. Prereleases
// (like "0.6.0-build.2") are ignored unless allowPrereleases is true.
func (c *Client) LatestVersion(allowPrereleases bool) (string, error) {
	comp, err := c.repo.LookupComponent(c.component)
	if err != nil {
		return "", fmt.Errorf("cannot lookup component %q: %w", c.component, err)
	}
	defer comp.Close()

	versions, err := comp.ListVersions()
	if err != nil {
		return "", fmt.Errorf("cannot list versions of component %q: %w", c.component, err)
	}

	var parsed []*semver.Version
	for _, v := range versions {
		sv, err := semver.NewVersion(v)
		if err != nil {
			continue // ignore non-semver versions
		}
		if sv.Prerelease() != "" && !allowPrereleases {
			continue
		}
		parsed = append(parsed, sv)
	}

	if len(parsed) == 0 {
		return "", fmt.Errorf("component %q has no usable versions", c.component)
	}

	sort.Sort(semver.Collection(parsed))

	return parsed[len(parsed)-1].Original(), nil
}

// Components returns all direct child components of the root component in the given version,
// including the Helm charts each of them provides.
func (c *Client) Components(version string) ([]Component, error) {
	cd, err := c.descriptor(c.component, version)
	if err != nil {
		return nil, err
	}

	components := make([]Component, 0, len(cd.References))
	for _, ref := range cd.References {
		charts, err := c.charts(ref.Name, ref.ComponentName, ref.Version, 0)
		if err != nil {
			return nil, fmt.Errorf("cannot resolve charts of component %s: %w", ref.ComponentName, err)
		}

		components = append(components, Component{
			Name:          ref.Name,
			ComponentName: ref.ComponentName,
			Version:       ref.Version,
			Charts:        charts,
		})
	}

	sort.Slice(components, func(i, j int) bool {
		return components[i].Name < components[j].Name
	})

	return components, nil
}

// LookupVersion returns the component version access for the given version, used by the
// mirror command.
func (c *Client) LookupVersion(version string) (ocmapi.ComponentVersionAccess, error) {
	return c.repo.LookupComponentVersion(c.component, version)
}

func (c *Client) descriptor(component, version string) (*compdesc.ComponentDescriptor, error) {
	cv, err := c.repo.LookupComponentVersion(component, version)
	if err != nil {
		return nil, fmt.Errorf("cannot lookup %s:%s: %w", component, version, err)
	}
	defer cv.Close()

	return cv.GetDescriptor().Copy(), nil
}

// charts collects all Helm charts of a component. Components either contain the chart
// resources directly (like cert-manager or traefik) or they point to a dedicated
// "chart" component (like account-operator -> helm-charts/account-operator).
func (c *Client) charts(name, component, version string, depth int) ([]Chart, error) {
	if depth > 2 {
		return nil, nil
	}

	cd, err := c.descriptor(component, version)
	if err != nil {
		return nil, err
	}

	var charts []Chart

	for _, res := range cd.Resources {
		if !isHelmChartType(res.Type) {
			continue
		}

		spec, err := c.ctx.AccessSpecForSpec(res.Access)
		if err != nil {
			return nil, fmt.Errorf("cannot decode access of resource %q: %w", res.Name, err)
		}

		artifact, ok := spec.(*ociartifact.AccessSpec)
		if !ok {
			continue // not an OCI artifact, nothing we can turn into an OCIRepository
		}

		repository, tag, digest, err := splitImageReference(artifact.ImageReference)
		if err != nil {
			return nil, fmt.Errorf("resource %q: %w", res.Name, err)
		}

		if digest == "" {
			digest = resourceDigest(res.Digest)
		}

		charts = append(charts, Chart{
			Name:          chartName(name, res.Name),
			Component:     component,
			Version:       res.Version,
			RepositoryURL: repository,
			Tag:           tag,
			Digest:        digest,
		})
	}

	if len(charts) > 0 {
		return charts, nil
	}

	for _, ref := range cd.References {
		if !isChartReference(ref.Name) {
			continue
		}

		sub, err := c.charts(chartName(name, ref.Name), ref.ComponentName, ref.Version, depth+1)
		if err != nil {
			return nil, err
		}

		charts = append(charts, sub...)
	}

	return charts, nil
}

// resourceDigest turns the digest information of a component descriptor resource into an
// OCI digest. For resources that are OCI artifacts, the digest of the (normalized) resource
// is the digest of its manifest, which is exactly what Flux expects.
func resourceDigest(digest *metav1.DigestSpec) string {
	if digest == nil || digest.Value == "" {
		return ""
	}

	if !strings.EqualFold(digest.HashAlgorithm, "SHA-256") && !strings.EqualFold(digest.HashAlgorithm, "sha256") {
		return "" // anything else is not a valid OCI digest
	}

	return "sha256:" + digest.Value
}

// isHelmChartType matches the Helm chart resource type, with or without a type version
// suffix ("helmChart", "helmChart/v1", …).
func isHelmChartType(resourceType string) bool {
	base, _, _ := strings.Cut(resourceType, "/")

	return base == resourceTypeHelmChart
}

// isChartReference decides whether it's worth descending into a component reference.
// References to image components are of no interest to the installer.
func isChartReference(name string) bool {
	lower := strings.ToLower(name)

	return strings.Contains(lower, "chart") || strings.Contains(lower, "crd")
}

// chartName builds the name of a chart based on the component it belongs to and the name of
// the resource/reference inside the component. The main chart of a component is simply named
// after the component ("chart" resource in the account-operator component becomes
// "account-operator"), while additional charts are suffixed ("crds" resource in the traefik
// component becomes "traefik-crds").
func chartName(component, resource string) string {
	lower := strings.ToLower(resource)
	if lower == "chart" || lower == "charts" || lower == component {
		return component
	}

	if strings.HasPrefix(lower, component+"-") {
		return lower
	}

	return component + "-" + lower
}

// splitImageReference turns "ghcr.io/foo/bar:1.2.3@sha256:deadbeef" into its components.
func splitImageReference(imageRef string) (repository string, tag string, digest string, err error) {
	ref, err := oci.ParseRef(imageRef)
	if err != nil {
		return "", "", "", fmt.Errorf("invalid image reference %q: %w", imageRef, err)
	}

	repository = ref.Host
	if ref.Repository != "" {
		repository += "/" + ref.Repository
	}

	if ref.Tag != nil {
		tag = *ref.Tag
	}

	if ref.Digest != nil {
		digest = string(*ref.Digest)
	}

	return repository, tag, digest, nil
}

// normalizeComponent strips the OCI "component-descriptors/" prefix, which users might
// copy&paste from an OCI registry UI.
func normalizeComponent(component string) string {
	return strings.TrimPrefix(strings.TrimPrefix(component, "/"), "component-descriptors/")
}
