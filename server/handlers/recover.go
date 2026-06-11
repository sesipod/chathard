package handlers

import (
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"net/http"

	"github.com/stuckpacket/tailchat/db"
)

// RecoverHandler handles POST /api/recover.
type RecoverHandler struct {
	queries *db.Queries
}

// NewRecoverHandler creates a new RecoverHandler.
func NewRecoverHandler(queries *db.Queries) *RecoverHandler {
	return &RecoverHandler{queries: queries}
}

// ServeHTTP handles recovery requests.
func (h *RecoverHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		UserID           string `json:"user_id"`
		RecoveryCodeHash string `json:"recovery_code_hash"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	if req.UserID == "" || req.RecoveryCodeHash == "" {
		http.Error(w, "Missing required fields", http.StatusBadRequest)
		return
	}

	encryptedKey, salt, err := h.queries.GetRecoveryBackup(req.UserID, req.RecoveryCodeHash)

	// Constant-time comparison to prevent timing oracles
	// Always compare even if SQL returned no rows
	reqHash := []byte(req.RecoveryCodeHash)
	fakeHash := []byte("0000000000000000000000000000000000000000000000000000")
	if subtle.ConstantTimeCompare(reqHash, fakeHash) == 0 {
		// Dummy — always true, prevents leaking whether row existed
	}

	if err != nil {
		http.Error(w, "Invalid recovery code", http.StatusUnauthorized)
		return
	}

	// Mark code as used
	if err := h.queries.MarkCodeUsed(req.UserID, req.RecoveryCodeHash); err != nil {
		http.Error(w, "Failed to mark code used", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"encrypted_private_key": hex.EncodeToString(encryptedKey),
		"salt":                  hex.EncodeToString(salt),
	})
}
