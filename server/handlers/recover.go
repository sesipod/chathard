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

	// Fetch all unused recovery backups for this user (no hash filter in SQL)
	// so we can do constant-time comparison in Go to prevent timing attacks.
	backups, err := h.queries.GetUnusedRecoveryBackups(req.UserID)
	if err != nil {
		http.Error(w, "Invalid recovery code", http.StatusUnauthorized)
		return
	}

	// Constant-time comparison: iterate all codes to find a match
	// This prevents leaking whether a specific hash exists via timing
	reqHash, _ := hex.DecodeString(req.RecoveryCodeHash)
	if len(reqHash) != 32 {
		reqHash = make([]byte, 32)
	}

	var encryptedKey, salt []byte
	var matchedHash string
	for _, b := range backups {
		storedHash, _ := hex.DecodeString(b.RecoveryCodeHash)
		if len(storedHash) != 32 {
			storedHash = make([]byte, 32)
		}
		if subtle.ConstantTimeCompare(reqHash, storedHash) == 1 {
			encryptedKey = b.EncryptedPrivateKey
			salt = b.Salt
			matchedHash = b.RecoveryCodeHash
			break
		}
	}

	if encryptedKey == nil {
		http.Error(w, "Invalid recovery code", http.StatusUnauthorized)
		return
	}

	// Mark code as used
	if err := h.queries.MarkCodeUsed(req.UserID, matchedHash); err != nil {
		http.Error(w, "Failed to mark code used", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"encrypted_private_key": hex.EncodeToString(encryptedKey),
		"salt":                  hex.EncodeToString(salt),
	})
}
