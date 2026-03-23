package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"
	"time"

	"github.com/iskra-messenger/iskra/internal/identity"
	"github.com/iskra-messenger/iskra/internal/mesh"
	"github.com/iskra-messenger/iskra/internal/security"
	"github.com/iskra-messenger/iskra/internal/store"
	"github.com/iskra-messenger/iskra/internal/web"
)

func main() {
	// Create temp directory — never touches real ~/.iskra
	demoDir := filepath.Join(os.TempDir(), fmt.Sprintf("iskra-panic-demo-%d", time.Now().UnixNano()))
	os.MkdirAll(demoDir, 0700)

	fmt.Println("🔥 ИСКРА — Демо: глазами товарища майора")
	fmt.Println("   Работает во временной директории. Ваши данные не затронуты.")
	fmt.Println()
	fmt.Println("   Генерация шума...")

	// Generate decoy data (as if panic just happened)
	security.GenerateDecoy(demoDir)

	fmt.Println("   ✓ Фейковые контакты и переписки созданы")
	fmt.Println()

	// Load the decoy seed to create a keypair
	seedData, _ := os.ReadFile(filepath.Join(demoDir, "seed.key"))
	var seed [32]byte
	copy(seed[:], seedData)
	keypair := identity.KeypairFromSeed(seed)
	mnemonic := identity.SeedToMnemonic(seed)

	// Initialize stores from decoy data
	hold, _ := store.NewHold(filepath.Join(demoDir, "hold"))
	bloom := store.NewBloom(1000000, 0.001)
	contacts, _ := store.NewContacts(filepath.Join(demoDir, "contacts.json"))
	inbox, _ := store.NewInbox(filepath.Join(demoDir, "inbox"))
	inbox.Load(filepath.Join(demoDir, "inbox.json"))
	groups, _ := store.NewGroups(filepath.Join(demoDir, "groups.json"))
	peers := mesh.NewPeerList()

	// Create API (no transport, no relay — everyone is "offline")
	api := &web.API{
		Keypair:  keypair,
		Mnemonic: mnemonic,
		Contacts: contacts,
		Inbox:    inbox,
		Hold:     hold,
		Bloom:    bloom,
		Peers:    peers,
		Groups:   groups,
		Mode:     "offline",
		DataDir:  demoDir,
		Locked:   false,
		UnlockCh: make(chan struct{}),
	}
	close(api.UnlockCh)

	// Start web server
	server := web.NewServer(api, 0)
	if err := server.Start(); err != nil {
		log.Fatalf("Не удалось запустить сервер: %v", err)
	}

	uiURL := fmt.Sprintf("http://localhost:%d", server.Port())
	fmt.Printf("   🌐 %s\n\n", uiURL)
	fmt.Println("   Это именно то, что видит товарищ майор.")
	fmt.Println("   Покликайте по контактам — Маша, Дима, Алёна, Серёга.")
	fmt.Println()
	fmt.Println("   Ctrl+C для выхода.")

	openBrowser(uiURL)

	// Wait for shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	fmt.Println("\nОчистка...")
	server.Stop()
	os.RemoveAll(demoDir)
	fmt.Println("Готово.")
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
	cmd.Start()
}
