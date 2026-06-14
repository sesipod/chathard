package main

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/stuckpacket/tailchat/db"
	"github.com/stuckpacket/tailchat/handlers"
	"github.com/stuckpacket/tailchat/middleware"
	"github.com/stuckpacket/tailchat/storage"
	_ "modernc.org/sqlite"
)

const version = "1.0.0"

var startTime = time.Now()

func main() {
	// CLI flags
	dbPath := flag.String("db-path", "", "Path to SQLite database")
	blobDir := flag.String("blob-dir", "", "Directory for encrypted blob storage")
	addr := flag.String("addr", "", "Server listen address")
	dev := flag.Bool("dev", false, "Enable Vite dev proxy (proxy / to localhost:5173)")
	showVersion := flag.Bool("version", false, "Print version and exit")
	configPath := flag.String("config", "config.yaml", "Path to config file")
	flag.Parse()

	if *showVersion {
		fmt.Println(version)
		os.Exit(0)
	}

	// Load config
	cfg, err := LoadConfig(*configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// CLI flags override config
	if *dbPath != "" {
		cfg.Storage.DBPath = *dbPath
	}
	if *blobDir != "" {
		cfg.Storage.BlobDir = *blobDir
	}
	if *addr != "" {
		cfg.Server.Addr = *addr
	}

	log.Printf("TailChat v%s starting...", version)

	// Open database
	sqlDB, err := sql.Open("sqlite", cfg.Storage.DBPath+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer sqlDB.Close()

	sqlDB.SetMaxOpenConns(1) // SQLite doesn't support concurrent writes

	// Run schema
	if err := runSchema(sqlDB); err != nil {
		log.Fatalf("Failed to run schema: %v", err)
	}

	queries := db.NewQueries(sqlDB)

	// Initialize blob store
	maxBlobBytes := parseSize(cfg.Storage.MaxBlobStorage)
	blobStore, err := storage.NewBlobStore(cfg.Storage.BlobDir, maxBlobBytes)
	if err != nil {
		log.Fatalf("Failed to create blob store: %v", err)
	}

	// Start cleanup goroutine
	cleanInterval, _ := time.ParseDuration(cfg.Cleanup.Interval)
	cleaner := storage.NewCleaner(blobStore, queries, cleanInterval)
	cleaner.Start()
	defer cleaner.Stop()

	// Periodic expired session cleanup
	go func() {
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()
		for range ticker.C {
			res, err := sqlDB.Exec(`DELETE FROM sessions WHERE expires_at < datetime('now')`)
			if err != nil {
				log.Printf("session cleanup: %v", err)
				continue
			}
			if n, _ := res.RowsAffected(); n > 0 {
				log.Printf("session cleanup: deleted %d expired sessions", n)
			}
		}
	}()

	// Initialize WebSocket hub
	handlers.InitHub()

	// Rate limiter
	rl := middleware.NewRateLimiter(middleware.DefaultRateLimitConfig())

	// Create mux
	mux := http.NewServeMux()

	// GET /api/health — health check endpoint (no auth)
	mux.HandleFunc("/api/health", func(w http.ResponseWriter, r *http.Request) {
		uptime := time.Since(startTime).Round(time.Second).String()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"status":  "ok",
			"version": version,
			"uptime":  uptime,
		})
	})

	// Register endpoint (Tailscale auth only, no session required)
	registerHandler := handlers.NewRegisterHandler(queries)
	mux.Handle("/api/register", rl.Middleware("register", rl.Cfg().Register)(tailscaleAuth(registerHandler)))

	// Auth endpoints (rate limited, no session required)
	authHandler := handlers.NewAuthHandler(queries)
	mux.Handle("/api/auth/", tailscaleAuth(rl.Middleware("auth", rl.Cfg().Messages)(authHandler)))

	// Recover endpoint (rate limited)
	recoverHandler := handlers.NewRecoverHandler(queries)
	mux.Handle("/api/recover", tailscaleAuth(rl.Middleware("recover", rl.Cfg().Recovery)(recoverHandler)))

	// User search (Tailscale auth + rate limit, no session required — used during registration)
	mux.Handle("/api/users/search", tailscaleAuth(rl.Middleware("search", rl.Cfg().UserSearch)(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodGet {
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
				return
			}
			query := r.URL.Query().Get("handle")
			if len(query) < 3 {
				http.Error(w, "Query too short", http.StatusBadRequest)
				return
			}
			limit := 20
			if l := r.URL.Query().Get("limit"); l != "" {
				if n, err := strconv.Atoi(l); err == nil && n > 0 && n <= 50 {
					limit = n
				}
			}
			users, err := queries.SearchUsers(query, limit)
			if err != nil {
				http.Error(w, "Search failed", http.StatusInternalServerError)
				return
			}
			if users == nil {
				users = []db.UserRow{}
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(users)
		}),
	)))

	// Authenticated routes
	authMux := http.NewServeMux()

	// GET /api/me
	authMux.HandleFunc("/api/me", func(w http.ResponseWriter, r *http.Request) {
		userID := r.Context().Value(handlers.CtxKeyUserID).(string)
		user, remaining, err := queries.GetMe(userID)
		if err != nil {
			http.Error(w, "User not found", http.StatusNotFound)
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"id":                        user.ID,
			"handle":                    user.Handle,
			"public_key_fingerprint":    hex.EncodeToString(user.PublicKeyEd25519[:8]),
			"recovery_codes_remaining":  remaining,
		})
	})

	// Messages
	msgHandler := handlers.NewMessagesHandler(queries)
	authMux.Handle("/api/messages", rl.Middleware("messages", rl.Cfg().Messages)(msgHandler))
	authMux.Handle("/api/messages/", rl.Middleware("messages", rl.Cfg().Messages)(msgHandler))
	authMux.Handle("/api/conversations", msgHandler)
	// Fallback: mux strips /api/ prefix, so also register without it
	authMux.Handle("/messages", rl.Middleware("messages", rl.Cfg().Messages)(msgHandler))
	authMux.Handle("/messages/", rl.Middleware("messages", rl.Cfg().Messages)(msgHandler))
	authMux.Handle("/conversations", msgHandler)

	// WebSocket
	wsHandler := handlers.NewWSHandler()
	authMux.Handle("/api/ws", wsHandler)
	authMux.Handle("/ws", wsHandler)
	// Also handle /ws directly (spec says GET /ws)
	mux.Handle("/ws", tailscaleAuth(sessionAuth(queries)(wsHandler)))

	// Files
	maxUpload := parseSize(cfg.Storage.MaxUploadSize)
	fileHandler := handlers.NewFilesHandler(queries, blobStore, maxUpload)
	authMux.Handle("/api/files/", fileHandler)
	authMux.Handle("/files/", fileHandler)
	authMux.HandleFunc("GET /api/conversations/{id}/files", fileHandler.GetConversationFiles)

	// Groups
	groupHandler := handlers.NewGroupsHandler(queries)
	authMux.Handle("/api/groups", groupHandler)
	authMux.Handle("/api/groups/", groupHandler)
	authMux.Handle("/groups", groupHandler)
	authMux.Handle("/groups/", groupHandler)

	// Wrap auth routes with session auth + rate limiting
	mux.Handle("/api/", tailscaleAuth(sessionAuth(queries)(authMux)))

	// Static file serving for SPA
	if *dev {
		// In dev mode, proxy to Vite dev server
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			// Skip API routes
			if strings.HasPrefix(r.URL.Path, "/api/") || r.URL.Path == "/ws" {
				return
			}
			http.Redirect(w, r, "http://localhost:5173"+r.URL.Path, http.StatusTemporaryRedirect)
		})
	} else {
		// Serve static files with SPA fallback
		fs := http.FileServer(http.Dir("web/dist"))
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			if strings.HasPrefix(r.URL.Path, "/api/") || r.URL.Path == "/ws" {
				return
			}
			path := "web/dist" + r.URL.Path
			if _, err := os.Stat(path); os.IsNotExist(err) {
				// SPA fallback: serve index.html
				http.ServeFile(w, r, "web/dist/index.html")
				return
			}
			fs.ServeHTTP(w, r)
		})
	}

	// Build middleware chain: security headers -> CORS -> body limit -> mux
	var handler http.Handler = mux
	handler = securityHeaders(handler)
	handler = corsMiddleware(cfg.Server.TailnetDomain)(handler)
	handler = bodyLimit(10 << 20)(handler) // 10MB max request body

	// Start server
	server := &http.Server{
		Addr:         cfg.Server.Addr,
		Handler:      handler,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// Graceful shutdown
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		log.Printf("Server listening on %s", cfg.Server.Addr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server error: %v", err)
		}
	}()

	<-stop
	log.Println("Shutting down...")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	server.Shutdown(shutdownCtx)
	log.Println("Server stopped")
}

