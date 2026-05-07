#!/usr/bin/env python3
"""
Reverse proxy for Plex `/media/providers` that rewrites Live TV provider labels.

Purpose:
- Plex can emit identical Live TV provider labels (e.g. all `friendlyName="plexKube"`),
  which makes source tabs indistinguishable on some clients.
- This proxy rewrites per-provider attributes using DVR lineup titles from `/livetv/dvrs`.
- Optional `--elevate-live-tv` mode mirrors `iptv-tunerr plex-label-proxy`:
  normal library requests keep the user's token, while allowlisted Live TV read
  requests use the PMS owner token.

Scope:
- Proxies all requests to PMS.
- Rewrites `/media/providers` and provider-scoped Live TV XML labels.
- In elevation mode, rewrites small XML `allowTuners` hints so Plex Web reveals
  Live TV entry points.

Notes:
- This can help clients that use provider metadata labels from `/media/providers`.
- Current Plex Web (4.156.0 in our testing) may still display the server name for
  owned multi-LiveTV sources because its UI code hardcodes `serverFriendlyName`.
- Owner-token elevation is a trust boundary. The raw owner token is not returned
  to clients by this script, but the script can act as a limited owner-token
  deputy for allowlisted Live TV reads.
"""

from __future__ import annotations

import argparse
import gzip
import http.client
import http.server
import io
import logging
import os
import re
import socketserver
import sys
import threading
import time
import urllib.parse
import xml.etree.ElementTree as ET
from dataclasses import dataclass
from typing import Dict, Optional


LIVE_PROVIDER_RE = re.compile(r"^tv\.plex\.providers\.epg\.xmltv:(\d+)$")
LIVE_PROVIDER_PATH_RE = re.compile(r"^/tv\.plex\.providers\.epg\.xmltv:(\d+)(?:/|$)")
HOP_HEADERS = {
    "connection",
    "keep-alive",
    "proxy-authenticate",
    "proxy-authorization",
    "te",
    "trailers",
    "transfer-encoding",
    "upgrade",
}


@dataclass
class ProxyConfig:
    upstream: str
    token: str
    owner_token: str
    elevate_live_tv: bool
    strip_prefix: str
    refresh_seconds: int


class LabelMapCache:
    def __init__(self, cfg: ProxyConfig):
        self.cfg = cfg
        self._lock = threading.Lock()
        self._last_refresh = 0.0
        self._map: Dict[str, str] = {}

    def get(self) -> Dict[str, str]:
        now = time.time()
        with self._lock:
            if now - self._last_refresh < self.cfg.refresh_seconds and self._map:
                return dict(self._map)
        self.refresh()
        with self._lock:
            return dict(self._map)

    def refresh(self) -> None:
        mp = fetch_dvr_label_map(self.cfg)
        with self._lock:
            self._map = mp
            self._last_refresh = time.time()
        logging.info("refreshed DVR label map (%d entries)", len(mp))


def build_http_conn(url: str) -> tuple[http.client.HTTPConnection, urllib.parse.ParseResult]:
    pu = urllib.parse.urlparse(url)
    if pu.scheme not in ("http", "https"):
        raise ValueError(f"unsupported upstream scheme: {pu.scheme}")
    host = pu.hostname or "127.0.0.1"
    port = pu.port or (443 if pu.scheme == "https" else 80)
    if pu.scheme == "https":
        conn = http.client.HTTPSConnection(host, port, timeout=60)
    else:
        conn = http.client.HTTPConnection(host, port, timeout=60)
    return conn, pu


def fetch_dvr_label_map(cfg: ProxyConfig) -> Dict[str, str]:
    conn, pu = build_http_conn(cfg.upstream)
    path = "/livetv/dvrs"
    qs = urllib.parse.urlencode({"X-Plex-Token": cfg.token})
    conn.request("GET", f"{path}?{qs}", headers={"Accept": "application/xml"})
    resp = conn.getresponse()
    body = resp.read()
    if resp.status != 200:
        raise RuntimeError(f"/livetv/dvrs returned {resp.status}: {body[:200]!r}")

    root = ET.fromstring(body)
    out: Dict[str, str] = {}
    for dvr in root.findall(".//Dvr"):
        dvr_id = dvr.attrib.get("key") or dvr.attrib.get("id")
        if not dvr_id:
            continue
        label = dvr.attrib.get("lineupTitle") or dvr.attrib.get("title") or ""
        if not label:
            lineup = dvr.attrib.get("lineup", "")
            if "#" in lineup:
                label = lineup.rsplit("#", 1)[-1]
        if not label:
            continue
        if cfg.strip_prefix and label.startswith(cfg.strip_prefix):
            label = label[len(cfg.strip_prefix) :]
        out[f"tv.plex.providers.epg.xmltv:{dvr_id}"] = label
    return out


