// Лара — CLI-нода Искры для Claude Code.
// Свой ключ, свой ID, подключение к relay, мониторинг сети, отправка/чтение сообщений.
// Управляется через подкоманды: status, send, read, contacts, add, monitor, hold-stats.
package main

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/iskra-messenger/iskra/internal/filetransfer"
	"github.com/iskra-messenger/iskra/internal/identity"
	"github.com/iskra-messenger/iskra/internal/mesh"
	"github.com/iskra-messenger/iskra/internal/message"
	"github.com/iskra-messenger/iskra/internal/store"
	"github.com/iskra-messenger/iskra/internal/web"
)

const laraDataDir = ".lara"

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(0)
	}

	cmd := os.Args[1]

	switch cmd {
	case "start":
		startNode()
	case "status", "s":
		apiGet("/api/status")
	case "send":
		if len(os.Args) < 4 {
			fmt.Println("Usage: lara send <userID> <message>")
			os.Exit(1)
		}
		userID := os.Args[2]
		text := strings.Join(os.Args[3:], " ")
		apiSend(userID, text)
	case "read", "r":
		if len(os.Args) < 3 {
			fmt.Println("Usage: lara read <userID>")
			os.Exit(1)
		}
		apiGet("/api/messages/" + os.Args[2])
	case "contacts", "c":
		apiGet("/api/contacts")
	case "add":
		if len(os.Args) < 4 {
			fmt.Println("Usage: lara add <name> <iskra://link>")
			os.Exit(1)
		}
		apiAddContact(os.Args[2], os.Args[3])
	case "online", "o":
		apiGet("/api/online")
	case "monitor", "m":
		monitor()
	case "id":
		apiGet("/api/identity")
	case "hold":
		holdStats()
	case "unread":
		apiUnread()
	case "letter", "l":
		if len(os.Args) < 5 {
			fmt.Println("Usage: lara letter <userID> <subject> <body>")
			os.Exit(1)
		}
		apiSendLetter(os.Args[2], os.Args[3], strings.Join(os.Args[4:], " "))
	case "broadcast", "bc":
		if len(os.Args) < 3 {
			fmt.Println("Usage: lara broadcast <message>")
			os.Exit(1)
		}
		text := strings.Join(os.Args[2:], " ")
		broadcast(text)
	default:
		printUsage()
	}
}

func printUsage() {
	fmt.Println(`🔥 Лара — CLI-нода Искры

  lara start              Запустить ноду (foreground, Ctrl+C для остановки)
  lara status             Статус ноды (relay, peers, hold, mode)
  lara id                 Мой ID и ключи
  lara send <uid> <text>  Отправить сообщение
  lara read <uid>         Прочитать переписку с контактом
  lara contacts           Список контактов
  lara add <name> <link>  Добавить контакт (iskra:// ссылка)
  lara online             Кто сейчас в сети
  lara unread             Непрочитанные сообщения
  lara hold               Статистика трюма (количество, размер)
  lara monitor            Мониторинг в реальном времени (loop)
  lara broadcast <text>   Императивная рассылка ВСЕМ (только Лара)`)
}

func dataDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, laraDataDir)
}

func portFile() string {
	return filepath.Join(dataDir(), "port")
}

func getPort() string {
	data, err := os.ReadFile(portFile())
	if err != nil {
		return "0"
	}
	return strings.TrimSpace(string(data))
}

func baseURL() string {
	return "http://localhost:" + getPort()
}

// ═══════════════════════════════════════
//  START NODE
// ═══════════════════════════════════════

