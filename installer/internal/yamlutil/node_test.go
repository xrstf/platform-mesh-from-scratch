package yamlutil

import (
	"bytes"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func parse(t *testing.T, content string) *yaml.Node {
	t.Helper()

	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(content), &doc); err != nil {
		t.Fatalf("cannot parse test YAML: %v", err)
	}

	if len(doc.Content) == 0 {
		return NewMapping()
	}

	return doc.Content[0]
}

func encode(t *testing.T, node *yaml.Node) string {
	t.Helper()

	var buf bytes.Buffer

	encoder := yaml.NewEncoder(&buf)
	encoder.SetIndent(2)

	if err := encoder.Encode(node); err != nil {
		t.Fatalf("cannot encode: %v", err)
	}

	_ = encoder.Close()

	return buf.String()
}

func TestSetStringCreatesPath(t *testing.T) {
	node := NewMapping()

	SetString(node, []string{"image", "tag"}, "v1.0.0")

	if result := encode(t, node); result != "image:\n  tag: v1.0.0\n" {
		t.Fatalf("unexpected result:\n%s", result)
	}
}

func TestSetStringKeepsComments(t *testing.T) {
	node := parse(t, `# top comment
image:
  # the tag we pinned
  tag: v0.1.0
  pullPolicy: Always
other: value
`)

	SetString(node, []string{"image", "tag"}, "v2.0.0")

	result := encode(t, node)

	if !strings.Contains(result, "# the tag we pinned") || !strings.Contains(result, "# top comment") {
		t.Errorf("expected comments to survive:\n%s", result)
	}

	if !strings.Contains(result, "tag: v2.0.0") {
		t.Errorf("expected the tag to be updated:\n%s", result)
	}

	if !strings.Contains(result, "pullPolicy: Always") || !strings.Contains(result, "other: value") {
		t.Errorf("expected other values to survive:\n%s", result)
	}
}

func TestSetStringQuotesNumbers(t *testing.T) {
	node := NewMapping()

	SetString(node, []string{"tag"}, "18.3")

	if result := encode(t, node); result != "tag: \"18.3\"\n" {
		t.Fatalf("expected the value to stay a string:\n%s", result)
	}
}

func TestSetStringReplacesScalarParent(t *testing.T) {
	node := parse(t, "image: not-a-map\n")

	SetString(node, []string{"image", "tag"}, "v1.0.0")

	if result := encode(t, node); result != "image:\n  tag: v1.0.0\n" {
		t.Fatalf("unexpected result:\n%s", result)
	}
}

func TestRemove(t *testing.T) {
	node := parse(t, "image:\n  tag: v1\n  digest: sha256:abc\n")

	Remove(node, []string{"image", "digest"})

	if result := encode(t, node); result != "image:\n  tag: v1\n" {
		t.Fatalf("unexpected result:\n%s", result)
	}

	// removing something that does not exist is a no-op
	Remove(node, []string{"image", "digest"})
	Remove(node, []string{"nope", "nothing"})
}

func TestHas(t *testing.T) {
	node := parse(t, "image:\n  tag: v1\n")

	if !Has(node, []string{"image", "tag"}) {
		t.Error("expected image.tag to exist")
	}

	if Has(node, []string{"image", "digest"}) {
		t.Error("did not expect image.digest to exist")
	}

	if Has(nil, []string{"image"}) {
		t.Error("did not expect anything in a nil node")
	}
}