def rewrite_media_providers_xml(xml_bytes: bytes, label_map: Dict[str, str]) -> bytes:
    root = ET.fromstring(xml_bytes)
    changed = 0

    for mp in root.findall(".//MediaProvider"):
        ident = mp.attrib.get("identifier", "")
        if not LIVE_PROVIDER_RE.match(ident):
            continue
        label = label_map.get(ident)
        if not label:
            continue

        # Provider-level labels used by some clients.
        mp.attrib["friendlyName"] = label
        mp.attrib["sourceTitle"] = label
        # Keep generic title? We set it for clients that display provider title directly.
        mp.attrib["title"] = label

        # Content root directory title often backs source lists on some clients.
        for directory in mp.findall("./Feature[@type='content']/Directory"):
            d_id = directory.attrib.get("id", "")
            d_key = directory.attrib.get("key", "")
            if d_id == ident:
                directory.attrib["title"] = label
            elif d_key == f"/{ident}/watchnow" and directory.attrib.get("title") == "Guide":
                directory.attrib["title"] = f"{label} Guide"
        changed += 1

    if not changed:
        return xml_bytes

    buf = io.BytesIO()
    ET.ElementTree(root).write(buf, encoding="utf-8", xml_declaration=True)
    return buf.getvalue()


def rewrite_provider_scoped_xml(path: str, xml_bytes: bytes, label_map: Dict[str, str]) -> bytes:
    m = LIVE_PROVIDER_PATH_RE.match(path)
    if not m:
        return xml_bytes
    dvr_id = m.group(1)
    ident = f"tv.plex.providers.epg.xmltv:{dvr_id}"
    label = label_map.get(ident)
    if not label:
        return xml_bytes

    root = ET.fromstring(xml_bytes)
    changed = False

    # Many provider endpoints return a root MediaContainer with generic titles.
    for attr in ("title", "title1"):
        if root.attrib.get(attr) in ("Plex Library", "Live TV & DVR", "Guide", ""):
            root.attrib[attr] = label
            changed = True
    if root.attrib.get("title2") in ("", "Guide", "Live TV & DVR"):
        root.attrib["title2"] = label
        changed = True
    if "friendlyName" in root.attrib:
        root.attrib["friendlyName"] = label
        changed = True

    for d in root.findall(".//Directory"):
        d_title = d.attrib.get("title", "")
        d_key = d.attrib.get("key", "")
        d_id = d.attrib.get("id", "")
        if d_id == ident and d_title in ("Live TV & DVR", "Guide", ""):
            d.attrib["title"] = label
            changed = True
        elif d_key.endswith("/watchnow") and d_title == "Guide":
            d.attrib["title"] = f"{label} Guide"
            changed = True
        elif d_title == "Live TV & DVR":
            d.attrib["title"] = label
            changed = True

    if not changed:
        return xml_bytes

    buf = io.BytesIO()
    ET.ElementTree(root).write(buf, encoding="utf-8", xml_declaration=True)
    return buf.getvalue()


def rewrite_tuner_entitlement_flags(body: bytes) -> bytes:
    if not body or b"allowTuners" not in body:
        return body
    out = body.replace(b'allowTuners="0"', b'allowTuners="1"')
    out = out.replace(
        b'<Setting id="allowTuners" value="0"',
        b'<Setting id="allowTuners" value="1"',
    )
    return out


def live_tv_text(value: str) -> bool:
    s = (value or "").lower()
    return (
        "/livetv/" in s
        or "tv.plex.providers.epg.xmltv:" in s
        or "livetv%2f" in s
        or "tv.plex.providers.epg.xmltv%3a" in s
    )


def live_tv_elevation_method(method: str) -> bool:
    return (method or "").upper() in ("GET", "HEAD", "OPTIONS")


def query_param_is_live_tv_path(query: Dict[str, list[str]], name: str) -> bool:
    return any(live_tv_text(v) for v in query.get(name, []))


