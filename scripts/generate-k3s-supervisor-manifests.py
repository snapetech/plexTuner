#!/usr/bin/env python3
from __future__ import annotations

import argparse
import copy
import json
import re
from pathlib import Path
from typing import Any

import yaml


CATEGORIES = [
    "bcastus",
    "newsus",
    "sportsa",
    "sportsb",
    "moviesprem",
    "generalent",
    "docsfam",
    "ukie",
    "eunordics",
    "eusouth",
    "eueast",
    "latin",
    "otherworld",
]

REGION_BUCKET_PRESETS = {
    "na_en": {
        # Use the broad live feed for the HDHR wizard lane so it resembles the
        # larger guide choices users expect (e.g. Rogers West), then prune/cap
        # in-app (music-strip heuristic + lineup max 479).
        "m3u_url": "http://iptv-m3u-server.plex.svc/live.m3u",
        "xmltv_url": "http://iptv-m3u-server.plex.svc/xmltv.xml",
        "prefer_langs": "en,eng",
        "prefer_latin": "true",
        "title_fallback": "channel",
        "lineup_max": 479,
        "live_epg_only": True,
        "epg_prune": True,
        "stream_transcode": "on",
        "lineup_shape": "na_en",
        "lineup_region_profile": "ca_west",
    },
}


def choose_hdhr_preset(country: str, postal_code: str, timezone_name: str) -> tuple[str, dict[str, Any]]:
    c = (country or "").strip().upper()
    pc = re.sub(r"\s+", "", (postal_code or "")).upper()
    tz = (timezone_name or "").strip()
    tz_l = tz.lower()
    # Prefer timezone as the strongest local signal (do not log it).
    if tz_l.startswith("america/"):
        return "na_en", dict(REGION_BUCKET_PRESETS["na_en"])
    # Current repo buckets don't split CA/US cleanly; bcastus is the best North America-ish
    # default for an English wizard path. Keep the decision local and do not log the postal code.
    if c in {"CA", "CAN", "US", "USA"}:
        return "na_en", dict(REGION_BUCKET_PRESETS["na_en"])
    # Postal heuristic fallback (Canada format A1A1A1)
    if re.match(r"^[A-Z]\d[A-Z]\d[A-Z]\d$", pc):
        return "na_en", dict(REGION_BUCKET_PRESETS["na_en"])
    return "na_en", dict(REGION_BUCKET_PRESETS["na_en"])


def load_yaml_docs(path: Path) -> list[dict[str, Any]]:
    return [d for d in yaml.safe_load_all(path.read_text()) if d]


def env_list_to_map(env_list: list[dict[str, Any]]) -> dict[str, str]:
    out: dict[str, str] = {}
    for item in env_list or []:
        if "value" in item:
            out[item["name"]] = str(item["value"])
    return out


def parse_addr(args: list[str]) -> str:
    for a in args:
        if a.startswith("-addr=:"):
            return a.split(":", 1)[1]
    return "5004"


