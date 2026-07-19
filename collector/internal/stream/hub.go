package stream

import (
	"sync"

	"github.com/ion-command/ion-command/collector/internal/telemetry"
)

type Client struct{ Messages chan []byte }

const maxRetainedMessages = 4096

type Hub struct {
	mu                  sync.RWMutex
	clients             map[*Client]struct{}
	clientQueueCapacity int
	stats               *telemetry.Stats
	// retained holds the latest message per retain key (semantic type plus
	// entity), replayed to new clients so state-like values are immediately
	// present instead of blank until the next live sample.
	retained map[string][]byte
}

func NewHub(clientQueueCapacity int, stats *telemetry.Stats) *Hub {
	return &Hub{clients: make(map[*Client]struct{}), clientQueueCapacity: clientQueueCapacity, stats: stats, retained: make(map[string][]byte)}
}

func (h *Hub) Register() *Client {
	client := &Client{Messages: make(chan []byte, h.clientQueueCapacity)}
	h.mu.Lock()
	for _, message := range h.retained {
		select {
		case client.Messages <- append([]byte(nil), message...):
		default:
		}
	}
	h.clients[client] = struct{}{}
	h.mu.Unlock()
	h.stats.ClientConnected()
	return client
}

func (h *Hub) Unregister(client *Client) {
	h.mu.Lock()
	if _, exists := h.clients[client]; exists {
		delete(h.clients, client)
		close(client.Messages)
		h.stats.ClientDisconnected()
	}
	h.mu.Unlock()
}

// Publish fans the message out to all live clients. A non-empty retainKey
// additionally stores it as the latest state for that key (bounded; new keys
// are ignored once the cap is reached rather than evicting arbitrary state).
func (h *Hub) Publish(message []byte, retainKey string) {
	if retainKey != "" {
		h.mu.Lock()
		if _, exists := h.retained[retainKey]; exists || len(h.retained) < maxRetainedMessages {
			h.retained[retainKey] = append([]byte(nil), message...)
		}
		h.mu.Unlock()
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	var dropped uint64
	for client := range h.clients {
		copyOfMessage := append([]byte(nil), message...)
		select {
		case client.Messages <- copyOfMessage:
		default:
			dropped++
		}
	}
	if dropped > 0 {
		h.stats.AddDroppedClients(dropped)
	}
}
