#!/usr/bin/env bash

set -euo pipefail
cd "$(dirname "$0")"

set -x

if [[ -z "${KUBECONFIG:-}" ]]; then
  echo "KUBECONFIG must be set." >&2
  exit 1
fi

if [[ "${1:-}" != "--yes" && "${WIPE_CLUSTER_CONFIRM:-}" != "yes" ]]; then
  cat >&2 <<'EOF'
Refusing to wipe the cluster without confirmation.

Re-run with one of:
  ./wipe-cluster.sh --yes
  WIPE_CLUSTER_CONFIRM=yes ./wipe-cluster.sh
EOF
  exit 1
fi

need() {
  command -v "$1" >/dev/null 2>&1 || {
    echo "Missing required command: $1" >&2
    exit 1
  }
}

log() {
  echo "[$(date +%H:%M:%S)] $*"
}

need kubectl
need python3

kubectl version --request-timeout=10s >/dev/null

readonly KEEP_NAMESPACES=(default garden kube-node-lease kube-public kube-system)
readonly KEEP_NAMESPACE_PREFIXES=(gardener- shoot--)
readonly KEEP_CLUSTERROLE_NAMES=(admin cluster-admin edit view)
readonly KEEP_CLUSTERROLE_PREFIXES=(gardener kubeadm: system:)
readonly KEEP_CRD_SUFFIXES=(
  .gardener.cloud
  .k8s.io
  .kubernetes.io
  .machine.sapcloud.io
)

json_filter() {
  local mode="$1"
  MODE="$mode" python3 -c '
import json
import os
import sys

mode = os.environ["MODE"]
data = json.load(sys.stdin)
items = data.get("items", [])

KEEP_NS_EXACT = {"default", "garden", "kube-node-lease", "kube-public", "kube-system"}
KEEP_NS_PREFIXES = ("gardener-", "shoot--")
KEEP_CR_EXACT = {"admin", "cluster-admin", "edit", "view"}
KEEP_CR_PREFIXES = ("gardener", "kubeadm:", "system:")
KEEP_CRD_SUFFIXES = (
    ".gardener.cloud",
    ".k8s.io",
    ".kubernetes.io",
    ".machine.sapcloud.io",
)


def meta_blob(obj):
    md = obj.get("metadata", {})
    chunks = [md.get("name", ""), md.get("namespace", "")]
    for source in (md.get("labels", {}), md.get("annotations", {})):
        for k, v in source.items():
            chunks.append(f"{k}={v}")
    return " ".join(chunks).lower()


def gardener_managed(obj):
    blob = meta_blob(obj)
    needles = (
        "gardener.cloud",
        "shoot.gardener.cloud",
        "resources.gardener.cloud",
        "garden.sapcloud.io",
        "operator.gardener.cloud",
    )
    return any(n in blob for n in needles)


for item in items:
    md = item.get("metadata", {})
    name = md.get("name", "")
    kind = item.get("kind", "")

    if mode == "delete-namespaces":
        keep = (
            name in KEEP_NS_EXACT
            or name.startswith(KEEP_NS_PREFIXES)
            or gardener_managed(item)
        )
        if not keep:
            print(name)

    elif mode == "delete-default-objects":
        protect = False
        if kind == "Service" and name == "kubernetes":
            protect = True
        elif kind == "Endpoints" and name == "kubernetes":
            protect = True
        elif kind == "EndpointSlice" and name.startswith("kubernetes"):
            protect = True
        elif kind == "ServiceAccount" and name == "default":
            protect = True
        elif kind == "ConfigMap" and name == "kube-root-ca.crt":
            protect = True
        elif kind == "Secret":
            ann = md.get("annotations", {})
            if ann.get("kubernetes.io/service-account.name") == "default":
                protect = True

        if not protect:
            print(name)

    elif mode == "delete-cluster-scoped":
        if gardener_managed(item):
            continue

        keep = False
        if kind in {"ClusterRole", "ClusterRoleBinding"}:
            keep = name in KEEP_CR_EXACT or name.startswith(KEEP_CR_PREFIXES)
        elif kind == "APIService":
            keep = name.endswith(".k8s.io") or name.endswith(".kubernetes.io")
        elif kind in {"MutatingWebhookConfiguration", "ValidatingWebhookConfiguration"}:
            keep = name.startswith(("gardener", "shoot-", "system:"))
        elif kind == "PriorityClass":
            keep = name.startswith("system-")
        elif kind == "StorageClass":
            ann = md.get("annotations", {})
            keep = ann.get("storageclass.kubernetes.io/is-default-class") == "true" or ann.get("storageclass.beta.kubernetes.io/is-default-class") == "true"
        else:
            keep = False

        if not keep:
            print(name)

    elif mode == "delete-crds":
        keep = gardener_managed(item) or name.endswith(KEEP_CRD_SUFFIXES)
        if not keep:
            spec = item.get("spec", {})
            names = spec.get("names", {})
            print("\t".join([
                name,
                spec.get("scope", ""),
                names.get("plural", ""),
                spec.get("group", ""),
            ]))
'
}