def is_live_tv_request(method: str, path: str, headers: Dict[str, str]) -> bool:
    if not live_tv_elevation_method(method):
        return False

    parsed = urllib.parse.urlsplit(path)
    req_path = parsed.path
    query = urllib.parse.parse_qs(parsed.query, keep_blank_values=True)

    if req_path in ("/media/providers", "/media/grabbers/devices"):
        return True
    if req_path.startswith("/livetv/"):
        return True
    if req_path.startswith("/tv.plex.providers.epg.xmltv:"):
        return True
    if req_path.startswith("/video/:/transcode/"):
        return query_param_is_live_tv_path(query, "path")
    if req_path.startswith("/playQueues"):
        return query_param_is_live_tv_path(query, "uri") or query_param_is_live_tv_path(query, "path")
    if req_path in ("/", "/identity"):
        return live_tv_text(headers.get("Referer", "") or headers.get("referer", ""))
    return False


def replace_query_token(path: str, token: str) -> str:
    parsed = urllib.parse.urlsplit(path)
    pairs = urllib.parse.parse_qsl(parsed.query, keep_blank_values=True)
    out: list[tuple[str, str]] = []
    saw = False
    for key, value in pairs:
        if key == "X-Plex-Token":
            out.append((key, token))
            saw = True
        else:
            out.append((key, value))
    if not saw:
        out.append(("X-Plex-Token", token))
    query = urllib.parse.urlencode(out)
    return urllib.parse.urlunsplit((parsed.scheme, parsed.netloc, parsed.path, query, parsed.fragment))


def apply_live_tv_token_elevation(
    method: str,
    path: str,
    headers: Dict[str, str],
    owner_token: str,
) -> tuple[str, Dict[str, str], bool]:
    token = (owner_token or "").strip()
    if not token or not is_live_tv_request(method, path, headers):
        return path, headers, False
    headers = dict(headers)
    headers["X-Plex-Token"] = token
    return replace_query_token(path, token), headers, True


def looks_like_xml(content_type: str, body: bytes) -> bool:
    return "xml" in (content_type or "").lower() or body.lstrip().startswith(b"<?xml")


class ThreadingHTTPServer(socketserver.ThreadingMixIn, http.server.HTTPServer):
    daemon_threads = True


