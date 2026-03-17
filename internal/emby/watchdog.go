package emby

import (
	"context"
	"log"
	"time"
)

// DVRWatchdog periodically checks that the media server has live TV channels indexed
// from this tuner. If the channel count drops to 0 (e.g. after the server is restarted
// and its tuner config is wiped), it re-registers automatically.
//
// interval is the check cadence; 5 minutes is recommended for production. The watchdog
// runs until ctx is cancelled. stateFile may be empty to skip state persistence.
func DVRWatchdog(ctx context.Context, cfg Config, stateFile string, interval time.Duration) {
	if interval <= 0 {
		interval = 5 * time.Minute
	}
	tag := cfg.logTag()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	reregister := func(reason string) {
		log.Printf("%s [watchdog] %s — re-registering", tag, reason)
		if err := FullRegister(cfg, stateFile); err != nil {
			log.Printf("%s [watchdog] re-registration failed: %v", tag, err)
		} else {
			log.Printf("%s [watchdog] re-registration complete", tag)
		}
	}

	check := func() {
		count := GetChannelCount(cfg)
		if count == 0 {
			reregister("channel count is 0 (server may have restarted)")
		} else {
			log.Printf("%s [watchdog] ok channels=%d", tag, count)
		}
	}

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			check()
		}
	}
}
