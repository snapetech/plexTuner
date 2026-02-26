#!/usr/bin/env python3
"""Generate PlexTuner per-channel transcode/profile overrides from ffprobe criteria.

This is an offline/helper tool that reuses the existing runtime override hooks:
  - PLEX_TUNER_PROFILE_OVERRIDES_FILE
  - PLEX_TUNER_TRANSCODE_OVERRIDES_FILE

It probes channel stream URLs (typically PlexTuner /lineup.json entries) and emits
JSON maps for channels that match criteria likely to cause Plex Web trouble
(interlaced video, >30fps, HE-AAC, unsupported codecs, high bitrate, etc.).
"""

from __future__ import annotations

import argparse
import json
import math
import os
import subprocess
import sys
import time
import urllib.parse
import urllib.request
from dataclasses import dataclass
from typing import Any


VIDEO_FRIENDLY = {"h264", "avc", "mpeg2video", "mpeg4"}
AUDIO_FRIENDLY = {"aac", "ac3", "eac3", "mp3", "mp2"}


@dataclass
class ChannelRow:
    id: str
    name: str
    guide: str
    url: str


def load_json(source: str) -> Any:
    if source.startswith(("http://", "https://")):
        with urllib.request.urlopen(source, timeout=20) as resp:
            return json.loads(resp.read().decode("utf-8", "replace"))
    with open(source, "r", encoding="utf-8") as f:
        return json.load(f)


def absolutize_url(base: str, url: str) -> str:
    if url.startswith(("http://", "https://")):
        return url
    if not base:
        return url
    return urllib.parse.urljoin(base.rstrip("/") + "/", url.lstrip("/"))


def channel_id_from_url(url: str) -> str:
    path = urllib.parse.urlparse(url).path
    if "/stream/" in path:
        return path.rsplit("/stream/", 1)[-1]
    if "/auto/" in path:
        rest = path.rsplit("/auto/", 1)[-1]
        if rest.startswith("v"):
            rest = rest[1:]
        return rest
    return path.rsplit("/", 1)[-1] or url


def parse_lineup(raw: Any, base_url: str) -> list[ChannelRow]:
    if not isinstance(raw, list):
        raise ValueError("Expected lineup JSON array")
    rows: list[ChannelRow] = []
    for item in raw:
        if not isinstance(item, dict):
            continue
        url = str(item.get("URL") or item.get("url") or "").strip()
        if not url:
            continue
        full_url = absolutize_url(base_url, url)
        guide = str(item.get("GuideNumber") or item.get("guideNumber") or "").strip()
        name = str(item.get("GuideName") or item.get("guideName") or item.get("Name") or "").strip()
        cid = channel_id_from_url(full_url)
        rows.append(ChannelRow(id=cid, name=name, guide=guide, url=full_url))
    return rows


def apply_url_rewrites(url: str, rewrites: list[tuple[str, str]]) -> str:
    for old, new in rewrites:
        if url.startswith(old):
            return new + url[len(old) :]
    return url


def ffprobe_json(url: str, timeout_s: int) -> dict[str, Any]:
    cmd = [
        "ffprobe",
        "-v",
        "error",
        "-rw_timeout",
        "15000000",
        "-read_intervals",
        "%+4",
        "-show_streams",
        "-show_format",
        "-of",
        "json",
        url,
    ]
    cp = subprocess.run(
        cmd,
        capture_output=True,
        text=True,
        timeout=timeout_s,
        errors="replace",
    )
    if cp.returncode != 0:
        msg = (cp.stderr or cp.stdout or "").strip()
        raise RuntimeError(msg or f"ffprobe exit {cp.returncode}")
    return json.loads(cp.stdout or "{}")


def _fps_from_ratio(v: str) -> float:
    try:
        num, den = v.split("/", 1)
        num_f = float(num)
        den_f = float(den)
        if den_f == 0:
            return 0.0
        return num_f / den_f
    except Exception:
        return 0.0


