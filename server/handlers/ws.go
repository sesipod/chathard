package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/stuckpacket/tailchat/db"
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
	WSTypeGroupChange = "group_changed"
)

// WSPayload is a generic WebSocket message.
type WSPayload struct {
	Type        string `json:"type"`
	MsgID       string `json:"msg_id,omitempty"`
	SenderID    string `json:"sender_id,omitempty"`
	RecipientID string `json:"recipient_id,omitempty"`
	GroupID     string `json:"group_id,omitempty"`
	UserID      string `json:"user_id,omitempty"`
	// ConversationID is what the RECEIVER should open: the sender's id for a
	// 1:1 chat, the group id for a group. IsGroup is set by the sending client.
	ConversationID string `json:"conversation_id,omitempty"`
	IsGroup        bool   `json:"is_group,omitempty"`
	IsTyping       *bool  `json:"is_typing,omitempty"`
	FromUserID     string `json:"from_user_id,omitempty"`
	UpToMsgID      string `json:"up_to_msg_id,omitempty"`
}

var allowedOrigins = map[string]bool{}

// SetAllowedOrigins sets the Origins allowed to open a WebSocket. Call once at
// startup, before serving.
func SetAllowedOrigins(origins map[string]bool) {
	allowedOrigins = origins
}

// The VPN gate has already checked the client IP; checking Origin as well
// stops another page in a VPN user's browser from hijacking their socket.
var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return allowedOrigins[r.Header.Get("Origin")]
	},
}

// Hub manages WebSocket connections per user.
type Hub struct {
	mu          sync.RWMutex
	connections map[string]map[*websocket.Conn]bool // userID -> set of conns
	queries     *db.Queries                         // group membership lookups
}

var globalHub *Hub

// InitHub creates the global WebSocket hub.
func InitHub(queries *db.Queries) {
	globalHub = &Hub{
		connections: make(map[string]map[*websocket.Conn]bool),
		queries:     queries,
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

// NotifyNewMessage tells everyone who can see a message to refresh the right
// conversation: the recipient (or group members) AND the sender's own other
// devices. The payload carries ids only, never content.
func (h *Hub) NotifyNewMessage(msgID, senderID, recipientID, groupID string, groupMemberIDs []string) {
	base := WSPayload{Type: WSTypeNewMessage, MsgID: msgID, SenderID: senderID, GroupID: groupID}

	if recipientID != "" {
		toRecipient := base
		toRecipient.ConversationID = senderID
		h.sendToUser(recipientID, toRecipient)

		toSender := base
		toSender.ConversationID = recipientID
		h.sendToUser(senderID, toSender)
		return
	}
	for _, memberID := range groupMemberIDs {
		p := base
		p.ConversationID = groupID
		h.sendToUser(memberID, p)
	}
}

// NotifyTyping tells the other side(s) of ONE conversation that userID is
// typing. 1:1 goes to the named recipient only; groups go to the other
// members, and only if the sender is a member.
func (h *Hub) NotifyTyping(userID, conversationID string, isGroup, isTyping bool) {
	if conversationID == "" {
		return
	}
	payload := WSPayload{Type: WSTypeTyping, UserID: userID, IsTyping: &isTyping}
	if !isGroup {
		payload.ConversationID = userID
		h.sendToUser(conversationID, payload)
		return
	}
	if ok, err := h.queries.IsGroupMember(conversationID, userID); err != nil || !ok {
		return
	}
	payload.ConversationID = conversationID
	payload.GroupID = conversationID
	h.sendToGroup(conversationID, userID, payload)
}

// NotifyReadReceipt tells the other side(s) that fromUserID read a conversation.
func (h *Hub) NotifyReadReceipt(fromUserID, conversationWith, groupID, upToMsgID string) {
	payload := WSPayload{
		Type:       WSTypeReadReceipt,
		FromUserID: fromUserID,
		UpToMsgID:  upToMsgID,
	}
	if conversationWith != "" {
		payload.ConversationID = fromUserID
		h.sendToUser(conversationWith, payload)
	}
	if groupID != "" {
		if ok, err := h.queries.IsGroupMember(groupID, fromUserID); err != nil || !ok {
			return
		}
		payload.ConversationID = groupID
		payload.GroupID = groupID
		h.sendToGroup(groupID, fromUserID, payload)
	}
}

// NotifyGroupChanged tells the given users that a group's roster or key changed
// so their clients refresh it (and the admin's client rotates the key).
func (h *Hub) NotifyGroupChanged(groupID string, userIDs ...string) {
	for _, id := range userIDs {
		h.sendToUser(id, WSPayload{Type: WSTypeGroupChange, GroupID: groupID, ConversationID: groupID})
	}
}

func (h *Hub) sendToGroup(groupID, exceptUserID string, payload WSPayload) {
	ids, err := h.queries.GetGroupMemberIDs(groupID)
	if err != nil {
		return
	}
	for _, id := range ids {
		if id != exceptUserID {
			h.sendToUser(id, payload)
		}
	}
}

func (h *Hub) sendToUser(userID string, payload WSPayload) {
	h.mu.Lock()
	defer h.mu.Unlock()
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

	conn.SetReadLimit(4 << 10)
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
			hub.NotifyTyping(userID, payload.ConversationID, payload.IsGroup, payload.IsTyping != nil && *payload.IsTyping)
		}
		// Read receipts are only sent by the REST endpoint, never accepted
		// from a socket, so a client cannot forge them for other people.
	}

	log.Printf("ws: user %s disconnected", userID)
}
