package storage

import (
	"log"
	"time"
)

// Cleaner handles periodic cleanup of expired messages and blobs.
type Cleaner struct {
	store    *BlobStore
	queries  interface {
		DeleteExpiredMessages() (int64, error)
		DeleteExpiredFiles() ([]string, error)
		RemoveFileRecord(path string) error
	}
	interval time.Duration
	stopCh   chan struct{}
}

// NewCleaner creates a periodic cleaner.
func NewCleaner(store *BlobStore, q interface {
	DeleteExpiredMessages() (int64, error)
	DeleteExpiredFiles() ([]string, error)
	RemoveFileRecord(path string) error
}, interval time.Duration) *Cleaner {
	if interval <= 0 {
		interval = time.Hour
	}
	return &Cleaner{
		store:    store,
		queries:  q,
		interval: interval,
		stopCh:   make(chan struct{}),
	}
}

// Start begins the periodic cleanup loop.
func (c *Cleaner) Start() {
	go func() {
		ticker := time.NewTicker(c.interval)
		defer ticker.Stop()

		// Run immediately on start
		c.run()

		for {
			select {
			case <-ticker.C:
				c.run()
			case <-c.stopCh:
				return
			}
		}
	}()
}

// Stop signals the cleanup goroutine to stop.
func (c *Cleaner) Stop() {
	close(c.stopCh)
}

func (c *Cleaner) run() {
	// Delete expired messages
	n, err := c.queries.DeleteExpiredMessages()
	if err != nil {
		log.Printf("cleanup: delete expired messages: %v", err)
	} else if n > 0 {
		log.Printf("cleanup: deleted %d expired messages", n)
	}

	// Delete expired files
	paths, err := c.queries.DeleteExpiredFiles()
	if err != nil {
		log.Printf("cleanup: find expired files: %v", err)
		return
	}
	for _, path := range paths {
		if err := c.store.DeleteBlobByPath(path); err != nil {
			log.Printf("cleanup: delete blob %s: %v", path, err)
			continue
		}
		if err := c.queries.RemoveFileRecord(path); err != nil {
			log.Printf("cleanup: remove file record %s: %v", path, err)
		}
	}
	if len(paths) > 0 {
		log.Printf("cleanup: deleted %d expired files", len(paths))
	}
}
