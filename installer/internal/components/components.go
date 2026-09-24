// Package components contains the installer's built-in knowledge about the Platform Mesh
// components: whether they are part of a default installation, into which namespace they
// are deployed and in which order.
package components

import (
	_ "embed"
	"fmt"

	"gopkg.in/yaml.v3"
)

//go:embed components.yaml
var defaultsYAML []byte

// Config describes how a single component is deployed.
type Config struct {
	// Enabled controls whether a HelmRelease is generated for this component. Defaults to true.
	Enabled *bool `yaml:"enabled,omitempty"`
	// TargetNamespace is the namespace the Helm chart is installed into. Defaults to the
	// release namespace.
	TargetNamespace string `yaml:"targetNamespace,omitempty"`
	// DependsOn lists other components that must be ready before this one is installed.
	DependsOn []string `yaml:"dependsOn,omitempty"`
}

// IsEnabled returns whether the component should be deployed.
func (c Config) IsEnabled() bool {
	return c.Enabled == nil || *c.Enabled
}

type file struct {
	Components map[string]Config `yaml:"components"`
}

// Defaults returns the built-in component metadata.
func Defaults() (map[string]Config, error) {
	var f file
	if err := yaml.Unmarshal(defaultsYAML, &f); err != nil {
		return nil, fmt.Errorf("invalid built-in component defaults: %w", err)
	}

	return f.Components, nil
}

// MustDefaults is like Defaults, but panics on error (the data is compiled into the binary,
// so an error means the binary is broken).
func MustDefaults() map[string]Config {
	cfg, err := Defaults()
	if err != nil {
		panic(err)
	}

	return cfg
}
