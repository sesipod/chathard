package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/google/uuid"
	"github.com/stuckpacket/tailchat/db"
)

type RegisterRequest struct {
	Handle               string              `json:"handle"`
	PublicKeyEd25519     []byte              `json:"public_key_ed25519"`
	PublicKeyX25519      []byte              `json:"public_key_x25519"`
	DerivedPublicKeyEd25519 []byte           `json:"derived_public_key_ed25519"`
	EncryptedKeyBackups  []EncryptedKeyBackup `json:"encrypted_key_backups"`
}

type EncryptedKeyBackup struct {
	RecoveryCodeHash     string `json:"recovery_code_hash"`
	EncryptedPrivateKey  []byte `json:"encrypted_private_key"`
	Salt                 []byte `json:"salt"`
	AuthSalt             []byte `json:"auth_salt"`
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

	if req.Handle == "" || len(req.PublicKeyEd25519) == 0 || len(req.PublicKeyX25519) == 0 || len(req.DerivedPublicKeyEd25519) == 0 {
		http.Error(w, "Missing required fields", http.StatusBadRequest)
		return
	}

	// Check handle uniqueness
	existing, _ := h.queries.GetUserByHandle(req.Handle)
	if existing != nil {
		http.Error(w, "Handle already taken", http.StatusConflict)
		return
	}

	// Get Tailscale identity
	tailscaleUser := r.Header.Get("Tailscale-User-Login")
	if tailscaleUser == "" {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	userID := uuid.New().String()

	if err := h.queries.CreateUser(userID, req.Handle, req.PublicKeyEd25519, req.PublicKeyX25519, req.DerivedPublicKeyEd25519); err != nil {
		http.Error(w, "Failed to create user", http.StatusInternalServerError)
		return
	}

	// Store encrypted key backups
	for _, kb := range req.EncryptedKeyBackups {
		if err := h.queries.InsertRecoveryBackup(userID, kb.RecoveryCodeHash, kb.EncryptedPrivateKey, kb.Salt, kb.AuthSalt); err != nil {
			// Continue even if one fails
			continue
		}
	}

	resp := map[string]string{
		"user_id":  userID,
		"handle":   req.Handle,
		"identity": tailscaleUser,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(resp)
}

// Ensure compilation check.
var _ http.Handler = (*RegisterHandler)(nil)
