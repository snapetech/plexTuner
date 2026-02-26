#!/usr/bin/env bash
set -euo pipefail

# Deploy or remove the Plex /media/providers label rewrite proxy in k8s.
#
# This installs a small Python proxy service in front of Plex for the /media/providers
# path only, rewriting Live TV provider labels per DVR (using /livetv/dvrs lineup titles).
#
# Actions:
#   apply    - create/update ConfigMap+Deployment+Service and patch Ingress path
#   remove   - remove Ingress path override and proxy resources
#
# Assumptions:
# - Namespace `plex`
# - Ingress name `plex`
# - Plex token secret `plex-token` with key `token`
# - PMS service name `plex` on port 32400

ACTION="${1:-apply}"
NAMESPACE="${NAMESPACE:-plex}"
INGRESS_NAME="${INGRESS_NAME:-plex}"
PROXY_DEPLOY="${PROXY_DEPLOY:-plex-label-proxy}"
PROXY_SVC="${PROXY_SVC:-plex-label-proxy}"
PROXY_PORT="${PROXY_PORT:-33240}"
PLEX_UPSTREAM_URL="${PLEX_UPSTREAM_URL:-http://plex.${NAMESPACE}.svc:32400}"
SCRIPT_PATH="${SCRIPT_PATH:-$(pwd)/scripts/plex-media-providers-label-proxy.py}"
TOKEN_SECRET_NAME="${TOKEN_SECRET_NAME:-plex-token}"
TOKEN_SECRET_KEY="${TOKEN_SECRET_KEY:-token}"

if [[ ! -f "$SCRIPT_PATH" ]]; then
  echo "script not found: $SCRIPT_PATH" >&2
  exit 1
fi

require_kubectl() {
  command -v kubectl >/dev/null 2>&1 || {
    echo "kubectl is required" >&2
    exit 1
  }
}

apply_proxy() {
  kubectl -n "$NAMESPACE" create configmap plex-media-providers-label-proxy-script \
    --from-file=plex-media-providers-label-proxy.py="$SCRIPT_PATH" \
    --dry-run=client -o yaml | kubectl apply -f -

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
        image: python:3.11-alpine
        command: ["sh","-lc"]
        args:
          - |
            python /opt/proxy/plex-media-providers-label-proxy.py \
              --listen 0.0.0.0:${PROXY_PORT} \
              --upstream ${PLEX_UPSTREAM_URL} \
              --token "\$PLEX_TOKEN" \
              --log-level INFO
        env:
        - name: PLEX_TOKEN
          valueFrom:
            secretKeyRef:
              name: ${TOKEN_SECRET_NAME}
              key: ${TOKEN_SECRET_KEY}
        ports:
        - containerPort: ${PROXY_PORT}
          name: http
        volumeMounts:
        - name: script
          mountPath: /opt/proxy
          readOnly: true
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
      volumes:
      - name: script
        configMap:
          name: plex-media-providers-label-proxy-script
          defaultMode: 0555
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

  echo
  echo "Ingress path order:"
  kubectl -n "$NAMESPACE" get ingress "$INGRESS_NAME" -o jsonpath='{range .spec.rules[0].http.paths[*]}{.path}{" -> "}{.backend.service.name}{":"}{.backend.service.port.number}{"\n"}{end}'
}

remove_proxy() {
  # Remove /media/providers path if it routes to the proxy.
  mapfile -t paths < <(kubectl -n "$NAMESPACE" get ingress "$INGRESS_NAME" -o jsonpath='{range .spec.rules[0].http.paths[*]}{.path}{"\n"}{end}')
  idx=-1
  for i in "${!paths[@]}"; do
    if [[ "${paths[$i]}" == "/media/providers" ]]; then
      idx="$i"
      break
    fi
  done
  if [[ "$idx" != "-1" ]]; then
    echo "Removing ingress path /media/providers (index $idx)"
    kubectl -n "$NAMESPACE" patch ingress "$INGRESS_NAME" --type='json' -p="[ {\"op\":\"remove\",\"path\":\"/spec/rules/0/http/paths/${idx}\"} ]"
  else
    echo "Ingress path /media/providers not present"
  fi

  kubectl -n "$NAMESPACE" delete deploy "$PROXY_DEPLOY" --ignore-not-found
  kubectl -n "$NAMESPACE" delete svc "$PROXY_SVC" --ignore-not-found
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

