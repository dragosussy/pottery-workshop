package api

import (
	"encoding/json"
	"errors"
	"slices"
	"sync"
	"time"
)

// Stream limits (api-contract §4.2).
const (
	maxStreams     = 16
	clientBuffer   = 16
	changeDebounce = 500 * time.Millisecond
)

// ErrTooManyStreams is returned when maxStreams clients are connected.
var ErrTooManyStreams = errors.New("too many streams")

// Hub fans events out to SSE clients. Changed calls are merged and sent
// after changeDebounce. It is safe for concurrent use.
type Hub struct {
	mu      sync.Mutex
	clients map[*client]struct{}
	closed  bool

	scopes map[string]bool
	ids    map[int64]bool
	timer  *time.Timer
}

// NewHub returns an empty hub.
func NewHub() *Hub {
	return &Hub{clients: make(map[*client]struct{}), scopes: map[string]bool{}, ids: map[int64]bool{}}
}

type message struct {
	event     string
	data      []byte
	droppable bool // readings: a newer one supersedes it
}

type client struct {
	mu     sync.Mutex
	queue  []message
	wake   chan struct{}
	done   chan struct{}
	closed bool
}

func (c *client) close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.closed {
		c.closed = true
		close(c.done)
	}
}

// push queues m. A full queue drops its oldest readings message first; if
// there is none and m is a changed event, the client is disconnected
// instead (it reconnects and refetches).
func (c *client) push(m message) {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return
	}
	if len(c.queue) >= clientBuffer {
		i := slices.IndexFunc(c.queue, func(x message) bool { return x.droppable })
		switch {
		case i >= 0:
			c.queue = slices.Delete(c.queue, i, i+1)
		case m.droppable:
			c.mu.Unlock()
			return
		default:
			c.mu.Unlock()
			c.close()
			return
		}
	}
	c.queue = append(c.queue, m)
	c.mu.Unlock()
	select {
	case c.wake <- struct{}{}:
	default:
	}
}

// take returns and clears the queue.
func (c *client) take() []message {
	c.mu.Lock()
	defer c.mu.Unlock()
	q := c.queue
	c.queue = nil
	return q
}

func (h *Hub) register() (*client, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed || len(h.clients) >= maxStreams {
		return nil, ErrTooManyStreams
	}
	c := &client{wake: make(chan struct{}, 1), done: make(chan struct{})}
	h.clients[c] = struct{}{}
	return c, nil
}

func (h *Hub) unregister(c *client) {
	h.mu.Lock()
	delete(h.clients, c)
	h.mu.Unlock()
	c.close()
}

func (h *Hub) broadcast(m message) {
	h.mu.Lock()
	clients := make([]*client, 0, len(h.clients))
	for c := range h.clients {
		clients = append(clients, c)
	}
	h.mu.Unlock()
	for _, c := range clients {
		c.push(m)
	}
}

// Publish sends an event to every client. Droppable events (readings) may
// be superseded when a client falls behind.
func (h *Hub) Publish(event string, data any, droppable bool) {
	b, err := json.Marshal(data)
	if err != nil {
		return
	}
	h.broadcast(message{event: event, data: b, droppable: droppable})
}

// Changed records a change; a merged changed event goes out after the
// debounce interval.
func (h *Hub) Changed(scopes []string, pieceIDs []int64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, s := range scopes {
		h.scopes[s] = true
	}
	for _, id := range pieceIDs {
		h.ids[id] = true
	}
	if h.timer == nil && !h.closed {
		h.timer = time.AfterFunc(changeDebounce, h.flushChanged)
	}
}

func (h *Hub) flushChanged() {
	h.mu.Lock()
	scopes := make([]string, 0, len(h.scopes))
	for s := range h.scopes {
		scopes = append(scopes, s)
	}
	ids := make([]int64, 0, len(h.ids))
	for id := range h.ids {
		ids = append(ids, id)
	}
	clear(h.scopes)
	clear(h.ids)
	h.timer = nil
	h.mu.Unlock()
	slices.Sort(scopes)
	slices.Sort(ids)
	h.Publish("changed", map[string]any{"scopes": scopes, "piece_ids": ids}, false)
}

// Close disconnects every client and refuses new ones. It is registered
// with the server's RegisterOnShutdown so Shutdown can finish.
func (h *Hub) Close() {
	h.mu.Lock()
	h.closed = true
	if h.timer != nil {
		h.timer.Stop()
	}
	clients := h.clients
	h.clients = map[*client]struct{}{}
	h.mu.Unlock()
	for c := range clients {
		c.close()
	}
}
