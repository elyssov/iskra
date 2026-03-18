package mesh

import (
	"sync"
	"time"
)

// Peer represents a discovered node.
type Peer struct {
	PubKey    [32]byte
	IP        string
	Port      uint16
	LastSeen  time.Time
	Connected bool
}

// PeerList manages known peers.
type PeerList struct {
	peers map[[32]byte]*Peer
	mu    sync.RWMutex
}

// NewPeerList creates a new peer list.
func NewPeerList() *PeerList {
	return &PeerList{
		peers: make(map[[32]byte]*Peer),
	}
}

// AddOrUpdate adds a new peer or updates an existing one.
func (pl *PeerList) AddOrUpdate(pubKey [32]byte, ip string, port uint16) {
	pl.mu.Lock()
	defer pl.mu.Unlock()

	if p, ok := pl.peers[pubKey]; ok {
		p.IP = ip
		p.Port = port
		p.LastSeen = time.Now()
	} else {
		pl.peers[pubKey] = &Peer{
			PubKey:   pubKey,
			IP:       ip,
			Port:     port,
			LastSeen: time.Now(),
		}
	}
}

// Get returns a peer by public key.
func (pl *PeerList) Get(pubKey [32]byte) *Peer {
	pl.mu.RLock()
	defer pl.mu.RUnlock()

	if p, ok := pl.peers[pubKey]; ok {
		cp := *p
		return &cp
	}
	return nil
}

// All returns all peers.
func (pl *PeerList) All() []*Peer {
	pl.mu.RLock()
	defer pl.mu.RUnlock()

	result := make([]*Peer, 0, len(pl.peers))
	for _, p := range pl.peers {
		cp := *p
		result = append(result, &cp)
	}
	return result
}

// SetConnected marks a peer as connected/disconnected.
func (pl *PeerList) SetConnected(pubKey [32]byte, connected bool) {
	pl.mu.Lock()
	defer pl.mu.Unlock()

	if p, ok := pl.peers[pubKey]; ok {
		p.Connected = connected
	}
}

// Count returns the number of known peers.
func (pl *PeerList) Count() int {
	pl.mu.RLock()
	defer pl.mu.RUnlock()
	return len(pl.peers)
}

// RemoveStale removes peers not seen in the given duration.
func (pl *PeerList) RemoveStale(maxAge time.Duration) {
	pl.mu.Lock()
	defer pl.mu.Unlock()

	cutoff := time.Now().Add(-maxAge)
	for k, p := range pl.peers {
		if p.LastSeen.Before(cutoff) {
			delete(pl.peers, k)
		}
	}
}
