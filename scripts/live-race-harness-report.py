#!/usr/bin/env python3
"""
Parse artifacts from scripts/live-race-harness.sh and produce a compact report.
"""
from __future__ import annotations

import argparse
import json
import os
import re
import statistics
import sys
from collections import Counter, defaultdict
from dataclasses import dataclass, field
from pathlib import Path
from typing import Any


PLEX_PREFIX_RE = re.compile(r"^\[plex-tuner\]\s+\d{4}/\d{2}/\d{2}\s+\d{2}:\d{2}:\d{2}\s+(.*)$")
REQ_RE = re.compile(r"\breq=(r\d+)\b")
BOOL_RE = {"true": True, "false": False}


def parse_duration_ms(s: str) -> float | None:
    s = s.strip()
    try:
        if s.endswith("ms"):
            return float(s[:-2])
        if s.endswith("s"):
            return float(s[:-1]) * 1000.0
        if s.endswith("us"):
            return float(s[:-2]) / 1000.0
        if s.endswith("µs"):
            return float(s[:-2]) / 1000.0
    except ValueError:
        return None
    return None


def stats(nums: list[float]) -> dict[str, float] | None:
    if not nums:
        return None
    out = {
        "count": float(len(nums)),
        "min": min(nums),
        "max": max(nums),
        "avg": statistics.fmean(nums),
    }
    if len(nums) >= 2:
        out["p50"] = statistics.median(nums)
    return out


@dataclass
class ReqTrace:
    req: str
    channel_id: str | None = None
    channel_name: str | None = None
    path: str | None = None
    recv: int = 0
    acquires: int = 0
    releases: int = 0
    tuner_reject: int = 0
    startup_gate_timeout: int = 0
    startup_gate_buffered: list[dict[str, Any]] = field(default_factory=list)
    null_keepalive_starts: int = 0
    null_keepalive_stops: Counter = field(default_factory=Counter)
    bootstrap_ts_bytes: list[int] = field(default_factory=list)
    first_bytes_startup_ms: list[float] = field(default_factory=list)
    first_bytes_sizes: list[int] = field(default_factory=list)
    ffmpeg_modes: Counter = field(default_factory=Counter)
    lines: int = 0


