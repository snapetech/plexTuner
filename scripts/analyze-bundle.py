#!/usr/bin/env python3
"""
analyze-bundle.py — Correlate Plex Media Server log, Tunerr stdout log,
Tunerr JSONL stream attempt log, and pcap captures into a unified diagnostic report.

Usage:
    python3 scripts/analyze-bundle.py ./debug-scratch/
    python3 scripts/analyze-bundle.py ./debug-scratch/ --output report.txt
    python3 scripts/analyze-bundle.py --pms ./PMS.log --tunerr ./tunerr.log --attempts ./attempts.jsonl --pcap ./capture.pcap
    python3 scripts/analyze-bundle.py ./debug-scratch/ --json

Sources auto-detected in directory:
    *.jsonl             → Tunerr stream attempt audit log (IPTV_TUNERR_STREAM_ATTEMPT_LOG)
    Plex*.log / *.pmslog or content-identified  → Plex Media Server log
    tunerr*.log / *.tunerr or content-identified → Tunerr stdout log
    *.pcap / *.pcapng   → network captures (analyzed with tshark if available)
    cf-learned.json     → Tunerr CF learned state (working UA per host)
    cookies.json        → Tunerr cookie jar (metadata only: names, domains, expiry)

Requires: Python 3.9+
Optional: tshark (Wireshark CLI) for pcap analysis and JA3 fingerprint extraction
"""
from __future__ import annotations

import argparse
import json
import os
import re
import subprocess
import sys
from dataclasses import dataclass, field
from datetime import datetime, timezone
from pathlib import Path
from typing import Any

# ---------------------------------------------------------------------------
# Event model
# ---------------------------------------------------------------------------

@dataclass
class Event:
    ts: datetime                   # UTC
    source: str                    # "tunerr_log" | "tunerr_attempts" | "pms" | "pcap"
    kind: str                      # "stream_start" | "stream_end" | "cf_block" | etc.
    channel: str = ""
    req_id: str = ""
    ua: str = ""
    status: int = 0
    url: str = ""
    detail: str = ""
    raw: str = ""

    def ts_str(self) -> str:
        return self.ts.strftime("%H:%M:%S.%f")[:-3]


@dataclass
class Finding:
    severity: str                  # "error" | "warn" | "info"
    code: str                      # machine-readable tag
    title: str
    detail: str
    confidence: int = 0            # 0–100
    evidence: list[str] = field(default_factory=list)


# ---------------------------------------------------------------------------
# Timestamp parsing helpers
# ---------------------------------------------------------------------------

_TS_FORMATS = [
    # Tunerr go stdlib: "2006/01/02 15:04:05"
    ("%Y/%m/%d %H:%M:%S", r"\d{4}/\d{2}/\d{2} \d{2}:\d{2}:\d{2}"),
    # ISO8601
    ("%Y-%m-%dT%H:%M:%S", r"\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}"),
    ("%Y-%m-%d %H:%M:%S.%f", r"\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}\.\d+"),
    ("%Y-%m-%d %H:%M:%S", r"\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}"),
    # Plex: "Jan 06, 2025 20:31:15.234"
    ("%b %d, %Y %H:%M:%S.%f", r"[A-Z][a-z]{2} \d{2}, \d{4} \d{2}:\d{2}:\d{2}\.\d+"),
    ("%b %d, %Y %H:%M:%S", r"[A-Z][a-z]{2} \d{2}, \d{4} \d{2}:\d{2}:\d{2}"),
]
_TS_RES = [(re.compile(r"(" + pat + r")"), fmt) for fmt, pat in _TS_FORMATS]


def parse_ts(s: str) -> datetime | None:
    s = s.strip()
    for rx, fmt in _TS_RES:
        m = rx.search(s)
        if m:
            try:
                dt = datetime.strptime(m.group(1), fmt)
                if dt.tzinfo is None:
                    dt = dt.replace(tzinfo=timezone.utc)
                return dt
            except ValueError:
                continue
    return None


def parse_iso(s: str) -> datetime | None:
    s = s.strip().rstrip("Z")
    for fmt in ("%Y-%m-%dT%H:%M:%S.%f", "%Y-%m-%dT%H:%M:%S", "%Y-%m-%d %H:%M:%S.%f", "%Y-%m-%d %H:%M:%S"):
        try:
            dt = datetime.strptime(s, fmt).replace(tzinfo=timezone.utc)
            return dt
        except ValueError:
            continue
    return None


# ---------------------------------------------------------------------------
# ANSI / escape stripping
# ---------------------------------------------------------------------------

_ANSI_RE = re.compile(r"\x1b\[[0-9;]*m")

def strip_ansi(s: str) -> str:
    return _ANSI_RE.sub("", s)


# ---------------------------------------------------------------------------
# Tunerr JSONL stream attempt log parser
# ---------------------------------------------------------------------------

