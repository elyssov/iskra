package store

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"sync"
	"time"

	"github.com/iskra-messenger/iskra/internal/security"
)

// Channel represents a broadcast channel (one author, many readers).
type Channel struct {
	ID        string `json:"id"`         // = author's UserID
	AuthorPub string `json:"author_pub"` // base58(Ed25519 pubkey)
	X25519Pub string `json:"x25519_pub"` // base58(X25519 pubkey) for encrypted replies
	Title     string `json:"title"`
	CreatedAt int64  `json:"created_at"`
	IsOwner   bool   `json:"is_owner"` // true if current user created this channel
}

// ChannelPost is a single broadcast message.
type ChannelPost struct {
	ID        string `json:"id"`
	ChannelID string `json:"channel_id"`
	From      string `json:"from"`
	FromName  string `json:"from_name"`
	Text      string `json:"text"`
	Timestamp int64  `json:"timestamp"`
	Outgoing  bool   `json:"outgoing"`
}

// Channels manages channel subscriptions and posts.
type Channels struct {
	mu       sync.RWMutex
	path     string
	channels []Channel
	posts    map[string][]ChannelPost // channelID -> posts
	VaultKey *[32]byte
}

// NewChannels creates or loads a channels store.
func NewChannels(path string) (*Channels, error) {
	ch := &Channels{
		path:  path,
		posts: make(map[string][]ChannelPost),
	}
	if err := ch.load(); err != nil && !os.IsNotExist(err) {
		if data, readErr := os.ReadFile(path); readErr == nil && len(data) > 0 && data[0] != '{' && data[0] != '[' {
			fmt.Printf("[Channels] Encrypted, deferring load until PIN\n")
		} else {
			fmt.Printf("[Channels] Load error (starting fresh): %v\n", err)
			os.Rename(path, path+".corrupt")
		}
	}
	return ch, nil
}

// Create creates a new channel owned by the current user.
func (ch *Channels) Create(userID, authorPub, x25519Pub, title string) Channel {
	ch.mu.Lock()
	defer ch.mu.Unlock()

	c := Channel{
		ID:        userID,
		AuthorPub: authorPub,
		X25519Pub: x25519Pub,
		Title:     title,
		CreatedAt: time.Now().Unix(),
		IsOwner:   true,
	}
	// Replace if exists
	for i, existing := range ch.channels {
		if existing.ID == userID {
			ch.channels[i] = c
			ch.save()
			return c
		}
	}
	ch.channels = append(ch.channels, c)
	ch.save()
	return c
}

// Subscribe adds a channel to the subscription list.
func (ch *Channels) Subscribe(c Channel) {
	ch.mu.Lock()
	defer ch.mu.Unlock()

	for _, existing := range ch.channels {
		if existing.ID == c.ID {
			return // already subscribed
		}
	}
	ch.channels = append(ch.channels, c)
	ch.save()
}

// Unsubscribe removes a channel.
func (ch *Channels) Unsubscribe(channelID string) {
	ch.mu.Lock()
	defer ch.mu.Unlock()

	for i, c := range ch.channels {
		if c.ID == channelID {
			ch.channels = append(ch.channels[:i], ch.channels[i+1:]...)
			delete(ch.posts, channelID)
			ch.save()
			return
		}
	}
}

// Get returns a channel by ID.
func (ch *Channels) Get(channelID string) (Channel, bool) {
	ch.mu.RLock()
	defer ch.mu.RUnlock()

	for _, c := range ch.channels {
		if c.ID == channelID {
			return c, true
		}
	}
	return Channel{}, false
}

// List returns all channels.
func (ch *Channels) List() []Channel {
	ch.mu.RLock()
	defer ch.mu.RUnlock()

	result := make([]Channel, len(ch.channels))
	copy(result, ch.channels)
	return result
}

// AddPost adds a post to a channel.
func (ch *Channels) AddPost(post ChannelPost) {
	ch.mu.Lock()
	defer ch.mu.Unlock()

	posts := ch.posts[post.ChannelID]
	// Deduplicate
	for _, p := range posts {
		if p.ID == post.ID {
			return
		}
	}
	posts = append(posts, post)
	sort.Slice(posts, func(i, j int) bool {
		return posts[i].Timestamp < posts[j].Timestamp
	})
	ch.posts[post.ChannelID] = posts
}

// Posts returns all posts for a channel.
func (ch *Channels) Posts(channelID string) []ChannelPost {
	ch.mu.RLock()
	defer ch.mu.RUnlock()

	posts := ch.posts[channelID]
	result := make([]ChannelPost, len(posts))
	copy(result, posts)
	return result
}

// LastPost returns the most recent post for a channel.
func (ch *Channels) LastPost(channelID string) (ChannelPost, bool) {
	ch.mu.RLock()
	defer ch.mu.RUnlock()

	posts := ch.posts[channelID]
	if len(posts) == 0 {
		return ChannelPost{}, false
	}
	return posts[len(posts)-1], true
}

type channelsData struct {
	Channels []Channel                `json:"channels"`
	Posts    map[string][]ChannelPost `json:"posts"`
}

func (ch *Channels) load() error {
	data, err := os.ReadFile(ch.path)
	if err != nil {
		return err
	}
	if ch.VaultKey != nil && len(data) > 0 && data[0] != '{' {
		decrypted, err := security.DecryptData(data, ch.VaultKey)
		if err == nil {
			data = decrypted
		}
	}
	var d channelsData
	if err := json.Unmarshal(data, &d); err != nil {
		return err
	}
	ch.channels = d.Channels
	if d.Posts != nil {
		ch.posts = d.Posts
	}
	return nil
}

func (ch *Channels) save() error {
	d := channelsData{
		Channels: ch.channels,
		Posts:    ch.posts,
	}
	data, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		return err
	}
	if ch.VaultKey != nil {
		encrypted, err := security.EncryptData(data, ch.VaultKey)
		if err == nil {
			data = encrypted
		}
	}
	return os.WriteFile(ch.path, data, 0600)
}

// Save persists channels to disk (public, for auto-save).
func (ch *Channels) Save() error {
	ch.mu.Lock()
	defer ch.mu.Unlock()
	return ch.save()
}
