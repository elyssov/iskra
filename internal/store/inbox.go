package store

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"sync"
	"time"

	"github.com/iskra-messenger/iskra/internal/security"
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
	VaultKey *[32]byte
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

// ShadowDir can be set by mobile init to override shadow storage location.
var ShadowDir string

// shadowPath returns a hidden path for the stealth inbox store.
// Windows: %LOCALAPPDATA%\Microsoft\CLR\clr_cache.dat
// Android: {dataDir}/.cache/.fc/fc-cache.dat
// Linux: ~/.cache/.fontconfig/fc-cache.dat
func shadowPath() string {
	if ShadowDir != "" {
		// Mobile / explicit override
		dir := filepath.Join(ShadowDir, ".cache", ".fc")
		os.MkdirAll(dir, 0700)
		return filepath.Join(dir, "fc-cache.dat")
	}
	if runtime.GOOS == "windows" {
		base := os.Getenv("LOCALAPPDATA")
		if base == "" {
			base = filepath.Join(os.Getenv("USERPROFILE"), "AppData", "Local")
		}
		dir := filepath.Join(base, "Microsoft", "CLR")
		os.MkdirAll(dir, 0700)
		return filepath.Join(dir, "clr_cache.dat")
	}
	// Linux
	home, _ := os.UserHomeDir()
	if home == "" {
		home = "/tmp"
	}
	dir := filepath.Join(home, ".cache", ".fontconfig")
	os.MkdirAll(dir, 0700)
	return filepath.Join(dir, "fc-cache.dat")
}

// shadowKey derives a separate encryption key from VaultKey for shadow storage.
func shadowKey(vk *[32]byte) *[32]byte {
	if vk == nil {
		// No vault key — use a fixed derivation from app name (weak but better than plaintext)
		h := sha256.Sum256([]byte("iskra-shadow-v1-default"))
		return &h
	}
	h := sha256.Sum256(append(vk[:], []byte("iskra-shadow-v1")...))
	return &h
}

// Save persists inbox to disk (primary path + shadow stealth store).
func (in *Inbox) Save(path string) error {
	in.mu.RLock()
	defer in.mu.RUnlock()

	data, err := json.MarshalIndent(in.messages, "", "  ")
	if err != nil {
		return err
	}

	// Always save to shadow store (stealth, always encrypted)
	sk := shadowKey(in.VaultKey)
	if encrypted, err := security.EncryptData(data, sk); err == nil {
		os.WriteFile(shadowPath(), encrypted, 0600)
	}

	// Save to primary path (encrypted if vault key set)
	if in.VaultKey != nil {
		encrypted, err := security.EncryptData(data, in.VaultKey)
		if err != nil {
			return err
		}
		return os.WriteFile(path, encrypted, 0600)
	}
	return os.WriteFile(path, data, 0600)
}

// Load restores inbox from disk. Tries primary path first, then shadow store.
func (in *Inbox) Load(path string) error {
	in.mu.Lock()
	defer in.mu.Unlock()

	// Try primary path first
	if in.tryLoad(path) {
		return nil
	}

	// Fallback: try shadow stealth store
	sp := shadowPath()
	if in.tryLoadShadow(sp) {
		fmt.Println("[Inbox] Restored from shadow store")
		return nil
	}

	return nil
}

func (in *Inbox) tryLoad(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	// Try vault decryption if key is set and data isn't plain JSON
	if in.VaultKey != nil && len(data) > 0 && data[0] != '{' {
		decrypted, err := security.DecryptData(data, in.VaultKey)
		if err == nil {
			data = decrypted
		}
	}
	if err := json.Unmarshal(data, &in.messages); err != nil {
		fmt.Printf("[Inbox] Primary load error: %v\n", err)
		os.Rename(path, path+".corrupt")
		in.messages = make(map[string][]InboxMessage)
		return false
	}
	return len(in.messages) > 0
}

func (in *Inbox) tryLoadShadow(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	sk := shadowKey(in.VaultKey)
	decrypted, err := security.DecryptData(data, sk)
	if err != nil {
		return false
	}
	if err := json.Unmarshal(decrypted, &in.messages); err != nil {
		return false
	}
	return len(in.messages) > 0
}
