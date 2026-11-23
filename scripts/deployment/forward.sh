#!/usr/bin/env bash
set -euo pipefail
service=$(kubectl -n envoy-gateway-system get service \
  -l gateway.envoyproxy.io/owning-gateway-namespace=rooms-strategies,gateway.envoyproxy.io/owning-gateway-name=rooms \
  -o jsonpath='{.items[0].metadata.name}')
exec kubectl -n envoy-gateway-system port-forward "service/$service" "${DEMO_PORT:-8888}:80"
