package generate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestWrite(t *testing.T) {
	var values yaml.Node
	if err := yaml.Unmarshal([]byte("# a comment\nreplicaCount: 3\n"), &values); err != nil {
		t.Fatalf("cannot parse test values: %v", err)
	}

	dir := t.TempDir()

	releases := []Release{
		{
			Name:            "openfga",
			Component:       "github.com/openfga/openfga",
			ChartURL:        "ghcr.io/platform-mesh/ocm/charts/openfga",
			ChartVersion:    "0.2.62",
			ChartDigest:     "sha256:2f2ee01ba2ae2a21789588ad6d673da682f48ee22e0d07dc98694d1084292f35",
			Namespace:       "platform-mesh-system",
			TargetNamespace: "platform-mesh-system",
			Values:          values.Content[0],
		},
		{
			Name:            "traefik",
			ChartURL:        "ghcr.io/platform-mesh/charts/traefik",
			ChartVersion:    "41.4.0",
			Namespace:       "platform-mesh-system",
			TargetNamespace: "default",
			DependsOn:       []string{"traefik-crds"},
		},
	}

	files, err := NewFluxWriter().Write(dir, releases)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(files) != 4 {
		t.Fatalf("expected 4 files, got %v", files)
	}

	repo := readFile(t, filepath.Join(dir, "ocirepositories", "openfga.yaml"))
	if !strings.Contains(repo, "url: oci://ghcr.io/platform-mesh/ocm/charts/openfga") {
		t.Errorf("unexpected OCIRepository:\n%s", repo)
	}

	if !strings.Contains(repo, "tag: 0.2.62") {
		t.Errorf("unexpected OCIRepository:\n%s", repo)
	}

	if !strings.Contains(repo, "digest: sha256:2f2ee01ba2ae2a21789588ad6d673da682f48ee22e0d07dc98694d1084292f35") {
		t.Errorf("expected the chart digest to be pinned:\n%s", repo)
	}

	release := readFile(t, filepath.Join(dir, "helmreleases", "openfga.yaml"))
	if !strings.Contains(release, "# a comment") {
		t.Errorf("expected values comments to survive:\n%s", release)
	}

	traefik := readFile(t, filepath.Join(dir, "helmreleases", "traefik.yaml"))
	if !strings.Contains(traefik, "dependsOn:\n    - name: traefik-crds") {
		t.Errorf("expected dependencies:\n%s", traefik)
	}

	if strings.Contains(traefik, "values:") {
		t.Errorf("expected no values key when there are no values:\n%s", traefik)
	}

	// everything must still be valid YAML
	for _, file := range files {
		var parsed map[string]any
		if err := yaml.Unmarshal([]byte(readFile(t, file)), &parsed); err != nil {
			t.Errorf("%s is not valid YAML: %v", file, err)
		}
	}
}

func TestWriteWithoutDigests(t *testing.T) {
	dir := t.TempDir()

	writer := NewFluxWriter()
	writer.Digests = false

	_, err := writer.Write(dir, []Release{{
		Name:            "openfga",
		ChartURL:        "ghcr.io/platform-mesh/ocm/charts/openfga",
		ChartVersion:    "0.2.62",
		ChartDigest:     "sha256:2f2ee01ba2ae2a21789588ad6d673da682f48ee22e0d07dc98694d1084292f35",
		Namespace:       "platform-mesh-system",
		TargetNamespace: "platform-mesh-system",
	}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	repo := readFile(t, filepath.Join(dir, "ocirepositories", "openfga.yaml"))
	if strings.Contains(repo, "digest:") {
		t.Errorf("expected no digest:\n%s", repo)
	}

	if !strings.Contains(repo, "tag: 0.2.62") {
		t.Errorf("expected the tag to remain:\n%s", repo)
	}
}

func readFile(t *testing.T, filename string) string {
	t.Helper()

	content, err := os.ReadFile(filename)
	if err != nil {
		t.Fatalf("cannot read %q: %v", filename, err)
	}

	return string(content)
}
