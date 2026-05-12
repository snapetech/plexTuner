---
id: k3s-deployment
type: how-to
status: stable
tags: [how-to, deployment, k3s, kubernetes, plex]
---

# Deploy IPTV Tunerr on k3s

Use this page when you want to run IPTV Tunerr as a normal k3s workload. For a single host install, see [Deploy IPTV Tunerr](deployment.md). For Plex multi-DVR patterns, see [Plex ops patterns](plex-ops-patterns.md).

This is a supported user deployment shape. It is not the active internal deployment shape for our local Plex/Tunerr host; that host uses systemd as the single registration owner.

## Contract

The same Plex DVR ownership rules apply in k3s:

- one active Tunerr workload per Plex DVR device identity and friendly name
- one stable `IPTV_TUNERR_BASE_URL` that Plex can reach
- one registration owner for a given Plex DVR row
- distinct base URLs, ports, device IDs, and friendly names for intentional multi-DVR buckets
- no local/systemd Tunerr service registering the same Plex DVR identity at the same time

For Plex, duplicate ownership is the failure mode to avoid. A k3s Deployment, a systemd service, and a one-shot registration job can all create the same kind of duplicate/empty DVR rows if they target the same Plex server with the same identity.

## Image

Use a published image:

```bash
ghcr.io/snapetech/iptvtunerr:v0.1.60
```

Or use `latest` if you intentionally track `main`:

```bash
ghcr.io/snapetech/iptvtunerr:latest
```

## Secret

Create provider and Plex credentials as a Secret. Use your own namespace and values.

```bash
kubectl -n media create secret generic iptvtunerr-env \
  --from-literal=IPTV_TUNERR_PROVIDER_USER='provider-user' \
  --from-literal=IPTV_TUNERR_PROVIDER_PASS='provider-pass' \
  --from-literal=IPTV_TUNERR_PROVIDER_URL='https://provider.example' \
  --from-literal=PLEX_HOST='http://plex.example.lan:32400' \
  --from-literal=PLEX_TOKEN='plex-token'
```

## Deployment

Start with one replica. Do not scale a Plex-registering Tunerr Deployment above one replica unless registration is disabled or each replica has distinct identity and routing.

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: iptvtunerr
  namespace: media
spec:
  replicas: 1
  selector:
    matchLabels:
      app: iptvtunerr
  template:
    metadata:
      labels:
        app: iptvtunerr
    spec:
      containers:
        - name: iptvtunerr
          image: ghcr.io/snapetech/iptvtunerr:v0.1.60
          imagePullPolicy: IfNotPresent
          args:
            - run
            - -addr
            - :5004
            - -mode
            - full
            - -register-plex=api
          env:
            - name: IPTV_TUNERR_BASE_URL
              value: http://iptvtunerr.example.lan:5004
            - name: IPTV_TUNERR_DEVICE_NAME
              value: iptvTunerr
            - name: IPTV_TUNERR_TUNER_COUNT
              value: "2"
          envFrom:
            - secretRef:
                name: iptvtunerr-env
          ports:
            - name: http
              containerPort: 5004
          readinessProbe:
            httpGet:
              path: /readyz
              port: http
            initialDelaySeconds: 5
            periodSeconds: 10
          livenessProbe:
            httpGet:
              path: /discover.json
              port: http
            initialDelaySeconds: 30
            periodSeconds: 30
```

## Service

Expose HTTP in a way Plex can reach. The exact Service type depends on your network.

```yaml
apiVersion: v1
kind: Service
metadata:
  name: iptvtunerr
  namespace: media
spec:
  selector:
    app: iptvtunerr
  ports:
    - name: http
      port: 5004
      targetPort: http
  type: LoadBalancer
```

If you use an Ingress, set `IPTV_TUNERR_BASE_URL` to the externally reachable URL, not the in-cluster Service DNS name, because Plex must fetch `/discover.json`, `/lineup.json`, `/guide.xml`, and `/stream/...` from that URL.

## HDHomeRun discovery

Plex wizard discovery uses UDP broadcast. In k3s that usually means either:

- manually adding the tuner by URL in Plex
- exposing the HDHR ports directly on the node with host networking or host ports
- skipping UDP discovery and relying on Plex API registration

For most k3s installs, manual URL add or API registration is simpler than trying to make UDP broadcast traverse the cluster network.

## Multi-DVR buckets

For primary/sports/category buckets, create separate Deployments or supervisor children with:

- unique `IPTV_TUNERR_BASE_URL`
- unique `IPTV_TUNERR_DEVICE_NAME`
- unique device ID settings if explicitly configured
- non-overlapping lineups or guide-number ranges
- only one registering owner per bucket

See [Plex ops patterns](plex-ops-patterns.md) before running multiple Plex DVR buckets.

## Verify

Check the workload:

```bash
kubectl -n media rollout status deploy/iptvtunerr
kubectl -n media logs deploy/iptvtunerr --tail=100
curl -s -o /dev/null -w "%{http_code}\n" http://iptvtunerr.example.lan:5004/discover.json
curl -s -o /dev/null -w "%{http_code}\n" http://iptvtunerr.example.lan:5004/guide.xml
```

Then check Plex. A simple setup should show one IPTV Tunerr DVR with mapped channels. If Plex shows empty duplicates, follow [Plex ops patterns](plex-ops-patterns.md#recovery-duplicate-or-empty-plex-dvr-rows): stop extra owners first, delete the empty rows, and register from the intended owner only.

## Internal production note

Do not use this page to recreate our local production Plex/Tunerr path. That host is intentionally owned by systemd so there is no split-brain between process managers. k3s remains supported for users and lab deployments, but it is not the fallback owner for that local Plex server.