clear_finalizers() {
  local resource="$1"
  local namespace="$2"
  local name="$3"

  if [[ -n "$namespace" ]]; then
    kubectl patch -n "$namespace" "$resource" "$name" --type=merge -p '{"metadata":{"finalizers":[]}}' >/dev/null 2>&1 || true
  else
    kubectl patch "$resource" "$name" --type=merge -p '{"metadata":{"finalizers":[]}}' >/dev/null 2>&1 || true
  fi
}

clear_object_finalizers_by_ref() {
  local resource="$1"
  local namespace="$2"
  local name="$3"

  if [[ -n "$namespace" ]]; then
    kubectl patch -n "$namespace" "$resource" "$name" --type=merge -p '{"metadata":{"finalizers":[]}}' >/dev/null 2>&1 || true
  else
    kubectl patch "$resource" "$name" --type=merge -p '{"metadata":{"finalizers":[]}}' >/dev/null 2>&1 || true
  fi
}

cleanup_default_namespace() {
  log "Cleaning namespace/default"

  mapfile -t resources < <(
    kubectl api-resources --verbs=list,delete --namespaced -o name \
      | grep -v -E '^(events|events.events.k8s.io)$' \
      | sort -u
  )

  for resource in "${resources[@]}"; do
    local json
    json="$(kubectl get -n default "$resource" -o json 2>/dev/null || true)"
    [[ -z "$json" ]] && continue

    mapfile -t names < <(printf '%s' "$json" | json_filter delete-default-objects)
    [[ ${#names[@]} -eq 0 ]] && continue

    for name in "${names[@]}"; do
      clear_finalizers "$resource" default "$name"
      kubectl delete -n default "$resource" "$name" --ignore-not-found --wait=false >/dev/null 2>&1 || true
    done
  done
}

cleanup_namespaces() {
  log "Deleting non-standard namespaces"

  local json
  json="$(kubectl get ns -o json)"
  mapfile -t namespaces < <(printf '%s' "$json" | json_filter delete-namespaces)

  [[ ${#namespaces[@]} -eq 0 ]] && return 0

  for ns in "${namespaces[@]}"; do
    log "Deleting namespace/$ns"
    kubectl delete ns "$ns" --ignore-not-found --wait=false >/dev/null 2>&1 || true
  done

  sleep 5

  for ns in "${namespaces[@]}"; do
    kubectl patch ns "$ns" --type=merge -p '{"metadata":{"finalizers":[]},"spec":{"finalizers":[]}}' >/dev/null 2>&1 || true
    kubectl wait --for=delete "namespace/$ns" --timeout=120s >/dev/null 2>&1 || true
  done
}

cleanup_cluster_resource() {
  local resource="$1"

  local json
  json="$(kubectl get "$resource" -o json 2>/dev/null || true)"
  [[ -z "$json" ]] && return 0

  mapfile -t names < <(printf '%s' "$json" | json_filter delete-cluster-scoped)
  [[ ${#names[@]} -eq 0 ]] && return 0

  log "Cleaning $resource"
  for name in "${names[@]}"; do
    clear_finalizers "$resource" "" "$name"
    kubectl delete "$resource" "$name" --ignore-not-found --wait=false >/dev/null 2>&1 || true
  done
}

cleanup_crds() {
  log "Deleting non-standard CRDs"

  local json
  json="$(kubectl get crd -o json 2>/dev/null || true)"
  [[ -z "$json" ]] && return 0

  mapfile -t crds < <(printf '%s' "$json" | json_filter delete-crds)
  [[ ${#crds[@]} -eq 0 ]] && return 0

  for line in "${crds[@]}"; do
    IFS=$'\t' read -r crd scope plural group <<<"$line"
    [[ -z "$crd" || -z "$plural" || -z "$group" ]] && continue

    local fq_resource="${plural}.${group}"
    log "Cleaning CRD/$crd"

    if [[ "$scope" == "Namespaced" ]]; then
      mapfile -t objects < <(kubectl get "$fq_resource" -A -o jsonpath='{range .items[*]}{.metadata.namespace}{"\t"}{.metadata.name}{"\n"}{end}' 2>/dev/null || true)
      for object_ref in "${objects[@]:-}"; do
        [[ -z "$object_ref" ]] && continue
        IFS=$'\t' read -r object_namespace object_name <<<"$object_ref"
        [[ -z "$object_namespace" || -z "$object_name" ]] && continue
        clear_object_finalizers_by_ref "$fq_resource" "$object_namespace" "$object_name"
        kubectl delete -n "$object_namespace" "$fq_resource" "$object_name" --ignore-not-found --wait=false >/dev/null 2>&1 || true
      done
    else
      mapfile -t objects < <(kubectl get "$fq_resource" -o jsonpath='{range .items[*]}{.metadata.name}{"\n"}{end}' 2>/dev/null || true)
      for object_name in "${objects[@]:-}"; do
        [[ -z "$object_name" ]] && continue
        clear_object_finalizers_by_ref "$fq_resource" "" "$object_name"
        kubectl delete "$fq_resource" "$object_name" --ignore-not-found --wait=false >/dev/null 2>&1 || true
      done
    fi

    kubectl patch crd "$crd" --type=merge -p '{"metadata":{"finalizers":[]}}' >/dev/null 2>&1 || true
    kubectl delete crd "$crd" --ignore-not-found --wait=false >/dev/null 2>&1 || true
  done

  kubectl delete crd etcdcopybackupstasks.druid.gardener.cloud
  kubectl delete crd etcdopstasks.druid.gardener.cloud
  kubectl delete crd etcds.druid.gardener.cloud
}

log "Target cluster: $(kubectl config current-context)"
log "This will remove non-standard workloads, namespaces, RBAC, webhooks, API services, and CRDs."

kubectl -n platform-mesh-system delete etcds --all || true
kubectl -n platform-mesh-system delete platformmeshes --all || true
kubectl -n platform-mesh-system delete helmreleases --all || true
kubectl -n platform-mesh-system delete ocirepositories --all || true

kubectl delete crd backups.postgresql.cnpg.io || true
kubectl delete crd clusterimagecatalogs.postgresql.cnpg.io || true
kubectl delete crd clusters.postgresql.cnpg.io || true
kubectl delete crd databases.postgresql.cnpg.io || true
kubectl delete crd failoverquorums.postgresql.cnpg.io || true
kubectl delete crd imagecatalogs.postgresql.cnpg.io || true
kubectl delete crd poolers.postgresql.cnpg.io || true
kubectl delete crd publications.postgresql.cnpg.io || true
kubectl delete crd scheduledbackups.postgresql.cnpg.io || true
kubectl delete crd subscriptions.postgresql.cnpg.io || true

kubectl delete crd accesscontrolpolicies.hub.traefik.io || true
kubectl delete crd aiservices.hub.traefik.io || true
kubectl delete crd apiauths.hub.traefik.io || true
kubectl delete crd apibundles.hub.traefik.io || true
kubectl delete crd apicatalogitems.hub.traefik.io || true
kubectl delete crd apiplans.hub.traefik.io || true
kubectl delete crd apiportalauths.hub.traefik.io || true
kubectl delete crd apiportals.hub.traefik.io || true
kubectl delete crd apiratelimits.hub.traefik.io || true
kubectl delete crd apis.hub.traefik.io || true
kubectl delete crd apiversions.hub.traefik.io || true
kubectl delete crd contentitems.hub.traefik.io || true
kubectl delete crd ingressroutes.traefik.io || true
kubectl delete crd ingressroutetcps.traefik.io || true
kubectl delete crd ingressrouteudps.traefik.io || true
kubectl delete crd managedapplications.hub.traefik.io || true
kubectl delete crd managedsubscriptions.hub.traefik.io || true
kubectl delete crd middlewares.traefik.io || true
kubectl delete crd middlewaretcps.traefik.io || true
kubectl delete crd serverstransports.traefik.io || true
kubectl delete crd serverstransporttcps.traefik.io || true
kubectl delete crd tlsoptions.traefik.io || true
kubectl delete crd tlsstores.traefik.io || true
kubectl delete crd traefikservices.traefik.io || true
kubectl delete crd uplinks.hub.traefik.io || true

cleanup_default_namespace
cleanup_namespaces

cleanup_cluster_resource clusterrolebindings.rbac.authorization.k8s.io
cleanup_cluster_resource clusterroles.rbac.authorization.k8s.io
cleanup_cluster_resource mutatingwebhookconfigurations.admissionregistration.k8s.io
cleanup_cluster_resource validatingwebhookconfigurations.admissionregistration.k8s.io
cleanup_cluster_resource apiservices.apiregistration.k8s.io
cleanup_cluster_resource priorityclasses.scheduling.k8s.io
cleanup_cluster_resource storageclasses.storage.k8s.io

cleanup_crds

log "Done. Remaining namespaces:"
kubectl get ns
