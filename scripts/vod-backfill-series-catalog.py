#!/usr/bin/env python3
"""
Repair/backfill series seasons/episodes in an existing PlexTuner catalog.json.

Use case:
- Older catalogs can contain series rows with empty `seasons` because provider
  `get_series_info` episode parsing missed the common Xtream shape:
  {"episodes": {"1": [...], "2": [...]}}.
- This script refetches per-series episode info and rewrites `series[].seasons`
  in-place (or to a new file) so VODFS TV libraries have actual episode files.

Credentials are derived from the first movie stream URL in the catalog:
  http(s)://host/movie/<user>/<pass>/<id>.<ext>
"""

from __future__ import annotations

import argparse
import json
import os
import sys
import time
import urllib.parse
import urllib.request
from concurrent.futures import ThreadPoolExecutor, as_completed
from typing import Any


def _derive_provider_from_catalog(catalog: dict[str, Any]) -> tuple[str, str, str, str]:
    movies = catalog.get("movies") or []
    if not movies:
        raise RuntimeError("catalog has no movies; cannot derive provider credentials")
    murl = movies[0].get("stream_url") or movies[0].get("streamURL") or ""
    if not murl:
        raise RuntimeError("first movie has no stream_url")
    u = urllib.parse.urlparse(murl)
    parts = [p for p in u.path.split("/") if p]
    try:
        mid = parts.index("movie")
        user = parts[mid + 1]
        pw = parts[mid + 2]
    except Exception as e:
        raise RuntimeError(f"cannot derive creds from movie url {murl!r}: {e}") from e
    api_base = f"{u.scheme}://{u.netloc}"
    stream_base = api_base.rstrip("/")
    return api_base, stream_base, user, pw


def _load_json(url: str, timeout: int) -> Any:
    req = urllib.request.Request(url, headers={"User-Agent": "PlexTuner/1.0"})
    with urllib.request.urlopen(req, timeout=timeout) as r:
        return json.loads(r.read().decode("utf-8", "replace"))


def _parse_xtream_episodes(v: Any) -> list[dict[str, Any]]:
    out: list[dict[str, Any]] = []
    if isinstance(v, dict):
        for season_key, mv in v.items():
            if isinstance(mv, dict):
                out.append(mv)
            elif isinstance(mv, list):
                for item in mv:
                    if not isinstance(item, dict):
                        continue
                    if not item.get("season_num"):
                        item = dict(item)
                        try:
                            item["season_num"] = int(season_key)
                        except Exception:
                            pass
                    out.append(item)
    elif isinstance(v, list):
        out.extend([x for x in v if isinstance(x, dict)])
    return out


