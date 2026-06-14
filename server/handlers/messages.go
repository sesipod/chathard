package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/stuckpacket/tailchat/db"
)

// parseDuration extends time.ParseDuration with day ("d") support.
// Go's time.ParseDuration only handles ns, us/µs, ms, s, m, h.
// This converts "7d", "30d", "90d" etc. to the equivalent hour duration.
func parseDuration(s string) (time.Duration, error) {
	if len(s) < 2 {
		return time.ParseDuration(s)
	}
	if s[len(s)-1] == 'd' {
		hours := s[:len(s)-1] + "h"
		d, err := time.ParseDuration(hours)
		if err != nil {
			return 0, err
		}
		return d * 24, nil
	}
	return time.ParseDuration(s)
}

// formatDuration converts a Go duration to a shorthand string the frontend
// understands (e.g. "1h", "24h", "7d", "30d", "90d"). Rounds days down.
func formatDuration(d time.Duration) string {
	hours := int(d.Hours())
	if hours < 1 {
		return "<1h"
	}
	if hours < 24 {
		return fmt.Sprintf("%dh", hours)
	}
	days := hours / 24
	return fmt.Sprintf("%dd", days)
}

// retentionSQLModifier converts a retention string like "1h" to a SQLite
// datetime modifier like "-1 hours" for filtering messages on read.
func retentionSQLModifier(ret string) string {
	if len(ret) < 2 {
		return ""
	}
	suffix := map[byte]string{'h': "hours", 'd': "days"}[ret[len(ret)-1]]
	if suffix == "" {
		return ""
	}
	return "-" + ret[:len(ret)-1] + " " + suffix
}

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
	case "/api/messages/hide", "/messages/hide":
		if r.Method == http.MethodPost {
			h.hideMessage(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	case "/api/messages/batch-hide", "/messages/batch-hide":
		if r.Method == http.MethodPost {
			h.batchHideMessages(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
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
	RecipientID     string `json:"recipient_id"`
	GroupID         string `json:"group_id"`
	Ciphertext      []byte `json:"ciphertext"`
	EphemeralPubKey []byte `json:"ephemeral_public_key"`
	Nonce           []byte `json:"nonce"`
	ExpiresIn       string `json:"expires_in"` // e.g. "1h", "7d"
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

	// Per-message TTL: only set expires_at if the client explicitly sent expires_in
	var expiresAt *time.Time
	if req.ExpiresIn != "" {
		d, err := parseDuration(req.ExpiresIn)
		if err != nil {
			http.Error(w, "Invalid expires_in", http.StatusBadRequest)
			return
		}
		t := time.Now().Add(d)
		expiresAt = &t
	}
	// Per-user retention is NOT applied here — it's filtered on read via user_retention table

	id := uuid.New().String()
	if err := h.queries.InsertMessage(id, userID, req.RecipientID, req.GroupID, req.Ciphertext, req.EphemeralPubKey, req.Nonce, expiresAt); err != nil {
		http.Error(w, "Failed to store message", http.StatusInternalServerError)
		return
	}

	// Notify via WebSocket
	hub := GetHub()
	if hub != nil {
		var groupMemberIDs []string
		if req.GroupID != "" {
			groupMemberIDs, _ = h.queries.GetGroupMemberIDs(req.GroupID)
		}
		hub.NotifyNewMessage(id, userID, req.RecipientID, req.GroupID, groupMemberIDs)
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

	// Apply per-user retention filter
	var retentionMod string
	if groupID != "" {
		ret, err := h.queries.GetUserRetention(userID, groupID, "group")
		if err == nil && ret != "" {
			retentionMod = retentionSQLModifier(ret)
		}
	} else if withID != "" {
		ret, err := h.queries.GetUserRetention(userID, withID, "direct")
		if err == nil && ret != "" {
			retentionMod = retentionSQLModifier(ret)
		}
	}

	messages := make([]db.MessageRow, 0)
	var err error
	if groupID != "" {
		messages, err = h.queries.GetGroupMessagesWithRetention(userID, groupID, after, before, limit, retentionMod)
	} else if withID != "" {
		messages, err = h.queries.GetDirectMessagesWithRetention(userID, withID, after, before, limit, retentionMod)
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

	var targetID, targetType string
	if req.ConversationWith != "" {
		targetID = req.ConversationWith
		targetType = "direct"
	} else if req.GroupID != "" {
		targetID = req.GroupID
		targetType = "group"
	} else {
		http.Error(w, "conversation_with or group_id required", http.StatusBadRequest)
		return
	}

	// Validate format if not clearing
	if req.ExpiresIn != "" {
		if _, err := parseDuration(req.ExpiresIn); err != nil {
			http.Error(w, "Invalid expires_in", http.StatusBadRequest)
			return
		}
	}

	// Store per-user retention setting
	if err := h.queries.SetUserRetention(userID, targetID, targetType, req.ExpiresIn); err != nil {
		http.Error(w, "Failed to update retention", http.StatusInternalServerError)
		return
	}

	// Set/clear expires_at on user's existing messages in this conversation
	var msgIDs []string
	var err error
	if targetType == "direct" {
		msgIDs, err = h.queries.GetConversationMessageIDs(userID, targetID)
	} else {
		msgIDs, err = h.queries.GetUserGroupMessageIDs(userID, targetID)
	}
	if err != nil {
		http.Error(w, "Failed to get message IDs", http.StatusInternalServerError)
		return
	}
	if len(msgIDs) > 0 {
		if err := h.queries.UpdateRetention(msgIDs, req.ExpiresIn); err != nil {
			http.Error(w, "Failed to update message retention", http.StatusInternalServerError)
			return
		}
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
		// Get the user's OWN retention setting for this conversation
		expiresIn := "Never"
		ret, err := h.queries.GetUserRetention(userID, c.UserID, "direct")
		if err == nil && ret != "" {
			expiresIn = ret
		}
		result = append(result, map[string]interface{}{
			"user_id":         c.UserID,
			"handle":          c.Handle,
			"last_message_at": c.LastMessage,
			"unread_count":    c.UnreadCount,
			"last_active":     c.LastActivity,
			"expires_in":      expiresIn,
			"type":            "direct",
		})
	}

	for _, g := range groups {
		// Get the user's OWN retention setting for this group
		expiresIn := "Never"
		ret, err := h.queries.GetUserRetention(userID, g.ID, "group")
		if err == nil && ret != "" {
			expiresIn = ret
		}
		result = append(result, map[string]interface{}{
			"id":              g.ID,
			"name":            g.EncryptedName,
			"last_message_at": g.LastActive,
			"unread_count":    0,
			"last_active":     g.LastActive,
			"expires_in":      expiresIn,
			"type":            "group",
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"conversations": result,
	})
}

type hideMessageReq struct {
	MessageID string `json:"message_id"`
}

func (h *MessagesHandler) hideMessage(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(CtxKeyUserID).(string)

	var req hideMessageReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	if req.MessageID == "" {
		http.Error(w, "message_id is required", http.StatusBadRequest)
		return
	}

	if err := h.queries.HideMessage(userID, req.MessageID); err != nil {
		http.Error(w, "Failed to hide message", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

type batchHideMessageReq struct {
	MessageIDs []string `json:"message_ids"`
}

func (h *MessagesHandler) batchHideMessages(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(CtxKeyUserID).(string)

	var req batchHideMessageReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	if len(req.MessageIDs) == 0 {
		http.Error(w, "message_ids is required", http.StatusBadRequest)
		return
	}

	if err := h.queries.HideMessages(userID, req.MessageIDs); err != nil {
		http.Error(w, "Failed to hide messages", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
