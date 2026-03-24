// Package iskramobile provides gomobile bindings for the Iskra messenger.
// Exported functions are callable from Kotlin/Java via the generated .aar.
package iskramobile

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"

	// gomobile bind requires x/mobile/bind in go.mod
	_ "golang.org/x/mobile/bind"

	"github.com/iskra-messenger/iskra/internal/identity"
	"github.com/iskra-messenger/iskra/internal/mesh"
	"github.com/iskra-messenger/iskra/internal/message"
	"github.com/iskra-messenger/iskra/internal/security"
	"github.com/iskra-messenger/iskra/internal/store"
	"github.com/iskra-messenger/iskra/internal/web"
)

var (
	server    *web.Server
	serverMu  sync.Mutex
	serverPort int
)

// Start initializes and starts the Iskra node.
// dataDir: path to app's internal storage (e.g., filesDir).
// port: HTTP port for WebView (0 = random).
// Returns the actual port number.
func Start(dataDir string, port int) int {
	serverMu.Lock()
	defer serverMu.Unlock()

	if server != nil {
		return serverPort
	}

	log.SetOutput(os.Stderr)

	os.MkdirAll(dataDir, 0700)

	// Load or create keypair
	keypair, mnemonic := loadOrCreateKeypairMobile(dataDir)

	// Initialize store
	hold, err := store.NewHold(filepath.Join(dataDir, "hold"))
	if err != nil {
		log.Printf("Failed to create hold: %v", err)
		return 0
	}
	bloom := store.NewBloom(1000000, 0.001)
	contacts, err := store.NewContacts(filepath.Join(dataDir, "contacts.json"))
	if err != nil {
		log.Printf("Failed to load contacts: %v", err)
		return 0
	}
	inbox, err := store.NewInbox(filepath.Join(dataDir, "inbox"))
	if err != nil {
		log.Printf("Failed to create inbox: %v", err)
		return 0
	}
	inbox.Load(filepath.Join(dataDir, "inbox.json"))

	groups, err := store.NewGroups(filepath.Join(dataDir, "groups.json"))
	if err != nil {
		log.Printf("Failed to load groups: %v", err)
		return 0
	}

	// Initialize mesh
	peers := mesh.NewPeerList()
	transport := mesh.NewTransport(keypair.Ed25519Pub, 0, peers)
	if err := transport.Start(); err != nil {
		log.Printf("Failed to start transport: %v", err)
		return 0
	}

	// Relay — always connect to default
	relayURL := "wss://iskra-relay.onrender.com/ws"
	relayClient := mesh.NewRelayClient(relayURL, keypair.Ed25519Pub, keypair.X25519Pub)
	mode := "relay"

	// Get seed for vault key derivation
	seedData, _ := os.ReadFile(filepath.Join(dataDir, "seed.key"))
	var seed [32]byte
	if len(seedData) == 32 {
		copy(seed[:], seedData)
	}

	hasPIN := security.HasPIN(dataDir)
	locked := hasPIN
	unlockCh := make(chan struct{})
	if !locked {
		close(unlockCh)
	}

	// API
	api := &web.API{
		Keypair:     keypair,
		Mnemonic:    mnemonic,
		Contacts:    contacts,
		Inbox:       inbox,
		Hold:        hold,
		Bloom:       bloom,
		Peers:       peers,
		Transport:   transport,
		RelayClient: relayClient,
		Groups:      groups,
		Mode:        mode,
		DataDir:     dataDir,
		Seed:        seed,
		Locked:      locked,
		UnlockCh:    unlockCh,
	}

	// Message handler
	handleMessage := func(msg *message.Message) {
		if bloom.Contains(msg.ID) {
			return
		}
		bloom.Add(msg.ID)
		api.HandleIncomingMessage(msg)
		if !msg.IsForRecipient(keypair.Ed25519Pub) {
			hold.Store(msg)
		}
	}

	transport.SetOnMessage(handleMessage)
	relayClient.SetOnMessage(handleMessage)
	relayClient.Start()

	// LAN discovery
	discovery := mesh.NewDiscovery(keypair.Ed25519Pub, transport.Port(), peers)
	discovery.SetOnPeer(func(pubKey [32]byte, ip string, peerPort uint16) {
		go func() {
			holdMsgs, _ := hold.GetAll()
			transport.ConnectAndSync(ip, peerPort, bloom.Export(), holdMsgs)
		}()
	})
	discovery.Start()

	// Web server
	server = web.NewServer(api, port)
	if err := server.Start(); err != nil {
		log.Printf("Failed to start web server: %v", err)
		return 0
	}

	serverPort = server.Port()
	fmt.Printf("Iskra mobile started on port %d\n", serverPort)
	return serverPort
}

// GetPort returns the HTTP port the server is listening on.
func GetPort() int {
	serverMu.Lock()
	defer serverMu.Unlock()
	return serverPort
}

// Stop gracefully shuts down the node.
func Stop() {
	serverMu.Lock()
	defer serverMu.Unlock()
	if server != nil {
		server.Stop()
		server = nil
		serverPort = 0
	}
}

func loadOrCreateKeypairMobile(dataDir string) (*identity.Keypair, []string) {
	seedPath := filepath.Join(dataDir, "seed.key")
	data, err := os.ReadFile(seedPath)
	if err == nil && len(data) == 32 {
		var seed [32]byte
		copy(seed[:], data)
		kp := identity.KeypairFromSeed(seed)
		return kp, identity.SeedToMnemonic(seed)
	}

	seed, err := identity.GenerateMnemonicSeed()
	if err != nil {
		log.Fatalf("Failed to generate seed: %v", err)
	}
	os.WriteFile(seedPath, seed[:], 0600)
	kp := identity.KeypairFromSeed(seed)
	return kp, identity.SeedToMnemonic(seed)
}