def build_supervisor_json(
    multi_deploys: list[dict[str, Any]],
    hdhr_deploy: dict[str, Any],
    *,
    hdhr_m3u_url: str,
    hdhr_xmltv_url: str,
    hdhr_lineup_max: int,
    hdhr_live_epg_only: bool,
    hdhr_epg_prune: bool,
    hdhr_stream_transcode: str,
    hdhr_prefer_langs: str,
    hdhr_prefer_latin: bool,
    hdhr_non_latin_title_fallback: str,
    hdhr_lineup_shape: str,
    hdhr_lineup_region_profile: str,
) -> dict[str, Any]:
    by_name = {d["metadata"]["name"]: d for d in multi_deploys}
    instances: list[dict[str, Any]] = []

    # HDHR child from the existing hdhr-test deployment (inherits many envs from parent envFrom),
    # but run it in wizard mode (no Plex DB registration) per desired testing flow.
    hdhr_container = hdhr_deploy["spec"]["template"]["spec"]["containers"][0]
    hdhr_base = "http://plextuner-hdhr.plex.home"
    for a in hdhr_container.get("args", []):
        if isinstance(a, str) and a.startswith("-base-url="):
            hdhr_base = a.split("=", 1)[1]
            break
    hdhr_args = [
        "run",
        "-mode=easy",
        "-addr=:5004",
        "-catalog=/data/hdhr-main/catalog.json",
        f"-base-url={hdhr_base}",
    ]
    instances.append(
        {
            "name": "hdhr-main",
            "args": hdhr_args,
            "env": {
                "PLEX_TUNER_HDHR_NETWORK_MODE": "true",
                "PLEX_TUNER_SSDP_DISABLED": "false",
                "PLEX_TUNER_HDHR_SCAN_POSSIBLE": "true",
                "PLEX_TUNER_FRIENDLY_NAME": "hdhr",
                "PLEX_TUNER_HDHR_FRIENDLY_NAME": "hdhr",
                "PLEX_TUNER_HDHR_MANUFACTURER": "Silicondust",
                "PLEX_TUNER_HDHR_MODEL_NUMBER": "HDHR5-2US",
                "PLEX_TUNER_HDHR_FIRMWARE_NAME": "hdhomerun5_atsc",
                "PLEX_TUNER_HDHR_FIRMWARE_VERSION": "20240101",
                "PLEX_TUNER_HDHR_DEVICE_AUTH": "plextuner",
                "PLEX_TUNER_M3U_URL": hdhr_m3u_url,
                "PLEX_TUNER_XMLTV_URL": hdhr_xmltv_url,
                "PLEX_TUNER_LIVE_EPG_ONLY": "true" if hdhr_live_epg_only else "false",
                "PLEX_TUNER_EPG_PRUNE_UNLINKED": "true" if hdhr_epg_prune else "false",
                "PLEX_TUNER_LINEUP_MAX_CHANNELS": str(hdhr_lineup_max),
                "PLEX_TUNER_LINEUP_DROP_MUSIC": "true",
                "PLEX_TUNER_LINEUP_SHAPE": hdhr_lineup_shape,
                "PLEX_TUNER_LINEUP_REGION_PROFILE": hdhr_lineup_region_profile,
                "PLEX_TUNER_STREAM_TRANSCODE": hdhr_stream_transcode,
                "PLEX_TUNER_STREAM_BUFFER_BYTES": "-1",
                "PLEX_TUNER_XMLTV_PREFER_LANGS": hdhr_prefer_langs,
                "PLEX_TUNER_XMLTV_PREFER_LATIN": "true" if hdhr_prefer_latin else "false",
                "PLEX_TUNER_XMLTV_NON_LATIN_TITLE_FALLBACK": hdhr_non_latin_title_fallback,
            },
        }
    )

    base_port = 5101
    for idx, cat in enumerate(CATEGORIES):
        dep_name = f"plextuner-{cat}"
        dep = by_name[dep_name]
        c = dep["spec"]["template"]["spec"]["containers"][0]
        env_map = env_list_to_map(c.get("env", []))
        child_port = str(base_port + idx)

        child_env = {}
        # Preserve category-specific settings, omit common reaper/token settings provided by parent env.
        for k in [
            "PLEX_TUNER_M3U_URL",
            "PLEX_TUNER_XMLTV_URL",
            "PLEX_TUNER_LIVE_EPG_ONLY",
            "PLEX_TUNER_EPG_PRUNE_UNLINKED",
            "PLEX_TUNER_STREAM_TRANSCODE",
            "PLEX_TUNER_STREAM_BUFFER_BYTES",
            "PLEX_TUNER_LINEUP_MAX_CHANNELS",
            "TZ",
        ]:
            if k in env_map:
                child_env[k] = env_map[k]
        # Identity signal for Plex DVR tab/title.
        child_env["PLEX_TUNER_DEVICE_ID"] = cat
        child_env["PLEX_TUNER_FRIENDLY_NAME"] = cat
        # Preserve old injected DVR URI shape so Plex reinjection is unnecessary.
        child_env["PLEX_TUNER_BASE_URL"] = f"http://plextuner-{cat}.plex.svc:5004"
        child_env["PLEX_TUNER_SSDP_DISABLED"] = "true"
        # Keep injected DVRs working, but make category tuners less attractive in Plex's HDHR wizard.
        child_env["PLEX_TUNER_HDHR_SCAN_POSSIBLE"] = "false"
        # In-app XMLTV guide text normalization (can be removed if undesired).
        child_env["PLEX_TUNER_XMLTV_PREFER_LANGS"] = "en,eng"
        child_env["PLEX_TUNER_XMLTV_PREFER_LATIN"] = "true"
        child_env["PLEX_TUNER_XMLTV_NON_LATIN_TITLE_FALLBACK"] = "channel"

        instances.append(
            {
                "name": cat,
                "args": ["run", "-mode=easy", f"-addr=:{child_port}", f"-catalog=/data/{cat}/catalog.json"],
                "env": child_env,
            }
        )

    return {
        "restart": True,
        "restartDelay": "2s",
        "failFast": False,
        "instances": instances,
    }


