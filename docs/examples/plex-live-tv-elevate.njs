// nginx njs helper for docs/examples/plex-live-tv-elevate.nginx.conf.example.
//
// It mirrors the hardened Tunerr owner-token elevation allowlist:
//   - safe methods only: GET, HEAD, OPTIONS
//   - explicit Live TV/provider paths
//   - transcode/playQueue helper requests only when path/uri targets Live TV
//   - no arbitrary bait-query elevation on normal library paths

function ownerToken() {
    return (ngx.env.PLEX_OWNER_TOKEN || '').trim();
}

function firstArg(r, name) {
    var v = r.args[name];
    if (Array.isArray(v)) {
        return v.length ? String(v[0]) : '';
    }
    return v === undefined ? '' : String(v);
}

function liveTVText(s) {
    s = String(s || '').toLowerCase();
    return s.indexOf('/livetv/') !== -1 ||
        s.indexOf('tv.plex.providers.epg.xmltv:') !== -1 ||
        s.indexOf('livetv%2f') !== -1 ||
        s.indexOf('tv.plex.providers.epg.xmltv%3a') !== -1;
}

function safeMethod(r) {
    return r.method === 'GET' || r.method === 'HEAD' || r.method === 'OPTIONS';
}

function shouldElevate(r) {
    if (!safeMethod(r) || ownerToken() === '') {
        return false;
    }

    var path = r.uri || '';
    if (path === '/media/providers' ||
        path === '/media/grabbers/devices' ||
        path.indexOf('/livetv/') === 0 ||
        path.indexOf('/tv.plex.providers.epg.xmltv:') === 0) {
        return true;
    }

    if (path.indexOf('/video/:/transcode/') === 0) {
        return liveTVText(firstArg(r, 'path'));
    }

    if (path.indexOf('/playQueues') === 0) {
        return liveTVText(firstArg(r, 'uri')) || liveTVText(firstArg(r, 'path'));
    }

    if (path === '/' || path === '/identity') {
        return liveTVText(r.headersIn.Referer || r.headersIn.referer || '');
    }

    return false;
}

function inboundQueryToken(r) {
    return firstArg(r, 'X-Plex-Token');
}

function inboundHeaderToken(r) {
    return String(r.headersIn['X-Plex-Token'] || '');
}

function effectiveToken(r) {
    if (shouldElevate(r)) {
        return ownerToken();
    }
    return inboundHeaderToken(r) || inboundQueryToken(r);
}

function pushArg(parts, key, value) {
    parts.push(encodeURIComponent(key) + '=' + encodeURIComponent(value));
}

function effectiveArgs(r) {
    var elevated = shouldElevate(r);
    var parts = [];
    var sawToken = false;

    for (var key in r.args) {
        var values = Array.isArray(r.args[key]) ? r.args[key] : [r.args[key]];
        for (var i = 0; i < values.length; i++) {
            if (key === 'X-Plex-Token') {
                sawToken = true;
                pushArg(parts, key, elevated ? ownerToken() : String(values[i]));
            } else {
                pushArg(parts, key, String(values[i]));
            }
        }
    }

    if (elevated && !sawToken) {
        pushArg(parts, 'X-Plex-Token', ownerToken());
    }

    return parts.join('&');
}

export default { effectiveArgs, effectiveToken };