def make_handler(cfg: ProxyConfig, cache: LabelMapCache):
    conn_pu = urllib.parse.urlparse(cfg.upstream)
    upstream_base_path = conn_pu.path.rstrip("/")

    class Handler(http.server.BaseHTTPRequestHandler):
        protocol_version = "HTTP/1.1"

        def do_GET(self):  # noqa: N802
            self._proxy()

        def do_POST(self):  # noqa: N802
            self._proxy()

        def do_PUT(self):  # noqa: N802
            self._proxy()

        def do_DELETE(self):  # noqa: N802
            self._proxy()

        def do_HEAD(self):  # noqa: N802
            self._proxy()

        def do_OPTIONS(self):  # noqa: N802
            self._proxy()

        def do_PATCH(self):  # noqa: N802
            self._proxy()

        def log_message(self, fmt: str, *args):
            logging.info("%s - %s", self.client_address[0], fmt % args)

        def _proxy(self):
            try:
                self._proxy_inner()
            except Exception as exc:  # noqa: BLE001
                logging.exception("proxy error: %s", exc)
                self.send_error(502, f"proxy error: {exc}")

        def _proxy_inner(self):
            content_len = int(self.headers.get("Content-Length", "0") or "0")
            body = self.rfile.read(content_len) if content_len else b""

            upstream_conn, pu = build_http_conn(cfg.upstream)
            path = self.path
            if upstream_base_path:
                if path.startswith("/"):
                    path = upstream_base_path + path
                else:
                    path = upstream_base_path + "/" + path

            headers = {}
            for k, v in self.headers.items():
                if k.lower() in HOP_HEADERS:
                    continue
                if k.lower() == "host":
                    continue
                headers[k] = v

            upstream_host = pu.netloc
            headers["Host"] = upstream_host
            if cfg.elevate_live_tv:
                path, headers, elevated = apply_live_tv_token_elevation(
                    self.command,
                    path,
                    headers,
                    cfg.owner_token,
                )
                if elevated:
                    logging.info("elevated Live TV request %s %s", self.command, urllib.parse.urlsplit(self.path).path)

            upstream_conn.request(self.command, path, body=body, headers=headers)
            resp = upstream_conn.getresponse()
            resp_body = resp.read()

            parsed = urllib.parse.urlparse(self.path)
            is_media_providers = parsed.path == "/media/providers"
            is_provider_scoped = bool(LIVE_PROVIDER_PATH_RE.match(parsed.path))
            ct = resp.getheader("Content-Type", "")
            content_encoding = (resp.getheader("Content-Encoding") or "").lower()
            should_rewrite_xml = (
                resp.status == 200
                and (is_media_providers or is_provider_scoped or cfg.elevate_live_tv)
                and looks_like_xml(ct, resp_body)
            )
            if should_rewrite_xml:
                try:
                    raw_body = resp_body
                    if content_encoding == "gzip":
                        raw_body = gzip.decompress(resp_body)
                    rewritten = raw_body
                    if cfg.elevate_live_tv:
                        rewritten = rewrite_tuner_entitlement_flags(rewritten)
                    labels = cache.get()
                    if is_media_providers:
                        rewritten = rewrite_media_providers_xml(rewritten, labels)
                    if is_provider_scoped:
                        rewritten = rewrite_provider_scoped_xml(parsed.path, rewritten, labels)
                    if content_encoding == "gzip":
                        resp_body = gzip.compress(rewritten)
                    else:
                        resp_body = rewritten
                except Exception as exc:  # noqa: BLE001
                    logging.exception("rewrite failed, passing through: %s", exc)

            self.send_response(resp.status, resp.reason)
            for k, v in resp.getheaders():
                lk = k.lower()
                if lk in HOP_HEADERS:
                    continue
                if lk == "content-length":
                    continue
                self.send_header(k, v)
            self.send_header("Content-Length", str(len(resp_body)))
            self.end_headers()
            if self.command != "HEAD":
                self.wfile.write(resp_body)

    return Handler


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument("--listen", default="127.0.0.1:33240", help="listen host:port")
    ap.add_argument("--upstream", required=True, help="Plex PMS URL, e.g. http://127.0.0.1:32400")
    ap.add_argument("--token", required=True, help="Plex token (used to query /livetv/dvrs for labels)")
    ap.add_argument(
        "--owner-token",
        default=os.environ.get("IPTV_TUNERR_PMS_OWNER_TOKEN")
        or os.environ.get("PLEX_OWNER_TOKEN")
        or "",
        help="PMS owner token for --elevate-live-tv (default: IPTV_TUNERR_PMS_OWNER_TOKEN or PLEX_OWNER_TOKEN)",
    )
    ap.add_argument(
        "--elevate-live-tv",
        action="store_true",
        help="replace X-Plex-Token with owner token only for allowlisted Live TV read/probe requests",
    )
    ap.add_argument("--strip-prefix", default="iptvtunerr-", help="strip this prefix from lineup titles")
    ap.add_argument("--refresh-seconds", type=int, default=30, help="DVR label map refresh interval")
    ap.add_argument("--dump-rewrite-test", metavar="FILE", help="rewrite a saved /media/providers XML file and print to stdout")
    ap.add_argument("--log-level", default="INFO", help="logging level")
    args = ap.parse_args()

    logging.basicConfig(
        level=getattr(logging, args.log_level.upper(), logging.INFO),
        format="%(asctime)s %(levelname)s %(message)s",
    )

    owner_token = (args.owner_token or "").strip()
    if args.elevate_live_tv and not owner_token:
        owner_token = (args.token or "").strip()
    if args.elevate_live_tv and not owner_token:
        ap.error("--elevate-live-tv requires --owner-token or IPTV_TUNERR_PMS_OWNER_TOKEN / PLEX_OWNER_TOKEN")

    cfg = ProxyConfig(
        upstream=args.upstream,
        token=args.token,
        owner_token=owner_token,
        elevate_live_tv=args.elevate_live_tv,
        strip_prefix=args.strip_prefix,
        refresh_seconds=max(1, args.refresh_seconds),
    )
    cache = LabelMapCache(cfg)

    if args.dump_rewrite_test:
        cache.refresh()
        data = open(args.dump_rewrite_test, "rb").read()
        sys.stdout.buffer.write(rewrite_media_providers_xml(data, cache.get()))
        return 0

    cache.refresh()
    host, port_s = args.listen.rsplit(":", 1)
    port = int(port_s)
    server = ThreadingHTTPServer((host, port), make_handler(cfg, cache))
    logging.info("listening on %s -> %s (elevate_live_tv=%s)", args.listen, args.upstream, args.elevate_live_tv)
    try:
        server.serve_forever()
    except KeyboardInterrupt:
        pass
    finally:
        server.server_close()
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