class Parser:
    recv_re = re.compile(r'req=(r\d+)\s+recv path="([^"]+)" channel="([^"]+)"')
    acquire_re = re.compile(r'req=(r\d+).*?\bacquire inuse=(\d+)/(\d+)')
    release_re = re.compile(r'req=(r\d+).*?\brelease inuse=(\d+)/(\d+) dur=([^\s]+)')
    reject_re = re.compile(r'req=(r\d+).*?reject all-tuners-in-use')
    ffmpeg_mode_re = re.compile(r'(ffmpeg-(?:transcode|remux))')
    first_bytes_re = re.compile(r'req=(r\d+).*?\bfirst-bytes=(\d+)\s+startup=([^\s]+)')
    startup_gate_re = re.compile(
        r'req=(r\d+).*?startup-gate buffered=(\d+).*?ts_pkts=(\d+)\s+idr=(true|false)\s+aac=(true|false)\s+align=(-?\d+)'
    )
    startup_gate_timeout_re = re.compile(r'req=(r\d+).*?startup-gate timeout')
    null_keepalive_start_re = re.compile(r'req=(r\d+).*?null-ts-keepalive start')
    null_keepalive_stop_re = re.compile(r'req=(r\d+).*?null-ts-keepalive stop=([a-z-]+)')
    bootstrap_ts_re = re.compile(r'req=(r\d+).*?bootstrap-ts bytes=(\d+)')

    curl_start_re = re.compile(r"^===\s+(\S+)\s+(\S+)\s+([0-9T:+-]+)\s+===$")
    wc_re = re.compile(r"^\s*(\d+)\s+(.+)$")

    pms_patterns = {
        "failed_consumer": re.compile(r"Failed to find consumer", re.IGNORECASE),
        "dash_init_404": re.compile(r"dash_init_404", re.IGNORECASE),
        "livetv_session_404": re.compile(r"/livetv/sessions/.+index\.m3u8", re.IGNORECASE),
    }
    pms_session_id_re = re.compile(r"/livetv/sessions/([^/\s]+)/")

    def __init__(self) -> None:
        self.reqs: dict[str, ReqTrace] = {}
        self.counters: Counter = Counter()
        self.inuse_samples: list[int] = []
        self.limit_samples: list[int] = []
        self.release_durations_ms: list[float] = []
        self.first_bytes_ms: list[float] = []
        self.curl_sections: list[dict[str, Any]] = []
        self.pms_counts: Counter = Counter()
        self.pms_session_ids: Counter = Counter()
        self.pms_samples: dict[str, list[str]] = defaultdict(list)

    def req(self, req_id: str) -> ReqTrace:
        if req_id not in self.reqs:
            self.reqs[req_id] = ReqTrace(req=req_id)
        return self.reqs[req_id]

    def parse_plex_log(self, path: Path) -> None:
        if not path.is_file():
            return
        with path.open("r", encoding="utf-8", errors="replace") as fh:
            for raw in fh:
                line = raw.rstrip("\n")
                m = PLEX_PREFIX_RE.match(line)
                msg = m.group(1) if m else line
                req_m = REQ_RE.search(msg)
                req_id = req_m.group(1) if req_m else None
                if req_id:
                    self.req(req_id).lines += 1

                if m := self.recv_re.search(msg):
                    req = self.req(m.group(1))
                    req.recv += 1
                    req.path = m.group(2)
                    req.channel_id = m.group(3)
                    self.counters["recv"] += 1
                    continue

                if m := self.acquire_re.search(msg):
                    req = self.req(m.group(1))
                    req.acquires += 1
                    inuse = int(m.group(2))
                    limit = int(m.group(3))
                    self.inuse_samples.append(inuse)
                    self.limit_samples.append(limit)
                    self.counters["acquire"] += 1
                    continue

                if m := self.release_re.search(msg):
                    req = self.req(m.group(1))
                    req.releases += 1
                    inuse = int(m.group(2))
                    limit = int(m.group(3))
                    self.inuse_samples.append(inuse)
                    self.limit_samples.append(limit)
                    dur_ms = parse_duration_ms(m.group(4))
                    if dur_ms is not None:
                        self.release_durations_ms.append(dur_ms)
                    self.counters["release"] += 1
                    continue

                if m := self.reject_re.search(msg):
                    req = self.req(m.group(1))
                    req.tuner_reject += 1
                    self.counters["all_tuners_in_use"] += 1
                    continue

                if m := self.ffmpeg_mode_re.search(msg):
                    if req_id:
                        self.req(req_id).ffmpeg_modes[m.group(1)] += 1

                if m := self.first_bytes_re.search(msg):
                    req = self.req(m.group(1))
                    req.first_bytes_sizes.append(int(m.group(2)))
                    d = parse_duration_ms(m.group(3))
                    if d is not None:
                        req.first_bytes_startup_ms.append(d)
                        self.first_bytes_ms.append(d)
                    self.counters["first_bytes"] += 1
                    continue

                if m := self.startup_gate_re.search(msg):
                    req = self.req(m.group(1))
                    req.startup_gate_buffered.append(
                        {
                            "buffered": int(m.group(2)),
                            "ts_pkts": int(m.group(3)),
                            "idr": BOOL_RE.get(m.group(4).lower(), False),
                            "aac": BOOL_RE.get(m.group(5).lower(), False),
                            "align": int(m.group(6)),
                        }
                    )
                    self.counters["startup_gate_buffered"] += 1
                    continue

                if m := self.startup_gate_timeout_re.search(msg):
                    req = self.req(m.group(1))
                    req.startup_gate_timeout += 1
                    self.counters["startup_gate_timeout"] += 1
                    continue

                if m := self.null_keepalive_start_re.search(msg):
                    req = self.req(m.group(1))
                    req.null_keepalive_starts += 1
                    self.counters["null_keepalive_start"] += 1
                    continue

                if m := self.null_keepalive_stop_re.search(msg):
                    req = self.req(m.group(1))
                    reason = m.group(2)
                    req.null_keepalive_stops[reason] += 1
                    self.counters[f"null_keepalive_stop_{reason}"] += 1
                    self.counters["null_keepalive_stop"] += 1
                    continue

                if m := self.bootstrap_ts_re.search(msg):
                    req = self.req(m.group(1))
                    req.bootstrap_ts_bytes.append(int(m.group(2)))
                    self.counters["bootstrap_ts"] += 1
                    continue

    def parse_curl_log(self, path: Path) -> None:
        if not path.is_file():
            return
        by_label: dict[str, dict[str, Any]] = {}
        cur: dict[str, Any] | None = None
        with path.open("r", encoding="utf-8", errors="replace") as fh:
            for raw in fh:
                line = raw.rstrip("\n")
                if m := self.curl_start_re.match(line):
                    cur = {"label": m.group(1), "url": m.group(2), "started": m.group(3), "bytes": None}
                    self.curl_sections.append(cur)
                    by_label[cur["label"]] = cur
                    continue
                if m := self.wc_re.match(line):
                    try:
                        n = int(m.group(1))
                    except ValueError:
                        n = None
                    if n is None:
                        continue
                    wc_path = m.group(2).strip()
                    label = Path(wc_path).name.removesuffix(".ts")
                    if label in by_label:
                        by_label[label]["bytes"] = n
                        continue
                    if cur is not None and cur.get("bytes") is None:
                        cur["bytes"] = n

        # Fallback: use artifact file sizes when curl.log writes interleave.
        for s in self.curl_sections:
            if s.get("bytes") is not None:
                continue
            label = s.get("label")
            if not label:
                continue
            ts_file = path.parent / f"{label}.ts"
            if ts_file.is_file():
                try:
                    s["bytes"] = ts_file.stat().st_size
                except OSError:
                    pass

    def parse_pms_logs(self, root: Path) -> None:
        if not root.is_dir():
            return
        for p in root.rglob("*"):
            if not p.is_file():
                continue
            # PMS logs are plain text; skip large binaries by extension heuristics.
            if p.suffix.lower() in {".zip", ".db", ".sqlite", ".pcap"}:
                continue
            try:
                with p.open("r", encoding="utf-8", errors="replace") as fh:
                    for raw in fh:
                        line = raw.rstrip("\n")
                        for key, rx in self.pms_patterns.items():
                            if rx.search(line):
                                self.pms_counts[key] += 1
                                if len(self.pms_samples[key]) < 8:
                                    self.pms_samples[key].append(f"{p.name}: {line}")
                        if m := self.pms_session_id_re.search(line):
                            self.pms_session_ids[m.group(1)] += 1
            except OSError:
                continue

    def build_report(self, out_dir: Path) -> dict[str, Any]:
        reqs = []
        for req_id in sorted(self.reqs):
            r = self.reqs[req_id]
            reqs.append(
                {
                    "req": r.req,
                    "path": r.path,
                    "channel_id": r.channel_id,
                    "channel_name": r.channel_name,
                    "recv": r.recv,
                    "acquires": r.acquires,
                    "releases": r.releases,
                    "tuner_reject": r.tuner_reject,
                    "startup_gate_timeout": r.startup_gate_timeout,
                    "startup_gate_buffered_count": len(r.startup_gate_buffered),
                    "startup_gate_good_like_count": sum(
                        1
                        for e in r.startup_gate_buffered
                        if e.get("idr") and e.get("aac") and int(e.get("ts_pkts", 0)) >= 8
                    ),
                    "null_keepalive_starts": r.null_keepalive_starts,
                    "null_keepalive_stops": dict(r.null_keepalive_stops),
                    "bootstrap_ts_count": len(r.bootstrap_ts_bytes),
                    "first_bytes_sizes": r.first_bytes_sizes,
                    "first_bytes_startup_ms": r.first_bytes_startup_ms,
                    "ffmpeg_modes": dict(r.ffmpeg_modes),
                    "lines": r.lines,
                }
            )

        curl_by_channel = defaultdict(list)
        for s in self.curl_sections:
            label = s.get("label", "")
            bytes_ = s.get("bytes")
            if label.startswith("synth-"):
                curl_by_channel["synth"].append(bytes_ or 0)
            elif label.startswith("replay-"):
                curl_by_channel["replay"].append(bytes_ or 0)

        synopsis = {
            "synthetic_probe_bytes": curl_by_channel.get("synth", []),
            "replay_probe_bytes": curl_by_channel.get("replay", []),
            "synthetic_probe_nonzero": sum(1 for n in curl_by_channel.get("synth", []) if n and n > 0),
            "replay_probe_nonzero": sum(1 for n in curl_by_channel.get("replay", []) if n and n > 0),
            "all_tuners_in_use": int(self.counters.get("all_tuners_in_use", 0)),
            "startup_gate_timeouts": int(self.counters.get("startup_gate_timeout", 0)),
            "null_keepalive_stops": {
                k.removeprefix("null_keepalive_stop_"): int(v)
                for k, v in self.counters.items()
                if k.startswith("null_keepalive_stop_")
            },
            "first_bytes_ms_stats": stats(self.first_bytes_ms),
            "release_duration_ms_stats": stats(self.release_durations_ms),
            "max_inuse_seen": max(self.inuse_samples) if self.inuse_samples else None,
            "limit_seen": max(self.limit_samples) if self.limit_samples else None,
            "pms_counts": {k: int(v) for k, v in self.pms_counts.items()},
            "pms_session_ids_top": self.pms_session_ids.most_common(10),
        }

        hypotheses = []
        if synopsis["synthetic_probe_nonzero"] and not synopsis["replay_probe_nonzero"]:
            hypotheses.append("Synthetic source succeeds while replay fails: replay/source semantics issue (container/timing) over provider jitter.")
        if synopsis["replay_probe_nonzero"] and synopsis["synthetic_probe_nonzero"]:
            hypotheses.append("Both synthetic and replay probes return bytes locally: base tuner pipeline is likely capable; remaining failures point to Plex session behavior or real-client path differences.")
        if synopsis["startup_gate_timeouts"] > 0:
            hypotheses.append("Startup gate timeouts observed: upstream/ffmpeg readiness latency remains a primary suspect.")
        if synopsis["all_tuners_in_use"] > 0:
            hypotheses.append("Tuner contention/rejects observed: parallel reads may be part of the Plex consumer startup failure.")
        if self.pms_counts.get("failed_consumer", 0):
            hypotheses.append("PMS logs include 'Failed to find consumer'; correlate those timestamps with req IDs showing slow/timeout startup.")
        if not hypotheses:
            hypotheses.append("No decisive pattern yet; review per-request traces and PMS samples for correlation by timestamp.")

        report = {
            "out_dir": str(out_dir),
            "synopsis": synopsis,
            "counters": {k: int(v) for k, v in self.counters.items()},
            "requests": reqs,
            "curl_sections": self.curl_sections,
            "pms_samples": {k: v for k, v in self.pms_samples.items()},
            "hypotheses": hypotheses,
        }
        return report


