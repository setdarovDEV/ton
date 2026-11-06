package http

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/coder/websocket"

	"tonnet/internal/domain"
)

// Hub fans domain events out to WebSocket clients. Events come from the
// indexer process through Postgres LISTEN/NOTIFY, so the API stays stateless
// and horizontally scalable.
type Hub struct {
	mu      sync.RWMutex
	clients map[*websocket.Conn]struct{}
	origins []string
	log     *slog.Logger
}

// NewHub creates a hub accepting the given origins.
func NewHub(origins []string, log *slog.Logger) *Hub {
	return &Hub{clients: map[*websocket.Conn]struct{}{}, origins: origins, log: log.With("component", "ws")}
}

// Broadcast sends the event to every connected client.
func (h *Hub) Broadcast(e domain.Event) {
	payload, err := json.Marshal(e)
	if err != nil {
		return
	}
	h.mu.RLock()
	conns := make([]*websocket.Conn, 0, len(h.clients))
	for c := range h.clients {
		conns = append(conns, c)
	}
	h.mu.RUnlock()
	for _, c := range conns {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		if err := c.Write(ctx, websocket.MessageText, payload); err != nil {
			h.remove(c)
			_ = c.Close(websocket.StatusGoingAway, "write failed")
		}
		cancel()
	}
}

// ClientCount returns the number of connected clients.
func (h *Hub) ClientCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}

func (h *Hub) remove(c *websocket.Conn) {
	h.mu.Lock()
	delete(h.clients, c)
	h.mu.Unlock()
}

// ServeHTTP upgrades the connection and keeps it until the client leaves.
func (h *Hub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	c, err := websocket.Accept(w, r, &websocket.AcceptOptions{OriginPatterns: h.origins})
	if err != nil {
		h.log.Debug("ws accept failed", "err", err)
		return
	}
	h.mu.Lock()
	h.clients[c] = struct{}{}
	h.mu.Unlock()
	defer h.remove(c)

	ctx := r.Context()
	_ = c.Write(ctx, websocket.MessageText, []byte(`{"type":"hello","status":"connected"}`))
	for {
		// Clients only listen; reading drains pings and detects disconnects.
		if _, _, err := c.Read(ctx); err != nil {
			_ = c.Close(websocket.StatusNormalClosure, "")
			return
		}
	}
}