def classify_probe(data: dict[str, Any], bitrate_threshold: int) -> tuple[dict[str, Any], list[str], str]:
    streams = data.get("streams") or []
    fmt = data.get("format") or {}
    video = next((s for s in streams if s.get("codec_type") == "video"), {})
    audio = next((s for s in streams if s.get("codec_type") == "audio"), {})
    vcodec = str(video.get("codec_name") or "").lower()
    acodec = str(audio.get("codec_name") or "").lower()
    aprofile = str(audio.get("profile") or "")
    vprofile = str(video.get("profile") or "")
    field_order = str(video.get("field_order") or "").lower()
    fps = _fps_from_ratio(str(video.get("avg_frame_rate") or video.get("r_frame_rate") or "0/0"))
    width = int(video.get("width") or 0)
    height = int(video.get("height") or 0)
    level = int(video.get("level") or 0)
    bitrate = int(fmt.get("bit_rate") or video.get("bit_rate") or audio.get("bit_rate") or 0)
    has_b_frames = int(video.get("has_b_frames") or 0)

    reasons: list[str] = []
    if vcodec and vcodec not in VIDEO_FRIENDLY:
        reasons.append(f"video_codec={vcodec}")
    if acodec and acodec not in AUDIO_FRIENDLY:
        reasons.append(f"audio_codec={acodec}")
    if acodec == "aac" and aprofile and aprofile.lower() not in {"lc", "aac-lc"}:
        reasons.append(f"aac_profile={aprofile}")
    if field_order and field_order not in {"progressive", "unknown"}:
        reasons.append(f"field_order={field_order}")
    if fps > 30.5:
        reasons.append(f"fps={fps:.2f}")
    if bitrate_threshold > 0 and bitrate > bitrate_threshold:
        reasons.append(f"bitrate={bitrate}")
    if vcodec == "h264" and level > 41:
        reasons.append(f"h264_level={level}")
    if vcodec == "h264" and has_b_frames > 2:
        reasons.append(f"bframes={has_b_frames}")

    # Conservative profile selection:
    # - aaccfr for interlace/high-fps/high-bitrate/weird AAC
    # - plexsafe for generic codec compatibility mismatches
    severe = any(
        r.startswith(("field_order=", "fps=", "bitrate=", "aac_profile=")) for r in reasons
    )
    if severe:
        profile = "aaccfr"
    elif reasons:
        profile = "plexsafe"
    else:
        profile = ""

    summary = {
        "video_codec": vcodec,
        "video_profile": vprofile,
        "width": width,
        "height": height,
        "fps": round(fps, 3),
        "field_order": field_order,
        "h264_level": level,
        "audio_codec": acodec,
        "audio_profile": aprofile,
        "bit_rate": bitrate,
    }
    return summary, reasons, profile


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("--lineup-json", required=True, help="Path or URL to HDHR/PlexTuner lineup.json")
    ap.add_argument("--base-url", default="", help="Base URL for relative lineup URLs")
    ap.add_argument(
        "--replace-url-prefix",
        action="append",
        default=[],
        help="Rewrite lineup stream URLs with OLD=NEW prefix replacement (repeatable)",
    )
    ap.add_argument("--channel-id", action="append", default=[], help="Only probe specific channel ID(s)")
    ap.add_argument("--limit", type=int, default=0, help="Probe at most N channels")
    ap.add_argument("--timeout", type=int, default=12, help="ffprobe timeout seconds per channel")
    ap.add_argument("--bitrate-threshold", type=int, default=5_000_000, help="Flag bitrate above this bps")
    ap.add_argument("--emit-profile-overrides", help="Write profile overrides JSON to this path")
    ap.add_argument("--emit-transcode-overrides", help="Write transcode overrides JSON to this path")
    ap.add_argument("--no-transcode-overrides", action="store_true", help="Do not emit transcode=true for flagged channels")
    ap.add_argument("--sleep-ms", type=int, default=0, help="Sleep between probes (ms)")
    args = ap.parse_args()

    lineup_raw = load_json(args.lineup_json)
    rows = parse_lineup(lineup_raw, args.base_url)
    rewrites: list[tuple[str, str]] = []
    for raw in args.replace_url_prefix:
        if "=" not in raw:
            ap.error(f"--replace-url-prefix requires OLD=NEW, got: {raw}")
        old, new = raw.split("=", 1)
        old = old.strip()
        new = new.strip()
        if not old or not new:
            ap.error(f"--replace-url-prefix requires non-empty OLD and NEW, got: {raw}")
        rewrites.append((old, new))
    if rewrites:
        rows = [
            ChannelRow(id=r.id, name=r.name, guide=r.guide, url=apply_url_rewrites(r.url, rewrites))
            for r in rows
        ]
    if args.channel_id:
        wanted = set(args.channel_id)
        rows = [r for r in rows if r.id in wanted]
    if args.limit > 0:
        rows = rows[: args.limit]

    profile_overrides: dict[str, str] = {}
    transcode_overrides: dict[str, bool] = {}
    report: list[dict[str, Any]] = []

    print(f"PROBE_START channels={len(rows)}", flush=True)
    for idx, row in enumerate(rows, start=1):
        started = time.time()
        item: dict[str, Any] = {
            "id": row.id,
            "guide": row.guide,
            "name": row.name,
            "url": row.url,
        }
        try:
            data = ffprobe_json(row.url, timeout_s=args.timeout)
            summary, reasons, profile = classify_probe(data, args.bitrate_threshold)
            item.update(summary)
            item["reasons"] = reasons
            item["suggest_profile"] = profile
            item["ok"] = True
            if profile:
                profile_overrides[row.id] = profile
                if not args.no_transcode_overrides:
                    transcode_overrides[row.id] = True
            status = "FLAG" if reasons else "OK"
            print(
                f"{status} {idx}/{len(rows)} id={row.id} guide={row.guide} "
                f"v={summary['video_codec']} {summary['width']}x{summary['height']}@{summary['fps']} "
                f"a={summary['audio_codec']} bitrate={summary['bit_rate']} "
                f"profile={profile or '-'} reasons={','.join(reasons) if reasons else '-'} "
                f"dur={time.time()-started:.1f}s",
                flush=True,
            )
        except Exception as e:
            item["ok"] = False
            item["error"] = str(e)
            print(f"ERR {idx}/{len(rows)} id={row.id} guide={row.guide} err={e}", flush=True)
        report.append(item)
        if args.sleep_ms > 0 and idx < len(rows):
            time.sleep(args.sleep_ms / 1000.0)

    if args.emit_profile_overrides:
        with open(args.emit_profile_overrides, "w", encoding="utf-8") as f:
            json.dump(profile_overrides, f, indent=2, sort_keys=True)
            f.write("\n")
        print(f"WROTE profile_overrides={args.emit_profile_overrides} entries={len(profile_overrides)}")
    if args.emit_transcode_overrides:
        with open(args.emit_transcode_overrides, "w", encoding="utf-8") as f:
            json.dump(transcode_overrides, f, indent=2, sort_keys=True)
            f.write("\n")
        print(f"WROTE transcode_overrides={args.emit_transcode_overrides} entries={len(transcode_overrides)}")

    flagged = sum(1 for r in report if r.get("reasons"))
    errs = sum(1 for r in report if not r.get("ok"))
    print(f"PROBE_DONE total={len(report)} flagged={flagged} errors={errs}")
    return 0 if errs == 0 else 2


if __name__ == "__main__":
    raise SystemExit(main())
