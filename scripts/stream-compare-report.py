#!/usr/bin/env python3
"""
Summarize artifacts from scripts/stream-compare-harness.sh.
"""
from __future__ import annotations

import argparse
import json
from pathlib import Path
from typing import Any


def read_json(path: Path) -> dict[str, Any]:
    if not path.is_file():
        return {}
    try:
        return json.loads(path.read_text(encoding="utf-8", errors="replace"))
    except json.JSONDecodeError:
        return {}


def read_int(path: Path) -> int | None:
    if not path.is_file():
        return None
    try:
        return int(path.read_text(encoding="utf-8", errors="replace").strip() or "0")
    except ValueError:
        return None


def preview(path: Path, limit: int = 30) -> list[str]:
    if not path.is_file():
        return []
    return path.read_text(encoding="utf-8", errors="replace").splitlines()[:limit]


def extract_user_agent_from_ff_stderr(path: Path) -> str:
    """Parse ffplay/ffprobe verbose stderr to find the User-Agent sent in HTTP requests."""
    if not path.is_file():
        return ""
    # libavformat logs request headers at verbose level, e.g.:
    #   [http @ 0x...] request: GET /path HTTP/1.1\r\nUser-Agent: Lavf62.12.100\r\n...
    # or on a separate line after the request line.
    lines = path.read_text(encoding="utf-8", errors="replace").splitlines()
    for line in lines:
        if "User-Agent:" in line:
            idx = line.index("User-Agent:")
            ua = line[idx + len("User-Agent:"):].strip().rstrip("\\r").rstrip("\\n").strip()
            if ua:
                return ua
    return ""


def summarize_target(base: Path, label: str) -> dict[str, Any]:
    target = base / label
    curl_meta = read_json(target / "curl.meta.json")
    ffprobe_data = read_json(target / "ffprobe.json")
    streams = ffprobe_data.get("streams") if isinstance(ffprobe_data.get("streams"), list) else []
    format_data = ffprobe_data.get("format") if isinstance(ffprobe_data.get("format"), dict) else {}
    ffplay_ua = extract_user_agent_from_ff_stderr(target / "ffplay.stderr")
    ffprobe_ua = extract_user_agent_from_ff_stderr(target / "ffprobe.stderr")
    return {
        "label": label,
        "meta": read_json(target / "meta.json"),
        "curl": curl_meta,
        "curl_preview": preview(target / "curl.preview.txt", 20),
        "ffprobe_exit": read_int(target / "ffprobe.exit"),
        "ffprobe_error": ffprobe_data.get("error", {}),
        "ffprobe_stream_count": len(streams),
        "ffprobe_streams": [
            {
                "index": s.get("index"),
                "codec_type": s.get("codec_type"),
                "codec_name": s.get("codec_name"),
                "profile": s.get("profile"),
                "width": s.get("width"),
                "height": s.get("height"),
                "sample_rate": s.get("sample_rate"),
            }
            for s in streams[:6]
        ],
        "ffprobe_format": {
            "format_name": format_data.get("format_name"),
            "format_long_name": format_data.get("format_long_name"),
            "start_time": format_data.get("start_time"),
            "duration": format_data.get("duration"),
            "size": format_data.get("size"),
            "bit_rate": format_data.get("bit_rate"),
        },
        "ffprobe_stderr": preview(target / "ffprobe.stderr", 40),
        "ffplay_exit": read_int(target / "ffplay.exit"),
        "ffplay_stderr": preview(target / "ffplay.stderr", 60),
        "ffplay_user_agent": ffplay_ua,
        "ffprobe_user_agent": ffprobe_ua,
        "stream_attempts": read_json(target / "stream-attempts.json"),
    }


