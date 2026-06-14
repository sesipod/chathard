package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/stuckpacket/tailchat/db"
)

// MessagesHandler handles message CRUD endpoints.
type MessagesHandler struct {
	queries *db.Queries
}

// NewMessagesHandler creates a new MessagesHandler.
func NewMessagesHandler(queries *db.Queries) *MessagesHandler {
	return &MessagesHandler{queries: queries}
}

// ServeHTTP routes message sub-paths.
func (h *MessagesHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/api/messages", "/messages":
		switch r.Method {
		case http.MethodPost:
			h.sendMessage(w, r)
		case http.MethodGet:
			h.getMessages(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	case "/api/messages/read", "/messages/read":
		if r.Method == http.MethodPost {
			h.markRead(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	case "/api/messages/retention", "/messages/retention":
		if r.Method == http.MethodPatch {
			h.updateRetention(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	// GET /api/conversations
	case "/api/conversations", "/conversations":
		if r.Method == http.MethodGet {
			h.getConversations(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	default:
		http.Error(w, "Not found", http.StatusNotFound)
	}
}

type sendMessageReq struct {
	RecipientID      string `json:"recipient_id"`
	GroupID          string `json:"group_id"`
	Ciphertext       []byte `json:"ciphertext"`
	EphemeralPubKey  []byte `json:"ephemeral_public_key"`
	Nonce            []byte `json:"nonce"`
	ExpiresIn        string `json:"expires_in"` // e.g. "1h", "7d"
}

func (h *MessagesHandler) sendMessage(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(CtxKeyUserID).(string)

	var req sendMessageReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	if len(req.Ciphertext) == 0 || len(req.EphemeralPubKey) == 0 || len(req.Nonce) == 0 {
		http.Error(w, "Missing required fields", http.StatusBadRequest)
		return
	}
	if req.RecipientID == "" && req.GroupID == "" {
		http.Error(w, "Must specify recipient_id or group_id", http.StatusBadRequest)
		return
	}

	var expiresAt *time.Time
	if req.ExpiresIn != "" {
		d, err := time.ParseDuration(req.ExpiresIn)
		if err != nil {
			http.Error(w, "Invalid expires_in", http.StatusBadRequest)
			return
		}
		t := time.Now().Add(d)
		expiresAt = &t
	}

	id := uuid.New().String()
	if err := h.queries.InsertMessage(id, userID, req.RecipientID, req.GroupID, req.Ciphertext, req.EphemeralPubKey, req.Nonce, expiresAt); err != nil {
		http.Error(w, "Failed to store message", http.StatusInternalServerError)
		return
	}

	// Notify via WebSocket
	hub := GetHub()
	if hub != nil {
		hub.NotifyNewMessage(id, userID, req.RecipientID, req.GroupID)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{"id": id})
}

func (h *MessagesHandler) getMessages(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(CtxKeyUserID).(string)
	q := r.URL.Query()

	withID := q.Get("with")
	groupID := q.Get("group_id")
	limitStr := q.Get("limit")

	limit := 50
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
			if limit > 200 {
				limit = 200
			}
		}
	}

	var after, before *time.Time
	if a := q.Get("after"); a != "" {
		t, err := time.Parse(time.RFC3339, a)
		if err == nil {
			after = &t
		}
	}
	if b := q.Get("before"); b != "" {
		t, err := time.Parse(time.RFC3339, b)
		if err == nil {
			before = &t
		}
	}

	messages := make([]db.MessageRow, 0)
	var err error
	if groupID != "" {
		messages, err = h.queries.GetGroupMessages(groupID, after, before, limit)
	} else if withID != "" {
		messages, err = h.queries.GetDirectMessages(userID, withID, after, before, limit)
	} else {
		http.Error(w, "Specify ?with= or ?group_id=", http.StatusBadRequest)
		return
	}

	if err != nil {
		http.Error(w, "Failed to fetch messages", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"messages": messages,
	})
}

type markReadReq struct {
	ConversationWith string `json:"conversation_with"`
	GroupID          string `json:"group_id"`
	UpToMsgID        string `json:"up_to_msg_id"`
}

func (h *MessagesHandler) markRead(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(CtxKeyUserID).(string)

	var req markReadReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	if req.ConversationWith != "" {
		if err := h.queries.MarkConversationRead(userID, req.ConversationWith, req.UpToMsgID); err != nil {
			http.Error(w, "Failed to mark read", http.StatusInternalServerError)
			return
		}
	}

	// Notify via WebSocket
	hub := GetHub()
	if hub != nil {
		hub.NotifyReadReceipt(userID, req.ConversationWith, req.GroupID, req.UpToMsgID)
	}

	w.WriteHeader(http.StatusNoContent)
}

type updateRetentionReq struct {
	ConversationWith string `json:"conversation_with"`
	GroupID          string `json:"group_id"`
	ExpiresIn        string `json:"expires_in"` // "" to clear
}

func (h *MessagesHandler) updateRetention(w http.ResponseWriter, r *http.Request) {
	var req updateRetentionReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	userID := r.Context().Value(CtxKeyUserID).(string)

	var msgIDs []string
	var err error

	if req.ConversationWith != "" {
		msgIDs, err = h.queries.GetConversationMessageIDs(userID, req.ConversationWith)
	} else if req.GroupID != "" {
		msgIDs, err = h.queries.GetGroupMessageIDs(req.GroupID)
	} else {
		http.Error(w, "conversation_with or group_id required", http.StatusBadRequest)
		return
	}
	if err != nil {
		http.Error(w, "Failed to lookup messages", http.StatusInternalServerError)
		return
	}

	var expiresAt *time.Time
	if req.ExpiresIn != "" {
		d, err := time.ParseDuration(req.ExpiresIn)
		if err != nil {
			http.Error(w, "Invalid expires_in", http.StatusBadRequest)
			return
		}
		t := time.Now().Add(d)
		expiresAt = &t
	}

	if err := h.queries.UpdateRetention(msgIDs, expiresAt); err != nil {
		http.Error(w, "Failed to update retention", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *MessagesHandler) getConversations(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(CtxKeyUserID).(string)

	// 1:1 conversations
	convs, err := h.queries.GetConversations(userID)
	if err != nil {
		http.Error(w, "Failed to fetch conversations", http.StatusInternalServerError)
		return
	}

	// Groups the user is a member of
	groups, err := h.queries.GetUserGroupsWithActivity(userID)
	if err != nil {
		http.Error(w, "Failed to fetch groups", http.StatusInternalServerError)
		return
	}

	// Build unified response with type field
	result := make([]map[string]interface{}, 0, len(convs)+len(groups))

	for _, c := range convs {
		result = append(result, map[string]interface{}{
			"user_id":         c.UserID,
			"handle":          c.Handle,
			"last_message_at": c.LastMessage,
			"unread_count":    c.UnreadCount,
			"last_active":     c.LastActivity,
			"type":            "direct",
		})
	}

	for _, g := range groups {
		result = append(result, map[string]interface{}{
			"id":              g.ID,
			"name":            g.EncryptedName,
			"last_message_at": g.LastActive,
			"unread_count":    0,
			"last_active":     g.LastActive,
			"type":            "group",
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"conversations": result,
	})
}
