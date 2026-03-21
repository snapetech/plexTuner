#!/usr/bin/env python3
"""
Compare a "good" and "bad" channel captured via stream-compare-harness runs.
"""
from __future__ import annotations

import argparse
import json
import sys
from pathlib import Path
from typing import Any
from urllib.parse import urlparse


def load_json(path: Path) -> Any:
    if not path.is_file():
        return {}
    try:
        return json.loads(path.read_text(encoding="utf-8", errors="replace"))
    except json.JSONDecodeError:
        return {}


def load_compare_run(path: Path) -> dict[str, Any]:
    report = load_json(path / "report.json")
    tunerr_attempts = load_json(path / "tunerr" / "stream-attempts.json")
    latest_attempt = {}
    if isinstance(tunerr_attempts, dict):
        rows = tunerr_attempts.get("attempts")
        if isinstance(rows, list) and rows:
            latest_attempt = rows[0] if isinstance(rows[0], dict) else {}
    return {
        "dir": str(path),
        "report": report if isinstance(report, dict) else {},
        "latest_attempt": latest_attempt,
    }


def attempt_summary(attempt: dict[str, Any]) -> dict[str, Any]:
    upstreams = attempt.get("upstreams") or []
    statuses = []
    hosts = []
    outcomes = []
    for up in upstreams:
        if not isinstance(up, dict):
            continue
        status = up.get("status_code")
        if status not in ("", None):
            statuses.append(status)
        outcome = up.get("outcome")
        if outcome:
            outcomes.append(str(outcome))
        host = urlparse(str(up.get("url") or "")).hostname or ""
        if host and host not in hosts:
            hosts.append(host)
    return {
        "channel_id": attempt.get("channel_id"),
        "channel_name": attempt.get("channel_name"),
        "final_status": attempt.get("final_status"),
        "final_mode": attempt.get("final_mode"),
        "duration_ms": attempt.get("duration_ms"),
        "effective_url": attempt.get("effective_url"),
        "effective_host": urlparse(str(attempt.get("effective_url") or "")).hostname or "",
        "upstream_statuses": statuses,
        "upstream_hosts": hosts,
        "upstream_outcomes": outcomes,
    }


def manifest_hosts(manifest: dict[str, Any]) -> list[str]:
    hosts: list[str] = []
    refs = manifest.get("refs") if isinstance(manifest, dict) else None
    if not isinstance(refs, list):
        return hosts
    for ref in refs:
        if not isinstance(ref, dict):
            continue
        resolved = str(ref.get("resolved_ref") or "")
        host = urlparse(resolved).hostname or ""
        if host and host not in hosts:
            hosts.append(host)
        seg = ref.get("tunerr_seg")
        if isinstance(seg, dict):
            decoded = str(seg.get("redacted_url") or "")
            host = urlparse(decoded).hostname or ""
            if host and host not in hosts:
                hosts.append(host)
    return hosts


def classify(good: dict[str, Any], bad: dict[str, Any]) -> list[str]:
    findings: list[str] = []
    g_report = good["report"]
    b_report = bad["report"]
    g_attempt = attempt_summary(good["latest_attempt"])
    b_attempt = attempt_summary(bad["latest_attempt"])

    g_direct = (g_report.get("direct") or {}).get("ffplay_exit")
    g_tunerr = (g_report.get("tunerr") or {}).get("ffplay_exit")
    b_direct = (b_report.get("direct") or {}).get("ffplay_exit")
    b_tunerr = (b_report.get("tunerr") or {}).get("ffplay_exit")

    if b_direct == 0 and b_tunerr not in (0, None):
        findings.append("Bad channel succeeds direct but fails or degrades through Tunerr; this still points at a Tunerr-path issue, not a dead upstream.")
    if b_direct not in (0, None):
        findings.append("Bad channel also fails direct; likely upstream/provider/CDN-specific rather than Tunerr-only.")
    if g_tunerr == 0 and b_tunerr not in (0, None):
        findings.append("Good channel succeeds through Tunerr while bad channel does not; compare channel-class differences rather than global server state.")

    if b_attempt.get("duration_ms") and g_attempt.get("duration_ms"):
        if int(b_attempt["duration_ms"]) > int(g_attempt["duration_ms"]) * 2:
            findings.append(
                f"Bad channel took much longer to stabilize ({b_attempt['duration_ms']}ms vs {g_attempt['duration_ms']}ms); startup latency may be the client-visible failure."
            )

    bad_statuses = {int(x) for x in b_attempt.get("upstream_statuses", []) if str(x).isdigit()}
    if 403 in bad_statuses:
        findings.append("Bad channel saw upstream 403s; inspect per-channel Referer/Origin/header requirements or provider token policy.")
    if 458 in bad_statuses or 509 in bad_statuses:
        findings.append("Bad channel hit provider concurrency/limit signaling; treat it as capacity/panel policy, not generic playback.")

    bad_outcomes = " ".join(b_attempt.get("upstream_outcomes", [])).lower()
    if "ffmpeg_hls_failed" in bad_outcomes or "ffmpeg" in bad_outcomes:
        findings.append("Bad channel still traversed an ffmpeg failure path before relay; remux avoidance may still need a tighter classifier for this channel class.")

    g_manifest = ((g_report.get("direct") or {}).get("manifest") or {})
    b_manifest = ((b_report.get("direct") or {}).get("manifest") or {})
    g_hosts = manifest_hosts(g_manifest)
    b_hosts = manifest_hosts(b_manifest)
    if len(b_hosts) > len(g_hosts):
        findings.append(
            f"Bad channel references a broader host set than the good channel ({len(b_hosts)} vs {len(g_hosts)}); cross-host CDN behavior may be the real split."
        )

    if not findings:
        findings.append("No decisive split detected at the summary layer; inspect the paired run dirs and PMS logs for client-side timeout differences.")
    return findings