def write_text_report(report: dict[str, Any], out_path: Path) -> None:
    syn = report["synopsis"]
    lines: list[str] = []
    lines.append("Live Race Harness Report")
    lines.append("=" * 24)
    lines.append(f"Out Dir: {report['out_dir']}")
    lines.append("")
    lines.append("Topline")
    lines.append(f"- Synthetic probe nonzero: {syn.get('synthetic_probe_nonzero', 0)} / {len(syn.get('synthetic_probe_bytes', []))}")
    lines.append(f"- Replay probe nonzero: {syn.get('replay_probe_nonzero', 0)} / {len(syn.get('replay_probe_bytes', []))}")
    lines.append(f"- Startup gate timeouts: {syn.get('startup_gate_timeouts', 0)}")
    lines.append(f"- All tuners in use rejects: {syn.get('all_tuners_in_use', 0)}")
    lines.append(f"- Max inuse seen: {syn.get('max_inuse_seen')} (limit={syn.get('limit_seen')})")

    fb = syn.get("first_bytes_ms_stats")
    if fb:
        lines.append(
            f"- First ffmpeg bytes startup (ms): count={int(fb['count'])} min={fb['min']:.1f} avg={fb['avg']:.1f} max={fb['max']:.1f}"
        )
    rd = syn.get("release_duration_ms_stats")
    if rd:
        lines.append(
            f"- Request duration (ms): count={int(rd['count'])} min={rd['min']:.1f} avg={rd['avg']:.1f} max={rd['max']:.1f}"
        )
    lines.append("")

    pms_counts = syn.get("pms_counts", {}) or {}
    lines.append("PMS Signals")
    lines.append(f"- failed_consumer: {pms_counts.get('failed_consumer', 0)}")
    lines.append(f"- dash_init_404: {pms_counts.get('dash_init_404', 0)}")
    lines.append(f"- livetv_session_404: {pms_counts.get('livetv_session_404', 0)}")
    lines.append("")

    lines.append("Null Keepalive Stop Reasons")
    stops = syn.get("null_keepalive_stops", {}) or {}
    if stops:
        for k in sorted(stops):
            lines.append(f"- {k}: {stops[k]}")
    else:
        lines.append("- none")
    lines.append("")

    lines.append("Hypotheses")
    for h in report.get("hypotheses", []):
        lines.append(f"- {h}")
    lines.append("")

    lines.append("Request Summary (first 20)")
    for r in report.get("requests", [])[:20]:
        lines.append(
            "- {req} path={path} acq={acq} rel={rel} reject={rej} gate_buf={gb} gate_to={gt} keepalive={ks}/{kstop}".format(
                req=r["req"],
                path=r.get("path"),
                acq=r.get("acquires", 0),
                rel=r.get("releases", 0),
                rej=r.get("tuner_reject", 0),
                gb=r.get("startup_gate_buffered_count", 0),
                gt=r.get("startup_gate_timeout", 0),
                ks=r.get("null_keepalive_starts", 0),
                kstop=sum((r.get("null_keepalive_stops") or {}).values()),
            )
        )

    out_path.write_text("\n".join(lines) + "\n", encoding="utf-8")


def main() -> int:
    ap = argparse.ArgumentParser(description="Summarize live-race harness artifacts")
    ap.add_argument("--dir", required=True, help="Harness output directory (.diag/live-race/<runid>)")
    ap.add_argument("--print", action="store_true", dest="print_report", help="Print text report to stdout")
    args = ap.parse_args()

    out_dir = Path(args.dir).resolve()
    if not out_dir.is_dir():
        print(f"ERROR: not a directory: {out_dir}", file=sys.stderr)
        return 2

    parser = Parser()
    parser.parse_plex_log(out_dir / "plex-tuner.log")
    parser.parse_curl_log(out_dir / "curl.log")
    pms_dir = out_dir / "pms-logs"
    if pms_dir.is_dir():
        parser.parse_pms_logs(pms_dir)

    report = parser.build_report(out_dir)
    json_path = out_dir / "report.json"
    txt_path = out_dir / "report.txt"
    json_path.write_text(json.dumps(report, indent=2, sort_keys=True), encoding="utf-8")
    write_text_report(report, txt_path)

    if args.print_report:
        sys.stdout.write(txt_path.read_text(encoding="utf-8"))
    else:
        print(f"Wrote {txt_path}")
        print(f"Wrote {json_path}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
