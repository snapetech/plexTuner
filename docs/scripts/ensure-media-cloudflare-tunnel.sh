#!/usr/bin/env bash
set -euo pipefail

# Reconcile a named Cloudflare Tunnel and CNAME for a Plex Live TV proxy.
#
# Required env:
#   CLOUDFLARE_ACCOUNT_ID
#   CLOUDFLARE_API_TOKEN     (Account Tunnel edit, Zone read, DNS edit)
# Optional env:
#   CLOUDFLARE_ZONE_NAME     default: example.com
#   MEDIA_TUNNEL_HOSTNAME    default: media.example.com
#   CLOUDFLARE_TUNNEL_NAME   default: media-plex-live-tv
#   CLOUDFLARED_CONFIG       default: /etc/cloudflared/media-tunnel.yml
#   CLOUDFLARED_CREDENTIALS_DIR default: /etc/cloudflared
#   CLOUDFLARED_SERVICE      default: cloudflared-media.service

hostname="${MEDIA_TUNNEL_HOSTNAME:-media.example.com}"
zone_name="${CLOUDFLARE_ZONE_NAME:-example.com}"
tunnel_name="${CLOUDFLARE_TUNNEL_NAME:-media-plex-live-tv}"
cloudflared_config="${CLOUDFLARED_CONFIG:-/etc/cloudflared/media-tunnel.yml}"
credentials_dir="${CLOUDFLARED_CREDENTIALS_DIR:-/etc/cloudflared}"
service_name="${CLOUDFLARED_SERVICE:-cloudflared-media.service}"

: "${CLOUDFLARE_ACCOUNT_ID:?missing CLOUDFLARE_ACCOUNT_ID}"
: "${CLOUDFLARE_API_TOKEN:?missing CLOUDFLARE_API_TOKEN}"

cf() {
  local method="$1"
  local path="$2"
  local data="${3:-}"
  local args=(-fsS -X "$method" -H "Authorization: Bearer $CLOUDFLARE_API_TOKEN" -H "Content-Type: application/json")
  if [[ -n "$data" ]]; then
    args+=(--data "$data")
  fi
  curl "${args[@]}" "https://api.cloudflare.com/client/v4${path}"
}

zone_id=$(cf GET "/zones?name=${zone_name}" | jq -er '.result[0].id')

tunnel_json=$(cf GET "/accounts/${CLOUDFLARE_ACCOUNT_ID}/cfd_tunnel?name=${tunnel_name}&is_deleted=false")
tunnel_id=$(jq -r '.result[0].id // empty' <<<"$tunnel_json")

if [[ -z "$tunnel_id" ]]; then
  tunnel_secret=$(openssl rand -base64 32)
  tunnel_payload=$(jq -n --arg name "$tunnel_name" --arg tunnel_secret "$tunnel_secret" \
    '{name:$name,tunnel_secret:$tunnel_secret}')
  tunnel_create=$(cf POST "/accounts/${CLOUDFLARE_ACCOUNT_ID}/cfd_tunnel" "$tunnel_payload")
  tunnel_id=$(jq -er '.result.id' <<<"$tunnel_create")

  install -d -m 0750 "$credentials_dir"
  jq -n \
    --arg AccountTag "$CLOUDFLARE_ACCOUNT_ID" \
    --arg TunnelID "$tunnel_id" \
    --arg TunnelName "$tunnel_name" \
    --arg TunnelSecret "$tunnel_secret" \
    '{AccountTag:$AccountTag,TunnelID:$TunnelID,TunnelName:$TunnelName,TunnelSecret:$TunnelSecret}' \
    > "${credentials_dir}/${tunnel_id}.json"
  chmod 0600 "${credentials_dir}/${tunnel_id}.json"
fi

cat > "$cloudflared_config" <<EOF
tunnel: ${tunnel_id}
credentials-file: ${credentials_dir}/${tunnel_id}.json
protocol: quic
metrics: 127.0.0.1:49312
no-autoupdate: true

ingress:
  - hostname: ${hostname}
    service: http://127.0.0.1:33240
    originRequest:
      httpHostHeader: ${hostname}
  - service: http_status:404
EOF
chmod 0640 "$cloudflared_config"

config_payload=$(jq -n --arg hostname "$hostname" \
  '{config:{ingress:[
    {hostname:$hostname, service:"http://127.0.0.1:33240", originRequest:{httpHostHeader:$hostname}},
    {service:"http_status:404"}
  ]}}')
cf PUT "/accounts/${CLOUDFLARE_ACCOUNT_ID}/cfd_tunnel/${tunnel_id}/configurations" "$config_payload" >/dev/null

cname_target="${tunnel_id}.cfargotunnel.com"
records=$(cf GET "/zones/${zone_id}/dns_records?name=${hostname}")
record_id=$(jq -r '.result[0].id // empty' <<<"$records")
record_payload=$(jq -n \
  --arg type CNAME \
  --arg name "$hostname" \
  --arg content "$cname_target" \
  '{type:$type,name:$name,content:$content,ttl:1,proxied:true,comment:"Managed by ensure-media-cloudflare-tunnel.sh"}')

if [[ -n "$record_id" ]]; then
  cf PUT "/zones/${zone_id}/dns_records/${record_id}" "$record_payload" >/dev/null
else
  cf POST "/zones/${zone_id}/dns_records" "$record_payload" >/dev/null
fi

if systemctl list-unit-files "$service_name" >/dev/null 2>&1; then
  systemctl restart "$service_name"
fi

printf '%s CNAME %s proxied\n' "$hostname" "$cname_target"
