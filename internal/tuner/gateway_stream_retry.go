package tuner

import "time"

func upstreamStreamRetryLimit() int {
	n := getenvInt("IPTV_TUNERR_UPSTREAM_RETRY_LIMIT", 2)
	if n < 0 {
		return 0
	}
	if n > 3 {
		return 3
	}
	return n
}

func upstreamStreamRetryBase(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	if attempt > 3 {
		attempt = 3
	}
	baseMs := getenvInt("IPTV_TUNERR_UPSTREAM_RETRY_BACKOFF_MS", 1000)
	if baseMs < 1 {
		baseMs = 1
	}
	if baseMs > 10000 {
		baseMs = 10000
	}
	return time.Duration(baseMs*(1<<(attempt-1))) * time.Millisecond
}

func upstreamStreamRetryDelay(attempt, status int, retryAfter time.Duration) time.Duration {
	delay := upstreamStreamRetryBase(attempt)
	if status > 0 {
		if mult := statusBackoffMultiplier(status); mult > 1.0 {
			scaled := time.Duration(float64(delay) * mult)
			if scaled > delay {
				delay = scaled
			}
		}
	}
	if retryAfter > delay {
		return retryAfter
	}
	return delay
}
