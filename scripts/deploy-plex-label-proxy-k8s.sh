#!/usr/bin/env bash
set -euo pipefail

# Deploy or remove the Plex /media/providers label rewrite proxy in k8s.
#
# This installs `iptv-tunerr plex-label-proxy` as a small Deployment in front of
# Plex. It rewrites Live TV provider labels per DVR (sourced from /livetv/dvrs
# lineupTitle) so multi-DVR setups render distinct source tabs across all Plex
# clients (TV/native + Plex Web via -spoof-identity).
#
# Actions:
#   apply    - create/update Deployment+Service and patch Ingress path
#   remove   - remove Ingress path override and proxy resources
#
# Defaults assume:
# - Namespace `plex`
# - Ingress name `plex`
# - Plex token secret `plex-token` with key `token`
# - PMS service name `plex` on port 32400
# - iptv-tunerr image already loaded into the cluster as `iptv-tunerr:latest`
#
# Override any of the variables below via environment.

ACTION="${1:-apply}"
NAMESPACE="${NAMESPACE:-plex}"
INGRESS_NAME="${INGRESS_NAME:-plex}"
PROXY_DEPLOY="${PROXY_DEPLOY:-plex-label-proxy}"
PROXY_SVC="${PROXY_SVC:-plex-label-proxy}"
PROXY_PORT="${PROXY_PORT:-33240}"
PLEX_UPSTREAM_URL="${PLEX_UPSTREAM_URL:-http://plex.${NAMESPACE}.svc:32400}"
TOKEN_SECRET_NAME="${TOKEN_SECRET_NAME:-plex-token}"
TOKEN_SECRET_KEY="${TOKEN_SECRET_KEY:-token}"
PROXY_IMAGE="${PROXY_IMAGE:-iptv-tunerr:latest}"
PROXY_IMAGE_PULL_POLICY="${PROXY_IMAGE_PULL_POLICY:-IfNotPresent}"
STRIP_PREFIX="${STRIP_PREFIX:-iptvtunerr-}"
REFRESH_SECONDS="${REFRESH_SECONDS:-30}"
SPOOF_IDENTITY="${SPOOF_IDENTITY:-true}"

# Plex Web (4.156.x) sources its source-tab labels from the server-level
# friendlyName, so without spoof-identity the rewrite is invisible there.
# Default it ON; set SPOOF_IDENTITY=false to opt out (TV/native clients only).
# Note: identity-spoof is best-effort — see runbook for limits (browsers strip
# fragments from Referer, so per-tab identity rewrite is rarely possible).
SPOOF_ARG_LINE=""
if [[ "$SPOOF_IDENTITY" == "true" ]]; then
  SPOOF_ARG_LINE='          - "-spoof-identity"'
fi

require_kubectl() {
  command -v kubectl >/dev/null 2>&1 || {
    echo "kubectl is required" >&2
    exit 1
  }
}