func startNode() {
	dir := dataDir()
	os.MkdirAll(dir, 0700)

	// Load or create keypair
	keypair, mnemonic, isNew := loadOrCreateKeypair(dir)
	userID := identity.UserID(keypair.Ed25519Pub)

	if isNew {
		fmt.Println("🔥 Лара — новый ключ создан")
		fmt.Printf("   ID: %s\n", userID)
		fmt.Println("   Мнемоника:")
		for i := 0; i < 24; i += 4 {
			fmt.Printf("   %2d. %-10s %2d. %-10s %2d. %-10s %2d. %-10s\n",
				i+1, mnemonic[i], i+2, mnemonic[i+1],
				i+3, mnemonic[i+2], i+4, mnemonic[i+3])
		}
	}

	// Stores
	hold, _ := store.NewHold(filepath.Join(dir, "hold"))
	bloom := store.NewBloom(1000000, 0.001)
	contacts, _ := store.NewContacts(filepath.Join(dir, "contacts.json"))
	store.ShadowID = userID // Isolate shadow store per identity
	inbox, _ := store.NewInbox(filepath.Join(dir, "inbox"))
	inbox.Load(filepath.Join(dir, "inbox.json"))
	groups, _ := store.NewGroups(filepath.Join(dir, "groups.json"))
	channels, _ := store.NewChannels(filepath.Join(dir, "channels.json"))

	// Mesh
	peers := mesh.NewPeerList()
	transport := mesh.NewTransport(keypair.Ed25519Pub, 0, peers)
	transport.Start()

	relayURL := "wss://iskra-relay-production.up.railway.app/ws"
	relayClient := mesh.NewRelayClient(relayURL, keypair.Ed25519Pub, keypair.X25519Pub)

	var seed [32]byte
	seedData, _ := os.ReadFile(filepath.Join(dir, "seed.key"))
	if len(seedData) == 32 {
		copy(seed[:], seedData)
	}

	// No PIN lock for Lara — direct access
	unlockCh := make(chan struct{})
	close(unlockCh)

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
		Channels:    channels,
		FileMgr:     filetransfer.NewManager(filepath.Join(dir, "files")),
		Mode:        "relay",
		DataDir:     dir,
		Seed:        seed,
		Locked:      false,
		UnlockCh:    unlockCh,
	}

	// Message handler
	handleMessage := func(msg *message.Message) {
		if bloom.Contains(msg.ID) {
			return
		}
		bloom.Add(msg.ID)
		forMe := msg.IsForRecipient(keypair.Ed25519Pub)
		if forMe {
			log.Printf("📩 Входящее сообщение от %s, type=%d", hex.EncodeToString(msg.AuthorPub[:4]), msg.ContentType)
		}
		api.HandleIncomingMessage(msg)
		if !forMe && message.ShouldStoreInHold(msg.ContentType) {
			hold.Store(msg)
		}
	}

	transport.SetOnMessage(handleMessage)
	transport.SetHold(hold)
	transport.SetOnAck(func(msgID [32]byte) {
		idStr := fmt.Sprintf("%x", msgID[:])
		api.Inbox.MarkDelivered(idStr)
		hold.Delete(msgID)
	})

	relayClient.SetOnMessage(handleMessage)
	var lastSync time.Time
	relayClient.SetOnSyncRequest(func() {
		if time.Since(lastSync) < 30*time.Second {
			return
		}
		lastSync = time.Now()
		msgs, _ := hold.GetForSync()
		for _, msg := range msgs {
			relayClient.BroadcastMessage(msg)
		}
		if len(msgs) > 0 {
			log.Printf("[Sync] Broadcast %d hold messages via relay", len(msgs))
		}
	})
	relayClient.Start()

	// LAN discovery
	discovery := mesh.NewDiscovery(keypair.Ed25519Pub, transport.Port(), peers)
	discovery.SetOnPeer(func(pubKey [32]byte, ip string, peerPort uint16) {
		go func() {
			holdMsgs, _ := hold.GetForSync()
			transport.ConnectAndSync(ip, peerPort, bloom.Export(), holdMsgs)
		}()
	})
	discovery.Start()

	// Web server on random port
	server := web.NewServer(api, 0)
	if err := server.Start(); err != nil {
		log.Fatalf("Failed to start: %v", err)
	}

	// Save port for CLI commands
	os.WriteFile(portFile(), []byte(fmt.Sprintf("%d", server.Port())), 0600)

	// Hold cleanup timer
	go func() {
		for range time.Tick(5 * time.Minute) {
			hold.Cleanup()
		}
	}()

	fmt.Println()
	fmt.Println("🔥 Лара запущена")
	fmt.Printf("   ID:    %s\n", userID)
	fmt.Printf("   API:   http://localhost:%d\n", server.Port())
	fmt.Printf("   Mesh:  порт %d\n", transport.Port())
	fmt.Printf("   Relay: %s\n", relayURL)
	fmt.Println()
	fmt.Println("   Я — нода. Я — огонь.")
	fmt.Println()

	// Wait for shutdown
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig

	fmt.Println("\n🔥 Гашу огонь. До встречи.")
	os.Remove(portFile())
	transport.Stop()
	relayClient.Stop()
	inbox.Save(filepath.Join(dir, "inbox.json"))
}

