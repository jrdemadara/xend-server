package realtime

import (
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"xend.chat/m/pkg/wsutil"
)

type clientConn struct {
	conn *websocket.Conn
	mu   sync.Mutex
}

type Hub struct {
	mu      sync.RWMutex
	clients map[string]map[string]*clientConn // userID -> deviceID -> conn
}

type Stats struct {
	Users       int `json:"users"`
	Connections int `json:"connections"`
}

type UserConnection struct {
	UserID    string   `json:"user_id"`
	DeviceIDs []string `json:"device_ids"`
}

type Details struct {
	Stats Stats            `json:"stats"`
	Users []UserConnection `json:"users"`
}

func NewHub() *Hub {
	return &Hub{clients: make(map[string]map[string]*clientConn)}
}

func (h *Hub) Add(userID, deviceID string, conn *websocket.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.clients[userID] == nil {
		h.clients[userID] = map[string]*clientConn{}
	}
	h.clients[userID][deviceID] = &clientConn{conn: conn}
}

func (h *Hub) Remove(userID, deviceID string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	devices := h.clients[userID]
	if devices == nil {
		return
	}
	if c := devices[deviceID]; c != nil {
		_ = c.conn.Close()
		delete(devices, deviceID)
	}
	if len(devices) == 0 {
		delete(h.clients, userID)
	}
}

func (h *Hub) SendToUser(userID string, event wsutil.Event) {
	h.mu.RLock()
	devices := h.clients[userID]
	// Copy so we can write outside read lock.
	conns := make([]*clientConn, 0, len(devices))
	for _, c := range devices {
		conns = append(conns, c)
	}
	h.mu.RUnlock()

	for _, c := range conns {
		c.mu.Lock()
		_ = c.conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
		_ = c.conn.WriteJSON(event)
		c.mu.Unlock()
	}
}

func (h *Hub) HasActiveUser(userID string) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients[userID]) > 0
}

func (h *Hub) Stats() Stats {
	h.mu.RLock()
	defer h.mu.RUnlock()

	stats := Stats{Users: len(h.clients)}
	for _, devices := range h.clients {
		stats.Connections += len(devices)
	}
	return stats
}

func (h *Hub) Details() Details {
	h.mu.RLock()
	defer h.mu.RUnlock()

	details := Details{
		Stats: Stats{Users: len(h.clients)},
		Users: make([]UserConnection, 0, len(h.clients)),
	}
	for userID, devices := range h.clients {
		item := UserConnection{
			UserID:    userID,
			DeviceIDs: make([]string, 0, len(devices)),
		}
		for deviceID := range devices {
			item.DeviceIDs = append(item.DeviceIDs, deviceID)
			details.Stats.Connections++
		}
		details.Users = append(details.Users, item)
	}
	return details
}
