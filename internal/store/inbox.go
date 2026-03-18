package store

import (
	"encoding/json"
	"os"
	"sort"
	"sync"
	"time"
)

// InboxMessage is a decrypted message stored locally.
type InboxMessage struct {
	ID        string `json:"id"`
	From      string `json:"from"`       // UserID of sender
	FromPub   string `json:"from_pub"`   // Base58 Ed25519 pub of sender
	Text      string `json:"text"`
	Timestamp int64  `json:"timestamp"`
	Status    string `json:"status"`     // "sent", "delivered"
	Outgoing  bool   `json:"outgoing"`   // true if we sent it
}

// Inbox manages decrypted message history per contact.
type Inbox struct {
	dir      string
	messages map[string][]InboxMessage // keyed by contact UserID
	mu       sync.RWMutex
}

// NewInbox creates or loads an inbox.
func NewInbox(dir string) (*Inbox, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	return &Inbox{
		dir:      dir,
		messages: make(map[string][]InboxMessage),
	}, nil
}

// AddMessage stores a decrypted message.
func (in *Inbox) AddMessage(contactID string, msg InboxMessage) {
	in.mu.Lock()
	defer in.mu.Unlock()

	// Check duplicate
	for _, existing := range in.messages[contactID] {
		if existing.ID == msg.ID {
			return
		}
	}

	if msg.Timestamp == 0 {
		msg.Timestamp = time.Now().Unix()
	}

	in.messages[contactID] = append(in.messages[contactID], msg)

	// Sort by timestamp
	sort.Slice(in.messages[contactID], func(i, j int) bool {
		return in.messages[contactID][i].Timestamp < in.messages[contactID][j].Timestamp
	})
}

// GetMessages returns message history with a contact.
func (in *Inbox) GetMessages(contactID string) []InboxMessage {
	in.mu.RLock()
	defer in.mu.RUnlock()

	msgs := in.messages[contactID]
	result := make([]InboxMessage, len(msgs))
	copy(result, msgs)
	return result
}

// DeleteChat removes all messages for a contact.
func (in *Inbox) DeleteChat(contactID string) {
	in.mu.Lock()
	defer in.mu.Unlock()
	delete(in.messages, contactID)
}

// MarkDelivered marks a message as delivered.
func (in *Inbox) MarkDelivered(msgID string) {
	in.mu.Lock()
	defer in.mu.Unlock()

	for contactID := range in.messages {
		for i := range in.messages[contactID] {
			if in.messages[contactID][i].ID == msgID {
				in.messages[contactID][i].Status = "delivered"
				return
			}
		}
	}
}

// Save persists inbox to disk.
func (in *Inbox) Save(path string) error {
	in.mu.RLock()
	defer in.mu.RUnlock()

	data, err := json.MarshalIndent(in.messages, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

// Load restores inbox from disk.
func (in *Inbox) Load(path string) error {
	in.mu.Lock()
	defer in.mu.Unlock()

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	return json.Unmarshal(data, &in.messages)
}
