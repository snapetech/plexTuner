#!/usr/bin/env python3
"""Compare two vod-webdav client harness bundles."""

from __future__ import annotations

import argparse
import json
from pathlib import Path

INTERESTING_HEADERS = {
    "accept-ranges",
    "allow",
    "content-length",
    "content-range",
    "content-type",
    "dav",
    "etag",
    "ms-author-via",
    "x-content-type-options",
}


def load_report(run_dir: Path) -> dict:
    return json.loads((run_dir / "report.json").read_text(encoding="utf-8"))


def index_results(report: dict) -> dict[str, dict]:
    return {item["id"]: item for item in report.get("results", [])}


def load_headers(run_dir: Path, item: dict) -> dict[str, str]:
    path = run_dir / item["headers_file"]
    headers: dict[str, str] = {}
    for line in path.read_text(encoding="utf-8", errors="replace").splitlines():
        if not line or ":" not in line:
            continue
        key, value = line.split(":", 1)
        key = key.strip().lower()
        if key in INTERESTING_HEADERS:
            headers[key] = value.strip()
    return headers


def summarize_diff(label_a: str, report_a: dict, label_b: str, report_b: dict) -> str:
    a = index_results(report_a)
    b = index_results(report_b)
    step_ids = sorted(set(a) | set(b))

    lines = []
    lines.append(f"Compare: {label_a} vs {label_b}")
    lines.append(f"{label_a}: {'PASS' if report_a.get('ok') else 'FAIL'}")
    lines.append(f"{label_b}: {'PASS' if report_b.get('ok') else 'FAIL'}")
    lines.append("")

    mismatches = 0
    header_mismatches = 0
    for step_id in step_ids:
        left = a.get(step_id)
        right = b.get(step_id)
        if left is None or right is None:
            mismatches += 1
            lines.append(f"[MISSING] {step_id} present only in {'left' if right is None else 'right'} bundle")
            continue
        left_status = left.get("status")
        right_status = right.get("status")
        left_ok = left.get("ok")
        right_ok = right.get("ok")
        if left_status != right_status or left_ok != right_ok:
            mismatches += 1
            lines.append(
                f"[DIFF] {step_id}: {label_a} -> {left_status} ({'OK' if left_ok else 'FAIL'}), "
                f"{label_b} -> {right_status} ({'OK' if right_ok else 'FAIL'})"
            )
            continue

        left_headers = load_headers(Path(report_a["_dir"]), left)
        right_headers = load_headers(Path(report_b["_dir"]), right)
        differing = []
        for key in sorted(set(left_headers) | set(right_headers)):
            if left_headers.get(key) != right_headers.get(key):
                differing.append((key, left_headers.get(key), right_headers.get(key)))
        if differing:
            header_mismatches += 1
            lines.append(f"[HEADER] {step_id}:")
            for key, left_value, right_value in differing:
                lines.append(f"  - {key}: {label_a}={left_value!r} {label_b}={right_value!r}")

    if mismatches == 0 and header_mismatches == 0:
        lines.append("No status or header differences.")
    else:
        lines.append("")
        lines.append(f"Status differences: {mismatches}")
        lines.append(f"Header differences: {header_mismatches}")
    return "\n".join(lines) + "\n"


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--left", required=True, help="Left/base harness run directory")
    parser.add_argument("--right", required=True, help="Right/compare harness run directory")
    parser.add_argument("--left-label", default="left", help="Label for the left run")
    parser.add_argument("--right-label", default="right", help="Label for the right run")
    parser.add_argument("--print", action="store_true", dest="do_print", help="Print diff summary to stdout")
    args = parser.parse_args()

    left_report = load_report(Path(args.left))
    right_report = load_report(Path(args.right))
    left_report["_dir"] = args.left
    right_report["_dir"] = args.right

    report = summarize_diff(
        args.left_label,
        left_report,
        args.right_label,
        right_report,
    )
    if args.do_print:
        print(report, end="")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
