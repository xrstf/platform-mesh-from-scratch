#!/usr/bin/env bash

set -euo pipefail
cd "$(dirname "$0")"

NAMESPACE="${NAMESPACE:-platform-mesh-system}"

info() {
  echo "[INFO] $*"
}

die() {
  echo "[ERROR] $*" >&2
  exit 1
}

command -v kubectl >/dev/null 2>&1 || die "kubectl not found"
command -v openssl >/dev/null 2>&1 || die "openssl not found"
command -v helm >/dev/null 2>&1 || die "helm not found"

secret_exists() {
  kubectl get secret "$1" -n "$NAMESPACE" >/dev/null 2>&1
}

secret_key() {
  kubectl get secret "$1" -n "$NAMESPACE" -o jsonpath="{.data.$2}" | base64 -d
}

ensure_keycloak_admin_secret() {
  if secret_exists keycloak-admin; then
    info "Secret keycloak-admin already exists, skipping"
    return
  fi

  local password
  local client_secret
  password="$(openssl rand -base64 32)"
  client_secret="$(openssl rand -base64 32)"

  kubectl create secret generic keycloak-admin \
    --namespace "$NAMESPACE" \
    --from-literal=username="keycloak-admin" \
    --from-literal=password="$password" \
    --from-literal=secret="$client_secret"

  info "Created secret keycloak-admin"
}

ensure_keycloak_db_secrets() {
  if secret_exists cnpg-keycloak-user && secret_exists keycloak-db-credentials; then
    info "Secrets cnpg-keycloak-user and keycloak-db-credentials already exist, skipping"
    return
  fi

  local username="keycloak"
  local password

  if secret_exists cnpg-keycloak-user; then
    username="$(secret_key cnpg-keycloak-user username)"
    password="$(secret_key cnpg-keycloak-user password)"
  elif secret_exists keycloak-db-credentials; then
    username="$(secret_key keycloak-db-credentials username)"
    password="$(secret_key keycloak-db-credentials password)"
  else
    password="$(openssl rand -base64 32)"
  fi

  kubectl create secret generic cnpg-keycloak-user \
    --namespace "$NAMESPACE" \
    --from-literal=username="$username" \
    --from-literal=password="$password"

  kubectl create secret generic keycloak-db-credentials \
    --namespace "$NAMESPACE" \
    --from-literal=username="$username" \
    --from-literal=password="$password"

  info "Ensured secrets cnpg-keycloak-user and keycloak-db-credentials"
}

ensure_openfga_db_secrets() {
  if secret_exists cnpg-openfga-user && secret_exists openfga-postgres-credentials; then
    info "Secrets cnpg-openfga-user and openfga-postgres-credentials already exist, skipping"
    return
  fi

  local username="openfga"
  local password

  if secret_exists cnpg-openfga-user; then
    username="$(secret_key cnpg-openfga-user username)"
    password="$(secret_key cnpg-openfga-user password)"
  elif secret_exists openfga-postgres-credentials; then
    password="$(secret_key openfga-postgres-credentials password)"
  else
    password="$(openssl rand -base64 32)"
  fi

  kubectl create secret generic cnpg-openfga-user \
    --namespace "$NAMESPACE" \
    --from-literal=username="$username" \
    --from-literal=password="$password"

  kubectl create secret generic openfga-postgres-credentials \
    --namespace "$NAMESPACE" \
    --from-literal=password="$password" \
    --from-literal=postgres-password="$password"

  info "Ensured secrets cnpg-openfga-user and openfga-postgres-credentials"
}

ensure_search_operator_secret() {
  if secret_exists search-operator-opensearch; then
    info "Secret search-operator-opensearch already exists, skipping"
    return
  fi

  if [[ -n "${OPENSEARCH_URL:-}" && -n "${OPENSEARCH_USERNAME:-}" && -n "${OPENSEARCH_PASSWORD:-}" ]]; then
    kubectl create secret generic search-operator-opensearch \
      --namespace "$NAMESPACE" \
      --from-literal=url="$OPENSEARCH_URL" \
      --from-literal=username="$OPENSEARCH_USERNAME" \
      --from-literal=password="$OPENSEARCH_PASSWORD"

    info "Created secret search-operator-opensearch"
  else
    info "Skipping search-operator-opensearch secret (OPENSEARCH_URL/USERNAME/PASSWORD not set)"
  fi
}

bootstrap_secrets() {
  info "Creating namespace ${NAMESPACE} (idempotent)"
  kubectl create namespace "$NAMESPACE" --dry-run=client -o yaml | kubectl apply -f -

  ensure_keycloak_admin_secret
  ensure_keycloak_db_secrets
  ensure_openfga_db_secrets
  ensure_search_operator_secret
}

install_flux() {
  local namespace=flux-system

  # Flux 2.18+ has issues with unknown fields that our templates emit. So we use 2.17.x.
  # This is still an issue with PM 0.5.2.
  helm upgrade \
    --install \
    --namespace "$namespace" --create-namespace \
    --version 2.17.2 \
    --set imageAutomationController.create=false \
    --set imageReflectionController.create=false \
    --set notificationController.create=false \
    --set helmController.container.additionalArgs[0]="--concurrent=19" \
    --set sourceController.container.additionalArgs[0]="--requeue-dependency=5s" \
    flux oci://ghcr.io/fluxcd-community/charts/flux2

  kubectl wait --namespace "$namespace" --for=condition=available deployment/helm-controller
  kubectl wait --namespace "$namespace" --for=condition=available deployment/source-controller
  kubectl wait --namespace "$namespace" --for=condition=available deployment/kustomize-controller
}

install_flux

kubectl create namespace kcp-operator --dry-run=client -o yaml | kubectl apply -f -
kubectl create namespace observability --dry-run=client -o yaml | kubectl apply -f -

bootstrap_secrets

# The PM operator needs these even when the deploy subroutine is disabled.
kubectl apply --filename ocmcrds

# Install Platform Mesh.
kubectl apply --filename ocirepositories
kubectl apply --filename helmreleases

# This can fail on the first attempt while CRDs are still settling.
kubectl apply --filename stuff || true
