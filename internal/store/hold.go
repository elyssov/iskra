package store

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/iskra-messenger/iskra/internal/message"
)

// Hold propagation constants
const (
	DefaultHopTTL     = 10             // Max hops before exhaustion
	DefaultFwdLimit   = 15             // Max forwards per hop (per device) — generous for small network
	MorgueTimeout     = 3 * time.Hour  // Time in morgue before deletion
	KillSwitchTTL     = 30 * 24 * time.Hour // 30 days absolute TTL
)

// holdMeta tracks propagation metadata per message.
type holdMeta struct {
	HopTTL       int   `json:"hop_ttl"`        // Remaining hops
	ForwardCount int   `json:"forward_count"`  // Times we forwarded this message
	StoredAt     int64 `json:"stored_at"`      // Unix timestamp when first stored on THIS device
	MsgTimestamp int64 `json:"msg_timestamp"`  // Unix timestamp from message creation (author's clock)
	Exhausted    bool  `json:"exhausted"`      // Forward limit reached
	MorgueAt     int64 `json:"morgue_at"`      // Unix timestamp when moved to morgue (0 = not in morgue)
}

// Hold is the store-and-forward "cargo hold" for messages in transit.
// Implements hop-based TTL, forward limiting, morgue, and kill switch.
type Hold struct {
	dir  string
	meta map[string]*holdMeta // msgID hex -> metadata
	mu   sync.RWMutex
}

// NewHold creates a new Hold at the given directory.
func NewHold(dir string) (*Hold, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create hold directory: %w", err)
	}
	h := &Hold{dir: dir, meta: make(map[string]*holdMeta)}
	h.loadMeta()
	return h, nil
}

// Store saves a serialized message to the hold with default HopTTL.
func (h *Hold) Store(msg *message.Message) error {
	return h.StoreWithTTL(msg, DefaultHopTTL)
}

// StoreWithTTL saves a message with a specific HopTTL (used when receiving from peer).
func (h *Hold) StoreWithTTL(msg *message.Message, hopTTL int) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	idHex := hex.EncodeToString(msg.ID[:])

	// Don't store if already in hold or morgue
	if _, exists := h.meta[idHex]; exists {
		return nil
	}

	// Don't store if HopTTL is 0 — it's already exhausted
	if hopTTL <= 0 {
		return nil
	}

	filename := idHex + ".msg"
	path := filepath.Join(h.dir, filename)
	data := msg.Serialize()
	if err := os.WriteFile(path, data, 0600); err != nil {
		return err
	}

	h.meta[idHex] = &holdMeta{
		HopTTL:       hopTTL,
		ForwardCount: 0,
		StoredAt:     time.Now().Unix(),
		MsgTimestamp: msg.Timestamp, // Author's creation time — used for absolute kill switch
	}
	h.saveMeta()
	return nil
}

// GetForSync returns messages eligible for forwarding to a peer.
// Decrements HopTTL for each returned message.
// Returns messages with their HopTTL-1 value for the receiver.
func (h *Hold) GetForSync() ([]*message.Message, []int) {
	h.mu.Lock()
	defer h.mu.Unlock()

	// First, run cleanup
	h.cleanupLocked()

	entries, err := os.ReadDir(h.dir)
	if err != nil {
		return nil, nil
	}

	var msgs []*message.Message
	var hopTTLs []int

	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".msg" {
			continue
		}
		idHex := entry.Name()[:len(entry.Name())-4]
		meta, ok := h.meta[idHex]
		if !ok || meta.Exhausted || meta.MorgueAt > 0 {
			continue // skip exhausted or in morgue
		}
		if meta.ForwardCount >= DefaultFwdLimit {
			// Move to morgue
			meta.Exhausted = true
			meta.MorgueAt = time.Now().Unix()
			log.Printf("[Hold] Message %s exhausted (%d forwards), moving to morgue", idHex[:8], meta.ForwardCount)
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

		// Don't increment ForwardCount here — call MarkForwarded() after actual send
		receiverTTL := meta.HopTTL - 1
		msgs = append(msgs, msg)
		hopTTLs = append(hopTTLs, receiverTTL)
	}

	// Always save metadata — even if no messages to send, counters may have changed
	h.saveMeta()
	return msgs, hopTTLs
}

// MarkForwarded increments the forward count for a message after successful send.
func (h *Hold) MarkForwarded(id [32]byte) {
	h.mu.Lock()
	defer h.mu.Unlock()
	idHex := fmt.Sprintf("%x", id[:])
	meta, ok := h.meta[idHex]
	if !ok {
		return
	}
	meta.ForwardCount++
	if meta.ForwardCount >= DefaultFwdLimit {
		meta.Exhausted = true
		meta.MorgueAt = time.Now().Unix()
		log.Printf("[Hold] Message %s hit forward limit (%d), moving to morgue", idHex[:8], meta.ForwardCount)
	}
	h.saveMeta()
}

// GetAll returns all messages in the hold (for legacy sync compatibility).
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

// Delete removes a message from the hold (delivery confirmed).
func (h *Hold) Delete(id [32]byte) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	idHex := hex.EncodeToString(id[:])
	filename := idHex + ".msg"
	path := filepath.Join(h.dir, filename)
	os.Remove(path)
	delete(h.meta, idHex)
	h.saveMeta()
	return nil
}

// Count returns the number of active messages in the hold.
func (h *Hold) Count() int {
	h.mu.RLock()
	defer h.mu.RUnlock()

	count := 0
	for _, m := range h.meta {
		if m.MorgueAt == 0 {
			count++
		}
	}
	return count
}

// Has checks if a message with the given ID exists in the hold.
func (h *Hold) Has(id [32]byte) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()

	idHex := hex.EncodeToString(id[:])
	_, exists := h.meta[idHex]
	return exists
}

// Cleanup removes expired messages (morgue timeout + kill switch).
// Call periodically.
func (h *Hold) Cleanup() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.cleanupLocked()
	h.saveMeta()
}

func (h *Hold) cleanupLocked() {
	now := time.Now().Unix()
	var toDelete []string

	for idHex, meta := range h.meta {
		// Kill switch: absolute TTL from message CREATION time (30 days)
		// Use StoredAt as fallback, but prefer message timestamp if available
		originTime := meta.StoredAt
		if meta.MsgTimestamp > 0 {
			originTime = meta.MsgTimestamp
		}
		if now-originTime > int64(KillSwitchTTL.Seconds()) {
			toDelete = append(toDelete, idHex)
			continue
		}
		// Morgue timeout: 3 hours after exhaustion
		if meta.MorgueAt > 0 && now-meta.MorgueAt > int64(MorgueTimeout.Seconds()) {
			toDelete = append(toDelete, idHex)
			continue
		}
	}

	for _, idHex := range toDelete {
		filename := idHex + ".msg"
		os.Remove(filepath.Join(h.dir, filename))
		delete(h.meta, idHex)
		log.Printf("[Hold] Cleaned up message %s", idHex[:8])
	}
}

// Metadata persistence
func (h *Hold) metaPath() string {
	return filepath.Join(h.dir, "_meta.json")
}

func (h *Hold) loadMeta() {
	data, err := os.ReadFile(h.metaPath())
	if err != nil {
		return
	}
	json.Unmarshal(data, &h.meta)
}

func (h *Hold) saveMeta() {
	data, _ := json.Marshal(h.meta)
	os.WriteFile(h.metaPath(), data, 0600)
}
