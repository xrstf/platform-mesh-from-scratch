package components

import (
	_ "embed"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

//go:embed images.yaml
var imagesYAML []byte

// Image injection styles.
const (
	// StyleSplit writes registry, repository and tag (and optionally digest) separately.
	StyleSplit = "split"
	// StyleCombined writes a host-qualified repository ("<registry>/<repository>") and a tag.
	StyleCombined = "combined"
)

// DefaultImagePath is where an image's tag is written if no path is configured.
const DefaultImagePath = "image.tag"

// ImageMapping describes how a single OCM image resource is injected into a chart's
// Helm values.
type ImageMapping struct {
	// Component is the OCM component (by reference name) to take the image from. Empty
	// means the component the release belongs to.
	Component string `yaml:"component,omitempty"`
	// Resource is the name of the image resource inside the component.
	Resource string `yaml:"resource,omitempty"`
	// Path points at the image tag inside the Helm values, e.g. "keycloak.operator.image.tag".
	Path string `yaml:"path,omitempty"`
	// Style is either "split" (default) or "combined".
	Style string `yaml:"style,omitempty"`
	// Digest controls whether the image digest is injected as well (only for "split").
	Digest *bool `yaml:"digest,omitempty"`
}

// ResourceName returns the name of the OCM resource, defaulting to "image".
func (m ImageMapping) ResourceName() string {
	if m.Resource == "" {
		return "image"
	}

	return m.Resource
}

// ValuePath returns the path to the image tag as a list of keys.
func (m ImageMapping) ValuePath() []string {
	path := m.Path
	if path == "" {
		path = DefaultImagePath
	}

	return strings.Split(path, ".")
}

// Combined returns whether registry and repository are written as a single value.
func (m ImageMapping) Combined() bool {
	return m.Style == StyleCombined
}

// WithDigest returns whether the image digest should be injected.
func (m ImageMapping) WithDigest() bool {
	if m.Combined() {
		return false // charts using a combined repository never support digests
	}

	return m.Digest == nil || *m.Digest
}

// String returns a human readable description, used in error messages.
func (m ImageMapping) String() string {
	path := m.Path
	if path == "" {
		path = DefaultImagePath
	}

	if m.Component != "" {
		return fmt.Sprintf("%s/%s -> %s", m.Component, m.ResourceName(), path)
	}

	return fmt.Sprintf("%s -> %s", m.ResourceName(), path)
}

type imageFile struct {
	Images map[string][]ImageMapping `yaml:"images"`
}

// ImageMappings returns the built-in image injection configuration, keyed by release name.
func ImageMappings() (map[string][]ImageMapping, error) {
	var f imageFile
	if err := yaml.Unmarshal(imagesYAML, &f); err != nil {
		return nil, fmt.Errorf("invalid built-in image mappings: %w", err)
	}

	for name, mappings := range f.Images {
		for _, mapping := range mappings {
			if style := mapping.Style; style != "" && style != StyleSplit && style != StyleCombined {
				return nil, fmt.Errorf("%s: invalid style %q for image %s", name, style, mapping.ResourceName())
			}
		}
	}

	return f.Images, nil
}
