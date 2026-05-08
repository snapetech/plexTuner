package webui

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// GET /api/v2/events  — Server-Sent Events stream.
// Sends a heartbeat every 15 s and broadcasts any events pushed via s.broadcast.
func (s *Server) v2Events(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming unsupported")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	send := func(event string, data any) {
		b, _ := json.Marshal(data)
		_, _ = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, b)
		flusher.Flush()
	}

	// Initial connected event.
	send("connected", map[string]string{"at": time.Now().UTC().Format(time.RFC3339)})

	tick := time.NewTicker(15 * time.Second)
	defer tick.Stop()

	// Subscribe to broadcast channel.
	ch := s.subscribe()
	defer s.unsubscribe(ch)

	for {
		select {
		case <-r.Context().Done():
			return
		case <-tick.C:
			send("heartbeat", map[string]string{"at": time.Now().UTC().Format(time.RFC3339)})
		case evt, ok := <-ch:
			if !ok {
				return
			}
			send(evt.Type, evt.Payload)
		}
	}
}

// BroadcastEvent pushes an SSE event to all connected clients.
func (s *Server) BroadcastEvent(eventType string, payload any) {
	s.broadcastMu.Lock()
	defer s.broadcastMu.Unlock()
	ev := sseEvent{Type: eventType, Payload: payload}
	for _, ch := range s.subscribers {
		select {
		case ch <- ev:
		default: // drop if subscriber is slow
		}
	}
}

func (s *Server) subscribe() chan sseEvent {
	ch := make(chan sseEvent, 32)
	s.broadcastMu.Lock()
	s.subscribers = append(s.subscribers, ch)
	s.broadcastMu.Unlock()
	return ch
}

func (s *Server) unsubscribe(ch chan sseEvent) {
	s.broadcastMu.Lock()
	defer s.broadcastMu.Unlock()
	out := s.subscribers[:0]
	for _, c := range s.subscribers {
		if c != ch {
			out = append(out, c)
		}
	}
	s.subscribers = out
}
