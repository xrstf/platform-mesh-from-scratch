#!/usr/bin/env bash

set -euo pipefail
cd $(dirname $0)

##############################################################
# install Flux

(
  namespace=flux-system

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

  kubectl wait --namespace "$namespace" --for=condition=available deployment helm-controller
  kubectl wait --namespace "$namespace" --for=condition=available deployment source-controller
  kubectl wait --namespace "$namespace" --for=condition=available deployment kustomize-controller
)

kubectl create ns platform-mesh-system || true
kubectl create ns kcp-operator || true

# this should be able to be done by the helm chart, if enabled
kubectl create ns observability || true

# the PM operator needs these, even when the deploy subroutine is disabled
kubectl apply -f ocmcrds

# install platform mesh
kubectl apply -f ocirepositories
kubectl apply -f helmreleases

# this can fail since we need to wait for CRDs to be installed.
kubectl apply -f stuff


###################################################################

# TODO: is this needed? i never ran this, but the secrets still exist...
###########################
exit 0

NAMESPACE=platform-mesh-system

# Keycloak admin secret
# The infra chart only renders keycloak-admin when keycloak.operator.admin.password is set.
# In production, pre-create this secret here so the chart skips rendering it.
if kubectl get secret keycloak-admin -n "${NAMESPACE}" >/dev/null 2>&1; then
  info "Secret keycloak-admin already exists, skipping"
else
  KEYCLOAK_ADMIN_PASSWORD=$(openssl rand -base64 32)
  KEYCLOAK_CLIENT_SECRET=$(openssl rand -base64 32)
  kubectl create secret generic keycloak-admin \
    -n "${NAMESPACE}" \
    --from-literal=username="keycloak-admin" \
    --from-literal=password="${KEYCLOAK_ADMIN_PASSWORD}" \
    --from-literal=secret="${KEYCLOAK_CLIENT_SECRET}"
  info "Created secret keycloak-admin"
fi

# Keycloak DB credentials (cnpg-keycloak-user + keycloak-db-credentials)
# The infra chart only renders these when keycloak.operator.db.password is set.
if kubectl get secret cnpg-keycloak-user -n "${NAMESPACE}" >/dev/null 2>&1; then
  info "Secret cnpg-keycloak-user already exists, skipping"
else
  KEYCLOAK_DB_PASSWORD=$(openssl rand -base64 32)
  kubectl create secret generic cnpg-keycloak-user \
    -n "${NAMESPACE}" \
    --from-literal=username="keycloak" \
    --from-literal=password="${KEYCLOAK_DB_PASSWORD}"
  kubectl create secret generic keycloak-db-credentials \
    -n "${NAMESPACE}" \
    --from-literal=username="keycloak" \
    --from-literal=password="${KEYCLOAK_DB_PASSWORD}"
  info "Created secrets cnpg-keycloak-user and keycloak-db-credentials"
fi

# OpenFGA DB credentials (cnpg-openfga-user + openfga-postgres-credentials)
# The infra chart only renders cnpg-openfga-user when cnpg.roles.keycloak.password is set.
if kubectl get secret cnpg-openfga-user -n "${NAMESPACE}" >/dev/null 2>&1; then
  info "Secret cnpg-openfga-user already exists, skipping"
else
  OPENFGA_DB_PASSWORD=$(openssl rand -base64 32)
  kubectl create secret generic cnpg-openfga-user \
    -n "${NAMESPACE}" \
    --from-literal=username="openfga" \
    --from-literal=password="${OPENFGA_DB_PASSWORD}"
  kubectl create secret generic openfga-postgres-credentials \
    -n "${NAMESPACE}" \
    --from-literal=password="${OPENFGA_DB_PASSWORD}" \
    --from-literal=postgres-password="${OPENFGA_DB_PASSWORD}"
  info "Created secrets cnpg-openfga-user and openfga-postgres-credentials"
fi

# OpenSearch credentials (used by search-operator via OPENSEARCH_URL / OPENSEARCH_USERNAME / OPENSEARCH_PASSWORD env vars)
# These are not auto-generated — OpenSearch must be provisioned separately.
# Set OPENSEARCH_URL, OPENSEARCH_USERNAME, OPENSEARCH_PASSWORD before running this script
# or create the secret manually afterwards.
if kubectl get secret search-operator-opensearch -n "${NAMESPACE}" >/dev/null 2>&1; then
  info "Secret search-operator-opensearch already exists, skipping"
elif [[ -n "${OPENSEARCH_URL:-}" && -n "${OPENSEARCH_USERNAME:-}" && -n "${OPENSEARCH_PASSWORD:-}" ]]; then
  kubectl create secret generic search-operator-opensearch \
    -n "${NAMESPACE}" \
    --from-literal=url="${OPENSEARCH_URL}" \
    --from-literal=username="${OPENSEARCH_USERNAME}" \
    --from-literal=password="${OPENSEARCH_PASSWORD}"
  info "Created secret search-operator-opensearch"
else
  info "Skipping search-operator-opensearch secret (OPENSEARCH_URL/USERNAME/PASSWORD not set)"
  info "  Create it manually: kubectl create secret generic search-operator-opensearch -n ${NAMESPACE} --from-literal=url=<url> --from-literal=username=<user> --from-literal=password=<pass>"
fi