def parse_tunerr_jsonl(path: Path) -> list[Event]:
    events: list[Event] = []
    for line in path.read_text(encoding="utf-8", errors="replace").splitlines():
        line = line.strip()
        if not line:
            continue
        try:
            obj = json.loads(line)
        except json.JSONDecodeError:
            continue
        ts = parse_iso(obj.get("started_at", ""))
        if ts is None:
            continue
        channel = obj.get("channel_name") or obj.get("channel_id") or ""
        req_id = obj.get("req_id", "")
        status_str = obj.get("final_status", "")
        err = obj.get("final_error", "")
        ua = obj.get("user_agent", "")  # client UA (from Plex)
        upstreams = obj.get("upstreams") or []

        # Determine upstream UA used
        upstream_ua = ""
        for up in upstreams:
            for h in (up.get("request_headers") or []):
                if h.lower().startswith("user-agent:"):
                    upstream_ua = h.split(":", 1)[1].strip()
                    break

        events.append(Event(
            ts=ts,
            source="tunerr_attempts",
            kind="stream_" + (status_str if status_str else "unknown"),
            channel=channel,
            req_id=req_id,
            ua=upstream_ua or ua,
            status=0,
            url=obj.get("effective_url", ""),
            detail=f"dur={obj.get('duration_ms',0)}ms mode={obj.get('final_mode','')} err={err}",
            raw=line,
        ))

        # Emit per-upstream events for CF detection
        for up in upstreams:
            code = up.get("status_code", 0)
            outcome = up.get("outcome", "")
            up_url = up.get("url", "")
            if code in (403, 503, 520, 521, 524) or "cf" in outcome.lower():
                events.append(Event(
                    ts=ts,
                    source="tunerr_attempts",
                    kind="cf_block",
                    channel=channel,
                    req_id=req_id,
                    status=code,
                    url=up_url,
                    detail=f"upstream[{up.get('index','')}] status={code} outcome={outcome}",
                ))

    return events


# ---------------------------------------------------------------------------
# Tunerr stdout log parser
# ---------------------------------------------------------------------------

_TUNERR_LOG_RE = re.compile(
    r"(\d{4}/\d{2}/\d{2} \d{2}:\d{2}:\d{2})"
    r"(?:\s+\[iptv-tunerr\])?"
    r"\s+(.*)"
)

_GW_RECV_RE = re.compile(r'req=(\S+).*path="(/stream/\S*)".*ua="([^"]*)"')
_GW_UPSTREAM_RE = re.compile(r'req=(\S+).*start upstream\[(\d+)/(\d+)\].*url=(\S+)')
_GW_STATUS_RE = re.compile(r'req=(\S+).*upstream\[(\d+)/(\d+)\] status=(\d+)')
_GW_PROXIED_RE = re.compile(r'(?:channel="([^"]+)".*)?proxied bytes=(\d+)')
_GW_CF_RE = re.compile(r'cf[-_](?:block|bootstrap|boot)')
_CF_HOST_RE = re.compile(r"(?:access check|cf_clearance|UA cycling|EnsureAccess)\s+(?:for\s+)?(\S+)")


def parse_tunerr_log(path: Path) -> list[Event]:
    events: list[Event] = []
    for line in path.read_text(encoding="utf-8", errors="replace").splitlines():
        line = strip_ansi(line).strip()
        m = _TUNERR_LOG_RE.match(line)
        if not m:
            continue
        ts = parse_ts(m.group(1))
        if ts is None:
            continue
        body = m.group(2)

        if "recv path=" in body:
            rm = _GW_RECV_RE.search(body)
            if rm:
                events.append(Event(
                    ts=ts, source="tunerr_log", kind="stream_recv",
                    req_id=rm.group(1), url=rm.group(2), ua=rm.group(3),
                    detail=body,
                ))
        elif "start upstream" in body:
            rm = _GW_UPSTREAM_RE.search(body)
            if rm:
                events.append(Event(
                    ts=ts, source="tunerr_log", kind="stream_upstream",
                    req_id=rm.group(1), url=rm.group(4),
                    detail=f"upstream [{rm.group(2)}/{rm.group(3)}] url={rm.group(4)}",
                ))
        elif "status=" in body and "upstream[" in body:
            rm = _GW_STATUS_RE.search(body)
            if rm:
                code = int(rm.group(4))
                kind = "cf_block" if code in (403, 503, 520, 521, 524) else "upstream_fail"
                events.append(Event(
                    ts=ts, source="tunerr_log", kind=kind,
                    req_id=rm.group(1), status=code, detail=body,
                ))
        elif "proxied bytes" in body:
            rm = _GW_PROXIED_RE.search(body)
            if rm:
                events.append(Event(
                    ts=ts, source="tunerr_log", kind="stream_ok",
                    channel=rm.group(1) or "", detail=f"bytes={rm.group(2)}",
                ))
        elif "cf-bootstrap" in body or "cf_boot" in body:
            detail = body
            host = ""
            hm = _CF_HOST_RE.search(body)
            if hm:
                host = hm.group(1)
            kind = "cf_bootstrap"
            if "resolved" in body or "UA cycling" in body or "clearance" in body:
                kind = "cf_resolved"
            events.append(Event(
                ts=ts, source="tunerr_log", kind=kind,
                url=host, detail=detail,
            ))
        elif "all" in body and "upstream(s) failed" in body:
            events.append(Event(
                ts=ts, source="tunerr_log", kind="stream_fail",
                detail=body,
            ))

    return events


