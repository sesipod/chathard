package storage

import (
	"log"
	"time"
)

// Cleaner handles periodic cleanup of expired messages and blobs.
type Cleaner struct {
	store          *BlobStore
	queries        interface {
		HideExpiredMessages() ([]string, error)
		DeleteExpiredFiles() ([]string, error)
		RemoveFileRecord(path string) error
	}
	mutualDeleteFn func(messageID string)
	interval       time.Duration
	stopCh         chan struct{}
}

// NewCleaner creates a periodic cleaner.
func NewCleaner(store *BlobStore, q interface {
	HideExpiredMessages() ([]string, error)
	DeleteExpiredFiles() ([]string, error)
	RemoveFileRecord(path string) error
}, mutualDeleteFn func(messageID string), interval time.Duration) *Cleaner {
	if interval <= 0 {
		interval = time.Hour
	}
	return &Cleaner{
		store:          store,
		queries:        q,
		mutualDeleteFn: mutualDeleteFn,
		interval:       interval,
		stopCh:         make(chan struct{}),
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
	// Hide expired messages (move to message_deletions per-user)
	messageIDs, err := c.queries.HideExpiredMessages()
	if err != nil {
		log.Printf("cleanup: hide expired messages: %v", err)
	} else if len(messageIDs) > 0 {
		log.Printf("cleanup: hidden %d expired messages", len(messageIDs))
		// Trigger mutual-deletion check for each message
		for _, msgID := range messageIDs {
			if c.mutualDeleteFn != nil {
				c.mutualDeleteFn(msgID)
			}
		}
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
