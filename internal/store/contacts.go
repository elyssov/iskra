package store

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/iskra-messenger/iskra/internal/identity"
)

// Contact represents a known user.
type Contact struct {
	Name       string `json:"name"`
	PubKey     string `json:"pubkey"`      // Base58-encoded Ed25519 public key
	X25519Pub  string `json:"x25519_pub"`  // Base58-encoded X25519 public key
	UserID     string `json:"user_id"`
	AddedAt    int64  `json:"added_at"`
	LastSeen   int64  `json:"last_seen"`
}

// Contacts manages the contact list.
type Contacts struct {
	path     string
	contacts []Contact
	mu       sync.RWMutex
}

// NewContacts creates or loads a contacts store.
func NewContacts(path string) (*Contacts, error) {
	c := &Contacts{path: path}
	if err := c.load(); err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	return c, nil
}

// Add adds a new contact.
func (c *Contacts) Add(name string, edPub [32]byte, x25519Pub [32]byte) {
	c.mu.Lock()
	defer c.mu.Unlock()

	userID := identity.UserID(edPub)

	// Check for duplicate
	for _, existing := range c.contacts {
		if existing.UserID == userID {
			return
		}
	}

	c.contacts = append(c.contacts, Contact{
		Name:      name,
		PubKey:    identity.ToBase58(edPub[:]),
		X25519Pub: identity.ToBase58(x25519Pub[:]),
		UserID:    userID,
		AddedAt:   time.Now().Unix(),
	})
	c.save()
}

// List returns all contacts.
func (c *Contacts) List() []Contact {
	c.mu.RLock()
	defer c.mu.RUnlock()

	result := make([]Contact, len(c.contacts))
	copy(result, c.contacts)
	return result
}

// GetByUserID finds a contact by UserID.
func (c *Contacts) GetByUserID(userID string) *Contact {
	c.mu.RLock()
	defer c.mu.RUnlock()

	for i := range c.contacts {
		if c.contacts[i].UserID == userID {
			ct := c.contacts[i]
			return &ct
		}
	}
	return nil
}

// UpdateLastSeen updates the last seen timestamp for a contact.
func (c *Contacts) UpdateLastSeen(userID string, timestamp int64) {
	c.mu.Lock()
	defer c.mu.Unlock()

	for i := range c.contacts {
		if c.contacts[i].UserID == userID {
			c.contacts[i].LastSeen = timestamp
			c.save()
			return
		}
	}
}

// Import loads contacts from a JSON file (Iskra-Most format).
// Expected format: [{"name": "...", "publicKey": "...", "x25519Key": "..."}]
func (c *Contacts) Import(filePath string) error {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read import file: %w", err)
	}

	var imported []struct {
		Name       string `json:"name"`
		PublicKey  string `json:"publicKey"`
		X25519Key  string `json:"x25519Key"`
	}

	if err := json.Unmarshal(data, &imported); err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	for _, imp := range imported {
		edPubBytes, err := identity.FromBase58(imp.PublicKey)
		if err != nil || len(edPubBytes) != 32 {
			continue
		}
		var edPub [32]byte
		copy(edPub[:], edPubBytes)

		var x25519Pub [32]byte
		if imp.X25519Key != "" {
			x25519Bytes, err := identity.FromBase58(imp.X25519Key)
			if err == nil && len(x25519Bytes) == 32 {
				copy(x25519Pub[:], x25519Bytes)
			}
		}

		userID := identity.UserID(edPub)

		// Skip duplicates
		dup := false
		for _, existing := range c.contacts {
			if existing.UserID == userID {
				dup = true
				break
			}
		}
		if dup {
			continue
		}

		c.contacts = append(c.contacts, Contact{
			Name:      imp.Name,
			PubKey:    imp.PublicKey,
			X25519Pub: identity.ToBase58(x25519Pub[:]),
			UserID:    userID,
			AddedAt:   time.Now().Unix(),
		})
	}

	return c.save()
}

func (c *Contacts) load() error {
	data, err := os.ReadFile(c.path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, &c.contacts)
}

func (c *Contacts) save() error {
	data, err := json.MarshalIndent(c.contacts, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(c.path, data, 0600)
}
