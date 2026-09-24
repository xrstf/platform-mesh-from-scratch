// Package generate turns resolved components plus user-provided Helm values into
// deployable manifests. Right now only Flux (HelmRelease + OCIRepository) is supported.
package generate

import (
	"gopkg.in/yaml.v3"
)

// Release is the installer's format-independent description of "install this chart with
// these values".
type Release struct {
	// Name of the release, e.g. "account-operator".
	Name string
	// Component is the OCM component this release originates from.
	Component string
	// ChartURL is the OCI repository of the Helm chart, without scheme.
	ChartURL string
	// ChartVersion is the version of the Helm chart.
	ChartVersion string
	// ChartDigest is the (optional) OCI digest of the Helm chart.
	ChartDigest string
	// Namespace is where the Flux objects themselves are created.
	Namespace string
	// TargetNamespace is where the Helm chart is installed into.
	TargetNamespace string
	// DependsOn lists other releases that must be ready first.
	DependsOn []string
	// Values are the user's Helm values (can be nil).
	Values *yaml.Node
}
