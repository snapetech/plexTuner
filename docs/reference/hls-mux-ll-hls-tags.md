---
id: hls-mux-ll-hls-tags
type: reference
status: stable
tags: [hls, ll-hls, mux, playlist]
---

# LL-HLS tag coverage (`?mux=hls` rewrite)

Native mux playlist rewrite (`rewriteHLSPlaylistToGatewayProxy`) handles:

| Pattern | Behavior |
|--------|----------|
| **`URI="..."` / `URI='...'` on tag lines** | Any `#` line containing **`URI="`** or **`URI='`** (case-insensitive **`uri=`**) runs through **`rewriteHLSQuotedURIAttrs`**, which rewrites both double- and single-quoted upstream URLs. This includes **`#EXT-X-PART`**, **`#EXT-X-PRELOAD-HINT`**, **`#EXT-X-RENDITION-REPORT`**, keys, maps, variants, media, session keys, etc. |
| **Same-line `#EXTINF` media** | Non-standard **`#EXTINF:<dur>,<path>.<ext>`** where `<path>` looks like a segment URL (extension **m4s**, **ts**, **mp4**, **mp2t**, **aac**, **webvtt**, **vtt**) is rewritten as a Tunerr **`?mux=hls&seg=`** URL. Conservative to avoid treating titles as URLs. |
| **Merged `#EXTINF` + `BYTERANGE=`** | Some packagers emit **`#EXTINF:<dur>[,<title>],BYTERANGE=<n>`** or quoted **`BYTERANGE="length@offset"`** on one line. [RFC 8216 bis](https://datatracker.ietf.org/doc/draft-pantos-hls-rfc8216bis/) keeps **`#EXTINF`** and **`#EXT-X-BYTERANGE`** as separate tags; Tunerr splits the line before the rest of the rewrite. |
| **Separate-line media** | Unquoted URI on its own line after **`#EXTINF`** / **`#EXT-X-BYTERANGE`** (unchanged): the media line is rewritten; **`Range`** is forwarded on **`seg=`** fetches (see [HLS mux how-to](../how-to/hls-mux-proxy.md)). |

**Not** rewritten here: **`#EXT-X-BYTERANGE`** values (the player sends **`Range`**); tags without **`URI=`** that still imply a URL through other attributes (open an issue with a sample if you hit one).

See also
--------

- [HLS mux toolkit](hls-mux-toolkit.md)
- [HLS mux how-to](../how-to/hls-mux-proxy.md)
