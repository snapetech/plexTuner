param(
    [string]$Root = (Split-Path -Parent $PSScriptRoot),
    [string]$RunId = (Get-Date -Format 'yyyyMMdd-HHmmss'),
    [string]$OutRoot = '',
    [switch]$KeepWorkdir
)

$ErrorActionPreference = 'Stop'

if (-not $OutRoot) {
    $OutRoot = Join-Path $Root '.diag\windows-baremetal'
}

$OutDir = Join-Path $OutRoot $RunId
$WorkDir = Join-Path $env:TEMP ("iptvtunerr-windows-smoke-" + $RunId)
$Bin = Join-Path $Root 'iptv-tunerr.exe'
$Pids = New-Object System.Collections.Generic.List[int]

function Log($Message) {
    Write-Host "[windows-baremetal-smoke] $Message"
}

function Fail($Message) {
    throw "[windows-baremetal-smoke] ERROR: $Message"
}

function Pick-Port {
    $listener = [System.Net.Sockets.TcpListener]::new([System.Net.IPAddress]::Loopback, 0)
    $listener.Start()
    $port = ($listener.LocalEndpoint).Port
    $listener.Stop()
    return $port
}

function Invoke-Request {
    param(
        [string]$Method,
        [string]$Url,
        [hashtable]$Headers = @{},
        [string]$Body = ''
    )
    $req = [System.Net.HttpWebRequest]::Create($Url)
    $req.Method = $Method
    foreach ($key in $Headers.Keys) {
        switch ($key.ToLowerInvariant()) {
            'accept' { $req.Accept = [string]$Headers[$key] }
            'content-type' { $req.ContentType = [string]$Headers[$key] }
            'user-agent' { $req.UserAgent = [string]$Headers[$key] }
            default { $req.Headers.Add($key, [string]$Headers[$key]) }
        }
    }
    if ($Body -and $Method -ne 'HEAD') {
        $bytes = [System.Text.Encoding]::UTF8.GetBytes($Body)
        $req.ContentLength = $bytes.Length
        $stream = $req.GetRequestStream()
        $stream.Write($bytes, 0, $bytes.Length)
        $stream.Dispose()
    }
    try {
        $resp = $req.GetResponse()
    } catch [System.Net.WebException] {
        if ($_.Exception.Response) {
            $resp = $_.Exception.Response
        } else {
            throw
        }
    }
    $bodyText = ''
    if ($Method -ne 'HEAD' -and $resp.GetResponseStream()) {
        $reader = [System.IO.StreamReader]::new($resp.GetResponseStream())
        $bodyText = $reader.ReadToEnd()
        $reader.Dispose()
    }
    return [pscustomobject]@{
        StatusCode = [int]$resp.StatusCode
        Headers = $resp.Headers
        Body = $bodyText
    }
}

function Wait-Status {
    param(
        [string]$Url,
        [int]$Want,
        [int]$Attempts = 80
    )
    for ($i = 0; $i -lt $Attempts; $i++) {
        try {
            $resp = Invoke-Request -Method 'GET' -Url $Url
            if ($resp.StatusCode -eq $Want) {
                return
            }
        } catch {
        }
        Start-Sleep -Milliseconds 200
    }
    Fail "timeout waiting for $Url => $Want"
}

function Assert-Status {
    param([string]$Url, [int]$Want)
    $resp = Invoke-Request -Method 'GET' -Url $Url
    if ($resp.StatusCode -ne $Want) {
        Fail "$Url status=$($resp.StatusCode) want=$Want body=$($resp.Body)"
    }
    return $resp
}

function Assert-Header {
    param([string]$Url, [string]$Header, [string]$Want)
    $resp = Invoke-Request -Method 'HEAD' -Url $Url
    $got = $resp.Headers[$Header]
    if ($got -ne $Want) {
        Fail "$Url header $Header=$got want=$Want"
    }
}

