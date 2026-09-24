package components

import (
	"slices"
	"testing"
)

func TestImageMappings(t *testing.T) {
	mappings, err := ImageMappings()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// every component that ships an image must have a mapping, otherwise mirrored
	// installations would silently pull from the original registry
	for _, component := range []string{"account-operator", "cert-manager", "infra", "openfga", "traefik", "observability"} {
		if len(mappings[component]) == 0 {
			t.Errorf("expected image mappings for %q", component)
		}
	}

	defaults := mappings["account-operator"][0]
	if defaults.ResourceName() != "image" {
		t.Errorf("expected the default resource name, got %q", defaults.ResourceName())
	}

	if path := defaults.ValuePath(); !slices.Equal(path, []string{"image", "tag"}) {
		t.Errorf("expected the default path, got %v", path)
	}

	if defaults.Combined() {
		t.Error("expected the default style to be split")
	}

	if !defaults.WithDigest() {
		t.Error("expected digests to be injected by default")
	}
}

func TestImageMappingStyles(t *testing.T) {
	mappings, err := ImageMappings()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// openfga's chart schema rejects registry and digest
	openfga := mappings["openfga"][0]
	if !openfga.Combined() {
		t.Error("expected openfga's image to use the combined style")
	}

	if openfga.WithDigest() {
		t.Error("combined images must never get a digest")
	}

	// traefik's image schema has no digest field
	traefik := mappings["traefik"][0]
	if traefik.Combined() {
		t.Error("expected traefik's image to use the split style")
	}

	if traefik.WithDigest() {
		t.Error("expected traefik's image to be injected without a digest")
	}

	// the kcp image lives in a component of its own
	kcp := false
	for _, mapping := range mappings["infra"] {
		if mapping.Component == "kcp" {
			kcp = true

			if !slices.Equal(mapping.ValuePath(), []string{"kcp", "image", "tag"}) {
				t.Errorf("unexpected path for the kcp image: %v", mapping.ValuePath())
			}
		}
	}

	if !kcp {
		t.Error("expected infra to source the kcp image from the kcp component")
	}
}
