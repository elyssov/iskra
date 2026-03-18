package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/iskra-messenger/iskra/internal/identity"
	"github.com/iskra-messenger/iskra/internal/mesh"
	"github.com/iskra-messenger/iskra/internal/message"
	"github.com/iskra-messenger/iskra/internal/store"
	"github.com/iskra-messenger/iskra/internal/web"
)

func main() {
	port := flag.Int("port", 0, "HTTP port for UI (0 = random)")
	dataDir := flag.String("data", defaultDataDir(), "Data directory")
	debug := flag.Bool("debug", false, "Enable debug logging")
	meshPort := flag.Int("mesh-port", 0, "Mesh transport port (0 = random)")
	relayURL := flag.String("relay", "wss://iskra-relay.onrender.com/ws", "Relay server URL (wss://host/ws)")
	restore := flag.String("restore", "", "Restore from mnemonic (24 words, space-separated)")
	flag.Parse()

	if !*debug {
		log.SetOutput(os.Stderr)
	}

	// Handle mnemonic restore
	if *restore != "" {
		restoreFromMnemonic(*dataDir, *restore)
	}

	// Ensure data directory exists
	os.MkdirAll(*dataDir, 0700)

	// Load or create keypair
	keypair, mnemonic, isNew := loadOrCreateKeypair(*dataDir)
	userID := identity.UserID(keypair.Ed25519Pub)

	if isNew {
		fmt.Println("╔══════════════════════════════════════════╗")
		fmt.Println("║          🔥 ИСКРА — Новый ключ          ║")
		fmt.Println("╠══════════════════════════════════════════╣")
		fmt.Printf("║ ID: %-36s ║\n", userID)
		fmt.Println("╠══════════════════════════════════════════╣")
		fmt.Println("║ Мнемоника (ЗАПИШИТЕ И СОХРАНИТЕ!):       ║")
		for i := 0; i < 24; i += 4 {
			fmt.Printf("║  %2d. %-8s %2d. %-8s %2d. %-8s %2d. %-8s\n",
				i+1, mnemonic[i], i+2, mnemonic[i+1],
				i+3, mnemonic[i+2], i+4, mnemonic[i+3])
		}
		fmt.Println("╚══════════════════════════════════════════╝")
	}

	// Initialize store
	hold, err := store.NewHold(filepath.Join(*dataDir, "hold"))
	if err != nil {
		log.Fatalf("Failed to create hold: %v", err)
	}
	bloom := store.NewBloom(1000000, 0.001)
	contacts, err := store.NewContacts(filepath.Join(*dataDir, "contacts.json"))
	if err != nil {
		log.Fatalf("Failed to load contacts: %v", err)
	}
	inbox, err := store.NewInbox(filepath.Join(*dataDir, "inbox"))
	if err != nil {
		log.Fatalf("Failed to create inbox: %v", err)
	}
	inbox.Load(filepath.Join(*dataDir, "inbox.json"))

	// Initialize mesh
	peers := mesh.NewPeerList()
	transport := mesh.NewTransport(keypair.Ed25519Pub, uint16(*meshPort), peers)
	if err := transport.Start(); err != nil {
		log.Fatalf("Failed to start transport: %v", err)
	}

	// Determine mode
	mode := "lan"

	// Initialize relay if specified
	var relayClient *mesh.RelayClient
	if *relayURL != "" {
		relayClient = mesh.NewRelayClient(*relayURL, keypair.Ed25519Pub)
		mode = "relay"
	}

	// Initialize API
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
		Mode:        mode,
	}

	// Message handler (shared between transport and relay)
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

	// Set message handlers
	transport.SetOnMessage(handleMessage)
	if relayClient != nil {
		relayClient.SetOnMessage(handleMessage)
		if err := relayClient.Start(); err != nil {
			log.Printf("Relay: will retry in background")
		}
	}

	// Start LAN discovery
	discovery := mesh.NewDiscovery(keypair.Ed25519Pub, transport.Port(), peers)
	discovery.SetOnPeer(func(pubKey [32]byte, ip string, peerPort uint16) {
		if *debug {
			log.Printf("Discovered peer: %s:%d", ip, peerPort)
		}
		go func() {
			holdMsgs, _ := hold.GetAll()
			transport.ConnectAndSync(ip, peerPort, bloom.Export(), holdMsgs)
		}()
	})
	discovery.Start()

	// Start web server
	server := web.NewServer(api, *port)
	if err := server.Start(); err != nil {
		log.Fatalf("Failed to start web server: %v", err)
	}

	fmt.Printf("\n🔥 Искра запущена\n")
	fmt.Printf("   ID:    %s\n", userID)
	fmt.Printf("   UI:    http://localhost:%d\n", server.Port())
	fmt.Printf("   Mesh:  порт %d\n", transport.Port())
	if *relayURL != "" {
		fmt.Printf("   Relay: %s\n", *relayURL)
	}
	fmt.Printf("   Режим: %s\n\n", mode)

	// Wait for shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	fmt.Println("\nОстановка...")
	inbox.Save(filepath.Join(*dataDir, "inbox.json"))
	discovery.Stop()
	transport.Stop()
	if relayClient != nil {
		relayClient.Stop()
	}
	server.Stop()
	fmt.Println("Готово.")
}

func loadOrCreateKeypair(dataDir string) (*identity.Keypair, []string, bool) {
	seedPath := filepath.Join(dataDir, "seed.key")
	data, err := os.ReadFile(seedPath)
	if err == nil && len(data) == 32 {
		var seed [32]byte
		copy(seed[:], data)
		kp := identity.KeypairFromSeed(seed)
		return kp, identity.SeedToMnemonic(seed), false
	}

	seed, err := identity.GenerateMnemonicSeed()
	if err != nil {
		log.Fatalf("Failed to generate seed: %v", err)
	}
	if err := os.WriteFile(seedPath, seed[:], 0600); err != nil {
		log.Fatalf("Failed to save seed: %v", err)
	}

	kp := identity.KeypairFromSeed(seed)
	return kp, identity.SeedToMnemonic(seed), true
}

func restoreFromMnemonic(dataDir, mnemonicStr string) {
	words := strings.Fields(mnemonicStr)
	if len(words) != 24 {
		log.Fatalf("Мнемоника должна содержать 24 слова, получено %d", len(words))
	}
	if !identity.ValidateMnemonic(words) {
		log.Fatal("Невалидная мнемоника")
	}
	seed, err := identity.MnemonicToSeed(words)
	if err != nil {
		log.Fatalf("Ошибка восстановления: %v", err)
	}
	os.MkdirAll(dataDir, 0700)
	if err := os.WriteFile(filepath.Join(dataDir, "seed.key"), seed[:], 0600); err != nil {
		log.Fatalf("Не удалось сохранить ключ: %v", err)
	}
	kp := identity.KeypairFromSeed(seed)
	fmt.Printf("✓ Ключ восстановлен. ID: %s\n", identity.UserID(kp.Ed25519Pub))
}

func defaultDataDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".iskra"
	}
	return filepath.Join(home, ".iskra")
}
