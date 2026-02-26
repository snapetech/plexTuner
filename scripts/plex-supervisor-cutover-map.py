#!/usr/bin/env python3
"""
Generate a cutover mapping for injected category DVRs when migrating to the
single-pod supervisor deployment.

By default this assumes the current 13-pod category scheme uses:
  http://plextuner-<category>.plex.svc:5004

and compares that to the per-child PLEX_TUNER_BASE_URL values in the supervisor
JSON config (excluding the HDHR child).
"""
from __future__ import annotations

import argparse
import json
import sys
from pathlib import Path


def load(path: Path) -> dict:
    with path.open() as f:
        return json.load(f)


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument(
        "--config",
        default="k8s/plextuner-supervisor-multi.example.json",
        help="Supervisor JSON config",
    )
    ap.add_argument(
        "--old-uri-template",
        default="http://plextuner-{category}.plex.svc:5004",
        help="Template for pre-supervisor injected DVR URIs",
    )
    ap.add_argument(
        "--out",
        default="-",
        help="Output TSV path or - for stdout",
    )
    args = ap.parse_args()

    data = load(Path(args.config))
    rows = []
    for inst in data.get("instances", []):
        name = (inst.get("name") or "").strip()
        if not name or name == "hdhr-main":
            continue
        env = inst.get("env") or {}
        new_uri = (env.get("PLEX_TUNER_BASE_URL") or "").strip()
        if not new_uri:
            continue
        old_uri = args.old_uri_template.format(category=name)
        rows.append(
            (
                name,
                old_uri,
                new_uri,
                "no" if old_uri == new_uri else "yes",
                (env.get("PLEX_TUNER_DEVICE_ID") or "").strip(),
                (env.get("PLEX_TUNER_FRIENDLY_NAME") or "").strip(),
            )
        )

    rows.sort(key=lambda r: r[0])
    out = sys.stdout if args.out == "-" else open(args.out, "w", encoding="utf-8")
    try:
        out.write("# category\told_uri\tnew_uri\turi_changed\tdevice_id\tfriendly_name\n")
        for r in rows:
            out.write("\t".join(r) + "\n")
    finally:
        if out is not sys.stdout:
            out.close()
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
