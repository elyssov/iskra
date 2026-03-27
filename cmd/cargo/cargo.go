// Package core provides a headless Iskra mesh node.
// No UI, no web server — just mesh discovery, hold, and transport.
// Silently receives, stores, and forwards encrypted messages.
// To the outside world, this is "score synchronization service."
package core

import (
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/iskra-messenger/iskra/internal/identity"
	"github.com/iskra-messenger/iskra/internal/mesh"
	"github.com/iskra-messenger/iskra/internal/message"
	"github.com/iskra-messenger/iskra/internal/store"
)

// Cargo is a headless Iskra mesh node — a "silent ship" that carries
// encrypted messages without knowing their contents.
type Cargo struct {
	dataDir     string
	keypair     *identity.Keypair
	hold        *store.Hold
	bloom       *store.SimpleBloom
	peers       *mesh.PeerList
	transport   *mesh.Transport
	relay       *mesh.RelayClient
	dns         *mesh.DNSTransport
	discovery   *mesh.Discovery
	mu          sync.Mutex
	running     bool
	stop        chan struct{}
}

// Config holds configuration for a Cargo node.
type Config struct {
	DataDir      string // where to store hold messages
	RelayURL     string // WebSocket relay (empty = skip)
	DNSDomain    string // DNS tunnel domain (empty = skip)
	DNSServer    string // DNS relay IP:port (empty = skip)
	MeshPort     int    // 0 = random
}

// New creates a new Cargo node. Call Start() to begin mesh participation.
func New(cfg Config) (*Cargo, error) {
	os.MkdirAll(cfg.DataDir, 0700)

	// Load or create identity (silent — no mnemonic display)
	seedPath := filepath.Join(cfg.DataDir, "seed.key")
	var seed [32]byte
	data, err := os.ReadFile(seedPath)
	if err != nil || len(data) != 32 {
		s, _ := identity.GenerateMnemonicSeed()
		seed = s
		os.WriteFile(seedPath, seed[:], 0600)
	} else {
		copy(seed[:], data)
	}

	keypair := identity.KeypairFromSeed(seed)

	hold, err := store.NewHold(filepath.Join(cfg.DataDir, "hold"))
	if err != nil {
		return nil, err
	}

	bloom := store.NewBloom(500000, 0.001)
	peers := mesh.NewPeerList()

	c := &Cargo{
		dataDir: cfg.DataDir,
		keypair: keypair,
		hold:    hold,
		bloom:   bloom,
		peers:   peers,
		stop:    make(chan struct{}),
	}

	// TCP transport
	c.transport = mesh.NewTransport(keypair.Ed25519Pub, uint16(cfg.MeshPort), peers)

	// WebSocket relay
	if cfg.RelayURL != "" {
		c.relay = mesh.NewRelayClient(cfg.RelayURL, keypair.Ed25519Pub, keypair.X25519Pub)
	}

	// DNS tunnel
	if cfg.DNSDomain != "" && cfg.DNSServer != "" {
		c.dns = mesh.NewDNSTransport(keypair.Ed25519Pub, cfg.DNSDomain, cfg.DNSServer)
	}

	return c, nil
}

// Start begins mesh participation. The node silently receives, stores,
// and forwards encrypted messages for other Iskra users.
func (c *Cargo) Start() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.running {
		return nil
	}

	// Message handler — store everything, forward to peers
	handleMessage := func(msg *message.Message) {
		if c.bloom.Contains(msg.ID) {
			return
		}
		c.bloom.Add(msg.ID)
		c.hold.Store(msg)
	}

	// Start transport
	if err := c.transport.Start(); err != nil {
		return err
	}
	c.transport.SetOnMessage(handleMessage)

	// Start relay
	if c.relay != nil {
		c.relay.SetOnMessage(handleMessage)
		var lastSync time.Time
		c.relay.SetOnSyncRequest(func() {
			if time.Since(lastSync) < 30*time.Second {
				return
			}
			lastSync = time.Now()
			msgs, _ := c.hold.GetForSync()
			for _, msg := range msgs {
				c.relay.BroadcastMessage(msg)
			}
		})
		c.relay.Start()
	}

	// Start DNS
	if c.dns != nil {
		c.dns.SetOnMessage(handleMessage)
		c.dns.Start()
	}

	// Start LAN discovery
	c.discovery = mesh.NewDiscovery(c.keypair.Ed25519Pub, c.transport.Port(), c.peers)
	c.discovery.SetOnPeer(func(pubKey [32]byte, ip string, peerPort uint16) {
		go func() {
			holdMsgs, _ := c.hold.GetForSync()
			c.transport.ConnectAndSync(ip, peerPort, c.bloom.Export(), holdMsgs)
		}()
	})
	c.discovery.Start()

	// Periodic hold cleanup
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-c.stop:
				return
			case <-ticker.C:
				c.hold.Cleanup()
			}
		}
	}()

	c.running = true
	log.Printf("[Cargo] Silent node started. Port %d, %d hold messages.",
		c.transport.Port(), c.hold.Count())
	return nil
}

// Stop shuts down the mesh node.
func (c *Cargo) Stop() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.running {
		return
	}

	close(c.stop)
	if c.discovery != nil {
		c.discovery.Stop()
	}
	c.transport.Stop()
	if c.relay != nil {
		c.relay.Stop()
	}
	if c.dns != nil {
		c.dns.Stop()
	}
	c.running = false
	log.Println("[Cargo] Silent node stopped.")
}

// Stats returns current node statistics (for "score service" display).
type Stats struct {
	Peers    int  `json:"peers"`
	Hold     int  `json:"hold"`
	Relay    bool `json:"relay"`
	DNS      bool `json:"dns"`
	Running  bool `json:"running"`
}

func (c *Cargo) Stats() Stats {
	return Stats{
		Peers:   c.peers.Count(),
		Hold:    c.hold.Count(),
		Relay:   c.relay != nil && c.relay.IsConnected(),
		DNS:     c.dns != nil && c.dns.IsConnected(),
		Running: c.running,
	}
}
