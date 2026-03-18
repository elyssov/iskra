package store

import (
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/iskra-messenger/iskra/internal/message"
)

// Hold is the store-and-forward "cargo hold" for messages in transit.
type Hold struct {
	dir string
	mu  sync.RWMutex
}

// NewHold creates a new Hold at the given directory.
func NewHold(dir string) (*Hold, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create hold directory: %w", err)
	}
	return &Hold{dir: dir}, nil
}

// Store saves a serialized message to the hold.
func (h *Hold) Store(msg *message.Message) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	filename := hex.EncodeToString(msg.ID[:]) + ".msg"
	path := filepath.Join(h.dir, filename)

	data := msg.Serialize()
	return os.WriteFile(path, data, 0600)
}

// Get retrieves a message by ID.
func (h *Hold) Get(id [32]byte) (*message.Message, error) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	filename := hex.EncodeToString(id[:]) + ".msg"
	path := filepath.Join(h.dir, filename)

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return message.Deserialize(data)
}

// Delete removes a message from the hold.
func (h *Hold) Delete(id [32]byte) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	filename := hex.EncodeToString(id[:]) + ".msg"
	path := filepath.Join(h.dir, filename)
	return os.Remove(path)
}

// GetAll returns all messages in the hold.
func (h *Hold) GetAll() ([]*message.Message, error) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	entries, err := os.ReadDir(h.dir)
	if err != nil {
		return nil, err
	}

	var msgs []*message.Message
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".msg" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(h.dir, entry.Name()))
		if err != nil {
			continue
		}
		msg, err := message.Deserialize(data)
		if err != nil {
			continue
		}
		msgs = append(msgs, msg)
	}
	return msgs, nil
}

// Count returns the number of messages in the hold.
func (h *Hold) Count() int {
	h.mu.RLock()
	defer h.mu.RUnlock()

	entries, err := os.ReadDir(h.dir)
	if err != nil {
		return 0
	}
	count := 0
	for _, e := range entries {
		if !e.IsDir() && filepath.Ext(e.Name()) == ".msg" {
			count++
		}
	}
	return count
}

// Has checks if a message with the given ID exists in the hold.
func (h *Hold) Has(id [32]byte) bool {
	filename := hex.EncodeToString(id[:]) + ".msg"
	path := filepath.Join(h.dir, filename)
	_, err := os.Stat(path)
	return err == nil
}
