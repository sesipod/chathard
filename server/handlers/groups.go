package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/stuckpacket/tailchat/db"
)

const (
	maxGroupMembers  = 100
	maxEnvelopeBytes = 512
	maxEncNameBytes  = 512
)

// GroupsHandler handles group endpoints. Group names and keys are opaque
// ciphertext to the server; this handler enforces WHO may do what.
//
//	POST   /api/groups                       create (caller becomes admin)
//	GET    /api/groups                       list my groups
//	GET    /api/groups/{id}/keys             my key envelopes (all epochs) + member roster
//	GET    /api/groups/{id}/messages         message history (members only)
//	POST   /api/groups/{id}/members          admin adds a member (current epoch)
//	DELETE /api/groups/{id}/members/{uid}    leave (self) — flags the group for rotation
//	POST   /api/groups/{id}/remove           admin removes a member and rotates the key
//	POST   /api/groups/{id}/rotate           admin rotates the key
type GroupsHandler struct {
	queries *db.Queries
}

// NewGroupsHandler creates a new GroupsHandler.
func NewGroupsHandler(queries *db.Queries) *GroupsHandler {
	return &GroupsHandler{queries: queries}
}

func (h *GroupsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/groups")
	path = strings.TrimPrefix(path, "/groups")
	path = strings.Trim(path, "/")

	if path == "" {
		switch r.Method {
		case http.MethodGet:
			h.listGroups(w, r)
		case http.MethodPost:
			h.createGroup(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
		return
	}

	parts := strings.Split(path, "/")
	groupID := parts[0]
	if _, err := uuid.Parse(groupID); err != nil {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	switch {
	case len(parts) == 2 && parts[1] == "keys" && r.Method == http.MethodGet:
		h.getKeys(w, r, groupID)
	case len(parts) == 2 && parts[1] == "messages" && r.Method == http.MethodGet:
		h.getGroupMessages(w, r, groupID)
	case len(parts) == 2 && parts[1] == "members" && r.Method == http.MethodPost:
		h.addMember(w, r, groupID)
	case len(parts) == 3 && parts[1] == "members" && r.Method == http.MethodDelete:
		h.leave(w, r, groupID, parts[2])
	case len(parts) == 2 && parts[1] == "remove" && r.Method == http.MethodPost:
		h.removeAndRotate(w, r, groupID)
	case len(parts) == 2 && parts[1] == "rotate" && r.Method == http.MethodPost:
		h.rotate(w, r, groupID)
	default:
		http.Error(w, "Not found", http.StatusNotFound)
	}
}

type memberEnvelopeReq struct {
	UserID   string `json:"user_id"`
	Envelope []byte `json:"envelope"`
}

func (h *GroupsHandler) validateEnvelopes(list []memberEnvelopeReq, signer string, epoch int) ([]db.GroupEnvelope, bool) {
	if len(list) == 0 || len(list) > maxGroupMembers {
		return nil, false
	}
	seen := map[string]bool{}
	out := make([]db.GroupEnvelope, 0, len(list))
	for _, m := range list {
		if _, err := uuid.Parse(m.UserID); err != nil || seen[m.UserID] {
			return nil, false
		}
		if len(m.Envelope) == 0 || len(m.Envelope) > maxEnvelopeBytes {
			return nil, false
		}
		seen[m.UserID] = true
		out = append(out, db.GroupEnvelope{UserID: m.UserID, SignerID: signer, Epoch: epoch, Envelope: m.Envelope})
	}
	return out, true
}

func (h *GroupsHandler) createGroup(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(CtxKeyUserID).(string)

	var req struct {
		ID            string              `json:"id"`
		EncryptedName []byte              `json:"encrypted_name"`
		Members       []memberEnvelopeReq `json:"members"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}
	// The client picks the id because the group name's ciphertext is bound to it.
	if _, err := uuid.Parse(req.ID); err != nil {
		http.Error(w, "Invalid group id", http.StatusBadRequest)
		return
	}
	if len(req.EncryptedName) == 0 || len(req.EncryptedName) > maxEncNameBytes {
		http.Error(w, "Invalid group name", http.StatusBadRequest)
		return
	}
	envs, ok := h.validateEnvelopes(req.Members, userID, 0)
	if !ok {
		http.Error(w, "Invalid members", http.StatusBadRequest)
		return
	}
	creatorIn := false
	ids := make([]string, 0, len(envs))
	for _, e := range envs {
		if e.UserID == userID {
			creatorIn = true
		}
		ids = append(ids, e.UserID)
	}
	if !creatorIn {
		http.Error(w, "Creator must be a member", http.StatusBadRequest)
		return
	}
	users, err := h.queries.GetUsersByIDs(ids)
	if err != nil || len(users) != len(ids) {
		http.Error(w, "Unknown member", http.StatusBadRequest)
		return
	}

	if err := h.queries.CreateGroupWithMembers(req.ID, req.EncryptedName, userID, envs); err != nil {
		http.Error(w, "Failed to create group", http.StatusConflict)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{"group_id": req.ID})
}

// notifyGroup pings the current members (and any extra users, e.g. someone just
// removed) so their clients refresh. Ids only; nothing secret.
func (h *GroupsHandler) notifyGroup(groupID string, except string, extra ...string) {
	hub := GetHub()
	if hub == nil {
		return
	}
	ids, _ := h.queries.GetGroupMemberIDs(groupID)
	if g, err := h.queries.GetGroup(groupID); err == nil {
		ids = append(ids, g.CreatedBy)
	}
	for _, e := range extra {
		if e != "" {
			ids = append(ids, e)
		}
	}
	seen := map[string]bool{except: true}
	var targets []string
	for _, id := range ids {
		if !seen[id] {
			seen[id] = true
			targets = append(targets, id)
		}
	}
	hub.NotifyGroupChanged(groupID, targets...)
}

// requireMember loads the group and checks membership; writes the error itself.
func (h *GroupsHandler) requireMember(w http.ResponseWriter, userID, groupID string) (*db.GroupInfo, bool) {
	g, err := h.queries.GetGroup(groupID)
	if err != nil {
		http.Error(w, "Not found", http.StatusNotFound)
		return nil, false
	}
	ok, err := h.queries.IsGroupMember(groupID, userID)
	if err != nil || !ok {
		// Same answer as "no such group" so ids can't be probed.
		http.Error(w, "Not found", http.StatusNotFound)
		return nil, false
	}
	return g, true
}

func (h *GroupsHandler) requireAdmin(w http.ResponseWriter, userID, groupID string) (*db.GroupInfo, bool) {
	g, ok := h.requireMember(w, userID, groupID)
	if !ok {
		return nil, false
	}
	if g.CreatedBy != userID {
		http.Error(w, "Only the group admin can do that", http.StatusForbidden)
		return nil, false
	}
	return g, true
}

func (h *GroupsHandler) getKeys(w http.ResponseWriter, r *http.Request, groupID string) {
	userID := r.Context().Value(CtxKeyUserID).(string)
	g, ok := h.requireMember(w, userID, groupID)
	if !ok {
		return
	}
	envs, err := h.queries.GetMemberEnvelopes(groupID, userID)
	if err != nil {
		http.Error(w, "Failed to load keys", http.StatusInternalServerError)
		return
	}
	memberIDs, err := h.queries.GetGroupMemberIDs(groupID)
	if err != nil {
		http.Error(w, "Failed to load members", http.StatusInternalServerError)
		return
	}
	members, err := h.queries.GetUsersByIDs(memberIDs)
	if err != nil {
		http.Error(w, "Failed to load members", http.StatusInternalServerError)
		return
	}
	sort.Slice(members, func(i, j int) bool { return members[i].Handle < members[j].Handle })
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"group":     g,
		"envelopes": envs,
		"members":   members,
	})
}

func (h *GroupsHandler) addMember(w http.ResponseWriter, r *http.Request, groupID string) {
	userID := r.Context().Value(CtxKeyUserID).(string)
	g, ok := h.requireAdmin(w, userID, groupID)
	if !ok {
		return
	}
	var req struct {
		UserID   string `json:"user_id"`
		Epoch    int    `json:"epoch"`
		Envelope []byte `json:"envelope"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}
	if req.Epoch != g.Epoch {
		http.Error(w, "Stale group key epoch", http.StatusConflict)
		return
	}
	if len(req.Envelope) == 0 || len(req.Envelope) > maxEnvelopeBytes {
		http.Error(w, "Invalid envelope", http.StatusBadRequest)
		return
	}
	if users, err := h.queries.GetUsersByIDs([]string{req.UserID}); err != nil || len(users) != 1 {
		http.Error(w, "Unknown user", http.StatusBadRequest)
		return
	}
	ids, _ := h.queries.GetGroupMemberIDs(groupID)
	if len(ids) >= maxGroupMembers {
		http.Error(w, "Group is full", http.StatusBadRequest)
		return
	}
	if already, _ := h.queries.IsGroupMember(groupID, req.UserID); already {
		http.Error(w, "Already a member", http.StatusConflict)
		return
	}
	if err := h.queries.AddGroupMember(groupID, db.GroupEnvelope{UserID: req.UserID, SignerID: userID, Epoch: req.Epoch, Envelope: req.Envelope}); err != nil {
		http.Error(w, "Failed to add member", http.StatusInternalServerError)
		return
	}
	h.notifyGroup(groupID, userID)
	w.WriteHeader(http.StatusNoContent)
}

func (h *GroupsHandler) leave(w http.ResponseWriter, r *http.Request, groupID, memberID string) {
	userID := r.Context().Value(CtxKeyUserID).(string)
	if memberID != userID {
		http.Error(w, "Use the remove endpoint to remove other members", http.StatusForbidden)
		return
	}
	g, ok := h.requireMember(w, userID, groupID)
	if !ok {
		return
	}
	if g.CreatedBy == userID {
		http.Error(w, "The group admin cannot leave; remove the other members first", http.StatusBadRequest)
		return
	}
	if err := h.queries.LeaveGroup(groupID, userID); err != nil {
		http.Error(w, "Failed to leave group", http.StatusInternalServerError)
		return
	}
	h.notifyGroup(groupID, userID)
	w.WriteHeader(http.StatusNoContent)
}

type rotateReq struct {
	UserID        string              `json:"user_id"` // remove only
	Epoch         int                 `json:"epoch"`   // the NEW epoch
	EncryptedName []byte              `json:"encrypted_name"`
	Members       []memberEnvelopeReq `json:"members"`
}

func (h *GroupsHandler) removeAndRotate(w http.ResponseWriter, r *http.Request, groupID string) {
	h.doRotate(w, r, groupID, true)
}

func (h *GroupsHandler) rotate(w http.ResponseWriter, r *http.Request, groupID string) {
	h.doRotate(w, r, groupID, false)
}

func (h *GroupsHandler) doRotate(w http.ResponseWriter, r *http.Request, groupID string, removing bool) {
	userID := r.Context().Value(CtxKeyUserID).(string)
	g, ok := h.requireAdmin(w, userID, groupID)
	if !ok {
		return
	}
	var req rotateReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}
	if req.Epoch != g.Epoch+1 {
		http.Error(w, "Stale group key epoch", http.StatusConflict)
		return
	}
	if len(req.EncryptedName) == 0 || len(req.EncryptedName) > maxEncNameBytes {
		http.Error(w, "Invalid group name", http.StatusBadRequest)
		return
	}
	remove := ""
	if removing {
		remove = req.UserID
		if remove == "" || remove == userID {
			http.Error(w, "Invalid member to remove", http.StatusBadRequest)
			return
		}
		if isMember, _ := h.queries.IsGroupMember(groupID, remove); !isMember {
			http.Error(w, "Not a member", http.StatusBadRequest)
			return
		}
	}
	envs, ok := h.validateEnvelopes(req.Members, userID, req.Epoch)
	if !ok {
		http.Error(w, "Invalid members", http.StatusBadRequest)
		return
	}
	// Envelopes must cover exactly the members who remain — no more, no less.
	current, err := h.queries.GetGroupMemberIDs(groupID)
	if err != nil {
		http.Error(w, "Failed to load members", http.StatusInternalServerError)
		return
	}
	want := map[string]bool{}
	for _, id := range current {
		if id != remove {
			want[id] = true
		}
	}
	if len(want) != len(envs) {
		http.Error(w, "Envelopes must cover exactly the remaining members", http.StatusBadRequest)
		return
	}
	for _, e := range envs {
		if !want[e.UserID] {
			http.Error(w, "Envelopes must cover exactly the remaining members", http.StatusBadRequest)
			return
		}
	}
	if err := h.queries.RotateGroup(groupID, req.Epoch, remove, req.EncryptedName, envs); err != nil {
		if err == sql.ErrNoRows {
			http.Error(w, "Stale group key epoch", http.StatusConflict)
			return
		}
		http.Error(w, "Failed to rotate group key", http.StatusInternalServerError)
		return
	}
	h.notifyGroup(groupID, userID, remove)
	w.WriteHeader(http.StatusNoContent)
}

func (h *GroupsHandler) getGroupMessages(w http.ResponseWriter, r *http.Request, groupID string) {
	userID := r.Context().Value(CtxKeyUserID).(string)
	if _, ok := h.requireMember(w, userID, groupID); !ok {
		return
	}

	limit := 50
	q := r.URL.Query()
	if l := q.Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 {
			limit = n
			if limit > 200 {
				limit = 200
			}
		}
	}
	var after, before *time.Time
	if a := q.Get("after"); a != "" {
		if t, err := time.Parse(time.RFC3339, a); err == nil {
			after = &t
		}
	}
	if b := q.Get("before"); b != "" {
		if t, err := time.Parse(time.RFC3339, b); err == nil {
			before = &t
		}
	}

	var retentionMod string
	if ret, err := h.queries.GetUserRetention(userID, groupID, "group"); err == nil && ret != "" {
		retentionMod = retentionSQLModifier(ret)
	}

	messages, err := h.queries.GetGroupMessagesWithRetention(userID, groupID, after, before, limit, retentionMod)
	if err != nil {
		http.Error(w, "Failed to fetch messages", http.StatusInternalServerError)
		return
	}
	if messages == nil {
		messages = []db.MessageRow{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"messages": messages})
}

func (h *GroupsHandler) listGroups(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(CtxKeyUserID).(string)
	groups, err := h.queries.GetUserGroupsWithActivity(userID)
	if err != nil {
		http.Error(w, "Failed to fetch groups", http.StatusInternalServerError)
		return
	}
	if groups == nil {
		groups = []db.GroupWithActivity{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"groups": groups})
}
