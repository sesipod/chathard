package storage

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// BlobStore handles encrypted blob storage on the filesystem.
type BlobStore struct {
	rootDir  string
	maxBytes int64 // 0 = unlimited
}

// NewBlobStore creates a BlobStore rooted at rootDir.
func NewBlobStore(rootDir string, maxBytes int64) (*BlobStore, error) {
	if err := os.MkdirAll(rootDir, 0700); err != nil {
		return nil, fmt.Errorf("create blob dir: %w", err)
	}
	return &BlobStore{rootDir: rootDir, maxBytes: maxBytes}, nil
}

// blobPath returns the path for a given UUID using first-2-hex sharding.
func (bs *BlobStore) blobPath(uuid string) string {
	shard := uuid[:2]
	return filepath.Join(bs.rootDir, shard, uuid+".enc")
}

// WriteBlob writes encrypted data to disk. Creates shard subdirectory if needed.
func (bs *BlobStore) WriteBlob(uuid string, data []byte) error {
	path := bs.blobPath(uuid)
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("create shard dir: %w", err)
	}

	// Check storage limit
	if bs.maxBytes > 0 {
		var total int64
		filepath.Walk(bs.rootDir, func(p string, fi os.FileInfo, err error) error {
			if err == nil && !fi.IsDir() {
				total += fi.Size()
			}
			return nil
		})
		if total+int64(len(data)) > bs.maxBytes {
			return fmt.Errorf("blob storage limit exceeded")
		}
	}

	// Write to temp file, rename atomically
	tmp, err := os.CreateTemp(dir, "*.tmp")
	if err != nil {
		return fmt.Errorf("create temp: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return fmt.Errorf("write temp: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		os.Remove(tmp.Name())
		return fmt.Errorf("rename: %w", err)
	}
	return nil
}

// WriteBlobChunked handles chunked upload: writes to temp, finalizes on Close.
type chunkedWriter struct {
	store     *BlobStore
	uuid      string
	tmpFile   *os.File
}

// WriteBlobChunked starts a chunked upload. Returns an io.WriteCloser.
// Call Close() to atomically finalize the blob.
func (bs *BlobStore) WriteBlobChunked(uuid string) (io.WriteCloser, error) {
	dir := filepath.Dir(bs.blobPath(uuid))
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, fmt.Errorf("create shard dir: %w", err)
	}
	tmp, err := os.CreateTemp(dir, "*.tmp")
	if err != nil {
		return nil, fmt.Errorf("create temp: %w", err)
	}
	return &chunkedWriter{store: bs, uuid: uuid, tmpFile: tmp}, nil
}

func (cw *chunkedWriter) Write(p []byte) (int, error) {
	return cw.tmpFile.Write(p)
}

func (cw *chunkedWriter) Close() error {
	path := cw.store.blobPath(cw.uuid)
	tmpName := cw.tmpFile.Name()
	if err := cw.tmpFile.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("rename: %w", err)
	}
	return nil
}

// ReadBlob reads encrypted data from disk.
func (bs *BlobStore) ReadBlob(uuid string) ([]byte, error) {
	path := bs.blobPath(uuid)
	return os.ReadFile(path)
}

// DeleteBlob removes a blob from disk.
func (bs *BlobStore) DeleteBlob(uuid string) error {
	path := bs.blobPath(uuid)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// DeleteBlobByPath removes a blob by its full path.
func (bs *BlobStore) DeleteBlobByPath(path string) error {
	return os.Remove(path)
}

// GenerateUUID generates a random hex UUID (32 hex chars).
func GenerateUUID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// TotalSize returns total bytes used by blob storage.
func (bs *BlobStore) TotalSize() (int64, error) {
	var total int64
	err := filepath.Walk(bs.rootDir, func(p string, fi os.FileInfo, err error) error {
		if err == nil && !fi.IsDir() {
			total += fi.Size()
		}
		return nil
	})
	return total, err
}