def compare(data: dict[str, Any]) -> dict[str, Any]:
    direct = data["direct"]
    tunerr = data["tunerr"]
    findings: list[str] = []
    if direct["curl"].get("http_code") and tunerr["curl"].get("http_code"):
        if direct["curl"]["http_code"] != tunerr["curl"]["http_code"]:
            findings.append(
                f"HTTP status differs: direct={direct['curl']['http_code']} tunerr={tunerr['curl']['http_code']}"
            )
    if direct["ffprobe_exit"] != tunerr["ffprobe_exit"]:
        findings.append(
            f"ffprobe exit differs: direct={direct['ffprobe_exit']} tunerr={tunerr['ffprobe_exit']}"
        )
    if direct["ffplay_exit"] != tunerr["ffplay_exit"]:
        findings.append(
            f"ffplay exit differs: direct={direct['ffplay_exit']} tunerr={tunerr['ffplay_exit']}"
        )
    if direct["ffprobe_stream_count"] != tunerr["ffprobe_stream_count"]:
        findings.append(
            f"ffprobe stream count differs: direct={direct['ffprobe_stream_count']} tunerr={tunerr['ffprobe_stream_count']}"
        )
    # User-Agent comparison: if direct succeeds and they use different UAs, this is often the CF cause.
    d_ua = direct.get("ffplay_user_agent") or direct.get("ffprobe_user_agent") or ""
    t_ua = tunerr.get("ffplay_user_agent") or tunerr.get("ffprobe_user_agent") or ""
    if d_ua and t_ua and d_ua != t_ua:
        findings.append(
            f"User-Agent differs: direct={d_ua!r} tunerr={t_ua!r} — "
            "if the stream is Cloudflare-protected, try: IPTV_TUNERR_UPSTREAM_USER_AGENT=lavf"
        )
    elif d_ua and not t_ua:
        findings.append(
            f"User-Agent visible for direct ({d_ua!r}) but not tunerr — "
            "run with FFPLAY_LOGLEVEL=verbose to capture tunerr UA"
        )
    if not findings:
        findings.append("No top-level status mismatch detected; inspect ffplay/ffprobe stderr and the packet capture for lower-level differences.")
    return {"findings": findings, "direct_ua": d_ua, "tunerr_ua": t_ua}


def render_text(data: dict[str, Any]) -> str:
    lines: list[str] = []
    lines.append(f"Run dir: {data['run_dir']}")
    lines.append("")
    for label in ("direct", "tunerr"):
        target = data[label]
        curl = target["curl"]
        fmt = target["ffprobe_format"]
        lines.append(f"[{label}] {target['meta'].get('url', '')}")
        lines.append(
            f"  curl: http={curl.get('http_code', '')} type={curl.get('content_type', '')} bytes={curl.get('size_download', '')} exit={curl.get('exit_code', '')}"
        )
        lines.append(
            f"  ffprobe: exit={target['ffprobe_exit']} streams={target['ffprobe_stream_count']} format={fmt.get('format_name', '')} bitrate={fmt.get('bit_rate', '')}"
        )
        ua = target.get("ffplay_user_agent") or target.get("ffprobe_user_agent") or "(not captured — use FFPLAY_LOGLEVEL=verbose)"
        lines.append(f"  ffplay: exit={target['ffplay_exit']} ua={ua}")
        if target["ffprobe_error"]:
            lines.append(f"  ffprobe error: {target['ffprobe_error']}")
        if target["curl_preview"]:
            lines.append("  curl preview:")
            lines.extend(f"    {line}" for line in target["curl_preview"][:5])
        if target["ffplay_stderr"]:
            lines.append("  ffplay stderr:")
            lines.extend(f"    {line}" for line in target["ffplay_stderr"][:6])
        attempts = target.get("stream_attempts", {})
        recent = attempts.get("attempts", []) if isinstance(attempts, dict) else []
        if recent:
            latest = recent[0]
            lines.append(
                f"  tunerr attempt: final_status={latest.get('final_status', '')} final_mode={latest.get('final_mode', '')} effective_url={latest.get('effective_url', '')}"
            )
        lines.append("")
    lines.append("Findings:")
    for finding in data["compare"]["findings"]:
        lines.append(f"  - {finding}")
    return "\n".join(lines).rstrip() + "\n"


def main() -> None:
    ap = argparse.ArgumentParser()
    ap.add_argument("--dir", required=True, help="Harness output directory")
    ap.add_argument("--json", action="store_true", help="Emit JSON instead of text")
    args = ap.parse_args()

    base = Path(args.dir)
    payload = {
        "run_dir": str(base),
        "direct": summarize_target(base, "direct"),
        "tunerr": summarize_target(base, "tunerr"),
    }
    payload["compare"] = compare(payload)

    if args.json:
        print(json.dumps(payload, indent=2, sort_keys=True))
        return
    print(render_text(payload), end="")


if __name__ == "__main__":
    main()
