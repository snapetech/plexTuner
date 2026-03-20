package tuner

import (
	"math"
	"net"
	"net/http"
	"time"

	"golang.org/x/time/rate"
)

func (g *Gateway) noteHLSMuxSegOutcome(kind string) {
	g.noteMuxSegOutcome("hls", kind, "", PromNoMuxSegHistogram)
}

func (g *Gateway) noteMuxSegOutcome(mux, kind, channelID string, elapsed time.Duration) {
	promNoteMuxSegOutcome(mux, kind, channelID, elapsed)
	if mux == "dash" {
		switch kind {
		case "success":
			g.dashMuxSegSuccess.Add(1)
		case "err_scheme":
			g.dashMuxSegErrScheme.Add(1)
		case "err_private":
			g.dashMuxSegErrPrivate.Add(1)
		case "err_param":
			g.dashMuxSegErrParam.Add(1)
		case "upstream_http":
			g.dashMuxSegUpstreamHTTPErrs.Add(1)
		case "502":
			g.dashMuxSeg502Fail.Add(1)
		case "503_limit":
			g.dashMuxSeg503LimitHits.Add(1)
		case "429_rate":
			g.dashMuxSegRateLimited.Add(1)
		case "err_redirect":
			g.dashMuxSeg502Fail.Add(1)
		default:
		}
		return
	}
	switch kind {
	case "success":
		g.hlsMuxSegSuccess.Add(1)
	case "err_scheme":
		g.hlsMuxSegErrScheme.Add(1)
	case "err_private":
		g.hlsMuxSegErrPrivate.Add(1)
	case "err_param":
		g.hlsMuxSegErrParam.Add(1)
	case "upstream_http":
		g.hlsMuxSegUpstreamHTTPErrs.Add(1)
	case "502":
		g.hlsMuxSeg502Fail.Add(1)
	case "503_limit":
		g.hlsMuxSeg503LimitHits.Add(1)
	case "429_rate":
		g.hlsMuxSegRateLimited.Add(1)
	case "err_redirect":
		g.hlsMuxSeg502Fail.Add(1)
	default:
	}
}

func (g *Gateway) allowMuxSegRate(r *http.Request) bool {
	rps := muxSegRPSPerIP()
	if rps <= 0 || r == nil {
		return true
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	if host == "" {
		host = "unknown"
	}
	g.segRaterMu.Lock()
	defer g.segRaterMu.Unlock()
	if g.segRaterByIP == nil {
		g.segRaterByIP = make(map[string]*rate.Limiter)
	}
	lim, ok := g.segRaterByIP[host]
	if !ok {
		burst := int(math.Ceil(rps))
		if burst < 1 {
			burst = 1
		}
		if burst > 100 {
			burst = 100
		}
		lim = rate.NewLimiter(rate.Limit(rps), burst)
		g.segRaterByIP[host] = lim
	}
	return lim.Allow()
}
