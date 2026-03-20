#!/usr/bin/env python3
"""
Analyze a captured manifest body from scripts/stream-compare-harness.sh.
"""
from __future__ import annotations

import argparse
import json
import re
from pathlib import Path
from typing import Any
from urllib.parse import parse_qs, unquote, urljoin, urlparse
from xml.etree import ElementTree as ET


URI_ATTR_RE = re.compile(r"""(?i)\buri\s*=\s*(?:"([^"]*)"|'([^']*)')""")
EXTINF_INLINE_URI_RE = re.compile(r"(?i)^#EXTINF:[^,]*,(.+)$")
URLISH_RE = re.compile(r"^(?:https?://|[^?#]+\.(?:m3u8|m3u|ts|m4s|mp4|aac|vtt|key)(?:[?#].*)?)$")
MAX_BODY_PREVIEW = 65536


def read_json(path: Path) -> dict[str, Any]:
    if not path.is_file():
        return {}
    try:
        return json.loads(path.read_text(encoding="utf-8", errors="replace"))
    except json.JSONDecodeError:
        return {}


def redact_url(raw: str) -> str:
    if not raw:
        return ""
    parsed = urlparse(raw)
    if not parsed.scheme or not parsed.netloc:
        return raw
    host = parsed.hostname or ""
    user = parsed.username
    password = parsed.password
    if user is not None or password is not None:
        auth = "***"
        if password is not None:
            auth = "***:***"
        hostport = host
        if parsed.port is not None:
            hostport = f"{host}:{parsed.port}"
        netloc = f"{auth}@{hostport}"
    else:
        netloc = parsed.netloc
    query = parsed.query
    if query:
        parts = []
        for pair in query.split("&"):
            if "=" not in pair:
                parts.append(pair)
                continue
            key, value = pair.split("=", 1)
            if key.lower() in {"username", "password", "pass", "token", "auth"}:
                parts.append(f"{key}=***")
            else:
                parts.append(f"{key}={value}")
        query = "&".join(parts)
    return parsed._replace(netloc=netloc, query=query).geturl()


def is_http_url(raw: str) -> bool:
    parsed = urlparse(raw)
    return parsed.scheme in {"http", "https"}


def decode_seg_target(raw: str) -> dict[str, Any] | None:
    parsed = urlparse(raw)
    query = parse_qs(parsed.query, keep_blank_values=True)
    seg_values = query.get("seg")
    if not seg_values:
        return None
    decoded = unquote(seg_values[0])
    return {
        "mux": (query.get("mux") or [""])[0],
        "decoded_url": decoded,
        "redacted_url": redact_url(decoded),
        "http_ok": is_http_url(decoded),
    }


def classify_manifest(body_text: str, content_type: str, source_url: str) -> str:
    lower_type = content_type.lower()
    stripped = body_text.lstrip()
    lower_url = source_url.lower()
    if "#EXTM3U" in body_text[:MAX_BODY_PREVIEW] or "mpegurl" in lower_type or lower_url.endswith(".m3u8"):
        return "hls"
    if stripped.startswith("<MPD") or "<MPD" in body_text[:MAX_BODY_PREVIEW] or "dash+xml" in lower_type or lower_url.endswith(".mpd"):
        return "dash"
    return ""


def looks_like_inline_uri(value: str) -> bool:
    candidate = value.strip()
    if not candidate:
        return False
    if "=" in candidate and "://" not in candidate:
        return False
    return bool(URLISH_RE.match(candidate))


def make_ref(raw_ref: str, source_url: str, ref_type: str, **extra: Any) -> dict[str, Any]:
    payload: dict[str, Any] = {
        "type": ref_type,
        "raw_ref": raw_ref,
        "resolved_ref": urljoin(source_url, raw_ref) if source_url else raw_ref,
    }
    payload.update(extra)
    seg = decode_seg_target(payload["resolved_ref"])
    if seg is not None:
        payload["tunerr_mux"] = seg["mux"]
        payload["tunerr_seg"] = {
            "redacted_url": seg["redacted_url"],
            "http_ok": seg["http_ok"],
        }
    return payload