def build_singlepod_manifest(
    supervisor_cfg: dict[str, Any],
    hdhr_deploy: dict[str, Any],
    image: str,
) -> list[dict[str, Any]]:
    hdhr_tmpl = hdhr_deploy["spec"]["template"]["spec"]
    hdhr_container = hdhr_tmpl["containers"][0]

    # ConfigMap with supervisor JSON
    configmap = {
        "apiVersion": "v1",
        "kind": "ConfigMap",
        "metadata": {"name": "plextuner-supervisor-config", "namespace": "plex"},
        "data": {"supervisor.json": json.dumps(supervisor_cfg, indent=2)},
    }

    # Base deployment from hdhr-test deployment, then mutate into supervisor mode.
    dep = {
        "apiVersion": "apps/v1",
        "kind": "Deployment",
        "metadata": {"name": "plextuner-supervisor", "namespace": "plex", "labels": {"app": "plextuner-supervisor"}},
        "spec": {
            "replicas": 1,
            "strategy": {"type": "Recreate"},
            "selector": {"matchLabels": {"app": "plextuner-supervisor"}},
            "template": {
                "metadata": {"labels": {"app": "plextuner-supervisor"}},
                "spec": {
                    "nodeSelector": copy.deepcopy(hdhr_tmpl.get("nodeSelector", {"media": "plex"})),
                    "hostNetwork": True,
                    "dnsPolicy": "ClusterFirstWithHostNet",
                    "dnsConfig": copy.deepcopy(hdhr_tmpl.get("dnsConfig", {})),
                    "containers": [
                        {
                            "name": "plextuner",
                            "image": image,
                            "imagePullPolicy": hdhr_container.get("imagePullPolicy", "IfNotPresent"),
                            "args": ["supervise", "-config", "/config/supervisor.json"],
                            "envFrom": copy.deepcopy(hdhr_container.get("envFrom", [])),
                            "env": [
                                {
                                    "name": "PLEX_TUNER_PMS_TOKEN",
                                    "valueFrom": {
                                        "secretKeyRef": {"name": "plex-token", "key": "token"}
                                    },
                                },
                                {"name": "PLEX_TUNER_PMS_URL", "value": "http://plex.plex.svc:32400"},
                                {"name": "PLEX_TUNER_PLEX_SESSION_REAPER", "value": "true"},
                                {"name": "PLEX_TUNER_PLEX_SESSION_REAPER_POLL_S", "value": "2"},
                                {"name": "PLEX_TUNER_PLEX_SESSION_REAPER_IDLE_S", "value": "15"},
                                {"name": "PLEX_TUNER_PLEX_SESSION_REAPER_RENEW_LEASE_S", "value": "20"},
                                {"name": "PLEX_TUNER_PLEX_SESSION_REAPER_HARD_LEASE_S", "value": "1800"},
                                {"name": "PLEX_TUNER_PLEX_SESSION_REAPER_SSE", "value": "true"},
                            ],
                            "ports": [],
                            "volumeMounts": [
                                {"name": "supervisor-config", "mountPath": "/config"},
                                {"name": "data", "mountPath": "/data"},
                            ],
                            "readinessProbe": {
                                "httpGet": {"path": "/discover.json", "port": 5004},
                                "initialDelaySeconds": 30,
                                "periodSeconds": 10,
                                "failureThreshold": 12,
                            },
                            "livenessProbe": {
                                "httpGet": {"path": "/discover.json", "port": 5004},
                                "initialDelaySeconds": 60,
                                "periodSeconds": 30,
                                "failureThreshold": 5,
                            },
                            "resources": copy.deepcopy(hdhr_container.get("resources", {})),
                        }
                    ],
                    "volumes": [
                        {"name": "supervisor-config", "configMap": {"name": "plextuner-supervisor-config"}},
                        {"name": "data", "emptyDir": {}},
                    ],
                },
            },
        },
    }

    # Ports: HDHR + all child HTTP ports
    ports = [{"name": "hdhr-http", "containerPort": 5004, "protocol": "TCP"}]
    for inst in supervisor_cfg["instances"]:
        if inst["name"] == "hdhr-main":
            continue
        port = int(parse_addr(inst["args"]))
        ports.append({"name": f"p{port}", "containerPort": port, "protocol": "TCP"})
    ports.append({"name": "hdhr-disc", "containerPort": 65001, "protocol": "UDP"})
    ports.append({"name": "hdhr-ctrl", "containerPort": 65001, "protocol": "TCP"})
    dep["spec"]["template"]["spec"]["containers"][0]["ports"] = ports

    # Services: one for HDHR HTTP, one per category preserving existing hostnames.
    services: list[dict[str, Any]] = []
    services.append(
        {
            "apiVersion": "v1",
            "kind": "Service",
            "metadata": {"name": "plextuner-hdhr-test", "namespace": "plex"},
            "spec": {
                "selector": {"app": "plextuner-supervisor"},
                "ports": [{"name": "http", "port": 5004, "targetPort": 5004, "protocol": "TCP"}],
            },
        }
    )
    for inst in supervisor_cfg["instances"]:
        if inst["name"] == "hdhr-main":
            continue
        cat = inst["name"]
        target = int(parse_addr(inst["args"]))
        services.append(
            {
                "apiVersion": "v1",
                "kind": "Service",
                "metadata": {"name": f"plextuner-{cat}", "namespace": "plex"},
                "spec": {
                    "selector": {"app": "plextuner-supervisor"},
                    "ports": [{"name": "http", "port": 5004, "targetPort": target, "protocol": "TCP"}],
                },
            }
        )
    return [configmap, dep, *services]


