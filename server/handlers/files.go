package handlers

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/stuckpacket/tailchat/db"
	"github.com/stuckpacket/tailchat/storage"
)

// FilesHandler stores and serves opaque encrypted blobs. File names, types and
// keys never reach the server: they travel inside the encrypted chat message.
type FilesHandler struct {
	queries   *db.Queries
	store     *storage.BlobStore
	maxSize   int64
	userQuota int64 // bytes of uploads one user may keep; 0 = unlimited
}

// SetUserQuota limits how much one account can store, so a single user cannot fill the disk.
func (h *FilesHandler) SetUserQuota(bytes int64) { h.userQuota = bytes }

// NewFilesHandler creates a new FilesHandler.
func NewFilesHandler(queries *db.Queries, store *storage.BlobStore, maxSize int64) *FilesHandler {
	return &FilesHandler{queries: queries, store: store, maxSize: maxSize}
}

// ServeHTTP routes file endpoints.
func (h *FilesHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/files")
	path = strings.TrimPrefix(path, "/files")
	path = strings.TrimLeft(path, "/")

	switch {
	case r.Method == http.MethodPost && path == "upload":
		h.uploadFile(w, r)
	case r.Method == http.MethodPatch && strings.HasSuffix(path, "/link-message"):
		h.linkFileToMessage(w, r, strings.TrimSuffix(path, "/link-message"))
	case r.Method == http.MethodGet && path != "" && path != "upload":
		h.downloadFile(w, r, path)
	case r.Method == http.MethodDelete && path != "" && path != "upload":
		h.deleteFile(w, r, path)
	default:
		http.Error(w, "Not found", http.StatusNotFound)
	}
}

// canUseTarget reports whether userID may attach files to / read files of a
// conversation: any existing user for a 1:1 chat, or a member of the group.
func (h *FilesHandler) canUseTarget(userID, targetID, targetType string) bool {
	switch targetType {
	case "direct":
		users, err := h.queries.GetUsersByIDs([]string{targetID})
		return err == nil && len(users) == 1
	case "group":
		ok, err := h.queries.IsGroupMember(targetID, userID)
		return err == nil && ok
	}
	return false
}

func (h *FilesHandler) uploadFile(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(CtxKeyUserID).(string)

	if !strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
		http.Error(w, "Expected multipart upload", http.StatusUnsupportedMediaType)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, h.maxSize+(1<<20))
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		http.Error(w, "File too large", http.StatusRequestEntityTooLarge)
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "Missing file", http.StatusBadRequest)
		return
	}
	defer file.Close()
	if header.Size > h.maxSize {
		http.Error(w, "File exceeds max upload size", http.StatusRequestEntityTooLarge)
		return
	}
	data, err := io.ReadAll(io.LimitReader(file, h.maxSize+1))
	if err != nil || int64(len(data)) > h.maxSize || len(data) == 0 {
		http.Error(w, "Failed to read file", http.StatusBadRequest)
		return
	}

	if h.userQuota > 0 {
		if used, err := h.queries.UserFileBytes(userID); err != nil || used+int64(len(data)) > h.userQuota {
			http.Error(w, "Storage quota exceeded", http.StatusRequestEntityTooLarge)
			return
		}
	}

	targetID := r.FormValue("target_id")
	targetType := r.FormValue("target_type")
	if !h.canUseTarget(userID, targetID, targetType) {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	var expiresAt *time.Time
	if in := r.FormValue("expires_in"); in != "" {
		d, err := parseDuration(in)
		if err != nil {
			http.Error(w, "Invalid expires_in", http.StatusBadRequest)
			return
		}
		t := time.Now().Add(d)
		expiresAt = &t
	}

	fileID := storage.GenerateUUID()
	if err := h.store.WriteBlob(fileID, data); err != nil {
		log.Printf("upload: write blob %s: %v", fileID, err)
		http.Error(w, "Failed to store file", http.StatusInternalServerError)
		return
	}
	if err := h.queries.InsertFile(fileID, userID, fileID[:2]+"/"+fileID+".enc", int64(len(data)), expiresAt, targetID, targetType); err != nil {
		h.store.DeleteBlob(fileID)
		log.Printf("upload: insert file record %s: %v", fileID, err)
		http.Error(w, "Failed to create file record", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{
		"file_id": fileID,
		"size":    strconv.Itoa(len(data)),
	})
}

// GetConversationFiles handles GET /api/conversations/{id}/files?type=direct|group.
// It lists blob records only (id, size, time, message link). Names and types
// are inside the encrypted messages, so the client matches them up itself.
func (h *FilesHandler) GetConversationFiles(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(CtxKeyUserID).(string)
	convID := r.PathValue("id")
	convType := r.URL.Query().Get("type")

	if convID == "" || (convType != "direct" && convType != "group") {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}
	if !h.canUseTarget(userID, convID, convType) {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	files, err := h.queries.GetConversationFiles(userID, convID, convType)
	if err != nil {
		http.Error(w, "Failed to fetch files", http.StatusInternalServerError)
		return
	}
	if files == nil {
		files = []db.FileRow{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"files": files})
}

// canRead: the uploader, the 1:1 recipient, or a member of the target group.
func (h *FilesHandler) canRead(userID string, f *db.FileRow) bool {
	if f.UploaderID == userID {
		return true
	}
	switch f.TargetType {
	case "direct":
		return f.TargetID == userID
	case "group":
		ok, err := h.queries.IsGroupMember(f.TargetID, userID)
		return err == nil && ok
	}
	return false
}

func (h *FilesHandler) downloadFile(w http.ResponseWriter, r *http.Request, fileID string) {
	userID := r.Context().Value(CtxKeyUserID).(string)

	f, err := h.queries.GetFile(fileID)
	if err != nil || !h.canRead(userID, f) {
		http.Error(w, "File not found", http.StatusNotFound)
		return
	}
	data, err := h.store.ReadBlob(fileID)
	if err != nil {
		http.Error(w, "File not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Length", strconv.FormatInt(int64(len(data)), 10))
	w.Header().Set("Cache-Control", "private, no-store")
	w.Write(data)
}

func (h *FilesHandler) deleteFile(w http.ResponseWriter, r *http.Request, fileID string) {
	userID := r.Context().Value(CtxKeyUserID).(string)

	f, err := h.queries.GetFile(fileID)
	if err != nil || f.UploaderID != userID {
		http.Error(w, "File not found", http.StatusNotFound)
		return
	}
	if err := h.store.DeleteBlob(fileID); err != nil {
		http.Error(w, "Failed to delete file", http.StatusInternalServerError)
		return
	}
	if err := h.queries.DeleteFile(fileID); err != nil {
		http.Error(w, "Failed to delete file record", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *FilesHandler) linkFileToMessage(w http.ResponseWriter, r *http.Request, fileID string) {
	userID := r.Context().Value(CtxKeyUserID).(string)

	f, err := h.queries.GetFile(fileID)
	if err != nil || f.UploaderID != userID {
		http.Error(w, "File not found", http.StatusNotFound)
		return
	}
	var req struct {
		MessageID string `json:"message_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.MessageID == "" {
		http.Error(w, "Missing message_id", http.StatusBadRequest)
		return
	}
	if ok, _ := h.queries.CanAccessMessage(userID, req.MessageID); !ok {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}
	if err := h.queries.LinkFileToMessage(fileID, req.MessageID); err != nil {
		http.Error(w, "Failed to link message", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
