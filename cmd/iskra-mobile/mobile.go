// Package iskramobile provides gomobile bindings for the Iskra messenger.
// Exported functions are callable from Kotlin/Java via the generated .aar.
package iskramobile

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

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
	server       *web.Server
	serverMu     sync.Mutex
	serverPort   int
	mobileInbox  *store.Inbox
	mobileGroups *store.Groups
	mobileDataDir string
	autoSaveStop chan struct{}
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
	locked := true
	_ = hasPIN
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

	// Save references for persistence on Stop
	mobileInbox = inbox
	mobileGroups = groups
	mobileDataDir = dataDir

	// Start auto-save goroutine (every 10 seconds)
	autoSaveStop = make(chan struct{})
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[Mobile] Auto-save goroutine panic: %v", r)
			}
		}()
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				serverMu.Lock()
				if mobileInbox != nil && mobileDataDir != "" {
					if err := mobileInbox.Save(filepath.Join(mobileDataDir, "inbox.json")); err != nil {
						log.Printf("[Mobile] Auto-save inbox error: %v", err)
					}
				}
				if mobileGroups != nil {
					if err := mobileGroups.Save(); err != nil {
						log.Printf("[Mobile] Auto-save groups error: %v", err)
					}
				}
				serverMu.Unlock()
			case <-autoSaveStop:
				return
			}
		}
	}()

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

	// Stop auto-save
	if autoSaveStop != nil {
		select {
		case <-autoSaveStop:
		default:
			close(autoSaveStop)
		}
	}

	// Save data before shutdown
	if mobileInbox != nil && mobileDataDir != "" {
		mobileInbox.Save(filepath.Join(mobileDataDir, "inbox.json"))
		log.Println("[Mobile] Inbox saved on stop")
	}
	if mobileGroups != nil {
		mobileGroups.Save()
		log.Println("[Mobile] Groups saved on stop")
	}

	if server != nil {
		server.Stop()
		server = nil
		serverPort = 0
	}
	mobileInbox = nil
	mobileGroups = nil
	mobileDataDir = ""
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