function Start-AssetServer {
    param(
        [string]$RootDir,
        [int]$Port,
        [string]$LogPath
    )
    $job = Start-Job -ScriptBlock {
        param($RootDir, $Port, $LogPath)
        Add-Content -Path $LogPath -Value "asset server starting on $Port"
        $listener = [System.Net.HttpListener]::new()
        $listener.Prefixes.Add("http://127.0.0.1:$Port/")
        $listener.Start()
        try {
            while ($listener.IsListening) {
                $ctx = $listener.GetContext()
                $path = $ctx.Request.Url.AbsolutePath.TrimStart('/')
                $file = Join-Path $RootDir $path
                if (Test-Path $file) {
                    $bytes = [System.IO.File]::ReadAllBytes($file)
                    $ctx.Response.StatusCode = 200
                    $ctx.Response.ContentLength64 = $bytes.Length
                    $ctx.Response.OutputStream.Write($bytes, 0, $bytes.Length)
                } else {
                    $ctx.Response.StatusCode = 404
                }
                $ctx.Response.Close()
            }
        } finally {
            $listener.Stop()
        }
    } -ArgumentList $RootDir, $Port, $LogPath
    return $job
}

if (-not (Test-Path $Bin)) {
    Fail "expected binary at $Bin"
}

New-Item -ItemType Directory -Force -Path $OutDir, $WorkDir | Out-Null
New-Item -ItemType Directory -Force -Path (Join-Path $WorkDir 'assets'), (Join-Path $WorkDir 'cache') | Out-Null

