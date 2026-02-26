#!/usr/bin/env python3
"""Drain active Plex Live TV sessions via Plex API.

Manual cleanup helper for cases where a client (for example an LG/webOS TV) keeps
the Plex Live TV session alive after the user switches inputs without stopping.
This is intentionally a manual tool (no TTL/auto-stop behavior).
"""

from __future__ import annotations

import argparse
import re
import subprocess
import sys
import threading
import time
import urllib.error
import urllib.parse
import urllib.request
import xml.etree.ElementTree as ET


def plex_get(base: str, token: str, path: str) -> ET.Element:
    qs = urllib.parse.urlencode({"X-Plex-Token": token})
    url = f"{base.rstrip('/')}{path}{'&' if '?' in path else '?'}{qs}"
    req = urllib.request.Request(url, headers={"Accept": "application/xml"})
    with urllib.request.urlopen(req, timeout=10) as resp:
        return ET.fromstring(resp.read())


def plex_put(base: str, token: str, path: str) -> int:
    qs = urllib.parse.urlencode({"X-Plex-Token": token})
    url = f"{base.rstrip('/')}{path}{'&' if '?' in path else '?'}{qs}"
    req = urllib.request.Request(url, data=b"", method="PUT")
    try:
        with urllib.request.urlopen(req, timeout=10) as resp:
            return getattr(resp, "status", 200)
    except urllib.error.HTTPError as e:
        return e.code


def parse_rows(root: ET.Element):
    rows = []
    for video in root.findall("Video"):
        key = (video.attrib.get("key") or "").strip()
        if not key.startswith("/livetv/sessions/"):
            continue
        player = video.find("Player")
        trans = video.find("TranscodeSession")
        session = video.find("Session")
        trans_key = (trans.attrib.get("key") if trans is not None else "") or ""
        trans_id = ""
        if "/transcode/sessions/" in trans_key:
            trans_id = trans_key.rsplit("/", 1)[-1]
        rows.append(
            {
                "title": video.attrib.get("title", ""),
                "live_key": key,
                "session_key": video.attrib.get("sessionKey", ""),
                "player_addr": (player.attrib.get("address") if player is not None else "") or "",
                "player_product": (player.attrib.get("product") if player is not None else "") or "",
                "player_platform": (player.attrib.get("platform") if player is not None else "") or "",
                "player_device": (player.attrib.get("device") if player is not None else "") or "",
                "machine_id": (player.attrib.get("machineIdentifier") if player is not None else "") or "",
                "state": (player.attrib.get("state") if player is not None else "") or "",
                "transcode_id": trans_id,
                "session_id": (session.attrib.get("id") if session is not None else "") or "",
            }
        )
    return rows


def stop_transcode(base: str, token: str, transcode_id: str) -> int:
    path = "/video/:/transcode/universal/stop?" + urllib.parse.urlencode({"session": transcode_id})
    return plex_put(base, token, path)


_REQ_RE = re.compile(r"\[(?P<ip>\d+\.\d+\.\d+\.\d+):\d+[^\]]*\]\s+\S+\s+(?P<path>/\S+)")


def fetch_pms_logs(cmd_template: str, since_seconds: int) -> str:
    cmd = cmd_template.format(since=max(1, int(since_seconds)))
    cp = subprocess.run(
        cmd,
        shell=True,
        text=True,
        capture_output=True,
        timeout=max(5, since_seconds + 5),
        errors="replace",
    )
    if cp.returncode != 0 and cp.stderr:
        # kubectl logs sometimes emits benign warnings on stderr; include for visibility.
        sys.stderr.write(cp.stderr)
    return (cp.stdout or "") + ("\n" + cp.stderr if cp.stderr else "")


def row_activity_hit(row: dict[str, str], logs_text: str) -> bool:
    live_uuid = row.get("live_key", "").rsplit("/", 1)[-1]
    live_path_fragment = f"/livetv/sessions/{live_uuid}/" if live_uuid else ""
    transcode_id = row.get("transcode_id", "")
    player_ip = row.get("player_addr", "")

    for line in logs_text.splitlines():
        m = _REQ_RE.search(line)
        if not m:
            continue
        ip = m.group("ip")
        path = m.group("path")
        # Client-facing requests that indicate the player is still consuming data.
        if live_path_fragment and live_path_fragment in path:
            return True
        if transcode_id and f"/transcode/universal/session/{transcode_id}/" in path:
            return True
        # Generic endpoints need the originating player IP to avoid cross-session false positives.
        if player_ip and ip == player_ip and path.startswith("/:/timeline"):
            return True
        if player_ip and ip == player_ip and path.endswith("/start.mpd"):
            return True
    return False


