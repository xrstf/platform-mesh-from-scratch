package values

import (
	"bytes"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestLoadSingleFile(t *testing.T) {
	set, err := Load("testdata/single/values.yaml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	components := set.Components()
	if len(components) != 2 || components[0] != "openfga" || components[1] != "traefik" {
		t.Fatalf("expected [openfga traefik], got %v", components)
	}

	if set.For("does-not-exist") != nil {
		t.Error("expected nil for unknown component")
	}

	encoded := encode(t, set.For("traefik"))
	if !strings.Contains(encoded, "# keep this in sync") {
		t.Errorf("expected comments to survive, got:\n%s", encoded)
	}

	if !strings.Contains(encoded, "clusterIP: 10.0.0.1") {
		t.Errorf("expected values to survive, got:\n%s", encoded)
	}
}

func TestLoadDirectory(t *testing.T) {
	set, err := Load("testdata/dir")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	components := set.Components()
	if len(components) != 2 || components[0] != "openfga" || components[1] != "traefik" {
		t.Fatalf("expected [openfga traefik], got %v", components)
	}

	encoded := encode(t, set.For("openfga"))
	if !strings.Contains(encoded, "# a comment") || !strings.Contains(encoded, "replicaCount: 3") {
		t.Errorf("unexpected values:\n%s", encoded)
	}
}

func TestLoadNothing(t *testing.T) {
	set, err := Load("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(set.Components()) != 0 {
		t.Errorf("expected no components, got %v", set.Components())
	}
}

func TestLoadMissingPath(t *testing.T) {
	if _, err := Load("testdata/does-not-exist"); err == nil {
		t.Error("expected an error for a non-existing path")
	}
}

func encode(t *testing.T, node *yaml.Node) string {
	t.Helper()

	if node == nil {
		t.Fatal("expected values, got nil")
	}

	var buf bytes.Buffer

	encoder := yaml.NewEncoder(&buf)
	encoder.SetIndent(2)

	if err := encoder.Encode(node); err != nil {
		t.Fatalf("cannot encode node: %v", err)
	}

	if err := encoder.Close(); err != nil {
		t.Fatalf("cannot encode node: %v", err)
	}

	return buf.String()
}
