package ocm

import (
	"testing"

	metav1 "ocm.software/ocm/api/ocm/compdesc/meta/v1"
)

func TestChartName(t *testing.T) {
	testcases := []struct {
		component string
		resource  string
		expected  string
	}{
		{"account-operator", "chart", "account-operator"},
		{"account-operator", "charts", "account-operator"},
		{"etcd-druid", "etcd-druid", "etcd-druid"},
		{"traefik", "crds", "traefik-crds"},
		{"traefik", "traefik-crds", "traefik-crds"},
		{"infra", "CRDs", "infra-crds"},
	}

	for _, testcase := range testcases {
		t.Run(testcase.component+"/"+testcase.resource, func(t *testing.T) {
			if result := chartName(testcase.component, testcase.resource); result != testcase.expected {
				t.Fatalf("expected %q, got %q", testcase.expected, result)
			}
		})
	}
}

func TestSplitImageReference(t *testing.T) {
	testcases := []struct {
		input      string
		repository string
		tag        string
		digest     string
	}{
		{
			input:      "ghcr.io/platform-mesh/helm-charts/account-operator:0.21.0",
			repository: "ghcr.io/platform-mesh/helm-charts/account-operator",
			tag:        "0.21.0",
		},
		{
			input:      "quay.io/jetstack/charts/cert-manager:v1.20.1",
			repository: "quay.io/jetstack/charts/cert-manager",
			tag:        "v1.20.1",
		},
		{
			input:      "ghcr.io/platform-mesh/charts/traefik:41.4.0@sha256:3125eaadc0da8915d0d007b5aa7090967a8dc4d09f3aeea0d17b28b44fc04fd6",
			repository: "ghcr.io/platform-mesh/charts/traefik",
			tag:        "41.4.0",
			digest:     "sha256:3125eaadc0da8915d0d007b5aa7090967a8dc4d09f3aeea0d17b28b44fc04fd6",
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.input, func(t *testing.T) {
			repository, tag, digest, err := splitImageReference(testcase.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if repository != testcase.repository {
				t.Errorf("expected repository %q, got %q", testcase.repository, repository)
			}

			if tag != testcase.tag {
				t.Errorf("expected tag %q, got %q", testcase.tag, tag)
			}

			if digest != testcase.digest {
				t.Errorf("expected digest %q, got %q", testcase.digest, digest)
			}
		})
	}
}

func TestIsHelmChartType(t *testing.T) {
	for _, valid := range []string{"helmChart", "helmChart/v1"} {
		if !isHelmChartType(valid) {
			t.Errorf("expected %q to be recognized as a Helm chart", valid)
		}
	}

	for _, invalid := range []string{"ociImage", "helmchart-imagemap", "application/gzip"} {
		if isHelmChartType(invalid) {
			t.Errorf("did not expect %q to be recognized as a Helm chart", invalid)
		}
	}
}

func TestResourceDigest(t *testing.T) {
	testcases := []struct {
		name     string
		input    *metav1.DigestSpec
		expected string
	}{
		{
			name:     "no digest",
			input:    nil,
			expected: "",
		},
		{
			name: "OCI artifact digest",
			input: &metav1.DigestSpec{
				HashAlgorithm:          "SHA-256",
				NormalisationAlgorithm: "ociArtifactDigest/v1",
				Value:                  "2f2ee01ba2ae2a21789588ad6d673da682f48ee22e0d07dc98694d1084292f35",
			},
			expected: "sha256:2f2ee01ba2ae2a21789588ad6d673da682f48ee22e0d07dc98694d1084292f35",
		},
		{
			name: "unsupported hash algorithm",
			input: &metav1.DigestSpec{
				HashAlgorithm:          "SHA-512",
				NormalisationAlgorithm: "ociArtifactDigest/v1",
				Value:                  "2f2ee01",
			},
			expected: "",
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			if result := resourceDigest(testcase.input); result != testcase.expected {
				t.Fatalf("expected %q, got %q", testcase.expected, result)
			}
		})
	}
}

func TestNormalizeComponent(t *testing.T) {
	expected := "github.com/platform-mesh/platform-mesh"

	for _, input := range []string{
		expected,
		"component-descriptors/github.com/platform-mesh/platform-mesh",
		"/component-descriptors/github.com/platform-mesh/platform-mesh",
	} {
		if result := normalizeComponent(input); result != expected {
			t.Errorf("expected %q, got %q", expected, result)
		}
	}
}
