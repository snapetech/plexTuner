#!/usr/bin/env python3
"""
Summarize artifacts from scripts/multi-stream-harness.sh.
"""
from __future__ import annotations

import argparse
import json
import sys
from pathlib import Path
from typing import Any


def load_json(path: Path) -> Any:
    with path.open("r", encoding="utf-8", errors="replace") as fh:
        return json.load(fh)


def latest_json(dir_path: Path) -> Any | None:
    if not dir_path.is_dir():
        return None
    files = sorted(dir_path.glob("*.json"))
    if not files:
        return None
    try:
        return load_json(files[-1])
    except Exception:
        return None


def summarize_channel(meta_path: Path, run_seconds: float, stagger: float, ordinal: int) -> dict[str, Any]:
    meta = load_json(meta_path)
    expected = max(1.0, run_seconds - stagger * (ordinal - 1))
    time_total = float(meta.get("time_total") or 0.0)
    bytes_written = int(meta.get("bytes_written") or 0)
    exit_code = int(meta.get("exit_code") or 0)
    premature = False
    if exit_code == 0 and time_total and time_total < expected * 0.75 and bytes_written > 0:
        premature = True
    sustained = bytes_written > 0 and (exit_code == 28 or time_total >= expected * 0.75)
    return {
        "label": meta.get("label"),
        "url": meta.get("url"),
        "http_code": meta.get("http_code"),
        "exit_code": exit_code,
        "bytes_written": bytes_written,
        "time_total": time_total,
        "expected_window_s": expected,
        "premature_exit": premature,
        "sustained_read": sustained,
    }


def build_report(out_dir: Path) -> dict[str, Any]:
    summary = {}
    summary_path = out_dir / "summary.txt"
    if summary_path.is_file():
        summary["summary_path"] = str(summary_path)

    run_seconds = 25.0
    stagger = 2.0
    labels: list[str] = []
    if summary_path.is_file():
        for raw in summary_path.read_text(encoding="utf-8", errors="replace").splitlines():
            line = raw.strip()
            if line.startswith("Run Seconds:"):
                try:
                    run_seconds = float(line.split(":", 1)[1].strip())
                except ValueError:
                    pass
            elif line.startswith("Start Stagger Seconds:"):
                try:
                    stagger = float(line.split(":", 1)[1].strip())
                except ValueError:
                    pass
            elif line.startswith("Channels:"):
                labels = [x for x in line.split(":", 1)[1].strip().split() if x]

    channels = []
    for idx, meta_path in enumerate(sorted(out_dir.glob("channel-*/meta.json")), start=1):
        try:
            channels.append(summarize_channel(meta_path, run_seconds, stagger, idx))
        except Exception as exc:
            channels.append({"label": meta_path.parent.name, "error": str(exc)})

    provider = latest_json(out_dir / "provider-profile")
    attempts = latest_json(out_dir / "stream-attempts")
    runtime = latest_json(out_dir / "runtime")

    sustained = sum(1 for row in channels if row.get("sustained_read"))
    premature = sum(1 for row in channels if row.get("premature_exit"))
    zero = sum(1 for row in channels if not row.get("bytes_written"))

    synopsis = {
        "channel_count": len(channels),
        "channel_labels": labels,
        "sustained_reads": sustained,
        "premature_exits": premature,
        "zero_byte_streams": zero,
    }
    if provider:
        synopsis["effective_tuner_limit"] = provider.get("effective_tuner_limit")
        synopsis["learned_tuner_limit"] = provider.get("learned_tuner_limit")
        synopsis["concurrency_signals_seen"] = provider.get("concurrency_signals_seen")
        synopsis["last_concurrency_status"] = provider.get("last_concurrency_status")
    if attempts and isinstance(attempts, dict):
        rows = attempts.get("attempts") or []
        synopsis["recent_final_statuses"] = [row.get("final_status") for row in rows[:10]]

    hypotheses = []
    if sustained >= 2 and premature == 0 and zero == 0:
        hypotheses.append("Parallel live pulls sustained across the run window; no obvious multi-stream collapse reproduced in this sample.")
    if premature > 0:
        hypotheses.append("One or more streams exited well before the expected run window while still producing bytes; inspect stream-attempt snapshots and provider concurrency state around that time.")
    if zero > 0:
        hypotheses.append("One or more streams produced no bytes at all; this is an admission/open-path failure rather than a mid-stream collapse.")
    if provider and (provider.get("concurrency_signals_seen") or 0) > 0:
        hypotheses.append("Provider concurrency pressure was observed during the run; compare learned/effective tuner limits and per-stream outcomes.")
    if not hypotheses:
        hypotheses.append("No decisive pattern yet; inspect per-channel curl stderr, provider snapshots, and stream-attempt records.")

    return {
        "out_dir": str(out_dir),
        "synopsis": synopsis,
        "channels": channels,
        "provider_profile": provider,
        "stream_attempts": attempts,
        "runtime": runtime,
        "hypotheses": hypotheses,
    }


def write_text(report: dict[str, Any], path: Path) -> None:
    syn = report["synopsis"]
    lines = [
        "Multi-Stream Harness Report",
        "",
        f"- Channel count: {syn.get('channel_count')}",
        f"- Sustained reads: {syn.get('sustained_reads')}",
        f"- Premature exits: {syn.get('premature_exits')}",
        f"- Zero-byte streams: {syn.get('zero_byte_streams')}",
    ]
    if syn.get("effective_tuner_limit") is not None:
        lines.append(f"- Effective tuner limit: {syn.get('effective_tuner_limit')}")
    if syn.get("learned_tuner_limit") is not None:
        lines.append(f"- Learned tuner limit: {syn.get('learned_tuner_limit')}")
    if syn.get("concurrency_signals_seen") is not None:
        lines.append(f"- Concurrency signals seen: {syn.get('concurrency_signals_seen')}")
    if syn.get("last_concurrency_status") is not None:
        lines.append(f"- Last concurrency status: {syn.get('last_concurrency_status')}")
    if syn.get("recent_final_statuses"):
        lines.append("- Recent final statuses: " + ", ".join(str(x) for x in syn["recent_final_statuses"]))
    lines.append("")
    lines.append("Channels")
    for row in report["channels"]:
        if "error" in row:
            lines.append(f"- {row.get('label')}: error={row['error']}")
            continue
        lines.append(
            f"- {row.get('label')}: bytes={row.get('bytes_written')} exit={row.get('exit_code')} "
            f"http={row.get('http_code')} time_total={row.get('time_total')} sustained={row.get('sustained_read')} premature={row.get('premature_exit')}"
        )
    lines.append("")
    lines.append("Hypotheses")
    for item in report["hypotheses"]:
        lines.append(f"- {item}")
    path.write_text("\n".join(lines) + "\n", encoding="utf-8")


def main() -> int:
    ap = argparse.ArgumentParser(description="Summarize multi-stream harness artifacts")
    ap.add_argument("--dir", required=True)
    ap.add_argument("--print", action="store_true", dest="print_report")
    args = ap.parse_args()

    out_dir = Path(args.dir).resolve()
    if not out_dir.is_dir():
        print(f"ERROR: not a directory: {out_dir}", file=sys.stderr)
        return 2

    report = build_report(out_dir)
    json_path = out_dir / "report.json"
    txt_path = out_dir / "report.txt"
    json_path.write_text(json.dumps(report, indent=2, sort_keys=True), encoding="utf-8")
    write_text(report, txt_path)

    if args.print_report:
        sys.stdout.write(txt_path.read_text(encoding="utf-8"))
    else:
        print(f"Wrote {txt_path}")
        print(f"Wrote {json_path}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
