// internal/realtime/hub.go
package realtime

import (
	"encoding/json"
	"log"
	"sync"

	"github.com/google/uuid"
)

type Client struct {
	ID     string
	UserID uuid.UUID
	Conn   *WebSocketConn
	Send   chan []byte
}

type Hub struct {
	clients    map[string]*Client
	broadcast  chan []byte
	register   chan *Client
	unregister chan *Client
	mu         sync.RWMutex
}

func NewHub() *Hub {
	return &Hub{
		clients:    make(map[string]*Client),
		broadcast:  make(chan []byte, 256),
		register:   make(chan *Client),
		unregister: make(chan *Client),
	}
}

func (h *Hub) RegisterClient(client *Client) {
	h.register <- client
}

func (h *Hub) UnregisterClient(client *Client) {
	h.unregister <- client
}

// BroadcastJSON: helper kalau kamu mau broadcast pakai struct/map
func (h *Hub) BroadcastJSON(v interface{}) {
	b, err := json.Marshal(v)
	if err != nil {
		log.Printf("Error marshaling broadcast payload: %v", err)
		return
	}
	h.broadcast <- b
}

// SendToUser sends message to specific user
func (h *Hub) SendToUser(userID uuid.UUID, data interface{}) {
	payload, err := json.Marshal(data)
	if err != nil {
		log.Printf("Error marshaling message: %v", err)
		return
	}

	h.mu.RLock()
	defer h.mu.RUnlock()

	for _, client := range h.clients {
		if client.UserID == userID {
			select {
			case client.Send <- payload:
			default:
				// kalau penuh, skip (jangan block)
			}
		}
	}
}

// SendToConversation sends message to both participants
func (h *Hub) SendToConversation(clientID, freelancerID uuid.UUID, data interface{}) {
	h.SendToUser(clientID, data)
	h.SendToUser(freelancerID, data)
	log.Printf("Message broadcasted to conversation: %s <-> %s", clientID, freelancerID)
}

func (h *Hub) Run() {
	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			h.clients[client.ID] = client
			h.mu.Unlock()
			log.Printf("Client registered: %s (UserID: %s)", client.ID, client.UserID)

		case client := <-h.unregister:
			h.mu.Lock()
			if old, ok := h.clients[client.ID]; ok {
				delete(h.clients, client.ID)
				close(old.Send)
				log.Printf("Client unregistered: %s", client.ID)
			}
			h.mu.Unlock()

		case message := <-h.broadcast:
			// ini harus LOCK (karena bisa delete)
			h.mu.Lock()
			for id, client := range h.clients {
				select {
				case client.Send <- message:
				default:
					close(client.Send)
					delete(h.clients, id)
				}
			}
			h.mu.Unlock()
		}
	}
}