# ---------------------------------------------------------------------------
# Plex Media Server log parser
# ---------------------------------------------------------------------------

# Plex uses two main log formats:
#   Jan 06, 2025 20:31:15.234 [0;32m[INFO][0m - [DVR] - message
#   2025-01-06 20:31:15.234 [INFO]  org.plex... - message
# We strip ANSI and extract: timestamp, level, component, message.

_PMS_LINE_RE = re.compile(
    r"^(?:"
    r"([A-Z][a-z]{2} \d{1,2}, \d{4} \d{2}:\d{2}:\d{2}(?:\.\d+)?)"  # group 1: Plex classic
    r"|(\d{4}-\d{2}-\d{2}[T ]\d{2}:\d{2}:\d{2}(?:\.\d+)?)"          # group 2: ISO
    r")"
    r"[\s\[]*(?:INFO|DEBUG|WARN|ERROR|TRACE)[\]:\s]*"
    r"(?:-\s+)?"
    r"(?:\[([A-Z][^\]]{0,30})\]\s+-)?\s*"                             # group 3: component like [DVR]
    r"(.*)"                                                            # group 4: message
)

_PMS_DVR_TUNE_RE = re.compile(r"(?:start|starting|begin|play|tune|live|stream)", re.I)
_PMS_DVR_STOP_RE = re.compile(r"(?:stop|stopping|end|kill|finish|remov|terminat)", re.I)
_PMS_SESSION_RE = re.compile(r"session[_\s-]?(?:key)?[=:\s]+(\S+)", re.I)
_PMS_CHANNEL_RE = re.compile(r"(?:channel|ch)[_\s-]?(?:id)?[=:\s]+(\S+)", re.I)
_PMS_HTTP_RE = re.compile(r"(GET|POST)\s+(https?://[^\s]+|/stream/\S*)", re.I)


def is_pms_log(path: Path) -> bool:
    sample = path.read_text(encoding="utf-8", errors="replace")[:4096]
    return ("Plex Media Server" in sample or
            "[DVR]" in sample or
            "Transcoder" in sample or
            "PlexMediaServer" in path.name)


def parse_pms_log(path: Path) -> list[Event]:
    events: list[Event] = []
    for line in path.read_text(encoding="utf-8", errors="replace").splitlines():
        line = strip_ansi(line).strip()
        m = _PMS_LINE_RE.match(line)
        if not m:
            # Try bare timestamp + message
            ts = parse_ts(line[:30])
            if ts is None:
                continue
            body = line[30:].strip(" -")
            component = ""
        else:
            raw_ts = m.group(1) or m.group(2) or ""
            ts = parse_ts(raw_ts)
            if ts is None:
                continue
            component = (m.group(3) or "").strip()
            body = (m.group(4) or "").strip()

        body_lower = body.lower()
        kind = ""
        channel = ""
        req_id = ""
        url = ""

        sm = _PMS_SESSION_RE.search(body)
        if sm:
            req_id = sm.group(1)
        cm = _PMS_CHANNEL_RE.search(body)
        if cm:
            channel = cm.group(1)
        hm = _PMS_HTTP_RE.search(body)
        if hm:
            url = hm.group(2)

        if "/stream/" in url or "/stream/" in body:
            if _PMS_DVR_TUNE_RE.search(body_lower):
                kind = "plex_tune_request"
            elif _PMS_DVR_STOP_RE.search(body_lower):
                kind = "plex_stop"
        elif ("dvr" in component.lower() or "dvr" in body_lower):
            if _PMS_DVR_TUNE_RE.search(body_lower):
                kind = "plex_dvr_tune"
            elif _PMS_DVR_STOP_RE.search(body_lower):
                kind = "plex_dvr_stop"
            else:
                kind = "plex_dvr"
        elif "transcode" in body_lower:
            if _PMS_DVR_TUNE_RE.search(body_lower):
                kind = "plex_transcode_start"
            elif _PMS_DVR_STOP_RE.search(body_lower):
                kind = "plex_transcode_stop"
        elif "error" in body_lower or "fail" in body_lower:
            kind = "plex_error"
        elif url:
            kind = "plex_http"

        if not kind:
            continue

        events.append(Event(
            ts=ts,
            source="pms",
            kind=kind,
            channel=channel,
            req_id=req_id,
            url=url,
            detail=f"[{component}] {body}" if component else body,
            raw=line,
        ))

    return events


