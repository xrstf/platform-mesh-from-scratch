package starter

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

var testInput = Input{
	BaseDomain:   "mesh.example.com",
	TraefikIP:    "10.96.0.50",
	FrontProxyIP: "10.96.0.51",
	Directory:    "manifests",
}

func TestRender(t *testing.T) {
	content, err := Render(testInput)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	rendered := string(content)

	for _, placeholder := range []string{"PM_BASE_DOMAIN", "TRAEFIK_SERVICE_IP", "FRONT_PROXY_SERVICE_IP", "{{"} {
		if strings.Contains(rendered, placeholder) {
			t.Errorf("rendered values still contain %q", placeholder)
		}
	}

	if !strings.Contains(rendered, "kcp.api.mesh.example.com") {
		t.Error("expected the base domain to be used")
	}

	var parsed map[string]any
	if err := yaml.Unmarshal(content, &parsed); err != nil {
		t.Fatalf("rendered values are not valid YAML: %v", err)
	}

	if _, exists := parsed["infra"]; !exists {
		t.Errorf("expected values for the infra component, got keys %v", keys(parsed))
	}
}

func TestRenderPlatformMesh(t *testing.T) {
	content, err := RenderPlatformMesh(testInput)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var parsed struct {
		Kind string `yaml:"kind"`
		Spec struct {
			Exposure struct {
				BaseDomain             string `yaml:"baseDomain"`
				TraefikClusterIP       string `yaml:"traefikClusterIP"`
				KCPFrontProxyClusterIP string `yaml:"kcpFrontProxyClusterIP"`
			} `yaml:"exposure"`
		} `yaml:"spec"`
	}

	if err := yaml.Unmarshal(content, &parsed); err != nil {
		t.Fatalf("rendered object is not valid YAML: %v", err)
	}

	if parsed.Kind != "PlatformMesh" {
		t.Errorf("expected a PlatformMesh object, got %q", parsed.Kind)
	}

	if parsed.Spec.Exposure.BaseDomain != testInput.BaseDomain {
		t.Errorf("expected base domain %q, got %q", testInput.BaseDomain, parsed.Spec.Exposure.BaseDomain)
	}

	if parsed.Spec.Exposure.TraefikClusterIP != testInput.TraefikIP {
		t.Errorf("expected Traefik IP %q, got %q", testInput.TraefikIP, parsed.Spec.Exposure.TraefikClusterIP)
	}

	if parsed.Spec.Exposure.KCPFrontProxyClusterIP != testInput.FrontProxyIP {
		t.Errorf("expected front-proxy IP %q, got %q", testInput.FrontProxyIP, parsed.Spec.Exposure.KCPFrontProxyClusterIP)
	}
}

func TestWrite(t *testing.T) {
	input := testInput
	input.Directory = t.TempDir()

	files, err := Write(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(files) != 2 {
		t.Fatalf("expected 2 files, got %v", files)
	}

	// running again must not clobber the user's files
	if _, err := Write(input); err == nil {
		t.Error("expected an error when files already exist")
	}
}

func keys(m map[string]any) []string {
	result := make([]string, 0, len(m))
	for key := range m {
		result = append(result, key)
	}

	return result
}