def analyze_hls(body_text: str, source_url: str, ref_limit: int) -> dict[str, Any]:
    refs: list[dict[str, Any]] = []
    issues: list[str] = []
    lines = body_text.splitlines()
    for idx, line in enumerate(lines, start=1):
        if len(refs) >= ref_limit:
            break
        stripped = line.strip()
        if not stripped:
            continue
        if stripped.startswith("#"):
            for match in URI_ATTR_RE.finditer(stripped):
                raw_ref = match.group(1) or match.group(2) or ""
                refs.append(make_ref(raw_ref, source_url, "uri_attr", line=idx, tag=stripped.split(":", 1)[0]))
                if len(refs) >= ref_limit:
                    break
            if len(refs) >= ref_limit:
                break
            inline = EXTINF_INLINE_URI_RE.match(stripped)
            if inline:
                candidate = inline.group(1).strip()
                if looks_like_inline_uri(candidate):
                    refs.append(make_ref(candidate, source_url, "extinf_inline", line=idx, tag="#EXTINF"))
            continue
        refs.append(make_ref(stripped, source_url, "uri_line", line=idx))
    if body_text and not body_text.lstrip().startswith("#EXTM3U"):
        issues.append("body does not begin with #EXTM3U")
    return {"refs": refs, "issues": issues, "line_count": len(lines)}


def local_name(tag: str) -> str:
    return tag.rsplit("}", 1)[-1]


def walk_path(parts: list[str], name: str) -> str:
    return "/".join(parts + [name])


def analyze_dash(body_text: str, source_url: str, ref_limit: int) -> dict[str, Any]:
    refs: list[dict[str, Any]] = []
    issues: list[str] = []
    try:
        root = ET.fromstring(body_text)
    except ET.ParseError as exc:
        return {"refs": refs, "issues": [f"xml parse error: {exc}"], "line_count": body_text.count("\n") + 1}

    attr_names = {"media", "initialization", "sourceURL", "segmentURL", "href"}

    def visit(elem: ET.Element, path_parts: list[str]) -> None:
        if len(refs) >= ref_limit:
            return
        name = local_name(elem.tag)
        current_path = path_parts + [name]
        if name == "BaseURL":
            text = (elem.text or "").strip()
            if text:
                refs.append(make_ref(text, source_url, "baseurl_text", path=walk_path(path_parts, name)))
        for attr_name, value in elem.attrib.items():
            if attr_name in attr_names and value:
                refs.append(
                    make_ref(
                        value,
                        source_url,
                        "xml_attr",
                        path=walk_path(path_parts, name),
                        attr=attr_name,
                    )
                )
                if len(refs) >= ref_limit:
                    return
        for child in list(elem):
            visit(child, current_path)
            if len(refs) >= ref_limit:
                return

    visit(root, [])
    return {"refs": refs, "issues": issues, "line_count": body_text.count("\n") + 1}


def main() -> None:
    ap = argparse.ArgumentParser()
    ap.add_argument("--body", required=True)
    ap.add_argument("--meta", required=True)
    ap.add_argument("--curl-meta", required=True)
    ap.add_argument("--out", required=True)
    ap.add_argument("--ref-limit", type=int, default=40)
    args = ap.parse_args()

    body_path = Path(args.body)
    meta = read_json(Path(args.meta))
    curl_meta = read_json(Path(args.curl_meta))
    body_bytes = body_path.read_bytes() if body_path.is_file() else b""
    body_text = body_bytes.decode("utf-8", errors="replace")
    source_url = str(curl_meta.get("url_effective") or meta.get("url") or "")
    content_type = str(curl_meta.get("content_type") or "")
    manifest_kind = classify_manifest(body_text, content_type, source_url)

    payload: dict[str, Any] = {
        "detected": bool(manifest_kind),
        "kind": manifest_kind,
        "content_type": content_type,
        "source_url": source_url,
        "bytes": len(body_bytes),
        "line_count": body_text.count("\n") + (1 if body_text else 0),
        "issues": [],
        "refs": [],
        "uri_ref_count": 0,
        "tunerr_seg_ref_count": 0,
        "ref_limit": args.ref_limit,
    }

    if manifest_kind == "hls":
        details = analyze_hls(body_text, source_url, args.ref_limit)
        payload["issues"] = details["issues"]
        payload["refs"] = details["refs"]
        payload["line_count"] = details["line_count"]
    elif manifest_kind == "dash":
        details = analyze_dash(body_text, source_url, args.ref_limit)
        payload["issues"] = details["issues"]
        payload["refs"] = details["refs"]
        payload["line_count"] = details["line_count"]
    else:
        snippet = body_text[:120].strip()
        if snippet:
            payload["issues"] = [f"body did not look like HLS or DASH: {snippet!r}"]

    payload["uri_ref_count"] = len(payload["refs"])
    payload["tunerr_seg_ref_count"] = sum(1 for ref in payload["refs"] if "tunerr_seg" in ref)

    Path(args.out).write_text(json.dumps(payload, indent=2, sort_keys=True) + "\n", encoding="utf-8")


if __name__ == "__main__":
    main()
