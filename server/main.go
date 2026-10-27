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
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/stuckpacket/tailchat/db"
	"github.com/stuckpacket/tailchat/handlers"
	"github.com/stuckpacket/tailchat/middleware"
	"github.com/stuckpacket/tailchat/storage"
	_ "modernc.org/sqlite"
)

const version = "2.0.0"

var startTime = time.Now()

func main() {
	// CLI flags
	dbPath := flag.String("db-path", "", "Path to SQLite database")
	blobDir := flag.String("blob-dir", "", "Directory for encrypted blob storage")
	addr := flag.String("addr", "", "Server listen address")
	dev := flag.Bool("dev", false, "Enable Vite dev proxy (proxy / to localhost:5173)")
	showVersion := flag.Bool("version", false, "Print version and exit")
	configPath := flag.String("config", "config.yaml", "Path to config file")
	healthcheck := flag.Bool("healthcheck", false, "Probe /api/health on the configured address and exit (for container health checks)")
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

	if *healthcheck {
		client := &http.Client{Timeout: 3 * time.Second}
		resp, err := client.Get("http://" + cfg.Server.Addr + "/api/health")
		if err != nil || resp.StatusCode != http.StatusOK {
			os.Exit(1)
		}
		os.Exit(0)
	}

	log.Printf("TailChat v%s starting (domain %s, VPN %s)...", version, cfg.Server.Domain, strings.Join(cfg.Server.VPNCIDRs, ","))

	// Only the app's own HTTPS origin (plus local Vite dev) may make
	// cross-origin requests or open WebSockets.
	allowedOrigins := map[string]bool{"https://" + cfg.Server.Domain: true}
	for _, o := range cfg.Server.ExtraOrigins {
		allowedOrigins[strings.TrimSpace(o)] = true
	}
	if *dev {
		allowedOrigins["http://localhost:5173"] = true
		allowedOrigins["http://localhost:3000"] = true
	}
	handlers.SetAllowedOrigins(allowedOrigins)
	allowedHosts := map[string]bool{strings.ToLower(cfg.Server.Domain): true}
	for o := range allowedOrigins {
		if u, err := url.Parse(o); err == nil {
			allowedHosts[strings.ToLower(u.Hostname())] = true
		}
	}

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
	cleaner := storage.NewCleaner(blobStore, queries, func(msgID string) {
		handlers.CheckAndDeleteMutuallyHidden(queries, msgID)
	}, cleanInterval)
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
	handlers.InitHub(queries)

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
			"network": "WireGuard",
			"domain":  cfg.Server.Domain,
		})
	})

	// Register endpoint (VPN gate only, no session required)
	registerHandler := handlers.NewRegisterHandler(queries)
	mux.Handle("/api/register", rl.Middleware("register", rl.Cfg().Register)(registerHandler))

	// Auth endpoints (rate limited, no session required)
	authHandler := handlers.NewAuthHandler(queries)
	mux.Handle("/api/auth/", rl.Middleware("auth", rl.Cfg().Messages)(authHandler))

	// Handle availability: the only unauthenticated lookup, answers yes/no for ONE
	// exact handle so sign-up can say "taken" without exposing the user list.
	mux.Handle("/api/handle-available", rl.Middleware("search", rl.Cfg().UserSearch)(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodGet {
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
				return
			}
			h := strings.TrimSpace(r.URL.Query().Get("handle"))
			_, err := queries.GetUserByHandle(h)
			taken := err == nil || err != sql.ErrNoRows // on any unexpected error, say "taken"
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]bool{"available": !taken})
		}),
	))

	// Authenticated routes
	authMux := http.NewServeMux()

	// User search: signed-in users only, prefix match, LIKE wildcards escaped.
	authMux.Handle("/api/users/search", rl.Middleware("search", rl.Cfg().UserSearch)(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodGet {
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
				return
			}
			query := strings.TrimSpace(r.URL.Query().Get("handle"))
			if len(query) < 3 {
				http.Error(w, "Query too short", http.StatusBadRequest)
				return
			}
			users, err := queries.SearchUsers(query, 20)
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
	))

	// POST /api/ws-ticket — one-time ticket for opening the WebSocket.
	authMux.HandleFunc("/api/ws-ticket", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		t := handlers.IssueWSTicket(r.Context().Value(handlers.CtxKeyUserID).(string))
		if t == "" {
			http.Error(w, "Busy, try again", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"ticket": t})
	})

	// GET /api/me
	authMux.HandleFunc("/api/me", func(w http.ResponseWriter, r *http.Request) {
		userID := r.Context().Value(handlers.CtxKeyUserID).(string)
		user, err := queries.GetUserByID(userID)
		if err != nil {
			http.Error(w, "User not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"id":                     user.ID,
			"handle":                 user.Handle,
			"public_key_ed25519":     user.PublicKeyEd25519,
			"public_key_x25519":      user.PublicKeyX25519,
			"public_key_fingerprint": hex.EncodeToString(user.PublicKeyEd25519[:8]),
		})
	})

	// GET /api/users/lookup?ids=a,b,c — public keys for people the caller talks to.
	authMux.HandleFunc("/api/users/lookup", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		ids := strings.Split(r.URL.Query().Get("ids"), ",")
		if len(ids) == 0 || len(ids) > 100 || ids[0] == "" {
			http.Error(w, "ids required (max 100)", http.StatusBadRequest)
			return
		}
		users, err := queries.GetUsersByIDs(ids)
		if err != nil {
			http.Error(w, "Lookup failed", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"users": users})
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
	// Authenticated by a one-time ticket inside the handler (see /api/ws-ticket).
	mux.Handle("/ws", wsHandler)

	// Files
	maxUpload := parseSize(cfg.Storage.MaxUploadSize)
	fileHandler := handlers.NewFilesHandler(queries, blobStore, maxUpload)
	fileHandler.SetUserQuota(parseSize(cfg.Storage.MaxUserStorage))
	authMux.Handle("/api/files/upload", rl.Middleware("upload", rl.Cfg().Upload)(fileHandler))
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
	// Throttle before authentication so token guessing and floods are limited too.
	mux.Handle("/api/", rl.Middleware("api", rl.Cfg().API)(sessionAuth(queries)(authMux)))

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
			if st, err := os.Stat(path); err != nil || st.IsDir() {
				// SPA fallback: serve index.html (never a directory listing)
				http.ServeFile(w, r, "web/dist/index.html")
				return
			}
			fs.ServeHTTP(w, r)
		})
	}

	// Build middleware chain: VPN gate -> body limit -> CORS -> security headers -> mux.
	// The gate is outermost so nothing (not even static files) is served to
	// clients outside the WireGuard subnet.
	gate, err := middleware.NewVPNGate(cfg.Server.VPNCIDRs, cfg.Server.TrustedProxies)
	if err != nil {
		log.Fatalf("Invalid VPN config: %v", err)
	}
	var handler http.Handler = mux
	handler = securityHeaders(allowedOrigins)(handler)
	handler = corsMiddleware(allowedOrigins)(handler)
	handler = bodyLimit(10<<20, parseSize(cfg.Storage.MaxUploadSize)+(2<<20))(handler)
	handler = hostGuard(allowedHosts)(handler)
	handler = gate.Middleware(handler)

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

