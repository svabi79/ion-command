package stream

import (
	"sync"

	"github.com/ion-command/ion-command/collector/internal/telemetry"
)

type Client struct{ Messages chan []byte }

type Hub struct {
	mu                  sync.RWMutex
	clients             map[*Client]struct{}
	clientQueueCapacity int
	stats               *telemetry.Stats
}

func NewHub(clientQueueCapacity int, stats *telemetry.Stats) *Hub {
	return &Hub{clients: make(map[*Client]struct{}), clientQueueCapacity: clientQueueCapacity, stats: stats}
}

func (h *Hub) Register() *Client {
	client := &Client{Messages: make(chan []byte, h.clientQueueCapacity)}
	h.mu.Lock()
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

func (h *Hub) Publish(message []byte) {
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