apply_proxy() {
  cat <<YAML | kubectl -n "$NAMESPACE" apply -f -
apiVersion: apps/v1
kind: Deployment
metadata:
  name: ${PROXY_DEPLOY}
spec:
  replicas: 1
  selector:
    matchLabels:
      app: ${PROXY_DEPLOY}
  template:
    metadata:
      labels:
        app: ${PROXY_DEPLOY}
    spec:
      containers:
      - name: proxy
        image: ${PROXY_IMAGE}
        imagePullPolicy: ${PROXY_IMAGE_PULL_POLICY}
        command: ["iptv-tunerr"]
        args:
          - "plex-label-proxy"
          - "-listen=0.0.0.0:${PROXY_PORT}"
          - "-upstream=${PLEX_UPSTREAM_URL}"
          - "-strip-prefix=${STRIP_PREFIX}"
          - "-refresh-seconds=${REFRESH_SECONDS}"
${SPOOF_ARG_LINE}
        env:
        - name: IPTV_TUNERR_PMS_TOKEN
          valueFrom:
            secretKeyRef:
              name: ${TOKEN_SECRET_NAME}
              key: ${TOKEN_SECRET_KEY}
        ports:
        - containerPort: ${PROXY_PORT}
          name: http
        readinessProbe:
          httpGet:
            path: /identity
            port: ${PROXY_PORT}
          initialDelaySeconds: 2
          periodSeconds: 5
        livenessProbe:
          httpGet:
            path: /identity
            port: ${PROXY_PORT}
          initialDelaySeconds: 10
          periodSeconds: 10
        resources:
          requests:
            cpu: "10m"
            memory: "32Mi"
          limits:
            cpu: "200m"
            memory: "128Mi"
---
apiVersion: v1
kind: Service
metadata:
  name: ${PROXY_SVC}
spec:
  selector:
    app: ${PROXY_DEPLOY}
  ports:
  - name: http
    port: ${PROXY_PORT}
    targetPort: ${PROXY_PORT}
YAML

  kubectl -n "$NAMESPACE" rollout status "deploy/${PROXY_DEPLOY}" --timeout=180s

  # Add /media/providers exact path to the front of paths list if not present.
  if kubectl -n "$NAMESPACE" get ingress "$INGRESS_NAME" -o jsonpath='{range .spec.rules[0].http.paths[*]}{.path}{"\n"}{end}' | grep -qx '/media/providers'; then
    echo "Ingress path /media/providers already present; patching backend to ${PROXY_SVC}:${PROXY_PORT}"
    kubectl -n "$NAMESPACE" patch ingress "$INGRESS_NAME" --type='json' -p="[
      {\"op\":\"replace\",\"path\":\"/spec/rules/0/http/paths/0/backend/service/name\",\"value\":\"${PROXY_SVC}\"},
      {\"op\":\"replace\",\"path\":\"/spec/rules/0/http/paths/0/backend/service/port/number\",\"value\":${PROXY_PORT}}
    ]"
  else
    echo "Adding /media/providers ingress route -> ${PROXY_SVC}:${PROXY_PORT}"
    kubectl -n "$NAMESPACE" patch ingress "$INGRESS_NAME" --type='json' -p="[
      {\"op\":\"add\",\"path\":\"/spec/rules/0/http/paths/0\",\"value\":{\"path\":\"/media/providers\",\"pathType\":\"Exact\",\"backend\":{\"service\":{\"name\":\"${PROXY_SVC}\",\"port\":{\"number\":${PROXY_PORT}}}}}}
    ]"
  fi

  # When -spoof-identity is enabled, also route / and /identity through the
  # proxy so the root MediaContainer friendlyName is rewritten for Plex Web.
  if [[ "$SPOOF_IDENTITY" == "true" ]]; then
    for extra_path in "/identity" "/"; do
      pt="Exact"
      if kubectl -n "$NAMESPACE" get ingress "$INGRESS_NAME" -o jsonpath='{range .spec.rules[0].http.paths[*]}{.path}{"\n"}{end}' | grep -qx "$extra_path"; then
        echo "Ingress path ${extra_path} already present; leaving as-is (verify backend points at ${PROXY_SVC})"
      else
        echo "Adding ${extra_path} ingress route -> ${PROXY_SVC}:${PROXY_PORT}"
        kubectl -n "$NAMESPACE" patch ingress "$INGRESS_NAME" --type='json' -p="[
          {\"op\":\"add\",\"path\":\"/spec/rules/0/http/paths/0\",\"value\":{\"path\":\"${extra_path}\",\"pathType\":\"${pt}\",\"backend\":{\"service\":{\"name\":\"${PROXY_SVC}\",\"port\":{\"number\":${PROXY_PORT}}}}}}
        ]"
      fi
    done
  fi

  echo
  echo "Ingress path order:"
  kubectl -n "$NAMESPACE" get ingress "$INGRESS_NAME" -o jsonpath='{range .spec.rules[0].http.paths[*]}{.path}{" -> "}{.backend.service.name}{":"}{.backend.service.port.number}{"\n"}{end}'
}

remove_proxy() {
  # Remove any ingress path routing to the proxy service.
  while true; do
    mapfile -t entries < <(kubectl -n "$NAMESPACE" get ingress "$INGRESS_NAME" -o jsonpath='{range .spec.rules[0].http.paths[*]}{.path}{"|"}{.backend.service.name}{"\n"}{end}')
    idx=-1
    for i in "${!entries[@]}"; do
      path="${entries[$i]%%|*}"
      svc="${entries[$i]##*|}"
      if [[ "$svc" == "$PROXY_SVC" ]]; then
        echo "Removing ingress path ${path} (index $i)"
        kubectl -n "$NAMESPACE" patch ingress "$INGRESS_NAME" --type='json' -p="[ {\"op\":\"remove\",\"path\":\"/spec/rules/0/http/paths/${i}\"} ]"
        idx="$i"
        break
      fi
    done
    if [[ "$idx" == "-1" ]]; then
      break
    fi
  done

  kubectl -n "$NAMESPACE" delete deploy "$PROXY_DEPLOY" --ignore-not-found
  kubectl -n "$NAMESPACE" delete svc "$PROXY_SVC" --ignore-not-found
  # Legacy ConfigMap from the previous Python-based deploy; tolerate absence.
  kubectl -n "$NAMESPACE" delete configmap plex-media-providers-label-proxy-script --ignore-not-found
}

require_kubectl

case "$ACTION" in
  apply) apply_proxy ;;
  remove) remove_proxy ;;
  *)
    echo "usage: $0 [apply|remove]" >&2
    exit 2
    ;;
esac
