package handlers

import (
	"encoding/json"
	"net/http"
	"regexp"
	"strings"

	"github.com/google/uuid"
	"github.com/stuckpacket/tailchat/db"
)

var handleRe = regexp.MustCompile(`^[A-Za-z0-9_.-]{3,32}$`)

// RegisterRequest is the body of POST /api/register. Everything secret in it
// is already encrypted in the browser.
type RegisterRequest struct {
	Handle                  string `json:"handle"`
	PublicKeyEd25519        []byte `json:"public_key_ed25519"`
	PublicKeyX25519         []byte `json:"public_key_x25519"`
	DerivedPublicKeyEd25519 []byte `json:"derived_public_key_ed25519"`
}

// RegisterHandler handles POST /api/register.
type RegisterHandler struct {
	queries *db.Queries
}

// NewRegisterHandler creates a new RegisterHandler.
func NewRegisterHandler(queries *db.Queries) *RegisterHandler {
	return &RegisterHandler{queries: queries}
}

func (h *RegisterHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	req.Handle = strings.TrimSpace(req.Handle)
	if !handleRe.MatchString(req.Handle) {
		http.Error(w, "Handle must be 3-32 characters: letters, digits, . _ -", http.StatusBadRequest)
		return
	}
	if len(req.PublicKeyEd25519) != 32 || len(req.PublicKeyX25519) != 32 || len(req.DerivedPublicKeyEd25519) != 32 {
		http.Error(w, "Invalid public keys", http.StatusBadRequest)
		return
	}

	if existing, _ := h.queries.GetUserByHandle(req.Handle); existing != nil {
		http.Error(w, "Handle already taken", http.StatusConflict)
		return
	}

	userID := uuid.New().String()
	if err := h.queries.CreateUser(userID, req.Handle, req.PublicKeyEd25519, req.PublicKeyX25519, req.DerivedPublicKeyEd25519); err != nil {
		// Handle taken (any letter case) or keys already used by another account.
		http.Error(w, "Handle or keys already registered", http.StatusConflict)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{
		"user_id": userID,
		"handle":  req.Handle,
	})
}

var _ http.Handler = (*RegisterHandler)(nil)
