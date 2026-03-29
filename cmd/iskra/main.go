package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/iskra-messenger/iskra/internal/filetransfer"
	"github.com/iskra-messenger/iskra/internal/firewall"
	"github.com/iskra-messenger/iskra/internal/identity"
	"github.com/iskra-messenger/iskra/internal/mesh"
	"github.com/iskra-messenger/iskra/internal/message"
	"github.com/iskra-messenger/iskra/internal/security"
	"github.com/iskra-messenger/iskra/internal/store"
	"github.com/iskra-messenger/iskra/internal/web"
)

func main() {
	port := flag.Int("port", 0, "HTTP port for UI (0 = random)")
	dataDir := flag.String("data", defaultDataDir(), "Data directory")
	debug := flag.Bool("debug", false, "Enable debug logging")
	meshPort := flag.Int("mesh-port", 0, "Mesh transport port (0 = random)")
	relayURL := flag.String("relay", "wss://iskra-relay.onrender.com/ws", "Relay server URL (wss://host/ws)")
	udpRelay := flag.String("udp-relay", "", "UDP relay address host:port (fallback when WS relay blocked)")
	dnsRelayDomain := flag.String("dns-domain", "", "DNS tunnel relay domain (e.g. tun.iskra-dns.example.com)")
	dnsRelayServer := flag.String("dns-server", "", "DNS tunnel relay server IP:port (e.g. 1.2.3.4:5353)")
	restore := flag.String("restore", "", "Restore from mnemonic (24 words, space-separated)")
	flag.Parse()

	log.SetOutput(os.Stderr)

	// Ensure Windows Firewall allows mesh traffic
	firewall.EnsureFirewallRule("Iskra Messenger")

	// Handle mnemonic restore
	if *restore != "" {
		restoreFromMnemonic(*dataDir, *restore)
	}

	// Ensure data directory exists
	os.MkdirAll(*dataDir, 0700)

	// Load or create keypair
	keypair, mnemonic, isNew := loadOrCreateKeypair(*dataDir)
	userID := identity.UserID(keypair.Ed25519Pub)

	// Get seed for vault key derivation
	seedData, _ := os.ReadFile(filepath.Join(*dataDir, "seed.key"))
	var seed [32]byte
	if len(seedData) == 32 {
		copy(seed[:], seedData)
	}

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

	// Always start locked — if no PIN exists, user must set one up
	hasPIN := security.HasPIN(*dataDir)
	locked := true
	_ = hasPIN

	// Initialize stores (they start without vault key; key set after PIN verify)
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

	groups, err := store.NewGroups(filepath.Join(*dataDir, "groups.json"))
	if err != nil {
		log.Fatalf("Failed to load groups: %v", err)
	}
	channels, err := store.NewChannels(filepath.Join(*dataDir, "channels.json"))
	if err != nil {
		log.Fatalf("Failed to load channels: %v", err)
	}

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
	var udpTransport *mesh.UDPTransport
	var dnsTransport *mesh.DNSTransport
	if *relayURL != "" {
		relayClient = mesh.NewRelayClient(*relayURL, keypair.Ed25519Pub, keypair.X25519Pub)
		mode = "relay"
	}

	// Initialize UDP transport (fallback when WebSocket relay is blocked)
	if *udpRelay != "" {
		var err error
		udpTransport, err = mesh.NewUDPTransport(keypair.Ed25519Pub, *udpRelay)
		if err != nil {
			log.Printf("[UDP] Failed to create UDP transport: %v", err)
		}
	}

	// Initialize DNS tunnel transport (last-resort fallback when everything is blocked)
	if *dnsRelayDomain != "" && *dnsRelayServer != "" {
		dnsTransport = mesh.NewDNSTransport(keypair.Ed25519Pub, *dnsRelayDomain, *dnsRelayServer)
	}

	unlockCh := make(chan struct{})
	if !locked {
		close(unlockCh) // already unlocked
	}

	// Initialize API
	api := &web.API{
		Keypair:      keypair,
		Mnemonic:     mnemonic,
		Contacts:     contacts,
		Inbox:        inbox,
		Hold:         hold,
		Bloom:        bloom,
		Peers:        peers,
		Transport:    transport,
		RelayClient:  relayClient,
		DNSTransport: dnsTransport,
		Groups:       groups,
		Channels:     channels,
		FileMgr:      filetransfer.NewManager(filepath.Join(*dataDir, "files")),
		Mode:         mode,
		DataDir:      *dataDir,
		Seed:         seed,
		Locked:       locked,
		UnlockCh:     unlockCh,
	}

	// Message handler (shared between transport and relay)
	handleMessage := func(msg *message.Message) {
		if bloom.Contains(msg.ID) {
			if *debug {
				log.Printf("[MSG] Duplicate message, skipping")
			}
			return
		}
		bloom.Add(msg.ID)
		forMe := msg.IsForRecipient(keypair.Ed25519Pub)
		log.Printf("[MSG] Received message id=%x forMe=%v type=%d", msg.ID[:4], forMe, msg.ContentType)
		api.HandleIncomingMessage(msg)
		if !forMe {
			hold.Store(msg)
		}
	}

	// Set message handlers
	transport.SetOnMessage(handleMessage)
	transport.SetHold(hold) // fallback for WANT when message not in sync snapshot
	if relayClient != nil {
		relayClient.SetOnMessage(handleMessage)
		var lastSync time.Time
		relayClient.SetOnSyncRequest(func() {
			// Cooldown: don't sync more than once per 30 seconds
			if time.Since(lastSync) < 30*time.Second {
				return
			}
			lastSync = time.Now()
			// New peer came online — broadcast our hold via relay
			msgs, _ := hold.GetForSync()
			for _, msg := range msgs {
				relayClient.BroadcastMessage(msg)
			}
			if len(msgs) > 0 {
				log.Printf("[Sync] Broadcast %d hold messages to new peer via relay", len(msgs))
			}
		})
		if err := relayClient.Start(); err != nil {
			log.Printf("Relay: will retry in background")
		}
	}
	if udpTransport != nil {
		udpTransport.SetOnMessage(handleMessage)
		if err := udpTransport.Start(); err != nil {
			log.Printf("[UDP] Start error: %v", err)
		} else {
			log.Printf("[UDP] Fallback transport active")
		}
	}
	if dnsTransport != nil {
		dnsTransport.SetOnMessage(handleMessage)
		if err := dnsTransport.Start(); err != nil {
			log.Printf("[DNS] Start error: %v", err)
		} else {
			log.Printf("[DNS] Tunnel transport active via %s", *dnsRelayDomain)
		}
	}

	// Start LAN discovery
	discovery := mesh.NewDiscovery(keypair.Ed25519Pub, transport.Port(), peers)
	discovery.SetOnPeer(func(pubKey [32]byte, ip string, peerPort uint16) {
		if *debug {
			log.Printf("Discovered peer: %s:%d", ip, peerPort)
		}
		go func() {
			holdMsgs, _ := hold.GetForSync()
			transport.ConnectAndSync(ip, peerPort, bloom.Export(), holdMsgs)
		}()
	})
	discovery.Start()

	// Start web server
	server := web.NewServer(api, *port)
	if err := server.Start(); err != nil {
		log.Fatalf("Failed to start web server: %v", err)
	}

	uiURL := fmt.Sprintf("http://localhost:%d", server.Port())

	fmt.Printf("\n🔥 Искра запущена\n")
	fmt.Printf("   ID:    %s\n", userID)
	fmt.Printf("   UI:    %s\n", uiURL)
	fmt.Printf("   Mesh:  порт %d\n", transport.Port())
	if *relayURL != "" {
		fmt.Printf("   Relay: %s\n", *relayURL)
	}
	if *udpRelay != "" {
		fmt.Printf("   UDP:   %s (обфускация)\n", *udpRelay)
	}
	if *dnsRelayDomain != "" {
		fmt.Printf("   DNS:   %s через %s (туннель)\n", *dnsRelayDomain, *dnsRelayServer)
	}
	fmt.Printf("   Режим: %s\n", mode)
	if locked {
		fmt.Printf("   🔒 PIN: требуется ввод\n")
	}
	fmt.Println()

	// Periodic hold cleanup (morgue + kill switch)
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			hold.Cleanup()
		}
	}()

	// Auto-open browser
	openBrowser(uiURL)

	// Wait for shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	fmt.Println("\nОстановка...")
	inbox.Save(api.InboxFilePath())
	groups.Save()
	channels.Save()
	discovery.Stop()
	transport.Stop()
	if relayClient != nil {
		relayClient.Stop()
	}
	if udpTransport != nil {
		udpTransport.Stop()
	}
	if dnsTransport != nil {
		dnsTransport.Stop()
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

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	if err := cmd.Start(); err != nil {
		log.Printf("Не удалось открыть браузер: %v", err)
		fmt.Printf("   Откройте вручную: %s\n\n", url)
	}
}

func defaultDataDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".iskra"
	}
	return filepath.Join(home, ".iskra")
}
