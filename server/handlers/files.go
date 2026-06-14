package handlers

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"

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
		fileID          string
		encryptedData   []byte
		encryptedMeta   []byte
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

		fileID = storage.GenerateUUID()
	} else {
		// Simple JSON upload for smaller files
		var req struct {
			Ciphertext       []byte `json:"ciphertext"`
			EncryptedMeta    []byte `json:"encrypted_metadata"`
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
		fileID = storage.GenerateUUID()
	}

	// Store blob
	if err := h.store.WriteBlob(fileID, encryptedData); err != nil {
		log.Printf("upload: write blob %s: %v", fileID, err)
		http.Error(w, "Failed to store file", http.StatusInternalServerError)
		return
	}

	if err := h.queries.InsertFile(fileID, userID, fileID[:2]+"/"+fileID+".enc", encryptedMeta, int64(len(encryptedData)), nil); err != nil {
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
