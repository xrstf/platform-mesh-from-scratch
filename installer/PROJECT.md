# Platform Mesh Installer

This directory contains a Go CLI application to prepare Platform Mesh configuration files for
installation in a Kubernetes cluster. This CLI has these main tasks:

* take in an OCM Component Descriptor and a version (for example `component-descriptors/github.com/platform-mesh/platform-mesh` in version `0.5.2`) to determine the exact component versions for each Helm chart to be installed.
* generate Flux HelmRelease manifests for each component
* apply custom Helm values from configuration files into each of the generated HelmReleases
* wrap OCM to allow for automated mirroring of the entire Component Descriptor (and all its sub
  components) into a custom OCI registry
* offer an interactive TUI to initially configure your configuration (input base domain, a few IPs,
  maybe a couple of other things)

## Ground Rules

* A clean, simple CLI.
* Use urfave/cli for CLI flag parsing and command handling.
* If possible for this set of features, it should try to use the OCM SDK (if that exists) to perform
  the OCM operations. Only if that is not feasable should you resort to relying on an ocm binary
  and exec'ing that and parsing its output. Prefer the SDK if you can.

## Functionality

This section describes the functionality in greater detail.

### Basic Concept

The main idea behind the installer is to automate the generation of HelmRelease objects for applying
into a Kubernetes cluster. These HelmReleases contain

* packging information (repository and version)
* customization options (Helm values)

Since the packaging information is dependent on what PM version the user will want to install, it
would need to be updated with every PM update. However this means the HelmRelease will have to be
rewritten rather often. Rewriting YAML files basically always loses comments and/or formatting,
leading to ugly git diffs.

The installer must avoid updating YAML files. The process should instead look like this:

* The user provides the Helm values standalone (in files that they own and manage and have checked
  in to git) and chooses the PM version.
* The installer takes the user's values and merges them into HelmRelease files that it will create.

The user can specify the Helm values in 2 way:

* either they specify a _directory_, in which case the installer will expect a YAML file for each
  component (e.g. `account-operator.yaml`, `keycloak-operator.yaml` etc.). This is nice for folks
  who have LOTS of customizations and where one big file would become annoying.
* alternatively, the user specifies a single YAML file, in which case all Helm values are sourced
  from there, with the assumption that top-level keys match the Helm chart names. This is for quick
  & dirty, kind of.

### Subcommands

### `start`

This is the first command the user is expected to run when they setup PM. And they will only use it
once, to bootstrap their config files. This is also the only command that offers an interactive
TUI (well, "TUI" might be a big word, all it should do is ask the user for a few pieces of information).

This command will

1. Show a nice greeting, welcoming the user to their Platform Mesh journey.
2. Ask the user for their project's _base domain_. Explains to the user that this is where PM will
   be reachable and also informs them that two levels of wildcards are required for this domain
   (e.g. if they chose "mesh.example.com", "*.*.mesh.example.com" would need to exist in DNS).
   (DNS management is out of scope, the user is free to handle this in any way they like, we can
   recommend stuff like external-dns to automate it, but _for now_, we do not care about DNS setup).
3. Ask the user to provide a Service IP for Traefik (the LoadBalancer) and the kcp front-proxy.
   These need to be in the ServiceCIDR range of their target cluster and need to not be used yet in
   that cluster.
4. Ask the user for the directory where their initial configuration shall be written to. Should default
   to `manifests/`. In this directory you will place the generated `helm-values.yaml` and a
   `platform-mesh.yaml` (which contains a Kube object of kind `PlatformMesh`; it's kind of a bit
   redundant, but has to be generated nontheless).

Armed with this information, the installer will now generate the starting set of Helm values for the
user. There should be a function somewhere in the installer that takes the set of user input and
generates a single YAML file (do not worry about the structure of that file, we can refine the
exact structure later).

Once the YAML file has been written, the installer informs the user about the next possible steps:
Reviewing their config file or running the `deploy` subcommand.

### `deploy`

The meat and potatos of the installer. This is where 90% of the logic lies. This command's job is to
take the user's Helm values and combine them with the OCM versions to produce the HelmReleases for
the user to apply to their cluster.

This command offers these CLI flags:

* --version: allows to override the PM version to be installed (by default we install the latest
  version as determined by OCM based on the component descriptor).
* --component: Allows to override the component we start from. By default this is hardcoded to `component-descriptors/github.com/platform-mesh/platform-mesh`.
* --values: Either the directory or filename of the Helm values.
* --format: Can in the future be used to either generate HelmReleases, ArgoCD Apps or maybe even
  shell scripts to perform a raw Helm install. For now we only support HelmReleases.
* --output: Directory where to place the generated files.

It:

1. Takes the given OCM --component and, if no --version is specified, uses OCM to ask for the latest
   available version. Once component and version are pinned down,
2. it uses OCM to determine the list of child components. This will yield account-operator,
   openfga, cert-manager etc. in specific versions.
3. It takes these components, finds any extra Helm values (either by reading the single file, or
   finding the matching file per component) and combines them into HelmReleases and OCIRepositories
4. It writes those HelmReleases and OCIRepositories into files in the --output directory.
5. It informs the user that their manifests are ready to be applied.

### `mirror`

This command only needs to be sketched out and not necessarily fully implemented. This command would
be used by airgapped users to mirror the entire PM Component Descriptor (incl. all Helm charts and
images) to their own OCI registry.

The command would take these CLI flags:

* --version: allows to override the PM version to be mirrored (by default we install the latest
  version as determined by OCM based on the component descriptor).
* --component: Allows to override the component we start from. By default this is hardcoded to `component-descriptors/github.com/platform-mesh/platform-mesh`.
* --to: The address of the target registry that we mirror to.
* (any additional flags for credentials, if needed)

The command should mirror the component tree and then, at the end, inform the user that for running
the `deploy` command, they can now use `--component <whereever-they-mirrored-to>`.

### References

For reference, we are to some extent moving the deployment subroutine from the PM Operator into a
CLI with this project. This first attempt that we vibe here will be a bit more limitted and not yet
have to fully fledged out templating stuff that the old operator offered (and that I think is also
unnecessarily complex).

* Platform Mesh Operator code is checked out at: /home/xrstf/gospace/src/github.com/platform-mesh/platform-mesh/main/operators/platform-mesh-operator
* PM Helm charts are checked out at /home/xrstf/gospace/src/github.com/platform-mesh/helm-charts/main
* A collection of raw, hand-crafted HelmReleases can be found in the parent directory from here.
  DO NOT CHANGE ANY FILE outside of this directory. But you can look at them for reference: Ultimately
  we want to generate the files very similar to those in helmreleases/ and ocirepositories/. You can
  also see an example for a PlatformMesh object in manifests/platform-mesh.yaml.