def sse_notifications(
    base: str,
    token: str,
    stop_evt: threading.Event,
    kick_evt: threading.Event,
    last_event_ts: list[float],
) -> None:
    url = f"{base.rstrip('/')}/:/eventsource/notifications?" + urllib.parse.urlencode({"X-Plex-Token": token})
    while not stop_evt.is_set():
        try:
            req = urllib.request.Request(url, headers={"Accept": "text/event-stream"})
            with urllib.request.urlopen(req, timeout=70) as resp:
                event_name = ""
                while not stop_evt.is_set():
                    raw = resp.readline()
                    if not raw:
                        break
                    line = raw.decode("utf-8", "replace").rstrip("\r\n")
                    if not line:
                        if event_name and event_name != "ping":
                            print(f"SSE event={event_name}")
                            # Use only player-facing events to renew activity. Transcode-only
                            # events can continue after a real client disappears.
                            if event_name in ("activity", "playing", "timeline"):
                                last_event_ts[0] = time.time()
                            kick_evt.set()
                        event_name = ""
                        continue
                    if line.startswith("event:"):
                        event_name = line.split(":", 1)[1].strip()
        except (urllib.error.URLError, TimeoutError, OSError) as e:
            if not stop_evt.is_set():
                print(f"SSE reconnect reason={type(e).__name__}")
                time.sleep(1.0)
        except Exception as e:  # keep watcher resilient
            if not stop_evt.is_set():
                print(f"SSE error={type(e).__name__}")
                time.sleep(1.0)


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("--plex-url", default="http://127.0.0.1:32400", help="Plex base URL")
    ap.add_argument("--token", required=True, help="Plex token")
    ap.add_argument("--machine-id", help="Only drain sessions for this player machineIdentifier")
    ap.add_argument("--player-ip", help="Only drain sessions for this player IP")
    ap.add_argument("--all-live", action="store_true", help="Drain all live sessions (default if no filter)")
    ap.add_argument(
        "--idle-seconds",
        type=float,
        default=0.0,
        help="Only stop sessions with no client-facing PMS request activity for this many seconds (requires --pms-log-cmd)",
    )
    ap.add_argument(
        "--pms-log-cmd",
        help=(
            "Shell command to fetch recent Plex logs for activity detection. "
            "Use {since} as a placeholder for seconds (example: "
            "'sudo kubectl -n plex logs deploy/plex --since={since}s')"
        ),
    )
    ap.add_argument(
        "--log-lookback",
        type=int,
        default=10,
        help="Seconds of Plex logs to scan per poll when --idle-seconds is used",
    )
    ap.add_argument("--wait", type=int, default=15)
    ap.add_argument("--poll", type=float, default=1.0)
    ap.add_argument("--watch", action="store_true", help="Continuously reap stale live sessions instead of one-shot drain")
    ap.add_argument("--watch-runtime", type=float, default=0.0, help="Exit watch mode after N seconds (0=run forever)")
    ap.add_argument(
        "--lease-seconds",
        type=float,
        default=0.0,
        help="Hard backstop: stop a live session after this age, even if activity was seen (0=disabled)",
    )
    ap.add_argument(
        "--renew-lease-seconds",
        type=float,
        default=0.0,
        help=(
            "Renewable heartbeat lease: stop a live session if no playback activity is seen for this many "
            "seconds (0=disabled). Uses the same activity signals as idle detection."
        ),
    )
    ap.add_argument(
        "--sse",
        action="store_true",
        help="In watch mode, subscribe to Plex SSE notifications to trigger faster rescans (polling remains authoritative)",
    )
    ap.add_argument("--dry-run", action="store_true")
    args = ap.parse_args()

    if not args.machine_id and not args.player_ip and not args.all_live:
        args.all_live = True
    if (args.idle_seconds > 0 or args.renew_lease_seconds > 0) and not args.pms_log_cmd:
        ap.error("--idle-seconds/--renew-lease-seconds require --pms-log-cmd")
    if args.sse and not args.watch:
        ap.error("--sse requires --watch")

    def matches():
        rows = parse_rows(plex_get(args.plex_url, args.token, "/status/sessions"))
        out = []
        for row in rows:
            if args.machine_id and row["machine_id"] != args.machine_id:
                continue
            if args.player_ip and row["player_addr"] != args.player_ip:
                continue
            out.append(row)
        return out

    idle_last_activity: dict[str, float] = {}
    first_seen: dict[str, float] = {}
    renew_lease_last_seen: dict[str, float] = {}
    sse_last_event_ts = [0.0]
    if args.idle_seconds > 0 or args.renew_lease_seconds > 0:
        now = time.time()
        for r in matches():
            key = r["transcode_id"] or r["live_key"]
            if key:
                idle_last_activity[key] = now
                first_seen.setdefault(key, now)
                renew_lease_last_seen[key] = now

    def idle_annotate(rows):
        track_activity = args.idle_seconds > 0 or args.renew_lease_seconds > 0
        if not track_activity:
            return rows
        now = time.time()
        logs_text = fetch_pms_logs(args.pms_log_cmd, max(args.log_lookback, int(args.poll) + 1))
        for r in rows:
            key = r["transcode_id"] or r["live_key"]
            if not key:
                continue
            idle_last_activity.setdefault(key, now)
            first_seen.setdefault(key, now)
            renew_lease_last_seen.setdefault(key, now)
            saw_activity = False
            if row_activity_hit(r, logs_text):
                saw_activity = True
            elif args.sse and (now - sse_last_event_ts[0]) <= max(args.poll * 2.0, 3.0):
                # SSE non-ping playback activity is a valid "still alive" signal even when
                # kubectl log windows miss a particular request line.
                saw_activity = True
            if saw_activity:
                idle_last_activity[key] = now
                renew_lease_last_seen[key] = now
            r["_idle_age"] = max(0.0, now - idle_last_activity[key])
            r["_idle_ready"] = r["_idle_age"] >= args.idle_seconds
            r["_session_age"] = max(0.0, now - first_seen[key])
            r["_renew_lease_age"] = max(0.0, now - renew_lease_last_seen[key])
        return rows

    rows = matches()
    rows = idle_annotate(rows)
    print(f"LIVE_MATCHED {len(rows)}")
    for r in rows:
        extra = ""
        if args.idle_seconds > 0:
            idle_age = r.get("_idle_age", 0.0)
            extra = f" idle_age={idle_age:.1f}s idle_ready={'yes' if r.get('_idle_ready') else 'no'}"
        print(
            f"LIVE machine={r['machine_id']} ip={r['player_addr']} "
            f"client={r['player_product']}/{r['player_platform']}/{r['player_device']} "
            f"state={r['state']} transcode={r['transcode_id']} title={r['title']}{extra}"
        )
    if args.dry_run and not args.watch:
        return 0

    if args.watch:
        stop_evt = threading.Event()
        kick_evt = threading.Event()
        sse_thread = None
        if args.sse:
            sse_thread = threading.Thread(
                target=sse_notifications,
                args=(args.plex_url, args.token, stop_evt, kick_evt, sse_last_event_ts),
                daemon=True,
            )
            sse_thread.start()
            print("WATCH sse=on")
        else:
            print("WATCH sse=off")
        started = time.time()
        try:
            while True:
                now = time.time()
                rows = idle_annotate(matches())
                print(f"WATCH_MATCHED {len(rows)}")
                active_keys = set()
                for r in rows:
                    key = r["transcode_id"] or r["live_key"]
                    if key:
                        active_keys.add(key)
                    idle_age = r.get("_idle_age", 0.0)
                    sess_age = r.get("_session_age", 0.0)
                    renew_lease_age = r.get("_renew_lease_age", 0.0)
                    lease_ready = bool(args.lease_seconds > 0 and sess_age >= args.lease_seconds)
                    renew_lease_ready = bool(
                        args.renew_lease_seconds > 0 and renew_lease_age >= args.renew_lease_seconds
                    )
                    print(
                        f"WATCH machine={r['machine_id']} ip={r['player_addr']} transcode={r['transcode_id']} "
                        f"state={r['state']} idle_age={idle_age:.1f}s session_age={sess_age:.1f}s "
                        f"renew_lease_age={renew_lease_age:.1f}s "
                        f"idle_ready={'yes' if r.get('_idle_ready') else 'no'} "
                        f"renew_lease_ready={'yes' if renew_lease_ready else 'no'} "
                        f"lease_ready={'yes' if lease_ready else 'no'} title={r['title']}"
                    )
                # Cleanup state for sessions no longer visible.
                for d in (idle_last_activity, first_seen, renew_lease_last_seen):
                    for key in list(d):
                        if key not in active_keys:
                            d.pop(key, None)

                kill_rows = []
                for r in rows:
                    sess_age = float(r.get("_session_age", 0.0))
                    renew_lease_age = float(r.get("_renew_lease_age", 0.0))
                    lease_ready = bool(args.lease_seconds > 0 and sess_age >= args.lease_seconds)
                    renew_lease_ready = bool(
                        args.renew_lease_seconds > 0 and renew_lease_age >= args.renew_lease_seconds
                    )
                    idle_ready = bool(r.get("_idle_ready")) if args.idle_seconds > 0 else False
                    if idle_ready or renew_lease_ready or lease_ready:
                        kill_rows.append(r)

                if kill_rows:
                    seen = set()
                    for r in kill_rows:
                        tid = r["transcode_id"]
                        if not tid or tid in seen:
                            continue
                        seen.add(tid)
                        why = []
                        if args.idle_seconds > 0 and r.get("_idle_ready"):
                            why.append(f"idle>={args.idle_seconds}s")
                        if (
                            args.renew_lease_seconds > 0
                            and float(r.get("_renew_lease_age", 0.0)) >= args.renew_lease_seconds
                        ):
                            why.append(f"renew_lease>={args.renew_lease_seconds}s")
                        if args.lease_seconds > 0 and float(r.get("_session_age", 0.0)) >= args.lease_seconds:
                            why.append(f"lease>={args.lease_seconds}s")
                        if args.dry_run:
                            print(f"WATCH_STOP_DRY transcode={tid} why={','.join(why)}")
                        else:
                            code = stop_transcode(args.plex_url, args.token, tid)
                            print(f"WATCH_STOP transcode={tid} status={code} why={','.join(why)}")

                if args.watch_runtime > 0 and (now - started) >= args.watch_runtime:
                    print("WATCH_DONE runtime")
                    return 0

                kick_evt.clear()
                kick_evt.wait(timeout=max(0.2, args.poll))
        finally:
            stop_evt.set()
            if sse_thread is not None:
                sse_thread.join(timeout=1.0)

    to_stop = rows
    if args.idle_seconds > 0:
        deadline = time.time() + max(0, args.wait)
        while True:
            rows = idle_annotate(matches())
            print(f"WAIT_MATCHED {len(rows)}")
            ready = []
            for r in rows:
                idle_age = r.get("_idle_age", 0.0)
                print(
                    f"WAIT machine={r['machine_id']} ip={r['player_addr']} transcode={r['transcode_id']} "
                    f"state={r['state']} idle_age={idle_age:.1f}s idle_ready={'yes' if r.get('_idle_ready') else 'no'} "
                    f"title={r['title']}"
                )
                if r.get("_idle_ready"):
                    ready.append(r)
            if ready:
                to_stop = ready
                break
            if not rows:
                print("DRAIN OK remaining=0")
                return 0
            if time.time() >= deadline:
                print(f"IDLE_WAIT TIMEOUT matched={len(rows)} ready=0")
                return 2
            time.sleep(max(0.1, args.poll))

    seen = set()
    for r in to_stop:
        tid = r["transcode_id"]
        if not tid or tid in seen:
            continue
        seen.add(tid)
        code = stop_transcode(args.plex_url, args.token, tid)
        print(f"STOP transcode={tid} status={code}")

    deadline = time.time() + max(0, args.wait)
    while time.time() < deadline:
        remain = matches()
        if not remain:
            print("DRAIN OK remaining=0")
            return 0
        time.sleep(max(0.1, args.poll))
    remain = matches()
    print(f"DRAIN TIMEOUT remaining={len(remain)}")
    for r in remain:
        print(f"REMAIN machine={r['machine_id']} ip={r['player_addr']} transcode={r['transcode_id']}")
    return 2


if __name__ == "__main__":
    raise SystemExit(main())
