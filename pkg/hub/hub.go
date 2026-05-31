package hub

import (
	"sync"

	"github.com/HanshalDabbiru/feature-flag-engine/pkg/domain"
)

// Hub maintains the registry of connected SSE clients and broadcasts flag updates to all of them.
type Hub struct {
	mu      sync.RWMutex
	clients map[uint64]chan domain.FeatureFlag
	nextID  uint64
}

// New creates a Hub with an initialized client registry.
func New() *Hub {
	return &Hub{clients: make(map[uint64]chan domain.FeatureFlag)}
}

// Register adds a new SSE client to the hub, returning its unique ID and the channel it should read from.
func (h *Hub) Register() (uint64, chan domain.FeatureFlag) {
	h.mu.Lock()
	defer h.mu.Unlock()
	ch := make(chan domain.FeatureFlag, 1)
	id := h.nextID
	h.clients[id] = ch
	h.nextID++
	return id, ch
}

// Unregister removes the client with the given ID from the hub and closes its channel.
func (h *Hub) Unregister(id uint64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	ch, ok := h.clients[id]
	if !ok {
		return
	}
	close(ch)
	delete(h.clients, id)
}

// Broadcast sends the given flag to every connected client's channel.
func (h *Hub) Broadcast(flag domain.FeatureFlag) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, ch := range h.clients {
		ch <- flag
	}
}
