package handlers

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"sync"
	"time"

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

// ── Connection hub ──
//
// Every socket gets its own bounded queue and writer goroutine, so a client
// that stops reading can only hurt itself: notifications to it are dropped and
// the connection is closed, while everyone else is unaffected.

const (
	writeWait       = 5 * time.Second
	pongWait        = 75 * time.Second
	pingEvery       = 30 * time.Second
	sendQueueSize   = 64
	maxConnsPerUser = 8
)

type wsClient struct {
	conn *websocket.Conn
	send chan []byte
	once sync.Once
}

func (c *wsClient) close() {
	c.once.Do(func() {
		close(c.send)
		c.conn.Close()
	})
}

// writePump is the only goroutine that writes to the socket.
func (c *wsClient) writePump() {
	ping := time.NewTicker(pingEvery)
	defer func() {
		ping.Stop()
		c.conn.Close()
	}()
	for {
		select {
		case msg, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				c.conn.WriteMessage(websocket.CloseMessage, nil)
				return
			}
			if err := c.conn.WriteMessage(websocket.TextMessage, msg); err != nil {
				return
			}
		case <-ping.C:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// Hub manages WebSocket connections per user.
type Hub struct {
	mu          sync.Mutex
	connections map[string]map[*wsClient]bool // userID -> set of clients
	queries     *db.Queries                   // group membership lookups
}

var globalHub *Hub

// InitHub creates the global WebSocket hub.
func InitHub(queries *db.Queries) {
	globalHub = &Hub{
		connections: make(map[string]map[*wsClient]bool),
		queries:     queries,
	}
}

// GetHub returns the global WebSocket hub.
func GetHub() *Hub {
	return globalHub
}

func (h *Hub) add(userID string, c *wsClient) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.connections[userID] == nil {
		h.connections[userID] = make(map[*wsClient]bool)
	}
	// Cap sockets per user: drop an arbitrary old one rather than grow without bound.
	for len(h.connections[userID]) >= maxConnsPerUser {
		for old := range h.connections[userID] {
			delete(h.connections[userID], old)
			go old.close()
			break
		}
	}
	h.connections[userID][c] = true
}

func (h *Hub) remove(userID string, c *wsClient) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if set, ok := h.connections[userID]; ok {
		delete(set, c)
		if len(set) == 0 {
			delete(h.connections, userID)
		}
	}
	c.close()
}

func (h *Hub) sendToUser(userID string, payload WSPayload) {
	msg, err := json.Marshal(payload)
	if err != nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	for c := range h.connections[userID] {
		select {
		case c.send <- msg:
		default: // client is not keeping up: drop it, never block the hub
			delete(h.connections[userID], c)
			go c.close()
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

// NotifyGroupChanged tells the given users that a group's roster or key changed
// so their clients refresh it (and the admin's client rotates the key).
func (h *Hub) NotifyGroupChanged(groupID string, userIDs ...string) {
	for _, id := range userIDs {
		h.sendToUser(id, WSPayload{Type: WSTypeGroupChange, GroupID: groupID, ConversationID: groupID})
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

// ── One-time connection tickets ──
//
// Browsers cannot set headers on a WebSocket, and putting the long-lived
// session token in the URL would leak it into proxy/access logs. Instead the
// client asks (with its Bearer token) for a ticket that works once, for a few
// seconds, only on /ws.

const ticketTTL = 30 * time.Second

type ticketStore struct {
	mu      sync.Mutex
	tickets map[string]ticketEntry
}

type ticketEntry struct {
	userID  string
	expires time.Time
}

var wsTickets = &ticketStore{tickets: map[string]ticketEntry{}}

// IssueWSTicket returns a single-use ticket for userID.
func IssueWSTicket(userID string) string {
	b := make([]byte, 24)
	rand.Read(b)
	t := hex.EncodeToString(b)
	wsTickets.mu.Lock()
	defer wsTickets.mu.Unlock()
	now := time.Now()
	for k, v := range wsTickets.tickets { // opportunistic cleanup keeps the map small
		if now.After(v.expires) {
			delete(wsTickets.tickets, k)
		}
	}
	if len(wsTickets.tickets) > 10000 {
		return ""
	}
	wsTickets.tickets[t] = ticketEntry{userID: userID, expires: now.Add(ticketTTL)}
	return t
}

// RedeemWSTicket consumes a ticket and returns its user, or "".
func RedeemWSTicket(t string) string {
	wsTickets.mu.Lock()
	defer wsTickets.mu.Unlock()
	e, ok := wsTickets.tickets[t]
	delete(wsTickets.tickets, t)
	if !ok || time.Now().After(e.expires) {
		return ""
	}
	return e.userID
}

// WSHandler handles GET /ws?ticket=…
type WSHandler struct{}

// NewWSHandler creates a new WSHandler.
func NewWSHandler() *WSHandler {
	return &WSHandler{}
}

func (h *WSHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	userID := RedeemWSTicket(r.URL.Query().Get("ticket"))
	if userID == "" {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	hub := GetHub()
	if hub == nil {
		conn.Close()
		return
	}

	client := &wsClient{conn: conn, send: make(chan []byte, sendQueueSize)}
	conn.SetReadLimit(4 << 10)
	conn.SetReadDeadline(time.Now().Add(pongWait))
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})
	hub.add(userID, client)
	go client.writePump()
	defer hub.remove(userID, client)

	for {
		_, msgBytes, err := conn.ReadMessage()
		if err != nil {
			return
		}
		conn.SetReadDeadline(time.Now().Add(pongWait))

		var payload WSPayload
		if err := json.Unmarshal(msgBytes, &payload); err != nil {
			continue
		}
		if payload.Type == WSTypeTyping {
			hub.NotifyTyping(userID, payload.ConversationID, payload.IsGroup, payload.IsTyping != nil && *payload.IsTyping)
		}
		// Read receipts are only sent by the REST endpoint, never accepted
		// from a socket, so a client cannot forge them for other people.
	}
}
