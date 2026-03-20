#!/usr/bin/env python3
"""List recent harness output directories under .diag/{live-race,stream-compare,multi-stream}."""

from __future__ import annotations

import argparse
import json
import sys
from pathlib import Path
from typing import Any

FAMILIES = ("live-race", "stream-compare", "multi-stream")


def runs_for_family(diag: Path, family: str, limit: int) -> list[dict[str, Any]]:
    base = diag / family
    if not base.is_dir():
        return []
    dirs = [p for p in base.iterdir() if p.is_dir()]
    dirs.sort(key=lambda p: p.stat().st_mtime, reverse=True)
    out: list[dict[str, Any]] = []
    for p in dirs[:limit]:
        st = p.stat()
        out.append(
            {
                "path": str(p.resolve()),
                "name": p.name,
                "mtime_sec": int(st.st_mtime),
            }
        )
    return out


def main() -> int:
    ap = argparse.ArgumentParser(
        description="List recent harness runs under .diag/ (newest first per family).",
    )
    ap.add_argument(
        "--root",
        type=Path,
        default=Path("."),
        help="Repository root (default: current directory)",
    )
    ap.add_argument(
        "--limit",
        type=int,
        default=5,
        help="Max runs per family (default: 5)",
    )
    ap.add_argument(
        "--json",
        action="store_true",
        help="Print JSON instead of human-readable text",
    )
    args = ap.parse_args()
    diag = args.root / ".diag"
    if not diag.is_dir():
        print(
            f"No {diag} directory — run a harness first or pass --root to the repo.",
            file=sys.stderr,
        )
        return 0

    report: dict[str, Any] = {"diag": str(diag.resolve()), "families": {}}
    for fam in FAMILIES:
        report["families"][fam] = runs_for_family(diag, fam, args.limit)

    if args.json:
        print(json.dumps(report, indent=2))
        return 0

    print(f"Harness runs under {diag.resolve()} (newest first, up to {args.limit} each):\n")
    for fam in FAMILIES:
        rows = report["families"][fam]
        print(f"[{fam}]")
        if not rows:
            print("  (none)\n")
            continue
        for r in rows:
            print(f"  {r['name']}")
            print(f"    {r['path']}")
        print()
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
