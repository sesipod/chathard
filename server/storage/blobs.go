package storage

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync/atomic"
)

// BlobStore handles encrypted blob storage on the filesystem.
type BlobStore struct {
	rootDir   string
	maxBytes  int64 // 0 = unlimited
	totalUsed atomic.Int64 // in-memory counter, avoids O(n²) filesystem walk
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

	// Check storage limit via in-memory counter (no filesystem walk)
	if bs.maxBytes > 0 && bs.totalUsed.Load()+int64(len(data)) > bs.maxBytes {
		return fmt.Errorf("blob storage limit exceeded")
	}

	// Write to temp file with restricted permissions, rename atomically
	tmp, err := os.CreateTemp(dir, "*.tmp")
	if err != nil {
		return fmt.Errorf("create temp: %w", err)
	}
	tmpName := tmp.Name()
	os.Chmod(tmpName, 0600)
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
	bs.totalUsed.Add(int64(len(data)))
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
	fi, err := cw.tmpFile.Stat()
	if err != nil {
		cw.tmpFile.Close()
		os.Remove(tmpName)
		return err
	}
	fileSize := fi.Size()
	if err := cw.tmpFile.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}
	os.Chmod(tmpName, 0600)
	if err := os.Rename(tmpName, path); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("rename: %w", err)
	}
	cw.store.totalUsed.Add(fileSize)
	return nil
}

// ReadBlob reads encrypted data from disk.
func (bs *BlobStore) ReadBlob(uuid string) ([]byte, error) {
	return os.ReadFile(bs.blobPath(uuid))
}

// DeleteBlob removes a blob from disk and decrements the usage counter.
func (bs *BlobStore) DeleteBlob(uuid string) error {
	path := bs.blobPath(uuid)
	fi, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	bs.totalUsed.Add(-fi.Size())
	return os.Remove(path)
}

// DeleteBlobByPath removes a blob by its relative path within the blob store.
func (bs *BlobStore) DeleteBlobByPath(path string) error {
	fullPath := filepath.Join(bs.rootDir, path)
	fi, err := os.Stat(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	bs.totalUsed.Add(-fi.Size())
	return os.Remove(fullPath)
}

// GenerateUUID generates a random hex UUID (32 hex chars).
func GenerateUUID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// TotalSize returns total bytes used by blob storage (from in-memory counter).
func (bs *BlobStore) TotalSize() (int64, error) {
	return bs.totalUsed.Load(), nil
}
