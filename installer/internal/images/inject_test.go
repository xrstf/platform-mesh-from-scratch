package images

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
		return nil
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

func TestInjectSplit(t *testing.T) {
	values, warnings := Inject(nil, []Injection{{
		Path:       []string{"image", "tag"},
		Registry:   "quay.io",
		Repository: "jetstack/cert-manager-controller",
		Tag:        "v1.20.1",
		Digest:     "sha256:deadbeef",
		WithDigest: true,
	}})

	if len(warnings) > 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}

	expected := `image:
  registry: quay.io
  repository: jetstack/cert-manager-controller
  tag: v1.20.1
  digest: sha256:deadbeef
`

	if result := encode(t, values); result != expected {
		t.Fatalf("unexpected result:\n%s", result)
	}
}

func TestInjectSplitWithoutDigest(t *testing.T) {
	values, _ := Inject(parse(t, "image:\n  digest: sha256:stale\n"), []Injection{{
		Path:       []string{"image", "tag"},
		Registry:   "docker.io",
		Repository: "library/traefik",
		Tag:        "v3.7.12",
		Digest:     "sha256:deadbeef",
		WithDigest: false,
	}})

	result := encode(t, values)

	if strings.Contains(result, "digest") {
		t.Fatalf("expected the stale digest to be removed:\n%s", result)
	}

	if !strings.Contains(result, "registry: docker.io") || !strings.Contains(result, "tag: v3.7.12") {
		t.Fatalf("unexpected result:\n%s", result)
	}
}

func TestInjectCombined(t *testing.T) {
	values, _ := Inject(nil, []Injection{{
		Path:       []string{"manager", "collectorImage", "tag"},
		Registry:   "ghcr.io",
		Repository: "open-telemetry/opentelemetry-collector-k8s",
		Tag:        "0.152.1",
		Digest:     "sha256:deadbeef",
		Combined:   true,
	}})

	expected := `manager:
  collectorImage:
    repository: ghcr.io/open-telemetry/opentelemetry-collector-k8s
    tag: 0.152.1
`

	if result := encode(t, values); result != expected {
		t.Fatalf("unexpected result:\n%s", result)
	}
}

func TestInjectKeepsUserValuesAndComments(t *testing.T) {
	input := parse(t, `# these are my values
replicaCount: 3
image:
  # we always pull
  pullPolicy: Always
`)

	values, _ := Inject(input, []Injection{{
		Path:       []string{"image", "tag"},
		Registry:   "ghcr.io",
		Repository: "platform-mesh/portal",
		Tag:        "v1.2.3",
		WithDigest: true,
	}})

	result := encode(t, values)

	for _, expected := range []string{"# these are my values", "# we always pull", "replicaCount: 3", "pullPolicy: Always", "tag: v1.2.3"} {
		if !strings.Contains(result, expected) {
			t.Errorf("expected %q in:\n%s", expected, result)
		}
	}
}

func TestInjectOverwritesUserCoordinates(t *testing.T) {
	// a user (or a previous, untransferred setup) pointing at the original registry must
	// not win over what OCM resolved
	input := parse(t, "image:\n  registry: ghcr.io\n  repository: platform-mesh/portal\n  tag: v1.0.0\n")

	values, _ := Inject(input, []Injection{{
		Path:       []string{"image", "tag"},
		Registry:   "registry.example.com",
		Repository: "pm/platform-mesh/portal",
		Tag:        "v1.2.3",
		WithDigest: true,
	}})

	result := encode(t, values)

	if strings.Contains(result, "ghcr.io") || strings.Contains(result, "v1.0.0") {
		t.Fatalf("expected the transferred location to win:\n%s", result)
	}
}

func TestInjectPathEndingInCoordinate(t *testing.T) {
	values, warnings := Inject(nil, []Injection{{
		Path:        []string{"image", "repository"},
		Registry:    "ghcr.io",
		Repository:  "platform-mesh/portal",
		Tag:         "v1.2.3",
		Description: "portal",
	}})

	if len(warnings) != 1 {
		t.Fatalf("expected a warning, got %v", warnings)
	}

	if result := encode(t, values); result != "image:\n  repository: v1.2.3\n" {
		t.Fatalf("unexpected result:\n%s", result)
	}
}

func TestInjectNothing(t *testing.T) {
	if values, _ := Inject(nil, nil); values != nil {
		t.Error("expected no values to be created when there is nothing to inject")
	}
}

func TestReference(t *testing.T) {
	injection := Injection{
		Registry:   "ghcr.io",
		Repository: "platform-mesh/portal",
		Tag:        "v1.2.3",
		Digest:     "sha256:deadbeef",
		WithDigest: true,
	}

	expected := "ghcr.io/platform-mesh/portal:v1.2.3@sha256:deadbeef"
	if result := injection.Reference(); result != expected {
		t.Fatalf("expected %q, got %q", expected, result)
	}
}
