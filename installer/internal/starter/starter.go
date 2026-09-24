// Package starter generates the initial configuration files for a new Platform Mesh
// installation, based on a handful of answers the user gives in the `start` command.
package starter

import (
	"bytes"
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"text/template"
)

//go:embed helm-values.yaml.tmpl
var helmValuesTemplate string

//go:embed platform-mesh.yaml.tmpl
var platformMeshTemplate string

const (
	// HelmValuesFilename is the name of the generated Helm values file.
	HelmValuesFilename = "helm-values.yaml"
	// PlatformMeshFilename is the name of the generated PlatformMesh object.
	PlatformMeshFilename = "platform-mesh.yaml"
)

// Input is everything the installer asks the user for.
type Input struct {
	// BaseDomain is the domain Platform Mesh will be reachable at, e.g. "mesh.example.com".
	BaseDomain string
	// TraefikIP is the ClusterIP of the Traefik LoadBalancer Service.
	TraefikIP string
	// FrontProxyIP is the ClusterIP of the kcp front-proxy Service.
	FrontProxyIP string
	// Directory is where the configuration files are written to.
	Directory string
}

// Render returns the contents of the initial Helm values file.
func Render(input Input) ([]byte, error) {
	return render("helm-values", helmValuesTemplate, input)
}

// RenderPlatformMesh returns the contents of the PlatformMesh object.
func RenderPlatformMesh(input Input) ([]byte, error) {
	return render("platform-mesh", platformMeshTemplate, input)
}

// Write renders and writes all configuration files into the input's directory and returns
// the list of written files.
func Write(input Input) ([]string, error) {
	if err := os.MkdirAll(input.Directory, 0o755); err != nil {
		return nil, fmt.Errorf("cannot create %q: %w", input.Directory, err)
	}

	files := map[string]func(Input) ([]byte, error){
		HelmValuesFilename:   Render,
		PlatformMeshFilename: RenderPlatformMesh,
	}

	written := []string{}

	for filename, renderer := range files {
		fullPath := filepath.Join(input.Directory, filename)

		if _, err := os.Stat(fullPath); err == nil {
			return nil, fmt.Errorf("%q already exists, refusing to overwrite it", fullPath)
		}

		content, err := renderer(input)
		if err != nil {
			return nil, err
		}

		if err := os.WriteFile(fullPath, content, 0o644); err != nil {
			return nil, fmt.Errorf("cannot write %q: %w", fullPath, err)
		}

		written = append(written, fullPath)
	}

	return written, nil
}

func render(name, tpl string, input Input) ([]byte, error) {
	parsed, err := template.New(name).Option("missingkey=error").Parse(tpl)
	if err != nil {
		return nil, fmt.Errorf("invalid %s template: %w", name, err)
	}

	var buf bytes.Buffer
	if err := parsed.Execute(&buf, input); err != nil {
		return nil, fmt.Errorf("cannot render %s: %w", name, err)
	}

	return buf.Bytes(), nil
}