// runSchema applies the base schema to a fresh database.
func runSchema(sqlDB *sql.DB) error {
	schema, err := os.ReadFile("db/schema.sql")
	if err != nil {
		// Try alternate path
		schema, err = os.ReadFile("server/db/schema.sql")
		if err != nil {
			return fmt.Errorf("read schema: %w", err)
		}
	}
	_, err = sqlDB.Exec(string(schema))
	return err
}

// tailscaleAuth is the first auth layer: rejects requests without Tailscale-User-Login (404).
func tailscaleAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Tailscale-User-Login") == "" {
			http.Error(w, "Not Found", http.StatusNotFound)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// sessionAuth is the second auth layer: validates session token for authenticated routes.
// Accepts token via Authorization: Bearer header (REST) or ?token query param (WebSocket).
func sessionAuth(queries *db.Queries) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			var token string

			// Try Authorization header first, then ?token query param (for WebSocket)
			const bearer = "Bearer "
			if strings.HasPrefix(authHeader, bearer) {
				token = authHeader[len(bearer):]
			} else if tok := r.URL.Query().Get("token"); tok != "" {
				token = tok
			}

			if token == "" {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}

			tokenHash := sha256.Sum256([]byte(token))
			session, err := queries.GetSessionByToken(hex.EncodeToString(tokenHash[:]))
			if err != nil || session == nil {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}

			if time.Now().After(session.ExpiresAt) {
				queries.DeleteSession(hex.EncodeToString(tokenHash[:]))
				http.Error(w, "Session expired", http.StatusUnauthorized)
				return
			}

			ctx := context.WithValue(r.Context(), handlers.CtxKeyUserID, session.UserID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// corsMiddleware allows only origins matching *.ts.net.
func corsMiddleware(tailnetDomain string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if origin != "" {
				if strings.HasSuffix(origin, "."+tailnetDomain) || origin == "http://localhost:5173" || origin == "http://localhost:3000" {
					w.Header().Set("Access-Control-Allow-Origin", origin)
					w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")
					w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, Tailscale-User-Login")
					w.Header().Set("Access-Control-Allow-Credentials", "true")
				}
			}
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// securityHeaders adds security-related HTTP headers.
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		next.ServeHTTP(w, r)
	})
}

// bodyLimit restricts the request body size to maxBytes.
func bodyLimit(maxBytes int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
			next.ServeHTTP(w, r)
		})
	}
}

// parseSize parses a size string like "10GB", "100MB" into bytes.
func parseSize(s string) int64 {
	s = strings.ToUpper(strings.TrimSpace(s))
	var multiplier int64 = 1
	switch {
	case strings.HasSuffix(s, "GB"):
		multiplier = 1 << 30
		s = strings.TrimSuffix(s, "GB")
	case strings.HasSuffix(s, "MB"):
		multiplier = 1 << 20
		s = strings.TrimSuffix(s, "MB")
	case strings.HasSuffix(s, "KB"):
		multiplier = 1 << 10
		s = strings.TrimSuffix(s, "KB")
	}
	var val float64
	fmt.Sscanf(s, "%f", &val)
	return int64(val * float64(multiplier))
}