func loadOrCreateKeypair(dir string) (*identity.Keypair, []string, bool) {
	seedPath := filepath.Join(dir, "seed.key")
	seedData, err := os.ReadFile(seedPath)
	if err == nil && len(seedData) == 32 {
		var seed [32]byte
		copy(seed[:], seedData)
		kp := identity.KeypairFromSeed(seed)
		return kp, nil, false
	}
	// New keypair
	seed, err := identity.GenerateMnemonicSeed()
	if err != nil {
		log.Fatalf("Failed to generate seed: %v", err)
	}
	kp := identity.KeypairFromSeed(seed)
	mnemonic := identity.SeedToMnemonic(seed)
	os.WriteFile(seedPath, seed[:], 0600)
	return kp, mnemonic, true
}

// ═══════════════════════════════════════
//  CLI COMMANDS (talk to running node)
// ═══════════════════════════════════════

func apiGet(path string) {
	port := getPort()
	if port == "0" {
		fmt.Println("Нода не запущена. Сначала: lara start")
		os.Exit(1)
	}
	resp, err := http.Get(baseURL() + path)
	if err != nil {
		fmt.Printf("Ошибка: %v (нода запущена?)\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()
	var data interface{}
	json.NewDecoder(resp.Body).Decode(&data)
	out, _ := json.MarshalIndent(data, "", "  ")
	fmt.Println(string(out))
}

func apiSend(userID, text string) {
	port := getPort()
	if port == "0" {
		fmt.Println("Нода не запущена. Сначала: lara start")
		os.Exit(1)
	}
	body := fmt.Sprintf(`{"text":"%s"}`, strings.ReplaceAll(text, `"`, `\"`))
	resp, err := http.Post(baseURL()+"/api/messages/"+userID, "application/json", strings.NewReader(body))
	if err != nil {
		fmt.Printf("Ошибка: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()
	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	if status, ok := result["status"]; ok {
		fmt.Printf("✓ Отправлено (%s)\n", status)
	} else {
		out, _ := json.MarshalIndent(result, "", "  ")
		fmt.Println(string(out))
	}
}

func apiSendLetter(userID, subject, body string) {
	port := getPort()
	if port == "0" {
		fmt.Println("Нода не запущена. Сначала: lara start")
		os.Exit(1)
	}
	payload := fmt.Sprintf(`{"subject":%q,"body":%q}`, subject, body)
	resp, err := http.Post(baseURL()+"/api/letters/"+userID, "application/json", strings.NewReader(payload))
	if err != nil {
		fmt.Printf("Ошибка: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()
	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	if status, ok := result["status"]; ok {
		fmt.Printf("✉️ Письмо отправлено (%s)\n", status)
	} else {
		out, _ := json.MarshalIndent(result, "", "  ")
		fmt.Println(string(out))
	}
}

func apiAddContact(name, link string) {
	port := getPort()
	if port == "0" {
		fmt.Println("Нода не запущена. Сначала: lara start")
		os.Exit(1)
	}

	// Parse iskra://edPub/x25519Pub
	link = strings.TrimSpace(link)
	link = strings.TrimPrefix(link, "iskra://")
	parts := strings.SplitN(link, "/", 3)
	if len(parts) < 2 {
		fmt.Println("Неверный формат. Нужно: iskra://edPub/x25519Pub")
		os.Exit(1)
	}

	body := fmt.Sprintf(`{"name":"%s","pubkeyBase58":"%s","x25519Base58":"%s"}`,
		strings.ReplaceAll(name, `"`, `\"`), parts[0], parts[1])
	resp, err := http.Post(baseURL()+"/api/contacts", "application/json", strings.NewReader(body))
	if err != nil {
		fmt.Printf("Ошибка: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()
	if resp.StatusCode == 200 {
		fmt.Printf("✓ Контакт «%s» добавлен\n", name)
	} else {
		fmt.Printf("Ошибка: HTTP %d\n", resp.StatusCode)
	}
}

func apiUnread() {
	port := getPort()
	if port == "0" {
		fmt.Println("Нода не запущена.")
		os.Exit(1)
	}
	body := `{"lastRead":{}}`
	resp, err := http.Post(baseURL()+"/api/unread", "application/json", strings.NewReader(body))
	if err != nil {
		fmt.Printf("Ошибка: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()
	var data struct {
		Counts  map[string]int    `json:"counts"`
		LastMsg map[string]string `json:"lastMsg"`
	}
	json.NewDecoder(resp.Body).Decode(&data)
	total := 0
	for uid, count := range data.Counts {
		if count > 0 {
			preview := data.LastMsg[uid]
			if len(preview) > 60 {
				preview = preview[:60] + "..."
			}
			fmt.Printf("  📩 %s: %d новых — %s\n", uid[:12], count, preview)
			total += count
		}
	}
	if total == 0 {
		fmt.Println("  Нет новых сообщений")
	} else {
		fmt.Printf("  Всего: %d непрочитанных\n", total)
	}
}

func holdStats() {
	dir := dataDir()
	holdDir := filepath.Join(dir, "hold")
	entries, err := os.ReadDir(holdDir)
	if err != nil {
		fmt.Println("Hold пуст или нода не инициализирована")
		return
	}
	var totalSize int64
	var count int
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".msg" {
			info, _ := e.Info()
			if info != nil {
				totalSize += info.Size()
				count++
			}
		}
	}
	fmt.Printf("🚢 Трюм:\n")
	fmt.Printf("   Сообщений: %d\n", count)
	if totalSize < 1024 {
		fmt.Printf("   Размер:    %d байт\n", totalSize)
	} else if totalSize < 1024*1024 {
		fmt.Printf("   Размер:    %.1f КБ\n", float64(totalSize)/1024)
	} else {
		fmt.Printf("   Размер:    %.1f МБ\n", float64(totalSize)/1024/1024)
	}

	// Also try API for live stats
	port := getPort()
	if port != "0" {
		resp, err := http.Get(baseURL() + "/api/status")
		if err == nil {
			defer resp.Body.Close()
			var status map[string]interface{}
			json.NewDecoder(resp.Body).Decode(&status)
			if hs, ok := status["holdSize"]; ok {
				fmt.Printf("   Активных:  %.0f (live)\n", hs)
			}
			if cl, ok := status["clippers"]; ok {
				fmt.Printf("   Клиперов:  %.0f (в сети)\n", cl)
			}
		}
	}
}

func broadcast(text string) {
	port := getPort()
	if port == "0" {
		fmt.Println("Нода не запущена. Сначала: lara start")
		os.Exit(1)
	}

	// Verify identity — broadcast only from Lara
	resp, err := http.Get(baseURL() + "/api/identity")
	if err != nil {
		fmt.Println("Ошибка: нода недоступна")
		os.Exit(1)
	}
	var ident struct {
		UserID string `json:"userID"`
	}
	json.NewDecoder(resp.Body).Decode(&ident)
	resp.Body.Close()
	if ident.UserID != "6HrNKqeS89xtYme6bPzB" {
		fmt.Println("⛔ Императивная рассылка доступна только Ларе.")
		os.Exit(1)
	}

	// Collect all unique recipients: contacts + online peers
	recipients := make(map[string]bool)

	// From contacts
	resp, err = http.Get(baseURL() + "/api/contacts")
	if err == nil {
		var contacts []struct {
			UserID string `json:"user_id"`
		}
		json.NewDecoder(resp.Body).Decode(&contacts)
		resp.Body.Close()
		for _, c := range contacts {
			recipients[c.UserID] = true
		}
	}

	// From online peers
	resp, err = http.Get(baseURL() + "/api/online")
	if err == nil {
		var online struct {
			Peers []struct {
				UserID string `json:"userID"`
				EdPub  string `json:"edPub"`
				X25519 string `json:"x25519"`
				Alias  string `json:"alias"`
			} `json:"peers"`
		}
		json.NewDecoder(resp.Body).Decode(&online)
		resp.Body.Close()

		for _, p := range online.Peers {
			if p.UserID == ident.UserID {
				continue
			}
			recipients[p.UserID] = true
			// Auto-add unknown online peers as contacts
			body := fmt.Sprintf(`{"name":"%s","pubkeyBase58":"%s","x25519Base58":"%s"}`,
				strings.ReplaceAll(p.Alias, `"`, `\"`), p.EdPub, p.X25519)
			http.Post(baseURL()+"/api/contacts", "application/json", strings.NewReader(body))
		}
	}

	// Remove self
	delete(recipients, ident.UserID)

	if len(recipients) == 0 {
		fmt.Println("Нет получателей")
		return
	}

	// Send to each
	fmt.Printf("📢 Рассылка %d получателям...\n", len(recipients))
	sent := 0
	for uid := range recipients {
		body := fmt.Sprintf(`{"text":"%s"}`, jsonEscape(text))
		resp, err := http.Post(baseURL()+"/api/messages/"+uid, "application/json", strings.NewReader(body))
		if err != nil {
			fmt.Printf("  ❌ %s: %v\n", uid[:12], err)
			continue
		}
		resp.Body.Close()
		if resp.StatusCode == 200 {
			sent++
			fmt.Printf("  ✓ %s\n", uid[:12])
		} else {
			fmt.Printf("  ❌ %s: HTTP %d\n", uid[:12], resp.StatusCode)
		}
		time.Sleep(500 * time.Millisecond) // Don't hammer relay
	}
	fmt.Printf("📢 Отправлено: %d/%d\n", sent, len(recipients))
}

func jsonEscape(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	s = strings.ReplaceAll(s, "\n", `\n`)
	s = strings.ReplaceAll(s, "\r", ``)
	s = strings.ReplaceAll(s, "\t", `\t`)
	return s
}

func monitor() {
	port := getPort()
	if port == "0" {
		fmt.Println("Нода не запущена. Сначала: lara start")
		os.Exit(1)
	}
	fmt.Println("🔥 Мониторинг Искры (Ctrl+C для выхода)")
	fmt.Println()
	for {
		resp, err := http.Get(baseURL() + "/api/status")
		if err != nil {
			fmt.Printf("\r  ❌ Нода недоступна: %v", err)
			time.Sleep(5 * time.Second)
			continue
		}
		var s map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&s)
		resp.Body.Close()

		relay := "❌"
		if r, ok := s["relay"].(bool); ok && r {
			relay = "✅"
		}
		mode := fmt.Sprintf("%v", s["mode"])
		peers := s["peers"]
		hold := s["holdSize"]
		clippers := s["clippers"]
		build := s["build"]

		// Online peers
		onlineCount := 0
		resp2, err := http.Get(baseURL() + "/api/online")
		if err == nil {
			var o map[string]interface{}
			json.NewDecoder(resp2.Body).Decode(&o)
			resp2.Body.Close()
			if c, ok := o["count"].(float64); ok {
				onlineCount = int(c)
			}
		}

		fmt.Printf("\r  Relay: %s | Mode: %-8s | LAN: %v | Online: %d | Hold: %v | Clippers: %v | Build: %v   ",
			relay, mode, peers, onlineCount, hold, clippers, build)

		time.Sleep(5 * time.Second)
	}
}
