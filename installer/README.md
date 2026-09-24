# Platform Mesh Installer

A small CLI that prepares the Kubernetes manifests for a Platform Mesh (PM) installation.

The installer

* resolves the exact chart versions of a PM release from its [OCM](https://ocm.software/)
  component descriptor,
* combines them with *your* Helm values into Flux `HelmRelease` and `OCIRepository` objects,
* helps you bootstrap your configuration interactively, and
* can mirror an entire PM release into your own OCI registry (for airgapped setups).

## Design

The central idea is that **the installer never rewrites your files**. Your Helm values live in
files that you own, edit and check into git. The installer only ever *generates* manifests
from them, so upgrading Platform Mesh is a matter of re-running `deploy` with a new version;
your values file stays untouched and your git history stays clean.

Comments and key ordering in your values files are preserved in the generated manifests.

## Installation

```bash
go build -o installer .
```

## Usage

### `installer start`

Run this once to bootstrap your configuration. It asks for

* the **base domain** of your installation (note that `*.<domain>` and `*.*.<domain>` must
  resolve to your cluster; DNS setup itself is out of scope, tools like `external-dns` help),
* the **Service IP for Traefik** (the LoadBalancer) and the **Service IP for the kcp
  front-proxy** (both must be free addresses inside your cluster's Service CIDR),
* the **directory** to write to (default: `manifests/`).

It writes two files:

| File | Purpose |
| ---- | ------- |
| `helm-values.yaml` | your Helm values for every PM component; this is the file you keep editing |
| `platform-mesh.yaml` | the `PlatformMesh` object that configures the PM operator |

All questions can also be answered via flags (`--base-domain`, `--traefik-ip`,
`--front-proxy-ip`, `--output`), which makes the command scriptable.

### `installer deploy`

Generates the manifests for your cluster:

```bash
installer deploy --values manifests/helm-values.yaml --output generated
```

Flags:

| Flag | Default | Description |
| ---- | ------- | ----------- |
| `--version` | latest release | PM version to install |
| `--component` | `github.com/platform-mesh/platform-mesh` | OCM component to start from; can also be a full OCM reference like `ghcr.io/platform-mesh//github.com/platform-mesh/platform-mesh` |
| `--repository` | `ghcr.io/platform-mesh` | OCM repository hosting the component |
| `--values`, `-f` | – | file **or** directory with your Helm values |
| `--format` | `helmrelease` | output format (only Flux HelmReleases for now) |
| `--output`, `-o` | `generated` | output directory |
| `--namespace` | `platform-mesh-system` | namespace for the generated Flux objects |
| `--enable` / `--disable` | – | enable/disable individual components (repeatable) |
| `--digests` | `true` | pin the OCI digest of every chart in addition to its tag (`--digests=false` to opt out) |
| `--prereleases` | `false` | consider prerelease versions when looking for the latest version |

The output directory is populated like this:

```
generated/
├── helmreleases/
│   ├── account-operator.yaml
│   └── …
└── ocirepositories/
    ├── account-operator.yaml
    └── …
```

Apply it with `kubectl apply --recursive --filename generated`.

#### Version pinning

By default every generated `OCIRepository` pins both the chart tag and its OCI digest:

```yaml
  ref:
    tag: 0.2.62
    digest: sha256:2f2ee01ba2ae2a21789588ad6d673da682f48ee22e0d07dc98694d1084292f35
```

The digest comes straight from the component descriptor (either from the resource's image
reference or from its `ociArtifactDigest`), so what you deploy is bit-for-bit what was
signed in OCM, even if a tag is moved later. Flux uses the digest for the actual pull; the
tag is kept for readability. Use `--digests=false` if you prefer tag-only references.

#### Providing Helm values

Two styles are supported:

* **One file**, with one top-level key per component:

  ```yaml
  account-operator:
    hostAliases:
      enabled: true
  openfga:
    replicaCount: 3
  ```

* **A directory**, with one file per component (`account-operator.yaml`, `openfga.yaml`, …),
  each containing just the values for that chart. Handy if you have a lot of customizations.

Values for components that do not exist in the chosen PM version are reported as a warning,
which usually means a typo.

### `installer mirror`

> [!NOTE]
> This command is a sketch: it works, but has seen little testing and does not offer
> dedicated credential flags yet (credentials are read from the usual OCM/Docker
> configuration).

```bash
installer mirror --to registry.example.com/platform-mesh
```

It performs a recursive, by-value transfer of the component tree (including all Helm charts
and container images) into your registry and then tells you how to point `deploy` at it:

```bash
installer deploy --repository registry.example.com/platform-mesh --version 0.5.2
```

Use `--dry-run` to only list what would be transferred.

## Component metadata

Besides versions and values, a HelmRelease needs a bit of deployment metadata: the target
namespace, dependencies between components and whether a component is part of a default
installation at all. This is not part of the component descriptor, so the installer ships
these defaults in `internal/components/components.yaml`. Components that are unknown to the
installer are deployed into the release namespace without dependencies.

## Project layout

```
internal/cmd/          CLI commands (start, deploy, mirror)
internal/ocm/          OCM SDK wrapper: version resolution and chart discovery
internal/components/   built-in component metadata (namespaces, dependencies, defaults)
internal/values/       loading of user Helm values (file or directory)
internal/generate/     manifest generation (Flux HelmRelease + OCIRepository)
internal/starter/      templates and rendering for `start`
internal/tui/          the modest interactive bits
```
