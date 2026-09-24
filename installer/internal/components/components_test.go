package components

import "testing"

func TestDefaults(t *testing.T) {
	config, err := Defaults()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	infra, exists := config["infra"]
	if !exists {
		t.Fatal("expected configuration for the infra component")
	}

	if !infra.IsEnabled() {
		t.Error("expected infra to be enabled by default")
	}

	if infra.TargetNamespace != "platform-mesh-system" {
		t.Errorf("unexpected target namespace %q", infra.TargetNamespace)
	}

	if len(infra.DependsOn) == 0 {
		t.Error("expected infra to depend on other components")
	}

	if config["example-httpbin-operator"].IsEnabled() {
		t.Error("expected the httpbin example to be disabled by default")
	}

	// unknown components are enabled and use the release namespace
	if !config["something-new"].IsEnabled() {
		t.Error("expected unknown components to be enabled")
	}

	if config["something-new"].TargetNamespace != "" {
		t.Error("expected unknown components to have no explicit namespace")
	}
}
