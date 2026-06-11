package handlers

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/stuckpacket/tailchat/db"
)

// GroupsHandler handles group CRUD endpoints.
type GroupsHandler struct {
	queries *db.Queries
}

// NewGroupsHandler creates a new GroupsHandler.
func NewGroupsHandler(queries *db.Queries) *GroupsHandler {
	return &GroupsHandler{queries: queries}
}

// ServeHTTP routes group endpoints.
func (h *GroupsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/groups")
	path = strings.TrimPrefix(path, "/")

	// GET /api/groups — list user's groups
	if path == "" && r.Method == http.MethodGet {
		h.listGroups(w, r)
		return
	}

	// POST /api/groups — create group
	if path == "" && r.Method == http.MethodPost {
		h.createGroup(w, r)
		return
	}

	parts := strings.Split(path, "/")
	if len(parts) >= 2 && parts[1] == "members" {
		groupID := parts[0]
		switch r.Method {
		case http.MethodPost:
			h.addMember(w, r, groupID)
		case http.MethodDelete:
			userID := parts[2]
			h.removeMember(w, r, groupID, userID)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
		return
	}

	// GET /api/groups/:id/messages
	if len(parts) >= 2 && parts[1] == "messages" && r.Method == http.MethodGet {
		groupID := parts[0]
		h.getGroupMessages(w, r, groupID)
		return
	}

	http.Error(w, "Not found", http.StatusNotFound)
}

type createGroupReq struct {
	EncryptedName         []byte                    `json:"encrypted_name"`
	EncryptedSymmetricKey []byte                    `json:"encrypted_symmetric_key"`
	Members               []createGroupMemberReq    `json:"members"`
}

type createGroupMemberReq struct {
	UserID                string `json:"user_id"`
	EncryptedGroupKey     []byte `json:"encrypted_group_key"`
	EncryptedMetadata     []byte `json:"encrypted_member_metadata"`
}

func (h *GroupsHandler) createGroup(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(CtxKeyUserID).(string)

	var req createGroupReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	if len(req.EncryptedName) == 0 || len(req.EncryptedSymmetricKey) == 0 {
		http.Error(w, "Missing required fields", http.StatusBadRequest)
		return
	}

	groupID := uuid.New().String()

	if err := h.queries.CreateGroup(groupID, req.EncryptedName, req.EncryptedSymmetricKey); err != nil {
		http.Error(w, "Failed to create group", http.StatusInternalServerError)
		return
	}

	// Add creator as member
	if err := h.queries.AddMember(groupID, userID, req.EncryptedSymmetricKey, []byte("{}")); err != nil {
		http.Error(w, "Failed to add creator", http.StatusInternalServerError)
		return
	}

	// Add other members
	for _, m := range req.Members {
		if err := h.queries.AddMember(groupID, m.UserID, m.EncryptedGroupKey, m.EncryptedMetadata); err != nil {
			continue
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{
		"group_id": groupID,
	})
}

func (h *GroupsHandler) addMember(w http.ResponseWriter, r *http.Request, groupID string) {
	userID := r.Context().Value(CtxKeyUserID).(string)
	_ = userID

	var req struct {
		UserID              string `json:"user_id"`
		EncryptedGroupKey   []byte `json:"encrypted_group_key"`
		EncryptedMetadata   []byte `json:"encrypted_member_metadata"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	if err := h.queries.AddMember(groupID, req.UserID, req.EncryptedGroupKey, req.EncryptedMetadata); err != nil {
		http.Error(w, "Failed to add member", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *GroupsHandler) removeMember(w http.ResponseWriter, r *http.Request, groupID, memberID string) {
	userID := r.Context().Value(CtxKeyUserID).(string)

	// Only the member themselves (leave) or group admin can remove
	if memberID != userID {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	if err := h.queries.RemoveMember(groupID, memberID); err != nil {
		http.Error(w, "Failed to remove member", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *GroupsHandler) getGroupMessages(w http.ResponseWriter, r *http.Request, groupID string) {
	limit := 50
	q := r.URL.Query()
	if l := q.Get("limit"); l != "" {
		if parsed, err := parseInt(l); err == nil && parsed > 0 {
			limit = parsed
			if limit > 200 {
				limit = 200
			}
		}
	}

	messages, err := h.queries.GetGroupMessages(groupID, nil, nil, limit)
	if err != nil {
		http.Error(w, "Failed to fetch messages", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"messages": messages,
	})
}

func (h *GroupsHandler) listGroups(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(CtxKeyUserID).(string)

	groups, err := h.queries.GetUserGroups(userID)
	if err != nil {
		http.Error(w, "Failed to fetch groups", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"groups": groups,
	})
}

func parseInt(s string) (int, error) {
	var n int
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, nil
		}
		n = n*10 + int(c-'0')
	}
	return n, nil
}


