package handlers

import (
	"encoding/json"
	"fmt"
	"log"
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

// Wire format of `ciphertext` (built in web/src/lib/crypto/messages.js):
//
//	1:1   : 0x02 | wrapForRecipient(80) | wrapForSender(80) | AES-GCM(body)
//	group : 0x03 | epoch(4, big endian)                      | AES-GCM(body)
//
// The server cannot read any of it; it only checks the shape so garbage and
// stale-key messages are rejected early.
const (
	msgVersionDirect = 0x02
	msgVersionGroup  = 0x03
	directHeaderLen  = 1 + 80 + 80
	groupHeaderLen   = 1 + 4
	gcmTagLen        = 16
	maxCiphertext    = 256 << 10
)

type sendMessageReq struct {
	RecipientID string `json:"recipient_id"`
	GroupID     string `json:"group_id"`
	Ciphertext  []byte `json:"ciphertext"`
	Nonce       []byte `json:"nonce"`
	ExpiresIn   string `json:"expires_in"` // e.g. "1h", "7d"
}

func (h *MessagesHandler) sendMessage(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(CtxKeyUserID).(string)

	var req sendMessageReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	if len(req.Nonce) != 12 || len(req.Ciphertext) == 0 || len(req.Ciphertext) > maxCiphertext {
		http.Error(w, "Missing or malformed message", http.StatusBadRequest)
		return
	}
	if (req.RecipientID == "") == (req.GroupID == "") {
		http.Error(w, "Specify exactly one of recipient_id or group_id", http.StatusBadRequest)
		return
	}
	if req.GroupID != "" {
		g, err := h.queries.GetGroup(req.GroupID)
		if err != nil {
			http.Error(w, "Not found", http.StatusNotFound)
			return
		}
		if ok, _ := h.queries.IsGroupMember(req.GroupID, userID); !ok {
			http.Error(w, "Not found", http.StatusNotFound)
			return
		}
		ct := req.Ciphertext
		if len(ct) < groupHeaderLen+gcmTagLen || ct[0] != msgVersionGroup {
			http.Error(w, "Malformed group message", http.StatusBadRequest)
			return
		}
		epoch := int(ct[1])<<24 | int(ct[2])<<16 | int(ct[3])<<8 | int(ct[4])
		if epoch != g.Epoch {
			// The group key rotated since the sender last synced; client refreshes and retries.
			http.Error(w, "Stale group key epoch", http.StatusConflict)
			return
		}
	} else {
		if len(req.Ciphertext) < directHeaderLen+gcmTagLen || req.Ciphertext[0] != msgVersionDirect {
			http.Error(w, "Malformed message", http.StatusBadRequest)
			return
		}
		if users, err := h.queries.GetUsersByIDs([]string{req.RecipientID}); err != nil || len(users) != 1 {
			http.Error(w, "Unknown recipient", http.StatusBadRequest)
			return
		}
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
	if err := h.queries.InsertMessage(id, userID, req.RecipientID, req.GroupID, req.Ciphertext, req.Nonce, expiresAt); err != nil {
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
	if groupID != "" {
		if ok, _ := h.queries.IsGroupMember(groupID, userID); !ok {
			http.Error(w, "Not found", http.StatusNotFound)
			return
		}
	}

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

	if targetType == "group" {
		if ok, _ := h.queries.IsGroupMember(targetID, userID); !ok {
			http.Error(w, "Not found", http.StatusNotFound)
			return
		}
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
		if req.ExpiresIn != "" {
			// When setting a retention duration, permanently hide messages
			// that are already past the new cutoff — they should never re-appear.
			d, _ := parseDuration(req.ExpiresIn)
			cutoff := time.Now().UTC().Add(-d)
			expiredIDs, err := h.queries.GetExpiredMessageIDs(msgIDs, cutoff)
			if err == nil && len(expiredIDs) > 0 {
				h.queries.HideMessages(userID, expiredIDs)
			}
			// Set expires_at on remaining messages for future cleanup
			if len(expiredIDs) < len(msgIDs) {
				activeIDs := subtractSlice(msgIDs, expiredIDs)
				if err := h.queries.UpdateRetention(activeIDs, req.ExpiresIn); err != nil {
					http.Error(w, "Failed to update message retention", http.StatusInternalServerError)
					return
				}
			}
		} else {
			// Clearing retention — just remove expires_at
			if err := h.queries.UpdateRetention(msgIDs, req.ExpiresIn); err != nil {
				http.Error(w, "Failed to update message retention", http.StatusInternalServerError)
				return
			}
		}
	}

	w.WriteHeader(http.StatusNoContent)
}

// CheckAndDeleteMutuallyHidden checks if both participants in a 1:1 conversation
// have hidden the given message. If so, physically deletes it from the DB.
func CheckAndDeleteMutuallyHidden(queries *db.Queries, messageID string) {
	_, recipientID, isGroup, err := queries.GetMessageParticipants(messageID)
	if err != nil || isGroup || recipientID == "" {
		return // not a 1:1 message, or error — skip
	}

	count, err := queries.CountMessageDeletions(messageID)
	if err != nil || count < 2 {
		return // not both participants have hidden it
	}

	// Both participants have hidden this 1:1 message — permanently delete
	if err := queries.PermanentlyDeleteMutuallyHiddenMessage(messageID); err != nil {
		log.Printf("mutual delete: failed to delete message %s: %v", messageID, err)
	} else {
		log.Printf("mutual delete: permanently deleted message %s (hidden by both participants)", messageID)
	}
}

// subtractSlice returns elements in a that are not in b.
func subtractSlice(a, b []string) []string {
	bSet := make(map[string]bool, len(b))
	for _, id := range b {
		bSet[id] = true
	}
	var out []string
	for _, id := range a {
		if !bSet[id] {
			out = append(out, id)
		}
	}
	return out
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
			"encrypted_name":  g.EncryptedName,
			"created_by":      g.CreatedBy,
			"epoch":           g.Epoch,
			"needs_rotation":  g.NeedsRotation,
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
	if ok, _ := h.queries.CanAccessMessage(userID, req.MessageID); !ok {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	if err := h.queries.HideMessage(userID, req.MessageID); err != nil {
		http.Error(w, "Failed to hide message", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)

	// Check if both participants have now hidden this 1:1 message
	go CheckAndDeleteMutuallyHidden(h.queries, req.MessageID)
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

	if len(req.MessageIDs) == 0 || len(req.MessageIDs) > 500 {
		http.Error(w, "message_ids is required (max 500)", http.StatusBadRequest)
		return
	}
	for _, id := range req.MessageIDs {
		if ok, _ := h.queries.CanAccessMessage(userID, id); !ok {
			http.Error(w, "Not found", http.StatusNotFound)
			return
		}
	}

	if err := h.queries.HideMessages(userID, req.MessageIDs); err != nil {
		http.Error(w, "Failed to hide messages", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)

	// Check if both participants have now hidden any of these 1:1 messages
	for _, msgID := range req.MessageIDs {
		go CheckAndDeleteMutuallyHidden(h.queries, msgID)
	}
}
