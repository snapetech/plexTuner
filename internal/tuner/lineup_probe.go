package tuner

import (
	"context"
	"log"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/snapetech/iptvtunerr/internal/catalog"
	"github.com/snapetech/iptvtunerr/internal/httpclient"
	"github.com/snapetech/iptvtunerr/internal/indexer"
)

func applyLineupProbeFilter(live []catalog.LiveChannel) []catalog.LiveChannel {
	if len(live) == 0 || !lineupProbeEnabled() {
		return live
	}
	start := time.Now()
	timeout := lineupProbeDurationEnv("IPTV_TUNERR_LINEUP_PROBE_TIMEOUT", 3*time.Second)
	concurrency := lineupProbeIntEnv("IPTV_TUNERR_LINEUP_PROBE_CONCURRENCY", 2)
	maxFeeds := lineupProbeIntEnv("IPTV_TUNERR_LINEUP_PROBE_MAX_FEEDS", 0)
	maxDuration := lineupProbeDurationEnv("IPTV_TUNERR_LINEUP_PROBE_MAX_DURATION", 2*time.Minute)
	cacheTTL := lineupProbeDurationEnv("IPTV_TUNERR_LINEUP_PROBE_CACHE_TTL", 10*time.Minute)
	cacheFile := strings.TrimSpace(os.Getenv("IPTV_TUNERR_LINEUP_PROBE_CACHE_FILE"))

	beforeChannels := len(live)
	beforeFeeds := lineupFeedCapacity(live)
	cache := indexer.LoadSmoketestCache(cacheFile)
	client := httpclient.WithTimeout(timeout)
	filtered := indexer.FilterLiveByFeedSmoketestWithCache(live, cache, cacheTTL, client, timeout, concurrency, maxFeeds, maxDuration)
	if cacheFile != "" {
		if err := cache.Save(cacheFile); err != nil {
			log.Printf("Lineup probe cache save failed: %v", err)
		}
	}
	log.Printf(
		"Lineup probe applied: channels=%d/%d feeds=%d/%d concurrency=%d timeout=%s max_duration=%s",
		len(filtered),
		beforeChannels,
		lineupFeedCapacity(filtered),
		beforeFeeds,
		concurrency,
		timeout,
		maxDuration,
	)
	filtered = applyLineupVisualProbeFilter(filtered)
	log.Printf("Lineup probe total duration: channels=%d feeds=%d duration=%s", len(filtered), lineupFeedCapacity(filtered), time.Since(start).Round(time.Millisecond))
	return filtered
}

func lineupProbeEnabled() bool {
	v := strings.TrimSpace(os.Getenv("IPTV_TUNERR_LINEUP_PROBE_ENABLED"))
	return v == "1" || strings.EqualFold(v, "true") || strings.EqualFold(v, "yes") || strings.EqualFold(v, "on")
}

func lineupProbeIntEnv(key string, defaultVal int) int {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return defaultVal
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 {
		return defaultVal
	}
	return n
}

func lineupProbeDurationEnv(key string, defaultVal time.Duration) time.Duration {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return defaultVal
	}
	d, err := time.ParseDuration(v)
	if err != nil || d <= 0 {
		return defaultVal
	}
	return d
}

func applyLineupVisualProbeFilter(live []catalog.LiveChannel) []catalog.LiveChannel {
	mode := lineupVisualProbeMode()
	if len(live) == 0 || mode == "off" {
		return live
	}
	ffmpegPath, err := resolveFFmpegPath()
	if err != nil {
		log.Printf("Lineup visual probe skipped: ffmpeg unavailable: %v", err)
		return live
	}
	sample := lineupProbeDurationEnv("IPTV_TUNERR_LINEUP_VISUAL_PROBE_SAMPLE", 4*time.Second)
	timeout := lineupProbeDurationEnv("IPTV_TUNERR_LINEUP_VISUAL_PROBE_TIMEOUT", sample+5*time.Second)
	concurrency := lineupProbeIntEnv("IPTV_TUNERR_LINEUP_VISUAL_PROBE_CONCURRENCY", 1)
	maxFeeds := lineupProbeIntEnv("IPTV_TUNERR_LINEUP_VISUAL_PROBE_MAX_FEEDS", 12)
	maxDuration := lineupProbeDurationEnv("IPTV_TUNERR_LINEUP_VISUAL_PROBE_MAX_DURATION", 90*time.Second)
	cacheTTL := lineupProbeDurationEnv("IPTV_TUNERR_LINEUP_VISUAL_PROBE_CACHE_TTL", 30*time.Minute)
	cacheFile := strings.TrimSpace(os.Getenv("IPTV_TUNERR_LINEUP_VISUAL_PROBE_CACHE_FILE"))
	if cacheFile == "" {
		if fastCache := strings.TrimSpace(os.Getenv("IPTV_TUNERR_LINEUP_PROBE_CACHE_FILE")); fastCache != "" {
			cacheFile = fastCache + ".visual"
		}
	}

	type candidate struct {
		url string
	}
	seen := map[string]struct{}{}
	var candidates []candidate
	for _, ch := range live {
		if mode != "all" && !lineupVisualProbeCandidate(ch) {
			continue
		}
		for _, raw := range channelProbeURLs(ch) {
			u := strings.TrimSpace(raw)
			if u == "" {
				continue
			}
			if _, ok := seen[u]; ok {
				continue
			}
			seen[u] = struct{}{}
			candidates = append(candidates, candidate{url: u})
			if maxFeeds > 0 && len(candidates) >= maxFeeds {
				break
			}
		}
		if maxFeeds > 0 && len(candidates) >= maxFeeds {
			break
		}
	}
	if len(candidates) == 0 {
		log.Printf("Lineup visual probe skipped: mode=%s candidates=0", mode)
		return live
	}

	cache := indexer.LoadSmoketestCache(cacheFile)
	results := map[string]visualProbeResult{}
	var needProbe []candidate
	for _, cand := range candidates {
		if cacheTTL > 0 {
			if pass, fresh := cache.IsFresh(cand.url, cacheTTL); fresh {
				results[cand.url] = visualProbeResult{pass: pass, known: true}
				continue
			}
		}
		needProbe = append(needProbe, cand)
	}

	start := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), maxDuration)
	defer cancel()
	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup
	var mu sync.Mutex
	for _, cand := range needProbe {
		cand := cand
		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				return
			}
			defer func() { <-sem }()
			pass := probeStreamVisual(ctx, ffmpegPath, cand.url, sample, timeout)
			mu.Lock()
			cache[cand.url] = indexer.NewSmoketestCacheEntry(pass, time.Now())
			results[cand.url] = visualProbeResult{pass: pass, known: true}
			mu.Unlock()
		}()
	}
	wg.Wait()
	if cacheFile != "" {
		if err := cache.Save(cacheFile); err != nil {
			log.Printf("Lineup visual probe cache save failed: %v", err)
		}
	}

	filtered := filterChannelsByProbeResults(live, results)
	log.Printf(
		"Lineup visual probe applied: mode=%s channels=%d/%d feeds=%d/%d candidates=%d probed=%d concurrency=%d sample=%s timeout=%s max_duration=%s duration=%s",
		mode,
		len(filtered),
		len(live),
		lineupFeedCapacity(filtered),
		lineupFeedCapacity(live),
		len(candidates),
		len(needProbe),
		concurrency,
		sample,
		timeout,
		maxDuration,
		time.Since(start).Round(time.Millisecond),
	)
	return filtered
}

