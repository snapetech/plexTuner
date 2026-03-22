#!/usr/bin/env python3
"""Compare two vod-webdav client harness bundles."""

from __future__ import annotations

import argparse
import json
from pathlib import Path


def load_report(run_dir: Path) -> dict:
    return json.loads((run_dir / "report.json").read_text(encoding="utf-8"))


def index_results(report: dict) -> dict[str, dict]:
    return {item["id"]: item for item in report.get("results", [])}


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

    if mismatches == 0:
        lines.append("No status differences.")
    else:
        lines.append("")
        lines.append(f"Status differences: {mismatches}")
    return "\n".join(lines) + "\n"


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--left", required=True, help="Left/base harness run directory")
    parser.add_argument("--right", required=True, help="Right/compare harness run directory")
    parser.add_argument("--left-label", default="left", help="Label for the left run")
    parser.add_argument("--right-label", default="right", help="Label for the right run")
    parser.add_argument("--print", action="store_true", dest="do_print", help="Print diff summary to stdout")
    args = parser.parse_args()

    report = summarize_diff(
        args.left_label,
        load_report(Path(args.left)),
        args.right_label,
        load_report(Path(args.right)),
    )
    if args.do_print:
        print(report, end="")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