try {
    Set-Content -Path (Join-Path $WorkDir 'assets\movie.bin') -Value 'movie-bytes' -NoNewline
    Set-Content -Path (Join-Path $WorkDir 'assets\episode.bin') -Value 'episode-bytes' -NoNewline

    $assetPort = Pick-Port
    $servePort = Pick-Port
    $emptyPort = Pick-Port
    $webuiPort = Pick-Port
    $vodPort = Pick-Port

    $catalogVod = @"
{
  "movies": [
    {
      "id": "m1",
      "title": "Smoke Movie",
      "year": 2024,
      "stream_url": "http://127.0.0.1:$assetPort/movie.bin"
    }
  ],
  "series": [
    {
      "id": "s1",
      "title": "Smoke Show",
      "year": 2023,
      "seasons": [
        {
          "number": 1,
          "episodes": [
            {
              "id": "e1",
              "season_num": 1,
              "episode_num": 1,
              "title": "Pilot",
              "stream_url": "http://127.0.0.1:$assetPort/episode.bin"
            }
          ]
        }
      ]
    }
  ],
  "live_channels": []
}
"@
    Set-Content -Path (Join-Path $WorkDir 'catalog-vod.json') -Value $catalogVod

    $programming = @"
{
  "selected_categories": ["news"],
  "order_mode": "source"
}
"@
    Set-Content -Path (Join-Path $WorkDir 'programming.json') -Value $programming

    $catalogFull = @"
{
  "movies": [],
  "series": [],
  "live_channels": [
    {
      "channel_id": "ch1",
      "dna_id": "dna-news",
      "guide_number": "101",
      "guide_name": "Smoke One",
      "group_title": "News",
      "stream_url": "http://example.invalid/stream-1.ts",
      "stream_urls": ["http://example.invalid/stream-1.ts"],
      "epg_linked": true,
      "tvg_id": "smoke.one"
    },
    {
      "channel_id": "ch2",
      "guide_number": "102",
      "guide_name": "Smoke Two",
      "group_title": "Sports",
      "stream_url": "http://example.invalid/stream-2.ts",
      "stream_urls": ["http://example.invalid/stream-2.ts"],
      "epg_linked": true,
      "tvg_id": "smoke.two"
    },
    {
      "channel_id": "ch3",
      "dna_id": "dna-news",
      "guide_number": "1001",
      "guide_name": "Smoke One",
      "group_title": "DirecTV",
      "source_tag": "directv",
      "stream_url": "http://example.invalid/stream-3.ts",
      "stream_urls": ["http://example.invalid/stream-3.ts"],
      "epg_linked": true,
      "tvg_id": "smoke.one"
    }
  ]
}
"@
    Set-Content -Path (Join-Path $WorkDir 'catalog-full.json') -Value $catalogFull

    $catalogEmpty = @"
{
  "movies": [],
  "series": [],
  "live_channels": []
}
"@
    Set-Content -Path (Join-Path $WorkDir 'catalog-empty.json') -Value $catalogEmpty

    $assetJob = Start-AssetServer -RootDir (Join-Path $WorkDir 'assets') -Port $assetPort -LogPath (Join-Path $OutDir 'assets.log')
    Wait-Status -Url "http://127.0.0.1:$assetPort/movie.bin" -Want 200

    $serveArgs = @(
        'serve',
        '-catalog', (Join-Path $WorkDir 'catalog-full.json'),
        '-addr', ":$servePort",
        '-base-url', "http://127.0.0.1:$servePort"
    )
    $serveProc = Start-Process -FilePath $Bin -ArgumentList $serveArgs -WorkingDirectory $Root -RedirectStandardOutput (Join-Path $OutDir 'serve-full.log') -RedirectStandardError (Join-Path $OutDir 'serve-full.log') -PassThru -WindowStyle Hidden -Environment @{
        IPTV_TUNERR_PROVIDER_EPG_ENABLED = 'false'
        IPTV_TUNERR_XMLTV_URL = ''
        IPTV_TUNERR_PROGRAMMING_RECIPE_FILE = (Join-Path $WorkDir 'programming.json')
        IPTV_TUNERR_WEBUI_DISABLED = '0'
        IPTV_TUNERR_WEBUI_USER = 'admin'
        IPTV_TUNERR_WEBUI_PASS = 'admin'
        IPTV_TUNERR_WEBUI_PORT = "$webuiPort"
    }
    $Pids.Add($serveProc.Id) | Out-Null
    Wait-Status -Url "http://127.0.0.1:$servePort/discover.json" -Want 200
    Wait-Status -Url "http://127.0.0.1:$webuiPort/login" -Want 200
    Assert-Status -Url "http://127.0.0.1:$servePort/readyz" -Want 200 | Out-Null
    Assert-Status -Url "http://127.0.0.1:$servePort/guide.xml" -Want 200 | Out-Null
    Assert-Header -Url "http://127.0.0.1:$servePort/guide.xml" -Header 'X-IptvTunerr-Guide-State' -Want 'ready'
    $lineup = Invoke-Request -Method 'GET' -Url "http://127.0.0.1:$servePort/lineup.json"
    if ($lineup.Body -notmatch '"GuideNumber":"101"') { Fail 'full lineup missing news row' }
    if ($lineup.Body -match '"GuideNumber":"102"') { Fail 'programming recipe did not filter sports row' }
    if ((Invoke-Request -Method 'GET' -Url "http://127.0.0.1:$servePort/programming/preview.json").Body -notmatch '"raw_channels": 3') { Fail 'preview missing raw count' }

    Stop-Process -Id $serveProc.Id -Force

    $emptyArgs = @(
        'serve',
        '-catalog', (Join-Path $WorkDir 'catalog-empty.json'),
        '-addr', ":$emptyPort",
        '-base-url', "http://127.0.0.1:$emptyPort"
    )
    $emptyProc = Start-Process -FilePath $Bin -ArgumentList $emptyArgs -WorkingDirectory $Root -RedirectStandardOutput (Join-Path $OutDir 'serve-empty.log') -RedirectStandardError (Join-Path $OutDir 'serve-empty.log') -PassThru -WindowStyle Hidden -Environment @{
        IPTV_TUNERR_PROVIDER_EPG_ENABLED = 'false'
        IPTV_TUNERR_XMLTV_URL = ''
        IPTV_TUNERR_WEBUI_DISABLED = '1'
    }
    $Pids.Add($emptyProc.Id) | Out-Null
    Wait-Status -Url "http://127.0.0.1:$emptyPort/discover.json" -Want 200
    Assert-Status -Url "http://127.0.0.1:$emptyPort/readyz" -Want 503 | Out-Null
    Assert-Status -Url "http://127.0.0.1:$emptyPort/guide.xml" -Want 503 | Out-Null
    Assert-Header -Url "http://127.0.0.1:$emptyPort/guide.xml" -Header 'X-IptvTunerr-Guide-State' -Want 'loading'

    $vodArgs = @(
        'vod-webdav',
        '-catalog', (Join-Path $WorkDir 'catalog-vod.json'),
        '-addr', "127.0.0.1:$vodPort",
        '-cache', (Join-Path $WorkDir 'cache')
    )
    $vodProc = Start-Process -FilePath $Bin -ArgumentList $vodArgs -WorkingDirectory $Root -RedirectStandardOutput (Join-Path $OutDir 'vod-webdav.log') -RedirectStandardError (Join-Path $OutDir 'vod-webdav.log') -PassThru -WindowStyle Hidden
    $Pids.Add($vodProc.Id) | Out-Null
    Wait-Status -Url "http://127.0.0.1:$vodPort/" -Want 405

    $options = Invoke-Request -Method 'OPTIONS' -Url "http://127.0.0.1:$vodPort/"
    if ($options.StatusCode -ne 200) { Fail "OPTIONS root status=$($options.StatusCode)" }
    if (-not $options.Headers['DAV']) { Fail 'OPTIONS root missing DAV header' }

    $propfindRoot = Invoke-Request -Method 'PROPFIND' -Url "http://127.0.0.1:$vodPort/" -Headers @{
        Depth = '1'
        'Content-Type' = 'text/xml'
        'User-Agent' = 'WebDAVFS/3.0 (03008000) Darwin/24.0.0'
    } -Body '<propfind xmlns="DAV:"><allprop/></propfind>'
    if ($propfindRoot.StatusCode -ne 207) { Fail "PROPFIND root status=$($propfindRoot.StatusCode)" }
    if ($propfindRoot.Body -notmatch 'Movies' -or $propfindRoot.Body -notmatch 'TV') { Fail 'PROPFIND root missing expected directories' }

    $headMovie = Invoke-Request -Method 'HEAD' -Url "http://127.0.0.1:$vodPort/Movies/Live:%20Smoke%20Movie%20%282024%29/Live:%20Smoke%20Movie%20%282024%29.mp4"
    if ($headMovie.StatusCode -ne 200) { Fail "movie HEAD status=$($headMovie.StatusCode)" }
    if ($headMovie.Headers['Accept-Ranges'] -ne 'bytes') { Fail 'movie HEAD missing Accept-Ranges' }

    $rangeMovie = Invoke-Request -Method 'GET' -Url "http://127.0.0.1:$vodPort/Movies/Live:%20Smoke%20Movie%20%282024%29/Live:%20Smoke%20Movie%20%282024%29.mp4" -Headers @{
        Range = 'bytes=0-4'
        'User-Agent' = 'Microsoft-WebDAV-MiniRedir/10.0.19045'
    }
    if ($rangeMovie.StatusCode -ne 206) { Fail "movie range status=$($rangeMovie.StatusCode)" }
    if ($rangeMovie.Body -ne 'movie') { Fail "movie range body=$($rangeMovie.Body)" }

    $putMovie = Invoke-Request -Method 'PUT' -Url "http://127.0.0.1:$vodPort/Movies/Live:%20Smoke%20Movie%20%282024%29/Live:%20Smoke%20Movie%20%282024%29.mp4" -Headers @{
        'Content-Type' = 'application/octet-stream'
    } -Body 'bad'
    if ($putMovie.StatusCode -ne 405) { Fail "movie PUT status=$($putMovie.StatusCode)" }

    @"
windows bare-metal smoke: PASS
serve_port=$servePort
webui_port=$webuiPort
empty_port=$emptyPort
vod_port=$vodPort
"@ | Set-Content -Path (Join-Path $OutDir 'summary.txt')
    Log "artifacts written to $OutDir"
}
finally {
    foreach ($pid in $Pids) {
        try { Stop-Process -Id $pid -Force -ErrorAction SilentlyContinue } catch {}
    }
    Get-Job | Remove-Job -Force -ErrorAction SilentlyContinue
    if (-not $KeepWorkdir) {
        Remove-Item -Recurse -Force $WorkDir -ErrorAction SilentlyContinue
    }
}
