# Platform Mesh Production Guide

This repository contains work-in-progress manifests and scripts to setup a production
Platform Mesh (PM). If you are looking to develop Platform Mesh or want a quick&dirty
local installation, use Platform Mesh's `local-setup` instead, but note that you should
absolutely not use it as a starting point for production.

This repository contains manifests for Platform Mesh version **0.5.2**.

## NOTE

This repository is the result of a spike, not any preview of any future Platform Mesh
installation procedures.

The setup was tested on a Gardener shoot using Kubernetes 1.36.3 and relying on Gardener's
DNS integration.

## Overview

This repository represents the absolute bare minimum that is required to get PM working.
You will have to perform some modifications to the manifests here before they can be
applied to your cluster.

The installation itself is rather simple:

* Prepare the manifests.
* Install Flux into the cluster.
* Run the `create-secrets.sh`; this only needs to be done on the very first installation.
* Apply the `HelmRelease` and `OCIRepositories`.
* Apply configuration from `manifests/`.
* Currently, as a last setup step, you will need to perform one manual step to setup a
  Secret for kcp.

Once all HelmReleases are ready, you can access the PM Portal in your browser.

## Installation

**NB:** This procedure is similar to "Linux from scratch". We are going to ignore most
convenience features that PM offers for this process, like using OCM, ArgoCD or the
advanced templating offered by the Platform Mesh Operator.

### Step 1: Prepare Manifests

To setup PM, you will need to customize the HelmReleases to your Kubernetes cluster.
Use your favorite tool (for example `sed`) to perform mass search&replace operations
and replace the following placeholders in all files in `helmreleases` and `manifests`:

* `PM_BASE_DOMAIN` – this is the domain you want your installation to be reachable under,
  for example `mesh.example.com`. Make sure your DNS is setup to forward traffic for
  `PM_BASE_DOMAIN`, `*.PM_BASE_DOMAIN` and `*.*.PM_BASE_DOMAIN` to the Traefik LoadBalancer,
  since each organization will get its own dedicated subdomain.
* `FRONT_PROXY_SERVICE_IP` – choose a free IP from your cluster's ServiceIP range. This is
  used to configure host aliases on many pods to improve network flow and bootstrapping.
* `TRAEFIK_SERVICE_IP` – same, but for the Traefik LoadBalancer Service.

### Step 2: Install Flux 2.17

In case Flux is not yet set up, install it any way you like, for example:

```bash
helm upgrade \
  --install \
  --namespace flux-system --create-namespace \
  --version 2.17.2 \
  --set imageAutomationController.create=false \
  --set imageReflectionController.create=false \
  --set notificationController.create=false \
  --set helmController.container.additionalArgs[0]="--concurrent=19" \
  --set sourceController.container.additionalArgs[0]="--requeue-dependency=5s" \
  flux oci://ghcr.io/fluxcd-community/charts/flux2

kubectl wait --namespace flux-system --for=condition=available deployment/helm-controller
kubectl wait --namespace flux-system --for=condition=available deployment/source-controller
kubectl wait --namespace flux-system --for=condition=available deployment/kustomize-controller
```

**NB:** Platform Mesh 0.5.2 is not compatible with Flux >= 2.18!

### Step 3: Run `create-secrets.sh`

You need to setup some credentials once, for Keycloak, OpenFGA etc. Simply run the script:

```bash
export KUBECONFIG=...
./create-secrets.sh
```

### Step 4: Install Components

Your cluster is now ready to receive the `HelmReleases` and `OCIRepositories` that make up
Platform Mesh. You will also need the CRDs for OCM, even though in this guide we are not making
use of OCM. You can just apply them all:

```bash
export KUBECONFIG=...
kubectl apply --filename ocmcrds
kubectl apply --filename ocirepositories
kubectl apply --filename helmreleases
```

Flux will now begin to install everything, but it's expected to see lots of failing pods for now.
We have to configure Platform Mesh first and apply one custom object.

### Step 5: Apply Manifests

There are four more manifest we need to apply. This is not yet fully automated, but soon will
be:

