package handlers

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/stuckpacket/tailchat/db"
)

type challengeEntry struct {
	Challenge        []byte
	UserID           string
	DerivedPublicKey []byte // for Ed25519 signature verification
	ExpiresAt        time.Time
}

// AuthHandler handles authentication endpoints.
type AuthHandler struct {
	queries    *db.Queries
	mu         sync.Mutex
	challenges map[string]*challengeEntry // challenge (hex) -> entry
}

// NewAuthHandler creates a new AuthHandler.
func NewAuthHandler(queries *db.Queries) *AuthHandler {
	h := &AuthHandler{
		queries:    queries,
		challenges: make(map[string]*challengeEntry),
	}
	// Periodic cleanup of expired challenges and sessions
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			h.mu.Lock()
			now := time.Now()
			for key, entry := range h.challenges {
				if now.After(entry.ExpiresAt) {
					delete(h.challenges, key)
				}
			}
			h.mu.Unlock()
		}
	}()
	return h
}

// ServeHTTP routes auth sub-paths.
func (h *AuthHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/api/auth/challenge":
		h.handleChallenge(w, r)
	case "/api/auth/verify":
		h.handleVerify(w, r)
	case "/api/auth/logout":
		h.handleLogout(w, r)
	default:
		http.Error(w, "Not found", http.StatusNotFound)
	}
}

// POST /api/auth/challenge: returns 32 random bytes with 5-min expiry.
func (h *AuthHandler) handleChallenge(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Handle string `json:"handle"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Handle == "" {
		http.Error(w, "Missing handle", http.StatusBadRequest)
		return
	}

	user, err := h.queries.GetUserByHandle(req.Handle)
	if err != nil {
		http.Error(w, "User not found", http.StatusNotFound)
		return
	}

	challenge := make([]byte, 32)
	if _, err := rand.Read(challenge); err != nil {
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	challengeHex := hex.EncodeToString(challenge)

	h.mu.Lock()
	h.challenges[challengeHex] = &challengeEntry{
		Challenge:        challenge,
		UserID:           user.ID,
		DerivedPublicKey: user.DerivedPublicKeyEd25519,
		ExpiresAt:        time.Now().Add(5 * time.Minute),
	}
	h.mu.Unlock()

	// Clean old challenges periodically
	go func() {
		time.Sleep(6 * time.Minute)
		h.mu.Lock()
		delete(h.challenges, challengeHex)
		h.mu.Unlock()
	}()

	json.NewEncoder(w).Encode(map[string]interface{}{
		"challenge":          hex.EncodeToString(challenge),
		"derived_public_key": hex.EncodeToString(user.DerivedPublicKeyEd25519),
	})
}

// POST /api/auth/verify: verifies signature, returns session token.
func (h *AuthHandler) handleVerify(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Handle    string `json:"handle"`
		Challenge string `json:"challenge"`
		Signature string `json:"signature"` // hex-encoded Ed25519 signature
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	h.mu.Lock()
	entry, ok := h.challenges[req.Challenge]
	h.mu.Unlock()

	if !ok || time.Now().After(entry.ExpiresAt) {
		http.Error(w, "Invalid or expired challenge", http.StatusUnauthorized)
		return
	}

	// Cleanup used challenge
	go func() {
		h.mu.Lock()
		delete(h.challenges, req.Challenge)
		h.mu.Unlock()
	}()

	// Verify Ed25519 signature against the derived public key
	sig, err := hex.DecodeString(req.Signature)
	if err != nil || len(sig) != ed25519.SignatureSize {
		http.Error(w, "Invalid signature", http.StatusUnauthorized)
		return
	}
	if !ed25519.Verify(entry.DerivedPublicKey, entry.Challenge, sig) {
		http.Error(w, "Invalid signature", http.StatusUnauthorized)
		return
	}

	// Generate session token
	token := make([]byte, 32)
	rand.Read(token)
	tokenHex := hex.EncodeToString(token)
	tokenHash := sha256.Sum256([]byte(tokenHex))

	expiresAt := time.Now().Add(7 * 24 * time.Hour)
	if err := h.queries.CreateSession(hex.EncodeToString(tokenHash[:]), entry.UserID, expiresAt); err != nil {
		http.Error(w, "Failed to create session", http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(map[string]string{
		"token":    tokenHex,
		"user_id":  entry.UserID,
		"expires":  expiresAt.UTC().Format(time.RFC3339),
	})
}

// POST /api/auth/logout: deletes session.
func (h *AuthHandler) handleLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	token := r.Header.Get("Authorization")
	if token == "" {
		http.Error(w, "Missing token", http.StatusUnauthorized)
		return
	}

	tokenHash := sha256.Sum256([]byte(token))
	if err := h.queries.DeleteSession(hex.EncodeToString(tokenHash[:])); err != nil {
		http.Error(w, "Failed to delete session", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