def build_cutover_tsv(supervisor_cfg: dict[str, Any]) -> str:
    lines = ["# category\told_uri\tnew_uri\turi_changed\tdevice_id\tfriendly_name"]
    for inst in sorted((i for i in supervisor_cfg["instances"] if i["name"] != "hdhr-main"), key=lambda x: x["name"]):
        cat = inst["name"]
        env = inst["env"]
        old_uri = f"http://plextuner-{cat}.plex.svc:5004"
        new_uri = env.get("PLEX_TUNER_BASE_URL", "")
        lines.append(
            "\t".join(
                [
                    cat,
                    old_uri,
                    new_uri,
                    "no" if old_uri == new_uri else "yes",
                    env.get("PLEX_TUNER_DEVICE_ID", ""),
                    env.get("PLEX_TUNER_FRIENDLY_NAME", ""),
                ]
            )
        )
    return "\n".join(lines) + "\n"


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("--k3s-plex-dir", default="/home/keith/Documents/code/k3s/plex")
    ap.add_argument("--out-json", default="plextuner-supervisor-multi.generated.json")
    ap.add_argument("--out-yaml", default="plextuner-supervisor-singlepod.generated.yaml")
    ap.add_argument("--out-tsv", default="plextuner-supervisor-cutover-map.generated.tsv")
    ap.add_argument("--country", default="", help="Country hint for HDHR wizard profile selection (e.g. CA, US)")
    ap.add_argument("--postal-code", default="", help="Postal/ZIP hint for HDHR wizard profile selection (used locally only; not logged)")
    ap.add_argument("--timezone", default="", help="Timezone hint (e.g. Area/City) for HDHR wizard profile selection; used locally only and not logged")
    ap.add_argument(
        "--hdhr-region-profile",
        default="auto",
        choices=["auto", "na_en"],
        help="HDHR wizard feed preset profile (auto selects by country/postal; defaults to English North America)",
    )
    ap.add_argument("--hdhr-m3u-url", default="", help="Override HDHR wizard-feed M3U URL")
    ap.add_argument("--hdhr-xmltv-url", default="", help="Override HDHR wizard-feed XMLTV URL")
    ap.add_argument("--hdhr-lineup-max", type=int, default=-1, help="Override HDHR child lineup max (wizard-safe default from preset)")
    ap.add_argument("--hdhr-live-epg-only", action="store_true", default=None, help="Keep only EPG-linked channels in HDHR child")
    ap.add_argument("--no-hdhr-live-epg-only", dest="hdhr_live_epg_only", action="store_false")
    ap.add_argument("--hdhr-epg-prune", action="store_true", default=None, help="Prune unlinked channels from HDHR guide/m3u")
    ap.add_argument("--no-hdhr-epg-prune", dest="hdhr_epg_prune", action="store_false")
    ap.add_argument(
        "--hdhr-stream-transcode",
        choices=["on", "off", "auto", "auto_cached"],
        default="",
        help="HDHR child stream transcode mode (default from region preset)",
    )
    args = ap.parse_args()

    root = Path(args.k3s_plex_dir)
    multi = load_yaml_docs(root / "plextuner-deployments-multi.yaml")
    hdhr = load_yaml_docs(root / "plextuner-hdhr-test-deployment.yaml")[0]
    image = hdhr["spec"]["template"]["spec"]["containers"][0]["image"]

    if args.hdhr_region_profile == "auto":
        preset_name, preset = choose_hdhr_preset(args.country, args.postal_code, args.timezone)
    else:
        preset_name, preset = args.hdhr_region_profile, dict(REGION_BUCKET_PRESETS[args.hdhr_region_profile])

    hdhr_m3u_url = args.hdhr_m3u_url or preset["m3u_url"]
    hdhr_xmltv_url = args.hdhr_xmltv_url or preset["xmltv_url"]
    hdhr_lineup_max = args.hdhr_lineup_max if args.hdhr_lineup_max >= 0 else int(preset["lineup_max"])
    hdhr_live_epg_only = preset["live_epg_only"] if args.hdhr_live_epg_only is None else bool(args.hdhr_live_epg_only)
    hdhr_epg_prune = preset["epg_prune"] if args.hdhr_epg_prune is None else bool(args.hdhr_epg_prune)
    hdhr_stream_transcode = args.hdhr_stream_transcode or preset["stream_transcode"]

    sup = build_supervisor_json(
        multi,
        hdhr,
        hdhr_m3u_url=hdhr_m3u_url,
        hdhr_xmltv_url=hdhr_xmltv_url,
        hdhr_lineup_max=hdhr_lineup_max,
        hdhr_live_epg_only=hdhr_live_epg_only,
        hdhr_epg_prune=hdhr_epg_prune,
        hdhr_stream_transcode=hdhr_stream_transcode,
        hdhr_prefer_langs=preset["prefer_langs"],
        hdhr_prefer_latin=(str(preset["prefer_latin"]).lower() == "true"),
        hdhr_non_latin_title_fallback=preset["title_fallback"],
        hdhr_lineup_shape=preset.get("lineup_shape", ""),
        hdhr_lineup_region_profile=preset.get("lineup_region_profile", ""),
    )
    manifest = build_singlepod_manifest(sup, hdhr, image)
    tsv = build_cutover_tsv(sup)

    (root / args.out_json).write_text(json.dumps(sup, indent=2) + "\n")
    (root / args.out_yaml).write_text(yaml.safe_dump_all(manifest, sort_keys=False))
    (root / args.out_tsv).write_text(tsv)

    print(f"HDHR preset: {preset_name} (timezone/country/postal used locally; not echoed)")
    print(f"Wrote {root / args.out_json}")
    print(f"Wrote {root / args.out_yaml}")
    print(f"Wrote {root / args.out_tsv}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