type visualProbeResult struct {
	pass  bool
	known bool
}

func lineupVisualProbeMode() string {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("IPTV_TUNERR_LINEUP_VISUAL_PROBE")))
	switch v {
	case "", "off", "false", "0", "no":
		return "off"
	case "all":
		return "all"
	default:
		return "event"
	}
}

func lineupVisualProbeCandidate(ch catalog.LiveChannel) bool {
	s := lineupRecipeSearchText(ch)
	for _, term := range []string{
		" peacock", " ppv", " nba pass", " tsn+", " live ", " next ", " game ",
		" playoffs", " vs ", " v. ", " ufc", " fight", " boxing",
	} {
		if strings.Contains(s, term) {
			return true
		}
	}
	_, ok := lineupRecipeSportsEventTime(ch)
	return ok
}

func channelProbeURLs(ch catalog.LiveChannel) []string {
	if len(ch.StreamURLs) > 0 {
		return ch.StreamURLs
	}
	if strings.TrimSpace(ch.StreamURL) != "" {
		return []string{ch.StreamURL}
	}
	return nil
}

func filterChannelsByProbeResults(live []catalog.LiveChannel, results map[string]visualProbeResult) []catalog.LiveChannel {
	out := make([]catalog.LiveChannel, 0, len(live))
	for _, ch := range live {
		var kept []string
		for _, raw := range channelProbeURLs(ch) {
			u := strings.TrimSpace(raw)
			if u == "" {
				continue
			}
			r, ok := results[u]
			if !ok || !r.known || r.pass {
				kept = append(kept, u)
			}
		}
		if len(kept) == 0 {
			continue
		}
		next := ch
		next.StreamURL = kept[0]
		next.StreamURLs = kept
		out = append(out, next)
	}
	return out
}

func probeStreamVisual(parent context.Context, ffmpegPath, streamURL string, sample, timeout time.Duration) bool {
	if sample <= 0 {
		sample = 4 * time.Second
	}
	if timeout <= 0 {
		timeout = sample + 5*time.Second
	}
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	args := []string{
		"-hide_banner", "-nostats", "-v", "info",
		"-t", strconv.Itoa(intMax(1, int(sample/time.Second))),
		"-user_agent", "IptvTunerr/1.0",
		"-i", streamURL,
		"-vf", "blackdetect=d=1:pix_th=0.10",
		"-an",
		"-f", "null", "-",
	}
	out, err := exec.CommandContext(ctx, ffmpegPath, args...).CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		return false
	}
	if err != nil {
		return false
	}
	return !visualProbeDetectedBlack(string(out), sample)
}

func visualProbeDetectedBlack(logText string, sample time.Duration) bool {
	logText = strings.ToLower(logText)
	if !strings.Contains(logText, "black_start:0") {
		return false
	}
	minDuration := sample.Seconds() - 0.75
	if minDuration < 1 {
		minDuration = 1
	}
	for _, part := range strings.Fields(logText) {
		if !strings.HasPrefix(part, "black_duration:") {
			continue
		}
		raw := strings.TrimPrefix(part, "black_duration:")
		d, err := strconv.ParseFloat(strings.Trim(raw, ","), 64)
		if err == nil && d >= minDuration {
			return true
		}
	}
	return false
}