# ---------------------------------------------------------------------------
# pcap / tshark parser
# ---------------------------------------------------------------------------

def tshark_available() -> bool:
    try:
        r = subprocess.run(["tshark", "--version"], capture_output=True, timeout=5)
        return r.returncode == 0
    except (FileNotFoundError, subprocess.TimeoutExpired):
        return False


def run_tshark(pcap: Path, fields: list[str], display_filter: str = "") -> list[list[str]]:
    cmd = ["tshark", "-r", str(pcap), "-T", "fields", "-E", "separator=\t"]
    if display_filter:
        cmd += ["-Y", display_filter]
    for f in fields:
        cmd += ["-e", f]
    try:
        r = subprocess.run(cmd, capture_output=True, text=True, timeout=60)
        rows = []
        for line in r.stdout.splitlines():
            parts = line.split("\t")
            rows.append(parts)
        return rows
    except (subprocess.TimeoutExpired, FileNotFoundError):
        return []


def parse_pcap(path: Path) -> tuple[list[Event], list[str]]:
    """Returns (events, notes). notes contains findings like JA3 fingerprints."""
    if not tshark_available():
        return [], ["tshark not found — install Wireshark/tshark for pcap analysis"]

    notes: list[str] = []
    events: list[Event] = []

    # HTTP requests: timestamp, dst IP, URI, User-Agent
    rows = run_tshark(path,
        ["frame.time_epoch", "ip.dst", "http.request.uri", "http.user_agent"],
        "http.request")
    for row in rows:
        if len(row) < 4:
            continue
        try:
            ts_epoch = float(row[0])
            ts = datetime.fromtimestamp(ts_epoch, tz=timezone.utc)
        except (ValueError, IndexError):
            continue
        dst_ip = row[1].strip()
        uri = row[2].strip()
        ua = row[3].strip()
        if not uri:
            continue
        events.append(Event(
            ts=ts, source="pcap", kind="http_request",
            url=f"{dst_ip}{uri}", ua=ua,
            detail=f"GET {uri} dst={dst_ip} ua={ua[:80]}",
        ))

    # HTTP responses: timestamp, src IP, status, Server header
    resp_rows = run_tshark(path,
        ["frame.time_epoch", "ip.src", "http.response.code", "http.server"],
        "http.response")
    for row in resp_rows:
        if len(row) < 3:
            continue
        try:
            ts_epoch = float(row[0])
            ts = datetime.fromtimestamp(ts_epoch, tz=timezone.utc)
            code = int(row[2].strip()) if row[2].strip() else 0
        except (ValueError, IndexError):
            continue
        server = row[3].strip() if len(row) > 3 else ""
        if code in (403, 503, 520, 521, 524) or server.lower() == "cloudflare":
            events.append(Event(
                ts=ts, source="pcap", kind="cf_block",
                status=code, url=row[1].strip(),
                detail=f"HTTP {code} from {row[1].strip()} server={server}",
            ))
        elif code > 0:
            events.append(Event(
                ts=ts, source="pcap", kind="http_response",
                status=code, url=row[1].strip(),
                detail=f"HTTP {code} from {row[1].strip()} server={server}",
            ))

    # JA3 TLS fingerprints
    ja3_rows = run_tshark(path,
        ["frame.time_epoch", "ip.dst", "tls.handshake.ja3"],
        "tls.handshake.type == 1")
    if not ja3_rows:
        # Try alternate field name
        ja3_rows = run_tshark(path,
            ["frame.time_epoch", "ip.dst", "ja3.hash"],
            "ssl.handshake.type == 1")

    known_go_ja3 = {
        "772,4865-4866-4867-49195-49199-49196-49200-52393-52392-49171-49172-156-157-47-53,",
        "b32309a26951912be7dba376398abc3b",  # common Go stdlib JA3
    }
    seen_ja3: dict[str, list[str]] = {}
    for row in ja3_rows:
        if len(row) < 3:
            continue
        try:
            ts_epoch = float(row[0])
            ts = datetime.fromtimestamp(ts_epoch, tz=timezone.utc)
        except ValueError:
            continue
        dst = row[1].strip()
        ja3 = row[2].strip()
        if not ja3:
            continue
        if dst not in seen_ja3:
            seen_ja3[dst] = []
        if ja3 not in seen_ja3[dst]:
            seen_ja3[dst].append(ja3)
        is_go_like = any(known in ja3 for known in known_go_ja3)
        events.append(Event(
            ts=ts, source="pcap", kind="tls_clienthello",
            url=dst, ua=ja3,
            detail=f"JA3={ja3[:32]} dst={dst}" + (" [Go stdlib — CF may fingerprint]" if is_go_like else ""),
        ))

    for dst, fingerprints in seen_ja3.items():
        for ja3 in fingerprints:
            is_go_like = any(known in ja3 for known in known_go_ja3)
            note = f"TLS fingerprint to {dst}: JA3={ja3[:32]}"
            if is_go_like:
                note += " ← matches Go stdlib (CF TLS fingerprinting risk; consider IPTV_TUNERR_TLS_IMPERSONATE)"
            notes.append(note)

    return events, notes