def build_payload(good_dir: Path, bad_dir: Path) -> dict[str, Any]:
    good = load_compare_run(good_dir)
    bad = load_compare_run(bad_dir)
    payload = {
        "good": {
            "dir": good["dir"],
            "report": good["report"],
            "attempt": attempt_summary(good["latest_attempt"]),
        },
        "bad": {
            "dir": bad["dir"],
            "report": bad["report"],
            "attempt": attempt_summary(bad["latest_attempt"]),
        },
    }
    payload["findings"] = classify(good, bad)
    return payload


def render_text(payload: dict[str, Any]) -> str:
    lines: list[str] = []
    lines.append("Channel Diff Report")
    lines.append("")
    for label in ("good", "bad"):
        item = payload[label]
        attempt = item["attempt"]
        report = item["report"]
        tunerr = report.get("tunerr") if isinstance(report, dict) else {}
        direct = report.get("direct") if isinstance(report, dict) else {}
        lines.append(f"[{label}] {attempt.get('channel_name') or attempt.get('channel_id') or item['dir']}")
        lines.append(f"  run: {item['dir']}")
        lines.append(
            f"  direct ffplay={direct.get('ffplay_exit')} tunerr ffplay={tunerr.get('ffplay_exit')} "
            f"tunerr http={((tunerr.get('curl') or {}).get('http_code'))}"
        )
        lines.append(
            f"  final_status={attempt.get('final_status')} final_mode={attempt.get('final_mode')} duration_ms={attempt.get('duration_ms')}"
        )
        lines.append(
            f"  effective_host={attempt.get('effective_host')} upstream_statuses={attempt.get('upstream_statuses')}"
        )
        if attempt.get("upstream_hosts"):
            lines.append(f"  upstream_hosts={attempt.get('upstream_hosts')}")
        if attempt.get("upstream_outcomes"):
            lines.append(f"  upstream_outcomes={attempt.get('upstream_outcomes')[:4]}")
        lines.append("")
    lines.append("Findings")
    for finding in payload["findings"]:
        lines.append(f"  - {finding}")
    return "\n".join(lines).rstrip() + "\n"


def main() -> int:
    ap = argparse.ArgumentParser(description="Compare good vs bad channel stream-compare runs")
    ap.add_argument("--good", required=True)
    ap.add_argument("--bad", required=True)
    ap.add_argument("--out-dir", default="")
    ap.add_argument("--json", action="store_true")
    ap.add_argument("--print", action="store_true", dest="print_report")
    args = ap.parse_args()

    good_dir = Path(args.good).resolve()
    bad_dir = Path(args.bad).resolve()
    payload = build_payload(good_dir, bad_dir)

    if args.out_dir:
        out_dir = Path(args.out_dir).resolve()
        out_dir.mkdir(parents=True, exist_ok=True)
        (out_dir / "report.json").write_text(json.dumps(payload, indent=2, sort_keys=True) + "\n", encoding="utf-8")
        (out_dir / "report.txt").write_text(render_text(payload), encoding="utf-8")

    if args.json:
        print(json.dumps(payload, indent=2, sort_keys=True))
        return 0
    if args.print_report:
        sys.stdout.write(render_text(payload))
        return 0
    print(render_text(payload), end="")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
