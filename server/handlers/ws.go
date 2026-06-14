package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
)

// CtxKey is a context key type.
type CtxKey string

// CtxKeyUserID is the context key for the authenticated user's ID.
const CtxKeyUserID CtxKey = "user_id"

// WebSocket message types.
const (
	WSTypeNewMessage  = "new_message"
	WSTypeTyping      = "typing"
	WSTypeReadReceipt = "read_receipt"
)

// WSPayload is a generic WebSocket message.
type WSPayload struct {
	Type        string `json:"type"`
	MsgID       string `json:"msg_id,omitempty"`
	SenderID    string `json:"sender_id,omitempty"`
	RecipientID string `json:"recipient_id,omitempty"`
	GroupID     string `json:"group_id,omitempty"`
	UserID      string `json:"user_id,omitempty"`
	IsTyping    *bool  `json:"is_typing,omitempty"`
	FromUserID  string `json:"from_user_id,omitempty"`
	UpToMsgID   string `json:"up_to_msg_id,omitempty"`
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // Verified by Tailscale auth middleware
	},
}

// Hub manages WebSocket connections per user.
type Hub struct {
	mu          sync.RWMutex
	connections map[string]map[*websocket.Conn]bool // userID -> set of conns
}

var globalHub *Hub

// InitHub creates the global WebSocket hub.
func InitHub() {
	globalHub = &Hub{
		connections: make(map[string]map[*websocket.Conn]bool),
	}
}

// GetHub returns the global WebSocket hub.
func GetHub() *Hub {
	return globalHub
}

// AddConnection registers a WebSocket connection for a user.
func (h *Hub) AddConnection(userID string, conn *websocket.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.connections[userID] == nil {
		h.connections[userID] = make(map[*websocket.Conn]bool)
	}
	h.connections[userID][conn] = true
}

// RemoveConnection removes a WebSocket connection.
func (h *Hub) RemoveConnection(userID string, conn *websocket.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if conns, ok := h.connections[userID]; ok {
		delete(conns, conn)
		if len(conns) == 0 {
			delete(h.connections, userID)
		}
	}
}

// NotifyNewMessage sends a new_message notification to relevant users.
func (h *Hub) NotifyNewMessage(msgID, senderID, recipientID, groupID string, groupMemberIDs []string) {
	payload := WSPayload{
		Type:     WSTypeNewMessage,
		MsgID:    msgID,
		SenderID: senderID,
		GroupID:  groupID,
	}

	// Notify recipient for 1:1
	if recipientID != "" {
		h.sendToUser(recipientID, payload)
	}
	// Notify all group members (except sender)
	for _, memberID := range groupMemberIDs {
		if memberID != senderID {
			h.sendToUser(memberID, payload)
		}
	}
}

// NotifyTyping sends a typing indicator.
func (h *Hub) NotifyTyping(userID, groupID string, isTyping bool) {
	payload := WSPayload{
		Type:     WSTypeTyping,
		UserID:   userID,
		GroupID:  groupID,
		IsTyping: &isTyping,
	}
	// For 1:1, hub doesn't know the other party — this is sent to all
	// group members. The client-side handles filtering.
	h.broadcast(payload)
}

// NotifyReadReceipt sends a read receipt.
func (h *Hub) NotifyReadReceipt(fromUserID, conversationWith, groupID, upToMsgID string) {
	payload := WSPayload{
		Type:        WSTypeReadReceipt,
		FromUserID:  fromUserID,
		RecipientID: conversationWith,
		GroupID:     groupID,
		UpToMsgID:   upToMsgID,
	}
	if conversationWith != "" {
		h.sendToUser(conversationWith, payload)
	}
	if groupID != "" {
		h.broadcast(payload)
	}
}

func (h *Hub) sendToUser(userID string, payload WSPayload) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	if conns, ok := h.connections[userID]; ok {
		for conn := range conns {
			if err := conn.WriteJSON(payload); err != nil {
				log.Printf("ws: write to %s: %v", userID, err)
				conn.Close()
				delete(conns, conn)
			}
		}
	}
}

func (h *Hub) broadcast(payload WSPayload) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, conns := range h.connections {
		for conn := range conns {
			if err := conn.WriteJSON(payload); err != nil {
				conn.Close()
			}
		}
	}
}

// WSHandler handles GET /ws.
type WSHandler struct{}

// NewWSHandler creates a new WSHandler.
func NewWSHandler() *WSHandler {
	return &WSHandler{}
}

func (h *WSHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(CtxKeyUserID).(string)
	if !ok || userID == "" {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("ws: upgrade: %v", err)
		return
	}

	hub := GetHub()
	if hub == nil {
		conn.Close()
		return
	}

	hub.AddConnection(userID, conn)
	defer hub.RemoveConnection(userID, conn)

	log.Printf("ws: user %s connected", userID)

	// Read loop — handle incoming messages (typing indicators, etc.)
	for {
		_, msgBytes, err := conn.ReadMessage()
		if err != nil {
			break
		}

		var payload WSPayload
		if err := json.Unmarshal(msgBytes, &payload); err != nil {
			continue
		}

		switch payload.Type {
		case WSTypeTyping:
			hub.NotifyTyping(userID, payload.GroupID, payload.IsTyping != nil && *payload.IsTyping)
		case WSTypeReadReceipt:
			hub.NotifyReadReceipt(userID, payload.RecipientID, payload.GroupID, payload.UpToMsgID)
		}
	}

	log.Printf("ws: user %s disconnected", userID)
}
