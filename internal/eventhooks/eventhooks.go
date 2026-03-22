package eventhooks

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/snapetech/iptvtunerr/internal/httpclient"
)

const recentLimit = 64

type Hook struct {
	Name    string            `json:"name"`
	URL     string            `json:"url"`
	Events  []string          `json:"events,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
	Timeout string            `json:"timeout,omitempty"`
}

type FileConfig struct {
	Webhooks []Hook `json:"webhooks"`
}

type Event struct {
	ID         string      `json:"id"`
	Name       string      `json:"name"`
	Source     string      `json:"source"`
	OccurredAt string      `json:"occurred_at"`
	Payload    interface{} `json:"payload,omitempty"`
}

type Delivery struct {
	EventName   string `json:"event_name"`
	EventID     string `json:"event_id"`
	HookName    string `json:"hook_name"`
	URL         string `json:"url"`
	Success     bool   `json:"success"`
	StatusCode  int    `json:"status_code,omitempty"`
	Error       string `json:"error,omitempty"`
	DurationMS  int64  `json:"duration_ms"`
	OccurredAt  string `json:"occurred_at"`
	DeliveredAt string `json:"delivered_at"`
}

type Report struct {
	Enabled      bool       `json:"enabled"`
	ConfigFile   string     `json:"config_file,omitempty"`
	Hooks        []Hook     `json:"hooks,omitempty"`
	Recent       []Delivery `json:"recent,omitempty"`
	RecentMax    int        `json:"recent_max"`
	TotalHooks   int        `json:"total_hooks"`
	LastDelivery string     `json:"last_delivery,omitempty"`
}

type Dispatcher struct {
	client     *http.Client
	configFile string
	hooks      []Hook
	mu         sync.Mutex
	recent     []Delivery
	seq        uint64
}

func Load(path string) (*Dispatcher, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg FileConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return nil, fmt.Errorf("parse event webhooks file: %w", err)
	}
	hooks := make([]Hook, 0, len(cfg.Webhooks))
	for _, hook := range cfg.Webhooks {
		hook.Name = strings.TrimSpace(hook.Name)
		hook.URL = strings.TrimSpace(hook.URL)
		if hook.URL == "" {
			continue
		}
		if hook.Name == "" {
			hook.Name = hook.URL
		}
		hooks = append(hooks, hook)
	}
	return &Dispatcher{
		client:     httpclient.Default(),
		configFile: path,
		hooks:      hooks,
	}, nil
}

func (d *Dispatcher) Enabled() bool {
	return d != nil && len(d.hooks) > 0
}

func (d *Dispatcher) Dispatch(name, source string, payload interface{}) {
	if d == nil || len(d.hooks) == 0 {
		return
	}
	d.mu.Lock()
	d.seq++
	eventID := fmt.Sprintf("evt-%06d", d.seq)
	d.mu.Unlock()
	event := Event{
		ID:         eventID,
		Name:       strings.TrimSpace(name),
		Source:     strings.TrimSpace(source),
		OccurredAt: time.Now().UTC().Format(time.RFC3339),
		Payload:    payload,
	}
	body, err := json.Marshal(event)
	if err != nil {
		d.appendRecent(Delivery{
			EventName:   event.Name,
			EventID:     event.ID,
			HookName:    "marshal",
			Error:       err.Error(),
			OccurredAt:  event.OccurredAt,
			DeliveredAt: time.Now().UTC().Format(time.RFC3339),
		})
		return
	}
	for _, hook := range d.hooks {
		if !hookMatches(hook, event.Name) {
			continue
		}
		go d.deliver(hook, event, body)
	}
}

func (d *Dispatcher) Report() Report {
	if d == nil {
		return Report{Enabled: false, RecentMax: recentLimit}
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	hooks := make([]Hook, len(d.hooks))
	copy(hooks, d.hooks)
	recent := make([]Delivery, len(d.recent))
	copy(recent, d.recent)
	report := Report{
		Enabled:    len(hooks) > 0,
		ConfigFile: d.configFile,
		Hooks:      hooks,
		Recent:     recent,
		RecentMax:  recentLimit,
		TotalHooks: len(hooks),
	}
	if len(recent) > 0 {
		report.LastDelivery = recent[0].DeliveredAt
	}
	return report
}

func (d *Dispatcher) deliver(hook Hook, event Event, body []byte) {
	start := time.Now()
	record := Delivery{
		EventName:  event.Name,
		EventID:    event.ID,
		HookName:   hook.Name,
		URL:        hook.URL,
		OccurredAt: event.OccurredAt,
	}
	client := d.client
	if timeout := parseHookTimeout(hook.Timeout); timeout > 0 {
		client = httpclient.WithTimeout(timeout)
	}
	req, err := http.NewRequest(http.MethodPost, hook.URL, bytes.NewReader(body))
	if err != nil {
		record.Error = err.Error()
		record.DeliveredAt = time.Now().UTC().Format(time.RFC3339)
		record.DurationMS = time.Since(start).Milliseconds()
		d.appendRecent(record)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-IPTVTunerr-Event", event.Name)
	req.Header.Set("X-IPTVTunerr-Event-ID", event.ID)
	for k, v := range hook.Headers {
		k = strings.TrimSpace(k)
		v = strings.TrimSpace(v)
		if k != "" && v != "" {
			req.Header.Set(k, v)
		}
	}
	resp, err := client.Do(req)
	if err != nil {
		record.Error = err.Error()
	} else {
		record.StatusCode = resp.StatusCode
		record.Success = resp.StatusCode >= 200 && resp.StatusCode < 300
		_ = resp.Body.Close()
		if !record.Success {
			record.Error = http.StatusText(resp.StatusCode)
		}
	}
	record.DeliveredAt = time.Now().UTC().Format(time.RFC3339)
	record.DurationMS = time.Since(start).Milliseconds()
	d.appendRecent(record)
}

func (d *Dispatcher) appendRecent(delivery Delivery) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.recent = append([]Delivery{delivery}, d.recent...)
	if len(d.recent) > recentLimit {
		d.recent = d.recent[:recentLimit]
	}
}

func hookMatches(h Hook, eventName string) bool {
	if len(h.Events) == 0 {
		return true
	}
	for _, candidate := range h.Events {
		candidate = strings.TrimSpace(candidate)
		switch candidate {
		case "", "*":
			return true
		case eventName:
			return true
		}
	}
	return false
}

func parseHookTimeout(raw string) time.Duration {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d <= 0 {
		return 0
	}
	return d
}
