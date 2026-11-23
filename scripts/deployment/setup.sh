#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/../.."
PROFILE=${PROFILE:-minikube}
EG_VERSION=${EG_VERSION:-v1.10.0}
minikube start -p "$PROFILE" --driver=docker --memory=3072
minikube -p "$PROFILE" update-context
helm upgrade --install eg oci://docker.io/envoyproxy/gateway-helm --version "$EG_VERSION" \
  -n envoy-gateway-system --create-namespace --wait --timeout 5m
for version in v1 v2; do
  minikube -p "$PROFILE" image build -t "meeting-rooms:deploy-$version" \
    --build-opt "build-arg=APP_VERSION=$version" .
done
kubectl apply -f deploy/database.yaml
kubectl -n rooms-strategies rollout status deployment/postgres --timeout=180s
kubectl apply -f deploy/migrate.yaml
kubectl -n rooms-strategies wait --for=condition=complete job/rooms-migrate --timeout=120s
kubectl apply -f deploy/apps.yaml -f deploy/gateway.yaml -f deploy/strategies/stable.yaml
kubectl -n rooms-strategies rollout status deployment/rooms-stable --timeout=180s
kubectl -n rooms-strategies rollout status deployment/rooms-green --timeout=180s
kubectl -n rooms-strategies wait --for=condition=Programmed gateway/rooms --timeout=180s