# ---------------------------------------------------------------------------
# CF learned state / cookie jar reader
# ---------------------------------------------------------------------------

def read_cf_learned(path: Path) -> dict[str, Any]:
    if not path.is_file():
        return {}
    try:
        return json.loads(path.read_text(encoding="utf-8", errors="replace"))
    except json.JSONDecodeError:
        return {}


def read_cookie_jar_meta(path: Path) -> dict[str, list[dict]]:
    """Returns host → list of {name, expires_in} (no values)."""
    if not path.is_file():
        return {}
    try:
        raw = json.loads(path.read_text(encoding="utf-8", errors="replace"))
    except json.JSONDecodeError:
        return {}
    now = datetime.now(tz=timezone.utc).timestamp()
    out: dict[str, list[dict]] = {}
    for host, cookies in raw.items():
        if not isinstance(cookies, dict):
            continue
        ck_list = []
        for _, ck in cookies.items():
            if not isinstance(ck, dict):
                continue
            exp = ck.get("expires", 0) or 0
            remaining = None
            expired = False
            if exp > 0:
                remaining_sec = exp - now
                expired = remaining_sec < 0
                remaining = f"{int(remaining_sec/3600)}h{int((remaining_sec%3600)/60)}m" if not expired else "EXPIRED"
            ck_list.append({
                "name": ck.get("name", ""),
                "expires": remaining,
                "expired": expired,
            })
        out[host] = ck_list
    return out


# ---------------------------------------------------------------------------
# Auto file detection
# ---------------------------------------------------------------------------

def is_tunerr_log(path: Path) -> bool:
    sample = path.read_text(encoding="utf-8", errors="replace")[:4096]
    return ("gateway:" in sample or
            "[iptv-tunerr]" in sample or
            "cf-bootstrap:" in sample or
            "iptv-tunerr" in path.name.lower())


def detect_files(directory: Path) -> dict[str, list[Path]]:
    result: dict[str, list[Path]] = {
        "attempts": [],
        "tunerr_log": [],
        "pms_log": [],
        "pcap": [],
        "cf_learned": [],
        "cookie_jar": [],
    }
    for f in sorted(directory.rglob("*")):
        if not f.is_file():
            continue
        name_lower = f.name.lower()
        if name_lower.endswith(".jsonl"):
            result["attempts"].append(f)
        elif name_lower in ("cf-learned.json",):
            result["cf_learned"].append(f)
        elif name_lower in ("cookies.json", "cf-cookies.json"):
            result["cookie_jar"].append(f)
        elif name_lower.endswith((".pcap", ".pcapng")):
            result["pcap"].append(f)
        elif name_lower.endswith(".json"):
            pass  # skip other JSON
        elif name_lower.endswith((".log", ".txt", ".pmslog", ".tunerrlog")):
            try:
                if is_pms_log(f):
                    result["pms_log"].append(f)
                elif is_tunerr_log(f):
                    result["tunerr_log"].append(f)
            except Exception:
                pass
        else:
            # Content-sniff unknown extension files
            try:
                sample = f.read_text(encoding="utf-8", errors="replace")[:512]
                if "[DVR]" in sample or "Plex Media Server" in sample:
                    result["pms_log"].append(f)
                elif "gateway:" in sample or "[iptv-tunerr]" in sample:
                    result["tunerr_log"].append(f)
                elif sample.strip().startswith("{") and "started_at" in sample:
                    result["attempts"].append(f)
            except Exception:
                pass
    return result


# ---------------------------------------------------------------------------
# Finding detection
# ---------------------------------------------------------------------------

