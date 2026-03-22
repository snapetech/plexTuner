#!/usr/bin/env python3
"""Summarize a vod-webdav client harness run."""

from __future__ import annotations

import argparse
import json
from pathlib import Path


def load_report(run_dir: Path) -> dict:
    path = run_dir / "report.json"
    return json.loads(path.read_text(encoding="utf-8"))


def summarize(report: dict) -> str:
    lines = []
    lines.append(f"Base URL: {report.get('base_url', '')}")
    lines.append(f"Overall: {'PASS' if report.get('ok') else 'FAIL'}")
    lines.append("")
    for item in report.get("results", []):
        status = item.get("status")
        expected = item.get("expected_status")
        mark = "OK" if item.get("ok") else "FAIL"
        lines.append(f"[{mark}] {item.get('id')} {item.get('method')} {item.get('path')} -> {status} (expected {expected})")
        stderr = (item.get("stderr") or "").strip()
        if stderr:
            lines.append(f"      stderr: {stderr}")
    return "\n".join(lines) + "\n"


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--dir", required=True, help="Harness run directory")
    parser.add_argument("--print", action="store_true", dest="do_print", help="Print summary to stdout")
    args = parser.parse_args()

    run_dir = Path(args.dir)
    report = load_report(run_dir)
    summary = summarize(report)
    if args.do_print:
      print(summary, end="")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
