package gateway

import (
	"sync"
)

type Hub struct {
	clients           map[string]*Client // key: client ID
	clientsByUsername map[string]*Client // key: username
	mu                sync.RWMutex
	register          chan *Client
	unregister        chan *Client
}

func NewHub() *Hub {
	return &Hub{
		clients:           make(map[string]*Client),
		clientsByUsername: make(map[string]*Client),
		register:          make(chan *Client),
		unregister:        make(chan *Client),
	}
}

func (h *Hub) Run() {
	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			h.clients[client.ID] = client
			if client.Username != "" {
				h.clientsByUsername[client.Username] = client
			}
			h.mu.Unlock()
		case client := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[client.ID]; ok {
				delete(h.clients, client.ID)
				if client.Username != "" && h.clientsByUsername[client.Username] == client {
					delete(h.clientsByUsername, client.Username)
				}
				close(client.send)
			}
			h.mu.Unlock()
		}
	}
}

func (h *Hub) Register(client *Client) {
	h.register <- client
}

func (h *Hub) Unregister(client *Client) {
	h.unregister <- client
}

func (h *Hub) GetClient(id string) (*Client, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	client, ok := h.clients[id]
	return client, ok
}

func (h *Hub) GetClientByUsername(username string) (*Client, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	client, ok := h.clientsByUsername[username]
	return client, ok
}

func (h *Hub) ClientCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}