def detect_findings(events: list[Event], cf_learned: dict, cookie_meta: dict) -> list[Finding]:
    findings: list[Finding] = []

    # --- CF block hits ---
    cf_events = [e for e in events if e.kind == "cf_block"]
    if cf_events:
        # Check if any resolved after
        cf_resolved = [e for e in events if e.kind == "cf_resolved"]
        if cf_resolved:
            findings.append(Finding(
                severity="warn", code="CF_BLOCK_RESOLVED", confidence=80,
                title="Cloudflare blocks detected — resolved by UA cycling or bootstrap",
                detail="CF blocks were seen but resolution was also logged. Streams may have delayed start.",
                evidence=[f"  {e.ts_str()} {e.kind}: {e.detail[:80]}" for e in cf_events[:3]],
            ))
        else:
            findings.append(Finding(
                severity="error", code="CF_BLOCK_UNRESOLVED", confidence=90,
                title=f"Cloudflare blocking detected ({len(cf_events)} events) — no resolution seen",
                detail=(
                    "CF blocks were seen with no successful UA cycling or bootstrap logged.\n"
                    "Try: IPTV_TUNERR_CF_AUTO_BOOT=true with IPTV_TUNERR_COOKIE_JAR_FILE set,\n"
                    "or: iptv-tunerr import-cookies -har /path/to/session.har"
                ),
                evidence=[f"  {e.ts_str()} CF {e.status} {e.url[:60]}" for e in cf_events[:5]],
            ))

    # --- UA mismatch: pcap UA vs attempt log UA ---
    pcap_req_events = [e for e in events if e.source == "pcap" and e.kind == "http_request" and e.ua]
    attempt_events = [e for e in events if e.source == "tunerr_attempts"]
    if pcap_req_events and attempt_events:
        pcap_uas = {e.ua for e in pcap_req_events if e.ua}
        attempt_uas = {e.ua for e in attempt_events if e.ua}
        # Find UAs in attempts that don't appear in pcap (would be suspicious)
        missing_in_pcap = attempt_uas - pcap_uas
        if missing_in_pcap:
            findings.append(Finding(
                severity="warn", code="UA_MISMATCH_PCAP", confidence=60,
                title="UA in attempt log not seen on wire (pcap)",
                detail=(
                    "Tunerr's stream attempt log shows a User-Agent that does not appear in the pcap capture. "
                    "This could indicate a proxy, load balancer, or MITM between Tunerr and the provider "
                    "that is rewriting the User-Agent header."
                ),
                evidence=[f"  in log (not in pcap): {ua[:80]}" for ua in list(missing_in_pcap)[:3]],
            ))

    # --- TLS fingerprinting risk (Go stdlib JA3) ---
    tls_events = [e for e in events if e.kind == "tls_clienthello"]
    go_tls = [e for e in tls_events if "Go stdlib" in e.detail]
    if go_tls:
        findings.append(Finding(
            severity="warn", code="TLS_FINGERPRINT_GO_STDLIB", confidence=70,
            title="Go stdlib TLS fingerprint detected in pcap — CF may block by JA3",
            detail=(
                "Even with the correct User-Agent, Cloudflare Bot Management can identify Go's\n"
                "TLS ClientHello (JA3 hash) and block it. This is the remaining gap after UA cycling.\n"
                "Workaround: route provider traffic through a browser or add utls transport support."
            ),
            evidence=[f"  {e.ts_str()} JA3={e.ua[:32]} → {e.url}" for e in go_tls[:3]],
        ))

    # --- Stale sessions: Plex stop event without corresponding Tunerr stream end ---
    plex_stops = [e for e in events if e.kind in ("plex_dvr_stop", "plex_transcode_stop")]
    tunerr_oks = [e for e in events if e.kind == "stream_ok" and e.source == "tunerr_attempts"]
    if plex_stops and not tunerr_oks:
        findings.append(Finding(
            severity="warn", code="PLEX_STOP_NO_TUNERR_STREAM",confidence=55,
            title="Plex session stop(s) seen with no corresponding Tunerr stream completion",
            detail=(
                "Plex reported stopping sessions but no successful Tunerr stream records are present.\n"
                "Possible causes: hidden session grab, CF block on stream start, or timing gap in logs."
            ),
            evidence=[f"  {e.ts_str()} PMS: {e.detail[:80]}" for e in plex_stops[:3]],
        ))

    # --- Plex tune without Tunerr receipt ---
    plex_tunes = [e for e in events if e.kind in ("plex_tune_request", "plex_dvr_tune", "plex_http") and "/stream/" in e.url]
    tunerr_recvs = [e for e in events if e.kind == "stream_recv"]
    if plex_tunes and not tunerr_recvs:
        findings.append(Finding(
            severity="error", code="PLEX_TUNE_NO_TUNERR_RECV", confidence=75,
            title="Plex sent stream request but Tunerr shows no recv — possible network/routing issue",
            detail=(
                "PMS.log shows Plex hitting /stream/ but no corresponding 'recv' in Tunerr log.\n"
                "Check: is Tunerr running? Is the DVR URL in Plex pointing to the right host:port?"
            ),
            evidence=[f"  {e.ts_str()} PMS → {e.url[:80]}" for e in plex_tunes[:3]],
        ))

    # --- CF clearance expiry ---
    for host, cks in cookie_meta.items():
        for ck in cks:
            if ck["name"] == "cf_clearance" and ck.get("expired"):
                findings.append(Finding(
                    severity="error", code="CF_CLEARANCE_EXPIRED", confidence=95,
                    title=f"cf_clearance cookie for {host} is EXPIRED",
                    detail=(
                        f"The Cloudflare clearance cookie for {host} has expired. "
                        "Run `iptv-tunerr import-cookies -har /path/to/session.har` or "
                        "enable CF_AUTO_BOOT to refresh automatically."
                    ),
                ))

    # --- Working UA from learned state ---
    if cf_learned:
        for host, entry in cf_learned.items():
            ua = entry.get("working_ua", "") if isinstance(entry, dict) else ""
            tagged = entry.get("cf_tagged", False) if isinstance(entry, dict) else False
            if tagged and not ua:
                findings.append(Finding(
                    severity="warn", code="CF_HOST_NO_WORKING_UA", confidence=65,
                    title=f"Host {host} is CF-tagged but no working UA recorded",
                    detail=(
                        f"{host} has triggered CF blocks but no UA has been found to bypass it. "
                        "CF may require a valid cf_clearance cookie (JS challenge, not just Bot Management). "
                        "Try: IPTV_TUNERR_CF_AUTO_BOOT=true"
                    ),
                ))

    # --- Stream failures without retry ---
    fail_events = [e for e in events if e.kind == "stream_fail"]
    if fail_events:
        findings.append(Finding(
            severity="error", code="STREAM_FAILURES", confidence=85,
            title=f"{len(fail_events)} stream failure(s) — all upstreams exhausted",
            detail="All upstream URL candidates failed for these requests.",
            evidence=[f"  {e.ts_str()} {e.detail[:80]}" for e in fail_events[:3]],
        ))

    # --- No events at all ---
    if not events:
        findings.append(Finding(
            severity="info", code="NO_EVENTS",
            title="No parseable events found in any source",
            detail="Check that the correct files are in the bundle directory.",
            confidence=100,
        ))

    findings.sort(key=lambda f: (-{"error": 3, "warn": 2, "info": 1}.get(f.severity, 0), -f.confidence))
    return findings