* `kubectl apply -f manifests/domain-certificate.yaml` – this sets up a self-signed certificate
  for your installation to use. You can also use Let's Encrypt or bring your own CA, but note
  that because of the wildcard DNS records, you will have to use the DNS challenge.
* `kubectl apply -f manifests/platform-mesh.yaml` – the main configuration for the Platform Mesh
  operator.
* `kubectl apply -f manifests/platform-mesh-profile.yaml` – an empty legacy configuration file,
  soon to be reworked but still required in PM 0.5.2.

And lastly, you need to configure a Secret for kcp: This is soon going to be automated, but
in PM 0.5.2 you have to perform this step manually:

1. Get the Secret for tbe rebac-authz-webhook's serving cert: `kubectl -n platform-mesh-system get secret rebac-authz-webhook-cert -o json`.
2. Copy the base64-encoded `ca.crt` into `manifests/kcp-webhook-secret.yaml` at the indicated place.
3. `kubectl apply -f manifests/kcp-webhook-secret.yaml`.

Once you apply this, keep a lookout for a `root-kcp-...` Pod in your `platform-mesh-system` namespace.
It it doesn't appear within a minute, restart the kcp-operator:

`k -n kcp-operator delete pods --all`.

### Time to Wait

Platform Mesh will now slowly come to life.

* First kcp has to become ready. This means the first pod to be ready is `root-kcp-...`. Once it
  runs, the `root-kcp-proxy` and the front-proxy will become ready.
* Once kcp is healthy, the Platform Mesh Operator will begin to bootstrap it and create a lot of
  missing kubeconfigs.
* These kubeconfigs will then trigger the rollout of more components, like the account and security
  operators.
* After some time, all pods and all `HelmReleases` should be ready.

### Test

You should now be able to open `https://<PM_BASE_DOMAIN>:8443/` in your browser.

## Runbook

If your Platform Mesh does not come up, check these things:

* etcd, Keycloak and OpenFGA are mostly standalone and should come up on their own, regardless of
  the rest of Platform Mesh. These must also be up and running before PM can fully be installed.
* `infra` Helm chart fails with API errors about fields not existing: You installed Flux >2.18.
   Downgrade Flux to 2.17.2.
* The setup is seemingly stuck and not progressing with the `HelmReleases`? Make sure you increased
  Flux's default concurrency: The default (5) will make it so the rollout will take forever, since
  right now no dependencies exist between the `HelmReleases` and Flux cannot yet better order the
  installation. Check the bash snippet above for the necessary changes to Flux.
* In order to make sure kcp runs:
   * Does the `rebac-authz-webhook-cert` Secret exist? If not, make sure the `rebac-authz-webhook`
     HelmRelease is getting installed (it's okay for it to be failing, but it needs to at least
     once provision the necessary certificate).
   * Did you manually create the `kcp-webhook-secret`, as described above? If not, do so.
   * If no `root-kcp-..` Pod shows up, restart the kcp-operator. Note that when the Secret is missing,
     the kcp-operator is not logging any errors. This is unfortunate since it hides the underlying
     issue that could prevent it from creating the RootShard. Can be fixed upstream.
   * If still no Pod shows up, check if the `infra` Helm chart deployed the `RootShard` object. If
     not, make sure Flux processed the Helm chart.
* Once kcp is up, you can expect the Platform Mesh Operator to provision resources inside kcp. This
  process takes some time and during it, the operator will log lots of errors relating to missing
  APIs like `ContentConfigurations` or `ProviderPermissions`. This is normal, give it a few minutes.
* Only when the Platform Mesh Operator has finished setting up kcp will it create a `-kubeconfig`
  Secret for all the other Platform Mesh components. You will notice a sudden burst of Pods changing
  from ContainerCreating to Running.
* From there, the Security Operator can provision OpenFGA, which will unblock further components from
  coming up.
* The rebac-authz-webhook needs kcp, but is often stuck in a CrashLoop. You can simply delete the
  Pod to kickstart a new one.
* The `HelmReleases` have a timeout of 15 minutes. Any operations will have a significant delay.
  You can lower the timeout to make incremental changes quicker to apply.
