// Package images injects the image coordinates resolved from OCM into Helm values.
//
// This is what makes transferred installations work: after `installer transfer`, every
// image lives in a different registry, and the charts must be told about it. The component
// descriptor is the source of truth for where an image lives, so the installer writes
// those coordinates into the values of the chart that deploys the image.
package images

import (
	"fmt"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"

	"go.platform-mesh.io/installer/internal/yamlutil"
)

// coordinates are the keys that are written next to the tag.
var coordinates = []string{"registry", "repository", "digest"}

// Injection is a single "write this image into these Helm values" instruction.
type Injection struct {
	// Path points at the image tag, e.g. ["keycloak", "operator", "image", "tag"].
	Path []string
	// Registry is the image host, e.g. "ghcr.io".
	Registry string
	// Repository is the image path inside the registry.
	Repository string
	// Tag is the image tag.
	Tag string
	// Digest is the image digest (can be empty).
	Digest string
	// Combined writes "<registry>/<repository>" into the repository key and omits
	// the registry and digest keys.
	Combined bool
	// WithDigest controls whether the digest is written.
	WithDigest bool
	// Description is used in warnings.
	Description string
}

// Inject applies all injections to the given values, which may be nil. The (possibly
// newly created) values node is returned, together with warnings about injections that
// could not be applied cleanly.
func Inject(values *yaml.Node, injections []Injection) (*yaml.Node, []string) {
	if len(injections) == 0 {
		return values, nil
	}

	if values == nil {
		values = yamlutil.NewMapping()
	}

	warnings := []string{}

	for _, injection := range injections {
		if len(injection.Path) == 0 {
			continue
		}

		parent := injection.Path[:len(injection.Path)-1]
		leaf := injection.Path[len(injection.Path)-1]

		// A path that itself ends in a coordinate name holds the full image reference,
		// so writing siblings would corrupt the values.
		if slices.Contains(coordinates, leaf) {
			yamlutil.SetString(values, injection.Path, injection.Tag)

			warnings = append(warnings, fmt.Sprintf(
				"%s: path ends in %q, only that value was set", injection.Description, leaf))

			continue
		}

		// Keys are written in a fixed order, so that repeated runs produce identical files.
		coords := injection.values()

		setOrRemove(values, parent, "registry", coords["registry"])
		setOrRemove(values, parent, "repository", coords["repository"])
		yamlutil.SetString(values, injection.Path, injection.Tag)
		setOrRemove(values, parent, "digest", coords["digest"])
	}

	return values, warnings
}

// setOrRemove writes a coordinate next to the tag, or removes it when it does not apply.
// Stale coordinates must go: a digest left over from the user's values (or a previous
// Platform Mesh version) takes precedence over the tag and would pull the wrong image.
func setOrRemove(values *yaml.Node, parent []string, key, value string) {
	path := append(slices.Clone(parent), key)

	if value == "" {
		yamlutil.Remove(values, path)
		return
	}

	yamlutil.SetString(values, path, value)
}

// Reference returns the image reference this injection writes, for logging.
func (i Injection) Reference() string {
	ref := strings.TrimPrefix(i.Registry+"/"+i.Repository, "/")
	if i.Tag != "" {
		ref += ":" + i.Tag
	}

	if i.WithDigest && i.Digest != "" {
		ref += "@" + i.Digest
	}

	return ref
}

// values returns the coordinates to write next to the tag; an empty value means the key
// must be removed.
func (i Injection) values() map[string]string {
	result := map[string]string{
		"registry":   "",
		"repository": "",
		"digest":     "",
	}

	if i.Combined {
		result["repository"] = strings.TrimPrefix(i.Registry+"/"+i.Repository, "/")

		return result
	}

	result["registry"] = i.Registry
	result["repository"] = i.Repository

	if i.WithDigest {
		result["digest"] = i.Digest
	}

	return result
}
