package httpclient

import (
	"net/url"
	"sync"
)

// HostSemaphore is a process-global per-host concurrency limiter.
// All HTTP clients in the process share the same semaphore for a given host,
// preventing thundering-herd when many goroutines or supervisor children
// hammer the same upstream at once.
//
// Usage: acquire before sending a request, release when the response arrives.
//
//	release := GlobalHostSem.Acquire(host)
//	defer release()
type HostSemaphore struct {
	mu    sync.Mutex
	sems  map[string]chan struct{}
	limit int
}

// GlobalHostSem is the shared per-host limiter. Default cap: 4 concurrent
// requests per host across the entire process.
var GlobalHostSem = NewHostSemaphore(4)

func NewHostSemaphore(concurrency int) *HostSemaphore {
	if concurrency < 1 {
		concurrency = 1
	}
	return &HostSemaphore{
		sems:  make(map[string]chan struct{}),
		limit: concurrency,
	}
}

// Acquire blocks until a slot is available for host and returns a release func.
// host should be the scheme+host (e.g. "http://example.com:8080").
func (h *HostSemaphore) Acquire(host string) func() {
	sem := h.semFor(host)
	sem <- struct{}{}
	return func() { <-sem }
}

func (h *HostSemaphore) semFor(host string) chan struct{} {
	// Normalise: strip path/query, keep scheme+host.
	if u, err := url.Parse(host); err == nil {
		host = u.Scheme + "://" + u.Host
	}
	h.mu.Lock()
	s, ok := h.sems[host]
	if !ok {
		s = make(chan struct{}, h.limit)
		h.sems[host] = s
	}
	h.mu.Unlock()
	return s
}
