// Package values loads the user-provided Helm values, either from a single YAML file or
// from a directory with one file per component.
//
// Values are kept as YAML nodes, so that comments and key order the user wrote survive
// into the generated manifests.
package values

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// Set holds the Helm values for all components.
type Set struct {
	// Source is the file or directory the values were read from.
	Source string

	values map[string]*yaml.Node
}

// Load reads Helm values from the given path. If the path is a directory, one file per
// component is expected (e.g. "account-operator.yaml"), otherwise the file is expected to
// contain one top-level key per component.
func Load(path string) (*Set, error) {
	if path == "" {
		return &Set{values: map[string]*yaml.Node{}}, nil
	}

	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("cannot read %q: %w", path, err)
	}

	if info.IsDir() {
		return loadDirectory(path)
	}

	return loadFile(path)
}

func loadDirectory(path string) (*Set, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, fmt.Errorf("cannot read directory %q: %w", path, err)
	}

	set := &Set{Source: path, values: map[string]*yaml.Node{}}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		ext := strings.ToLower(filepath.Ext(entry.Name()))
		if ext != ".yaml" && ext != ".yml" {
			continue
		}

		filename := filepath.Join(path, entry.Name())

		node, err := parseFile(filename)
		if err != nil {
			return nil, err
		}

		if node == nil {
			continue // empty file
		}

		component := strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))
		set.values[component] = node
	}

	return set, nil
}

func loadFile(path string) (*Set, error) {
	node, err := parseFile(path)
	if err != nil {
		return nil, err
	}

	set := &Set{Source: path, values: map[string]*yaml.Node{}}
	if node == nil {
		return set, nil
	}

	if node.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("%s: expected a map of component names to Helm values", path)
	}

	for i := 0; i+1 < len(node.Content); i += 2 {
		key := node.Content[i]
		value := node.Content[i+1]

		if value.Tag == "!!null" {
			continue
		}

		// keep comments that were attached to the component key itself
		if key.HeadComment != "" && value.HeadComment == "" {
			value.HeadComment = key.HeadComment
		}

		set.values[key.Value] = value
	}

	return set, nil
}

func parseFile(path string) (*yaml.Node, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("cannot read %q: %w", path, err)
	}

	var doc yaml.Node
	if err := yaml.Unmarshal(content, &doc); err != nil {
		return nil, fmt.Errorf("cannot parse %q: %w", path, err)
	}

	if doc.Kind == 0 || len(doc.Content) == 0 {
		return nil, nil
	}

	return doc.Content[0], nil
}

// For returns the Helm values for the given component, or nil if the user did not
// configure anything for it.
func (s *Set) For(component string) *yaml.Node {
	return s.values[component]
}

// Components returns the sorted list of components the user provided values for.
func (s *Set) Components() []string {
	result := make([]string, 0, len(s.values))
	for name := range s.values {
		result = append(result, name)
	}

	sort.Strings(result)

	return result
}