def _rebuild_series(
    series_row: dict[str, Any],
    api_base: str,
    stream_base: str,
    user: str,
    pw: str,
    timeout: int,
) -> tuple[dict[str, Any], dict[str, Any]]:
    sid = str(series_row.get("id") or "")
    if not sid:
        row = dict(series_row)
        row["seasons"] = []
        return row, {"ok": False, "sid": "", "err": "missing id"}

    q = urllib.parse.urlencode(
        {
            "username": user,
            "password": pw,
            "action": "get_series_info",
            "series_id": sid,
        }
    )
    url = f"{api_base}/player_api.php?{q}"
    try:
        info = _load_json(url, timeout=timeout)
    except Exception as e:
        row = dict(series_row)
        row["seasons"] = []
        return row, {"ok": False, "sid": sid, "err": str(e)}

    eps = _parse_xtream_episodes((info or {}).get("episodes"))
    by_season: dict[int, list[dict[str, Any]]] = {}
    for ep in eps:
        try:
            ep_id = str(ep.get("id") or "")
            sn = int(ep.get("season_num") or 0)
            en = int(ep.get("episode_num") or 0)
        except Exception:
            continue
        if not ep_id or sn <= 0 or en <= 0:
            continue
        ext = (str(ep.get("container_extension") or "mp4").strip() or "mp4")
        stream_url = f"{stream_base}/series/{user}/{pw}/{ep_id}.{ext}"
        by_season.setdefault(sn, []).append(
            {
                "id": ep_id,
                "season_num": sn,
                "episode_num": en,
                "title": ep.get("title") or "",
                "airdate": ep.get("releaseDate") or "",
                "stream_url": stream_url,
            }
        )

    seasons: list[dict[str, Any]] = []
    total_eps = 0
    for sn in sorted(by_season):
        by_season[sn].sort(key=lambda e: (e.get("episode_num", 0), e.get("id", "")))
        total_eps += len(by_season[sn])
        seasons.append({"number": sn, "episodes": by_season[sn]})

    row = dict(series_row)
    row["seasons"] = seasons
    return row, {"ok": True, "sid": sid, "seasons": len(seasons), "eps": total_eps}


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("--catalog-in", required=True, help="Input catalog.json")
    ap.add_argument("--catalog-out", required=True, help="Output catalog.json")
    ap.add_argument("--progress-out", default="", help="Write progress JSON here")
    ap.add_argument("--workers", type=int, default=6, help="Concurrent get_series_info workers")
    ap.add_argument("--timeout", type=int, default=60, help="Per-request timeout seconds")
    ap.add_argument("--limit", type=int, default=0, help="Only process first N series (debug)")
    ap.add_argument(
        "--retry-failed-from",
        default="",
        help="Progress JSON from a previous run; only retry listed fail_examples SIDs",
    )
    args = ap.parse_args()

    with open(args.catalog_in) as f:
        catalog = json.load(f)

    api_base, stream_base, user, pw = _derive_provider_from_catalog(catalog)
    series = list(catalog.get("series") or [])
    if args.limit > 0:
        series = series[: args.limit]

    retry_sids: set[str] | None = None
    if args.retry_failed_from:
        with open(args.retry_failed_from) as f:
            p = json.load(f)
        retry_sids = {
            str(x.get("sid"))
            for x in (p.get("fail_examples") or [])
            if isinstance(x, dict) and x.get("sid")
        }
        if retry_sids:
            print(json.dumps({"retry_sids": sorted(retry_sids)}), flush=True)

    print(
        json.dumps(
            {
                "api_base": api_base,
                "movies": len(catalog.get("movies") or []),
                "series_total": len(catalog.get("series") or []),
                "series_processing": len(series),
                "workers": args.workers,
            }
        ),
        flush=True,
    )

    start = time.time()
    out_series = list(catalog.get("series") or [])
    by_sid_index = {str(s.get("id") or ""): i for i, s in enumerate(out_series)}
    stats = {"done": 0, "ok": 0, "fail": 0, "eps": 0, "seasons": 0, "started_at": int(start)}
    fail_examples: list[dict[str, Any]] = []

    items: list[tuple[int, dict[str, Any]]] = []
    if retry_sids is not None:
        for sid in retry_sids:
            i = by_sid_index.get(sid)
            if i is None:
                continue
            items.append((i, out_series[i]))
    else:
        items = [(i, s) for i, s in enumerate(series)]

    with ThreadPoolExecutor(max_workers=args.workers) as ex:
        fut_map = {
            ex.submit(_rebuild_series, s, api_base, stream_base, user, pw, args.timeout): idx
            for idx, s in items
        }
        for fut in as_completed(fut_map):
            idx = fut_map[fut]
            row, meta = fut.result()
            out_series[idx] = row
            stats["done"] += 1
            if meta.get("ok"):
                stats["ok"] += 1
                stats["eps"] += int(meta.get("eps", 0) or 0)
                stats["seasons"] += int(meta.get("seasons", 0) or 0)
            else:
                stats["fail"] += 1
                if len(fail_examples) < 50:
                    fail_examples.append(meta)

            if stats["done"] % 100 == 0 or stats["done"] == len(items):
                snap = {
                    **stats,
                    "elapsed_s": round(time.time() - start, 1),
                    "fail_examples": fail_examples,
                }
                if args.progress_out:
                    with open(args.progress_out, "w") as f:
                        json.dump(snap, f)
                print(json.dumps(snap), flush=True)

    out_catalog = dict(catalog)
    out_catalog["series"] = out_series
    with open(args.catalog_out, "w") as f:
        json.dump(out_catalog, f)

    summary = {
        **stats,
        "elapsed_s": round(time.time() - start, 1),
        "catalog_out": args.catalog_out,
        "series_with_seasons": sum(1 for s in out_series if (s.get("seasons") or [])),
        "mode": "retry" if retry_sids is not None else "full",
        "fail_examples": fail_examples,
    }
    if args.progress_out:
        with open(args.progress_out, "w") as f:
            json.dump(summary, f)
    print(json.dumps(summary), flush=True)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
