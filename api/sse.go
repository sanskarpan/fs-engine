package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	fstype "github.com/yourname/fs-engine/internal/fs"
)

// SSEEvent is an event to be sent over SSE.
type SSEEvent struct {
	Version int                    `json:"version"`
	Type    string                 `json:"type"`
	Source  string                 `json:"source"`
	Path    string                 `json:"path,omitempty"`
	Data    map[string]interface{} `json:"data,omitempty"`
	TS      string                 `json:"ts"`
}

// SSEBus is a channel-based pub/sub for SSE.
type SSEBus struct {
	mu      sync.RWMutex
	clients map[chan SSEEvent]struct{}
	source  <-chan fstype.Event
	done    chan struct{}
}

// NewSSEBus creates a new SSEBus that subscribes to the filesystem event channel.
func NewSSEBus(source <-chan fstype.Event) *SSEBus {
	bus := &SSEBus{
		clients: make(map[chan SSEEvent]struct{}),
		source:  source,
		done:    make(chan struct{}),
	}
	go bus.pump()
	return bus
}

// pump reads from the filesystem event source and fans out to all clients.
func (b *SSEBus) pump() {
	for {
		select {
		case evt, ok := <-b.source:
			if !ok {
				return
			}
			b.broadcast(SSEEvent{
				Version: evt.Version,
				Type:    evt.Type,
				Source:  evt.Source,
				Path:    evt.Path,
				Data:    evt.Data,
				TS:      evt.TS.Format(time.RFC3339Nano),
			})
		case <-b.done:
			return
		}
	}
}

// broadcast sends an event to all registered clients.
func (b *SSEBus) broadcast(evt SSEEvent) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	for ch := range b.clients {
		select {
		case ch <- evt:
		default:
			// Client too slow; drop event
		}
	}
}

// Subscribe registers a new client channel.
func (b *SSEBus) Subscribe() chan SSEEvent {
	ch := make(chan SSEEvent, 32)
	b.mu.Lock()
	b.clients[ch] = struct{}{}
	b.mu.Unlock()
	return ch
}

// Unsubscribe removes a client channel.
func (b *SSEBus) Unsubscribe(ch chan SSEEvent) {
	b.mu.Lock()
	delete(b.clients, ch)
	b.mu.Unlock()
	close(ch)
}

// Close stops the bus.
func (b *SSEBus) Close() {
	close(b.done)
}

// ServeHTTP implements the SSE HTTP handler.
func (b *SSEBus) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	if origin := r.Header.Get("Origin"); origin != "" && allowedOrigin(origin, r) {
		w.Header().Set("Access-Control-Allow-Origin", origin)
	}

	ch := b.Subscribe()
	defer b.Unsubscribe(ch)

	// Send a connected event
	_, _ = fmt.Fprintf(w, "data: {\"version\":%d,\"type\":\"connected\",\"source\":\"sse\",\"ts\":\"%s\"}\n\n", fstype.EventSchemaVersion, time.Now().Format(time.RFC3339Nano))
	flusher.Flush()

	for {
		select {
		case evt, ok := <-ch:
			if !ok {
				return
			}
			data, err := json.Marshal(evt)
			if err != nil {
				continue
			}
			_, _ = fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		case <-r.Context().Done():
			return
		}
	}
}
