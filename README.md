# Platform Mesh Production Guide

This repository contains work-in-progress manifests and scripts to setup a production
Platform Mesh (PM). If you are looking to develop Platform Mesh or want a quick&dirty
local installation, use Platform Mesh's `local-setup` instead, but note that you should
absolutely not use it as a starting point for production.

## Overview

This repository represents the absolute bare minimum that is required to get PM working.
You will have to perform some modifications to the manifests here before they can be
applied to your cluster.

The installation itself is rather simple:

* Prepare the manifests.
* Run the `setup.sh`, which will
   * install Flux
   * create secure internal credential Secrets
   * apply the OpenComponentModel CRDs
   * apply all OCIRepositories used by Platform Mesh
   * apply all HelmReleases used by Platform Mesh
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

### Step 2: Run `setup.sh`

The script will take care of most of the bootstrapping for you. If you already have Flux
installed from somewhere else, you can comment it out in the `setup.sh`, it's just there for
convenience.

Once the script has finished, your installation is nearly complete. You should see a lot of
`HelmRelease` objects in the `platform-mesh-system` namespace, most of them not yet ready.
That's okay, we need a bit more fine tuning.

### Step 3: Apply Manifests

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
