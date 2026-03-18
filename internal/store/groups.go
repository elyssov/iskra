package store

import (
	"crypto/rand"
	"encoding/json"
	"os"
	"sort"
	"sync"
	"time"
)

// GroupMember represents a member of a group chat.
type GroupMember struct {
	UserID    string `json:"user_id"`
	Name      string `json:"name"`
	PubKey    string `json:"pubkey"`
	X25519Pub string `json:"x25519_pub"`
}

// Group represents a group chat.
type Group struct {
	ID        string        `json:"id"`        // Hex-encoded 32-byte random ID
	Name      string        `json:"name"`
	Members   []GroupMember `json:"members"`
	CreatedAt int64         `json:"created_at"`
	CreatedBy string        `json:"created_by"` // UserID of creator
}

// GroupMessage is a message in a group chat.
type GroupMessage struct {
	ID        string `json:"id"`
	GroupID   string `json:"group_id"`
	From      string `json:"from"`      // UserID of sender
	FromName  string `json:"from_name"` // Display name of sender
	Text      string `json:"text"`
	Timestamp int64  `json:"timestamp"`
	Outgoing  bool   `json:"outgoing"`
}

// Groups manages group chats.
type Groups struct {
	path     string
	groups   []Group
	messages map[string][]GroupMessage // keyed by group ID
	mu       sync.RWMutex
}

// NewGroups creates or loads a groups store.
func NewGroups(path string) (*Groups, error) {
	g := &Groups{
		path:     path,
		messages: make(map[string][]GroupMessage),
	}
	if err := g.load(); err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	return g, nil
}

// Create creates a new group and returns it.
func (g *Groups) Create(name string, createdBy string, members []GroupMember) *Group {
	g.mu.Lock()
	defer g.mu.Unlock()

	var idBytes [32]byte
	rand.Read(idBytes[:])

	group := Group{
		ID:        hexEncode(idBytes[:]),
		Name:      name,
		Members:   members,
		CreatedAt: time.Now().Unix(),
		CreatedBy: createdBy,
	}
	g.groups = append(g.groups, group)
	g.save()
	return &group
}

// List returns all groups.
func (g *Groups) List() []Group {
	g.mu.RLock()
	defer g.mu.RUnlock()

	result := make([]Group, len(g.groups))
	copy(result, g.groups)
	return result
}

// Get returns a group by ID.
func (g *Groups) Get(groupID string) *Group {
	g.mu.RLock()
	defer g.mu.RUnlock()

	for i := range g.groups {
		if g.groups[i].ID == groupID {
			gr := g.groups[i]
			return &gr
		}
	}
	return nil
}

// AddByInvite adds a group received via invite.
func (g *Groups) AddByInvite(group Group) {
	g.mu.Lock()
	defer g.mu.Unlock()

	for _, existing := range g.groups {
		if existing.ID == group.ID {
			return
		}
	}
	g.groups = append(g.groups, group)
	g.save()
}

// Delete removes a group.
func (g *Groups) Delete(groupID string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()

	for i := range g.groups {
		if g.groups[i].ID == groupID {
			g.groups = append(g.groups[:i], g.groups[i+1:]...)
			delete(g.messages, groupID)
			g.save()
			return true
		}
	}
	return false
}

// AddMessage stores a group message.
func (g *Groups) AddMessage(msg GroupMessage) {
	g.mu.Lock()
	defer g.mu.Unlock()

	// Check duplicate
	for _, existing := range g.messages[msg.GroupID] {
		if existing.ID == msg.ID {
			return
		}
	}

	if msg.Timestamp == 0 {
		msg.Timestamp = time.Now().Unix()
	}

	g.messages[msg.GroupID] = append(g.messages[msg.GroupID], msg)

	sort.Slice(g.messages[msg.GroupID], func(i, j int) bool {
		return g.messages[msg.GroupID][i].Timestamp < g.messages[msg.GroupID][j].Timestamp
	})
}

// GetMessages returns message history for a group.
func (g *Groups) GetMessages(groupID string) []GroupMessage {
	g.mu.RLock()
	defer g.mu.RUnlock()

	msgs := g.messages[groupID]
	result := make([]GroupMessage, len(msgs))
	copy(result, msgs)
	return result
}

// DeleteMessages removes all messages for a group.
func (g *Groups) DeleteMessages(groupID string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	delete(g.messages, groupID)
}

// Save persists groups and messages to disk.
func (g *Groups) Save() error {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.save()
}

func (g *Groups) load() error {
	data, err := os.ReadFile(g.path)
	if err != nil {
		return err
	}
	var stored struct {
		Groups   []Group                  `json:"groups"`
		Messages map[string][]GroupMessage `json:"messages"`
	}
	if err := json.Unmarshal(data, &stored); err != nil {
		return err
	}
	g.groups = stored.Groups
	if stored.Messages != nil {
		g.messages = stored.Messages
	}
	return nil
}

func (g *Groups) save() error {
	stored := struct {
		Groups   []Group                  `json:"groups"`
		Messages map[string][]GroupMessage `json:"messages"`
	}{
		Groups:   g.groups,
		Messages: g.messages,
	}
	data, err := json.MarshalIndent(stored, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(g.path, data, 0600)
}

func hexEncode(b []byte) string {
	const hex = "0123456789abcdef"
	s := make([]byte, len(b)*2)
	for i, v := range b {
		s[i*2] = hex[v>>4]
		s[i*2+1] = hex[v&0x0f]
	}
	return string(s)
}
