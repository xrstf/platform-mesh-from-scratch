// Package yamlutil provides the few YAML node operations the installer needs to inject
// values into user-provided YAML documents without losing their comments or key order.
package yamlutil

import (
	"gopkg.in/yaml.v3"
)

// NewMapping returns an empty YAML mapping node.
func NewMapping() *yaml.Node {
	return &yaml.Node{
		Kind: yaml.MappingNode,
		Tag:  "!!map",
	}
}

// SetString sets a string value at the given path, creating intermediate mappings as
// needed. If the key already exists, only its value is replaced, so any comments the user
// attached to the key survive.
func SetString(root *yaml.Node, path []string, value string) {
	if root == nil || len(path) == 0 {
		return
	}

	parent := ensureMapping(root, path[:len(path)-1])
	if parent == nil {
		return
	}

	key := path[len(path)-1]

	if existing := findValue(parent, key); existing != nil {
		existing.Kind = yaml.ScalarNode
		existing.Tag = "!!str"
		existing.Style = 0
		existing.Value = value
		existing.Content = nil

		return
	}

	parent.Content = append(parent.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key},
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value},
	)
}

// Remove deletes the key at the given path, if it exists.
func Remove(root *yaml.Node, path []string) {
	if root == nil || len(path) == 0 {
		return
	}

	parent := lookupMapping(root, path[:len(path)-1])
	if parent == nil {
		return
	}

	key := path[len(path)-1]

	for i := 0; i+1 < len(parent.Content); i += 2 {
		if parent.Content[i].Value == key {
			parent.Content = append(parent.Content[:i], parent.Content[i+2:]...)
			return
		}
	}
}

// Has returns whether a value exists at the given path.
func Has(root *yaml.Node, path []string) bool {
	if root == nil || len(path) == 0 {
		return false
	}

	parent := lookupMapping(root, path[:len(path)-1])
	if parent == nil {
		return false
	}

	return findValue(parent, path[len(path)-1]) != nil
}

// ensureMapping walks the path and creates missing (or replaces non-mapping) nodes.
func ensureMapping(root *yaml.Node, path []string) *yaml.Node {
	current := root

	if current.Kind != yaml.MappingNode {
		toMapping(current)
	}

	for _, key := range path {
		value := findValue(current, key)

		if value == nil {
			value = NewMapping()
			current.Content = append(current.Content,
				&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key},
				value,
			)
		} else if value.Kind != yaml.MappingNode {
			// the user configured a scalar (or null) where we need a map
			toMapping(value)
		}

		current = value
	}

	return current
}

// lookupMapping walks the path without creating anything.
func lookupMapping(root *yaml.Node, path []string) *yaml.Node {
	current := root

	for _, key := range path {
		if current.Kind != yaml.MappingNode {
			return nil
		}

		current = findValue(current, key)
		if current == nil {
			return nil
		}
	}

	if current.Kind != yaml.MappingNode {
		return nil
	}

	return current
}

func findValue(mapping *yaml.Node, key string) *yaml.Node {
	if mapping.Kind != yaml.MappingNode {
		return nil
	}

	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			return mapping.Content[i+1]
		}
	}

	return nil
}

func toMapping(node *yaml.Node) {
	node.Kind = yaml.MappingNode
	node.Tag = "!!map"
	node.Style = 0
	node.Value = ""
	node.Content = nil
}
