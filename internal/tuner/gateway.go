package tuner

import (
	"errors"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/snapetech/iptvtunerr/internal/catalog"
	"github.com/snapetech/iptvtunerr/internal/eventhooks"
	"golang.org/x/time/rate"
)

// errCFBlock is returned by fetchAndWriteSegment when FetchCFReject is true and a segment
// is redirected to the Cloudflare abuse page (cloudflare-terms-of-service-abuse.com).
// The HLS relay loop treats this as a fatal error that aborts the entire stream immediately.
var errCFBlock = errors.New("cloudflare-abuse-block")

// Gateway proxies live stream requests to provider URLs with optional auth.
// Limit concurrent streams to TunerCount (tuner semantics).
type Gateway struct {
	Channels                   []catalog.LiveChannel
	EventHooks                 *eventhooks.Dispatcher
	ProviderUser               string
	ProviderPass               string
	TunerCount                 int
	StreamAttemptLimit         int
	StreamBufferBytes          int    // 0 = no buffer, -1 = auto
	StreamTranscodeMode        string // "off" | "on" | "auto"
	TranscodeOverrides         map[string]bool
	DefaultProfile             string
	ProfileOverrides           map[string]string
	NamedProfiles              map[string]NamedStreamProfile
	CustomHeaders              map[string]string // extra headers to send on all upstream requests (e.g. Referer, Origin)
	CustomUserAgent            string            // override User-Agent sent to upstream; supports preset names: lavf, ffmpeg, vlc, kodi, firefox
	DetectedFFmpegUA           string            // auto-detected Lavf/X.Y.Z from installed ffmpeg, used when CustomUserAgent is "lavf"/"ffmpeg"
	AddSecFetchHeaders         bool
	AutoCFBoot                 bool // when true, automatically bootstrap CF clearance at startup and on first CF hit
	DisableFFmpeg              bool
	DisableFFmpegDNS           bool
	Client                     *http.Client
	CookieJarFile              string // path to persist cookies for Cloudflare clearance
	persistentCookieJar        *persistentCookieJar
	cfBoot                     *cfBootstrapper // nil unless AutoCFBoot is true
	cfLearnedStore             *cfLearnedStore // persisted per-host CF state (working UA, CF-tagged)
	learnedUAMu                sync.Mutex
	learnedUAByHost            map[string]string // hostname → working UA found by cycling
	StreamAttemptLogFile       string            // if set, stream attempt records are appended as JSON lines
	FetchCFReject              bool              // abort HLS stream on segment redirected to CF abuse page
	PlexPMSURL                 string
	PlexPMSToken               string
	PlexClientAdapt            bool
	Autopilot                  *autopilotStore
	mu                         sync.Mutex
	inUse                      int
	hlsPackagerInUse           int
	hlsMuxSegInUse             int // concurrent ?mux=hls&seg= proxies (bounded; see effectiveHLSMuxSegLimitLocked)
	hlsPackagerSessions        map[string]*ffmpegHLSPackagerSession
	hlsMuxSegSuccess           atomic.Uint64
	hlsMuxSegErrScheme         atomic.Uint64
	hlsMuxSegErrPrivate        atomic.Uint64
	hlsMuxSegErrParam          atomic.Uint64
	hlsMuxSegUpstreamHTTPErrs  atomic.Uint64
	hlsMuxSeg502Fail           atomic.Uint64
	hlsMuxSeg503LimitHits      atomic.Uint64
	hlsMuxSegRateLimited       atomic.Uint64
	upstreamQuarantineSkips    atomic.Uint64 // URLs dropped by host quarantine when backups remained
	dashMuxSegSuccess          atomic.Uint64
	dashMuxSegErrScheme        atomic.Uint64
	dashMuxSegErrPrivate       atomic.Uint64
	dashMuxSegErrParam         atomic.Uint64
	dashMuxSegUpstreamHTTPErrs atomic.Uint64
	dashMuxSeg502Fail          atomic.Uint64
	dashMuxSeg503LimitHits     atomic.Uint64
	dashMuxSegRateLimited      atomic.Uint64
	segRaterMu                 sync.Mutex
	segRaterByIP               map[string]*rate.Limiter
	muxSegAutoMu               sync.Mutex
	muxSegAutoRejectAt         []time.Time // timestamps of 503 seg-limit rejects (for IPTV_TUNERR_HLS_MUX_SEG_SLOTS_AUTO)
	learnedUpstreamLimit       int
	reqSeq                     uint64
	activeMu                   sync.Mutex
	activeStreams              map[string]activeStreamEntry
	sharedRelays               map[string]*sharedRelaySession
	accountLeaseMu             sync.Mutex
	accountLeases              map[string]int
	accountLimitStore          *accountLimitStore
	providerStateMu            sync.Mutex
	learnedAccountLimits       map[string]int
	accountConcurrencySignals  map[string]int
	concurrencyHits            int
	lastConcurrencyAt          time.Time
	lastConcurrencyBody        string
	lastConcurrencyCode        int
	cfBlockHits                int
	lastCFBlockAt              time.Time
	lastCFBlockURL             string
	hlsPlaylistFailures        int
	lastHLSPlaylistAt          time.Time
	lastHLSPlaylistURL         string
	hlsSegmentFailures         int
	lastHLSSegmentAt           time.Time
	lastHLSSegmentURL          string
	lastHLSMuxOutcome          string
	lastHLSMuxAt               time.Time
	lastHLSMuxURL              string
	lastDashMuxOutcome         string
	lastDashMuxAt              time.Time
	lastDashMuxURL             string
	hostFailures               map[string]hostFailureStat
	hlsRemuxFailures           map[string]hostFailureStat
	attemptsMu                 sync.Mutex
	recentAttempts             []StreamAttemptRecord
	adaptStickyMu              sync.Mutex
	adaptStickyUntil           map[string]time.Time // HR-004: Plex session+channel → websafe sticky expiry
	hlsPackagerJanitorOnce     sync.Once
}

type gatewayReqIDKey struct{}
type gatewayChannelKey struct{}