// sessionAuth is the second auth layer: validates session token for authenticated routes.
// Accepts the token via the Authorization: Bearer header only.
func sessionAuth(queries *db.Queries) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Tokens are accepted ONLY in the Authorization header, never in a URL
			// (URLs end up in logs and referrers). WebSockets use one-time tickets.
			authHeader := r.Header.Get("Authorization")
			var token string
			const bearer = "Bearer "
			if strings.HasPrefix(authHeader, bearer) {
				token = authHeader[len(bearer):]
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

// corsMiddleware allows only the configured app origin (and dev origins).
func corsMiddleware(allowed map[string]bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if origin != "" {
				if allowed[origin] {
					w.Header().Set("Access-Control-Allow-Origin", origin)
					w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")
					w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
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

// securityHeaders sets browser protections. The Content-Security-Policy is the
// main defence for the account key: it stops injected or third-party script
// from running on the page, and stops any data being sent to other origins.
func securityHeaders(origins map[string]bool) func(http.Handler) http.Handler {
	connect := []string{"'self'"}
	for o := range origins {
		switch {
		case strings.HasPrefix(o, "https://"):
			connect = append(connect, "wss://"+strings.TrimPrefix(o, "https://"))
		case strings.HasPrefix(o, "http://"):
			connect = append(connect, "ws://"+strings.TrimPrefix(o, "http://"))
		}
	}
	sort.Strings(connect[1:])
	csp := strings.Join([]string{
		"default-src 'none'",
		"script-src 'self'",
		"style-src 'self' 'unsafe-inline'", // Svelte components use inline style attributes
		"img-src 'self' blob: data:",
		"font-src 'self'",
		"connect-src " + strings.Join(connect, " "),
		"manifest-src 'self'",
		"worker-src 'self'",
		"base-uri 'none'",
		"form-action 'self'",
		"frame-ancestors 'none'",
		"object-src 'none'",
	}, "; ")
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h := w.Header()
			h.Set("Content-Security-Policy", csp)
			h.Set("X-Content-Type-Options", "nosniff")
			h.Set("X-Frame-Options", "DENY")
			h.Set("Referrer-Policy", "no-referrer")
			h.Set("Permissions-Policy", "camera=(self), microphone=(), geolocation=(), payment=()")
			h.Set("Cross-Origin-Opener-Policy", "same-origin")
			h.Set("Cross-Origin-Resource-Policy", "same-origin")
			if strings.HasPrefix(r.URL.Path, "/api/") {
				// Ciphertext, key envelopes and user lists must never sit in a cache.
				h.Set("Cache-Control", "no-store")
			}
			next.ServeHTTP(w, r)
		})
	}
}

// hostGuard blocks DNS-rebinding: a page on some other domain that is
// re-pointed at this server's VPN address would arrive with that other
// domain in the Host header. Only the configured domain (or a bare IP) passes.
func hostGuard(allowed map[string]bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			host := r.Host
			if h, _, err := net.SplitHostPort(host); err == nil {
				host = h
			}
			host = strings.Trim(strings.ToLower(host), "[]")
			if net.ParseIP(host) == nil && !allowed[host] {
				http.Error(w, "Not Found", http.StatusNotFound)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// bodyLimit restricts request bodies: maxBytes everywhere except file uploads,
// which get uploadBytes.
func bodyLimit(maxBytes, uploadBytes int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			limit := maxBytes
			if r.URL.Path == "/api/files/upload" {
				limit = uploadBytes
			}
			r.Body = http.MaxBytesReader(w, r.Body, limit)
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