# ---------------------------------------------------------------------------
# Report renderer
# ---------------------------------------------------------------------------

_SEV_LABEL = {"error": "✗ ERROR", "warn": "⚠ WARN ", "info": "· INFO "}


def render_text(
    events: list[Event],
    findings: list[Finding],
    pcap_notes: list[str],
    sources_used: dict[str, list[str]],
    cf_learned: dict,
    cookie_meta: dict,
    json_out: bool = False,
) -> str:
    if json_out:
        return json.dumps({
            "sources": sources_used,
            "findings": [
                {
                    "severity": f.severity,
                    "code": f.code,
                    "title": f.title,
                    "detail": f.detail,
                    "confidence": f.confidence,
                    "evidence": f.evidence,
                }
                for f in findings
            ],
            "pcap_notes": pcap_notes,
            "event_count": len(events),
            "timeline": [
                {
                    "ts": e.ts.isoformat(),
                    "source": e.source,
                    "kind": e.kind,
                    "channel": e.channel,
                    "ua": e.ua,
                    "url": e.url,
                    "detail": e.detail[:120],
                }
                for e in events[-200:]
            ],
        }, indent=2)

    lines = []
    lines.append("=" * 72)
    lines.append("  TUNERR DIAGNOSTIC BUNDLE ANALYSIS")
    lines.append("=" * 72)
    lines.append("")

    lines.append("SOURCES")
    lines.append("-" * 40)
    for src, paths in sources_used.items():
        if paths:
            lines.append(f"  {src}: {', '.join(paths)}")
    if not any(sources_used.values()):
        lines.append("  (none found)")
    lines.append(f"\n  Total events parsed: {len(events)}")
    lines.append("")

    if findings:
        lines.append("FINDINGS  (ranked by severity + confidence)")
        lines.append("-" * 40)
        for i, f in enumerate(findings, 1):
            label = _SEV_LABEL.get(f.severity, f.severity)
            lines.append(f"  [{i}] {label}  {f.title}  [{f.confidence}% confidence]")
            for dline in f.detail.splitlines():
                lines.append(f"        {dline}")
            if f.evidence:
                lines.append("      Evidence:")
                for ev in f.evidence:
                    lines.append(f"        {ev}")
            lines.append("")

    if pcap_notes:
        lines.append("PCAP NOTES")
        lines.append("-" * 40)
        for note in pcap_notes:
            lines.append(f"  {note}")
        lines.append("")

    if cf_learned:
        lines.append("CF LEARNED STATE")
        lines.append("-" * 40)
        for host, entry in sorted(cf_learned.items()):
            if not isinstance(entry, dict):
                continue
            ua = entry.get("working_ua", "-") or "-"
            tagged = entry.get("cf_tagged", False)
            lines.append(f"  {host}  cf_tagged={tagged}  working_ua={ua[:50]}")
        lines.append("")

    if cookie_meta:
        lines.append("COOKIE JAR  (names + expiry only, no values)")
        lines.append("-" * 40)
        for host, cks in sorted(cookie_meta.items()):
            for ck in cks:
                exp_str = ck.get("expires") or "session"
                expired_mark = " ← EXPIRED" if ck.get("expired") else ""
                lines.append(f"  {host}  {ck['name']}  expires: {exp_str}{expired_mark}")
        lines.append("")

    # Timeline: show last 60 events (or all if fewer)
    if events:
        timeline_events = events[-60:]
        lines.append(f"TIMELINE  (last {len(timeline_events)} of {len(events)} events, oldest first)")
        lines.append("-" * 40)
        src_abbrev = {
            "tunerr_log": "TUN",
            "tunerr_attempts": "ATT",
            "pms": "PMS",
            "pcap": "CAP",
        }
        for e in timeline_events:
            src = src_abbrev.get(e.source, e.source[:3].upper())
            channel_str = f" ch={e.channel[:12]}" if e.channel else ""
            ua_str = f" ua={e.ua[:30]}" if e.ua and e.kind not in ("tls_clienthello",) else ""
            detail_str = e.detail[:60] if e.detail else ""
            lines.append(f"  {e.ts_str()} [{src}] {e.kind:<22} {channel_str}{ua_str}  {detail_str}")
        lines.append("")

    lines.append("=" * 72)
    return "\n".join(lines)


# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------

def main() -> int:
    ap = argparse.ArgumentParser(
        description="Analyze Tunerr/Plex/pcap debug bundle and produce a correlated diagnostic report.",
        epilog="Sources are auto-detected when a directory is given.",
    )
    ap.add_argument("directory", nargs="?", help="Directory to scan for debug files")
    ap.add_argument("--pms", metavar="FILE", help="Plex Media Server log")
    ap.add_argument("--tunerr", metavar="FILE", help="Tunerr stdout log")
    ap.add_argument("--attempts", metavar="FILE", help="Tunerr JSONL stream attempt log")
    ap.add_argument("--pcap", metavar="FILE", help="pcap/pcapng capture file")
    ap.add_argument("--cf-learned", metavar="FILE", help="cf-learned.json state file")
    ap.add_argument("--cookie-jar", metavar="FILE", help="Tunerr cookie jar JSON")
    ap.add_argument("--output", "-o", metavar="FILE", help="Write report to file (default: stdout)")
    ap.add_argument("--json", action="store_true", help="Output as JSON")
    args = ap.parse_args()

    explicit_files: dict[str, Path | None] = {
        "pms_log": Path(args.pms) if args.pms else None,
        "tunerr_log": Path(args.tunerr) if args.tunerr else None,
        "attempts": Path(args.attempts) if args.attempts else None,
        "pcap": Path(args.pcap) if args.pcap else None,
        "cf_learned": Path(args.cf_learned) if args.cf_learned else None,
        "cookie_jar": Path(args.cookie_jar) if args.cookie_jar else None,
    }

    file_groups: dict[str, list[Path]] = {k: [] for k in explicit_files}
    if args.directory:
        d = Path(args.directory)
        if not d.is_dir():
            print(f"error: not a directory: {d}", file=sys.stderr)
            return 1
        detected = detect_files(d)
        for k, paths in detected.items():
            file_groups[k].extend(paths)

    for k, p in explicit_files.items():
        if p is not None:
            if not p.is_file():
                print(f"warning: file not found: {p}", file=sys.stderr)
            else:
                if p not in file_groups.get(k, []):
                    file_groups.setdefault(k, []).append(p)

    events: list[Event] = []
    pcap_notes: list[str] = []
    sources_used: dict[str, list[str]] = {}

    for p in file_groups.get("attempts", []):
        evs = parse_tunerr_jsonl(p)
        events.extend(evs)
        sources_used.setdefault("tunerr_attempts", []).append(p.name)
        print(f"  parsed {len(evs)} events from {p.name}", file=sys.stderr)

    for p in file_groups.get("tunerr_log", []):
        evs = parse_tunerr_log(p)
        events.extend(evs)
        sources_used.setdefault("tunerr_log", []).append(p.name)
        print(f"  parsed {len(evs)} events from {p.name}", file=sys.stderr)

    for p in file_groups.get("pms_log", []):
        evs = parse_pms_log(p)
        events.extend(evs)
        sources_used.setdefault("pms", []).append(p.name)
        print(f"  parsed {len(evs)} events from {p.name}", file=sys.stderr)

    for p in file_groups.get("pcap", []):
        evs, notes = parse_pcap(p)
        events.extend(evs)
        pcap_notes.extend(notes)
        sources_used.setdefault("pcap", []).append(p.name)
        print(f"  parsed {len(evs)} events from {p.name}", file=sys.stderr)

    cf_learned: dict = {}
    for p in file_groups.get("cf_learned", []):
        cf_learned.update(read_cf_learned(p))

    cookie_meta: dict = {}
    for p in file_groups.get("cookie_jar", []):
        cookie_meta.update(read_cookie_jar_meta(p))

    # Sort all events by timestamp
    events.sort(key=lambda e: e.ts)

    findings = detect_findings(events, cf_learned, cookie_meta)

    report = render_text(events, findings, pcap_notes, sources_used, cf_learned, cookie_meta, json_out=args.json)

    if args.output:
        Path(args.output).write_text(report, encoding="utf-8")
        print(f"Report written to {args.output}", file=sys.stderr)
    else:
        print(report)

    return 1 if any(f.severity == "error" for f in findings) else 0


if __name__ == "__main__":
    sys.exit(main())
