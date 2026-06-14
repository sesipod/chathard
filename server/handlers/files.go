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

// FilesHandler handles file upload/download endpoints.
type FilesHandler struct {
	queries *db.Queries
	store   *storage.BlobStore
	maxSize int64
}

// NewFilesHandler creates a new FilesHandler.
func NewFilesHandler(queries *db.Queries, store *storage.BlobStore, maxSize int64) *FilesHandler {
	return &FilesHandler{
		queries: queries,
		store:   store,
		maxSize: maxSize,
	}
}

// ServeHTTP routes file endpoints.
func (h *FilesHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Parse path — handle both /api/files/xxx and files/xxx (mux may strip /api/)
	path := strings.TrimPrefix(r.URL.Path, "/api/files")
	path = strings.TrimPrefix(path, "/files")
	path = strings.TrimLeft(path, "/")

	switch {
	case r.Method == http.MethodPost && path == "upload":
		h.uploadFile(w, r)
	case r.Method == http.MethodGet && path != "" && path != "upload":
		h.downloadFile(w, r, path)
	case r.Method == http.MethodDelete && path != "" && path != "upload":
		h.deleteFile(w, r, path)
	default:
		http.Error(w, "Not found", http.StatusNotFound)
	}
}

func (h *FilesHandler) uploadFile(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(CtxKeyUserID).(string)

	contentType := r.Header.Get("Content-Type")
	isChunked := strings.HasPrefix(contentType, "multipart/form-data")

	var (
		fileID        string
		encryptedData []byte
		encryptedMeta []byte
		expiresIn     string
		targetID      string
		targetType    string
	)

	if isChunked {
		// Parse multipart form (chunked upload)
		if err := r.ParseMultipartForm(h.maxSize); err != nil {
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

		encryptedData, err = io.ReadAll(file)
		if err != nil {
			http.Error(w, "Failed to read file", http.StatusInternalServerError)
			return
		}

		encryptedMetaStr := r.FormValue("encrypted_metadata")
		encryptedMeta = []byte(encryptedMetaStr)
		expiresIn = r.FormValue("expires_in")
		targetID = r.FormValue("target_id")
		targetType = r.FormValue("target_type")

		fileID = storage.GenerateUUID()
	} else {
		// Simple JSON upload for smaller files
		var req struct {
			Ciphertext    []byte `json:"ciphertext"`
			EncryptedMeta []byte `json:"encrypted_metadata"`
			ExpiresIn     string `json:"expires_in"`
			TargetID      string `json:"target_id"`
			TargetType    string `json:"target_type"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid JSON", http.StatusBadRequest)
			return
		}

		if int64(len(req.Ciphertext)) > h.maxSize {
			http.Error(w, "File exceeds max upload size", http.StatusRequestEntityTooLarge)
			return
		}

		encryptedData = req.Ciphertext
		encryptedMeta = req.EncryptedMeta
		expiresIn = req.ExpiresIn
		targetID = req.TargetID
		targetType = req.TargetType
		fileID = storage.GenerateUUID()
	}

	// Compute file expiration
	var expiresAt *time.Time
	if expiresIn != "" {
		d, err := parseDuration(expiresIn)
		if err != nil {
			http.Error(w, "Invalid expires_in", http.StatusBadRequest)
			return
		}
		t := time.Now().Add(d)
		expiresAt = &t
	}

	// Store blob
	if err := h.store.WriteBlob(fileID, encryptedData); err != nil {
		log.Printf("upload: write blob %s: %v", fileID, err)
		http.Error(w, "Failed to store file", http.StatusInternalServerError)
		return
	}

	if err := h.queries.InsertFile(fileID, userID, fileID[:2]+"/"+fileID+".enc", encryptedMeta, int64(len(encryptedData)), expiresAt, targetID, targetType); err != nil {
		h.store.DeleteBlob(fileID)
		log.Printf("upload: insert file record %s (meta=%d data=%d): %v", fileID, len(encryptedMeta), len(encryptedData), err)
		http.Error(w, "Failed to create file record", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{
		"file_id": fileID,
		"size":    strconv.FormatInt(int64(len(encryptedData)), 10),
	})
}

// GetConversationFiles handles GET /api/conversations/{id}/files?type=direct|group
func (h *FilesHandler) GetConversationFiles(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(CtxKeyUserID).(string)
	convID := r.PathValue("id")
	convType := r.URL.Query().Get("type")

	if convID == "" {
		http.Error(w, "Missing conversation id", http.StatusBadRequest)
		return
	}
	if convType != "direct" && convType != "group" {
		http.Error(w, "type must be 'direct' or 'group'", http.StatusBadRequest)
		return
	}

	// Auth check: verify the user is a participant
	if convType == "group" {
		memberIDs, err := h.queries.GetGroupMemberIDs(convID)
		if err != nil {
			http.Error(w, "Group not found", http.StatusNotFound)
			return
		}
		isMember := false
		for _, mid := range memberIDs {
			if mid == userID {
				isMember = true
				break
			}
		}
		if !isMember {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
	} else {
		// direct: verify the user has a conversation with the target user
		convs, err := h.queries.GetConversations(userID)
		if err != nil {
			http.Error(w, "Failed to verify conversation", http.StatusInternalServerError)
			return
		}
		isParticipant := false
		for _, c := range convs {
			if c.UserID == convID {
				isParticipant = true
				break
			}
		}
		if !isParticipant {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
	}

	files, err := h.queries.GetConversationFiles(convID, convType)
	if err != nil {
		http.Error(w, "Failed to fetch files", http.StatusInternalServerError)
		return
	}
	if files == nil {
		files = []db.FileRow{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"files": files,
	})
}

func (h *FilesHandler) downloadFile(w http.ResponseWriter, r *http.Request, fileID string) {
	userID := r.Context().Value(CtxKeyUserID).(string)

	fileRec, err := h.queries.GetFile(fileID)
	if err != nil {
		http.Error(w, "File not found", http.StatusNotFound)
		return
	}

	// Allow download by conversation participants (file_id is shared via encrypted chat)
	if fileRec.UploaderID != userID {
		convs, err := h.queries.GetConversations(userID)
		authorized := false
		if err == nil {
			for _, c := range convs {
				if c.UserID == fileRec.UploaderID {
					authorized = true
					break
				}
			}
		}
		if !authorized {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
	}

	data, err := h.store.ReadBlob(fileID)
	if err != nil {
		http.Error(w, "File not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Length", strconv.FormatInt(fileRec.SizeBytes, 10))
	w.Write(data)
}

func (h *FilesHandler) deleteFile(w http.ResponseWriter, r *http.Request, fileID string) {
	userID := r.Context().Value(CtxKeyUserID).(string)

	fileRec, err := h.queries.GetFile(fileID)
	if err != nil {
		http.Error(w, "File not found", http.StatusNotFound)
		return
	}

	// Only uploader can delete
	if fileRec.UploaderID != userID {
		http.Error(w, "Forbidden", http.StatusForbidden)
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
