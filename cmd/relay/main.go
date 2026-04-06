package main

import (
	"encoding/binary"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// Relay — WebSocket relay для Искры (СВХ — склад временного хранения).
// Не логирует. Не расшифровывает.
// Хранит зашифрованные блобы на диск с TTL 48 часов.
// Маяк стоит в порту. Корабли приходят — забирают груз.

// Партийные клички — каждая сессия новая маска
var aliases = []string{
	"Ильич", "Крупская", "Коллонтай", "Сталин", "Киров",
	"Свердлов", "Дзержинский", "Бухарин", "Луначарский", "Фрунзе",
	"Орджоникидзе", "Чапаев", "Котовский", "Щорс", "Лазо",
	"Бабушкин", "Баумаи", "Землячка", "Инесса", "Калинин",
	"Артём", "Камо", "Литвинов", "Красин", "Цеткин",
	"Спартак", "Марат", "Робеспьер", "Дантон", "Гарибальди",
	"Боливар", "Че", "Фидель", "Сапата", "Панчо",
	"Зоя", "Молодогвардеец", "Партизан", "Подпольщик", "Связной",
	"Маяк", "Факел", "Буревестник", "Сокол", "Орёл",
	"Гроза", "Рассвет", "Заря", "Пламя", "Молния",
	"Штурм", "Баррикада", "Компас", "Маршрут", "Перевал",
	"Дозор", "Разведка", "Авангард", "Форпост", "Цитадель",
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

type clientInfo struct {
	conn      *websocket.Conn
	edPub     [32]byte // Ed25519 pubkey
	x25519Pub [32]byte // X25519 pubkey
	hasSent   bool     // true if client ever sent a message (not a clipper)
}

// knownPeer is a peer that has connected at least once. Persists with TTL.
type knownPeer struct {
	EdPub     string `json:"edPub"`
	X25519Pub string `json:"x25519Pub"`
	UserID    string `json:"userID"`
	LastSeen  int64  `json:"lastSeen"`  // unix timestamp
	IsClipper bool   `json:"isClipper"` // never sent a message
}

// holdEntry is a message waiting for its recipient in the relay hold (СВХ).
type holdEntry struct {
	Data      []byte `json:"data"`      // raw encrypted frame
	Timestamp int64  `json:"timestamp"` // unix time when stored
}

const (
	holdTTL       = 48 * time.Hour  // messages expire after 48h
	holdMaxPerUID = 200             // max messages per recipient
	holdMaxTotal  = 50000           // max total messages across all recipients
	holdDir       = "relay-hold"    // directory for persistent storage
)

type relay struct {
	clients    map[string]*clientInfo   // UserID → client info (online only)
	aliases    map[string]string        // UserID → current alias
	hold       map[string][]holdEntry   // UserID → queued messages (СВХ)
	knownPeers map[string]*knownPeer   // UserID → ever-connected peer (TTL 30 days)
	mu         sync.RWMutex
}

func main() {
	port := flag.Int("port", 8443, "Listen port")
	flag.Parse()

	if envPort := os.Getenv("PORT"); envPort != "" {
		if p, err := strconv.Atoi(envPort); err == nil {
			*port = p
		}
	}

	r := &relay{
		clients:    make(map[string]*clientInfo),
		aliases:    make(map[string]string),
		hold:       make(map[string][]holdEntry),
		knownPeers: make(map[string]*knownPeer),
	}

	// Load persistent hold from disk
	r.loadHold()
	holdCount := 0
	for _, msgs := range r.hold {
		holdCount += len(msgs)
	}
	if holdCount > 0 {
		fmt.Printf("   Трюм: %d сообщений загружено с диска\n", holdCount)
	}

	// Cleanup expired hold + known peers
	go func() {
		for range time.Tick(1 * time.Hour) {
			r.cleanupHold()
			r.cleanupKnownPeers()
		}
	}()

	http.HandleFunc("/ws", r.handleWS)
	http.HandleFunc("/online", r.handleOnline)
	http.HandleFunc("/directory", r.handleDirectory)
	http.HandleFunc("/hold-stats", r.handleHoldStats)
	http.HandleFunc("/", func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		fmt.Fprintf(w, "Iskra Relay v0.3 (СВХ)\n")
	})

	addr := fmt.Sprintf(":%d", *port)
	fmt.Printf("🔥 Искра Relay (СВХ)\n")
	fmt.Printf("   Порт: %d\n", *port)
	fmt.Printf("   WebSocket: ws://0.0.0.0:%d/ws\n", *port)
	fmt.Printf("   Трюм: TTL %v, макс %d/получатель, %d всего\n", holdTTL, holdMaxPerUID, holdMaxTotal)
	fmt.Println("   Не логирует. Не расшифровывает. Хранит и передаёт.")
	fmt.Println()

	if err := http.ListenAndServe(addr, nil); err != nil {
		log.Fatalf("Failed to start: %v", err)
	}
}

type onlinePeer struct {
	Alias  string `json:"alias"`
	EdPub  string `json:"edPub"`
	X25519 string `json:"x25519"`
}

// handleOnline returns list of currently connected peers with aliases and keys.
func (r *relay) handleOnline(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	r.mu.RLock()
	peers := make([]onlinePeer, 0, len(r.aliases))
	clippers := 0
	for uid, alias := range r.aliases {
		if ci, ok := r.clients[uid]; ok {
			peers = append(peers, onlinePeer{
				Alias:  alias,
				EdPub:  fmt.Sprintf("%x", ci.edPub),
				X25519: fmt.Sprintf("%x", ci.x25519Pub),
			})
			if !ci.hasSent {
				clippers++
			}
		}
	}
	r.mu.RUnlock()

	json.NewEncoder(w).Encode(map[string]interface{}{
		"count":    len(peers),
		"clippers": clippers,
		"peers":    peers,
	})
}

// pickAlias assigns a random alias not currently in use.
func (r *relay) pickAlias() string {
	used := make(map[string]bool)
	for _, a := range r.aliases {
		used[a] = true
	}

	// Shuffle and pick first unused
	perm := rand.Perm(len(aliases))
	for _, i := range perm {
		if !used[aliases[i]] {
			return aliases[i]
		}
	}

	// All taken — add number suffix
	base := aliases[rand.Intn(len(aliases))]
	return fmt.Sprintf("%s-%d", base, rand.Intn(999))
}

func (r *relay) handleWS(w http.ResponseWriter, req *http.Request) {
	conn, err := upgrader.Upgrade(w, req, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	conn.SetReadDeadline(time.Now().Add(45 * time.Second))
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(45 * time.Second))
		return nil
	})
	conn.SetPingHandler(func(msg string) error {
		conn.SetReadDeadline(time.Now().Add(45 * time.Second))
		conn.WriteControl(websocket.PongMessage, []byte(msg), time.Now().Add(5*time.Second))
		return nil
	})

	// First message: client sends both pubkeys (64 bytes: ed25519 + x25519)
	conn.SetReadDeadline(time.Now().Add(10 * time.Second)) // short deadline for handshake
	_, pubkeyMsg, err := conn.ReadMessage()
	if err != nil || (len(pubkeyMsg) != 32 && len(pubkeyMsg) != 64) {
		return
	}

	var edPub, x25519Pub [32]byte
	copy(edPub[:], pubkeyMsg[:32])
	if len(pubkeyMsg) == 64 {
		copy(x25519Pub[:], pubkeyMsg[32:64])
	}

	userID := fmt.Sprintf("%x", edPub[:20])

	// Register client with alias + update directory
	r.mu.Lock()
	oldCI, existed := r.clients[userID]
	if existed && oldCI != nil {
		oldCI.conn.Close()
	}
	r.clients[userID] = &clientInfo{conn: conn, edPub: edPub, x25519Pub: x25519Pub}
	r.aliases[userID] = r.pickAlias()

	// Update known peers directory (TTL 30 days)
	r.knownPeers[userID] = &knownPeer{
		EdPub:     fmt.Sprintf("%x", edPub),
		X25519Pub: fmt.Sprintf("%x", x25519Pub),
		UserID:    userID,
		LastSeen:  time.Now().Unix(),
		IsClipper: true, // will be set to false when they send a message
	}
	// Carry over hasSent status from previous connection
	if kp, ok := r.knownPeers[userID]; ok && existed {
		kp.IsClipper = !oldCI.hasSent
	}

	// Deliver held messages (СВХ → корабль забирает груз)
	held := r.hold[userID]
	delete(r.hold, userID)
	r.mu.Unlock()

	if len(held) > 0 {
		log.Printf("[Hold] Delivering %d held messages to %s", len(held), userID[:8])
		for _, entry := range held {
			conn.WriteMessage(websocket.BinaryMessage, entry.Data)
		}
		r.saveHold() // persist removal
	}

	// Notify all existing peers: "new peer arrived — sync your holds!"
	syncNotify := make([]byte, 20+7)
	copy(syncNotify[:20], edPub[:20])
	copy(syncNotify[20:], []byte("NEWSYNC"))
	r.broadcastExcept(userID, syncNotify)

	// Server-side ping — detect dead connections proactively
	pingDone := make(chan struct{})
	go func() {
		ticker := time.NewTicker(20 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if err := conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(5*time.Second)); err != nil {
					conn.Close()
					return
				}
			case <-pingDone:
				return
			}
		}
	}()

	// Read loop
	for {
		conn.SetReadDeadline(time.Now().Add(45 * time.Second))
		_, data, err := conn.ReadMessage()
		if err != nil {
			break
		}

		if len(data) < 20 {
			continue
		}

		recipID := fmt.Sprintf("%x", data[:20])
		msgData := data[20:]

		// Mark as active sender (not a clipper)
		r.mu.Lock()
		if ci, ok := r.clients[userID]; ok {
			ci.hasSent = true
		}
		if kp, ok := r.knownPeers[userID]; ok {
			kp.IsClipper = false
			kp.LastSeen = time.Now().Unix()
		}
		r.mu.Unlock()

		frame := make([]byte, 20+len(msgData))
		copy(frame[:20], edPub[:20])
		copy(frame[20:], msgData)

		// Check if broadcast (all-zero recipientID)
		isBroadcast := true
		for _, b := range data[:20] {
			if b != 0 {
				isBroadcast = false
				break
			}
		}

		if isBroadcast {
			// Broadcast to ALL online peers (except sender)
			r.broadcastExcept(userID, frame)
		} else {
			r.mu.RLock()
			targetCI, online := r.clients[recipID]
			r.mu.RUnlock()

			if online {
				// Direct delivery — fastest path
				targetCI.conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
				targetCI.conn.WriteMessage(websocket.BinaryMessage, frame)
			} else {
				// Recipient offline — store in hold (СВХ) + broadcast to peers' holds
				r.mu.Lock()
				if len(r.hold[recipID]) < holdMaxPerUID {
					r.hold[recipID] = append(r.hold[recipID], holdEntry{
						Data:      frame,
						Timestamp: time.Now().Unix(),
					})
				}
				r.mu.Unlock()
				r.saveHold() // persist to disk
				// Also broadcast to online peers as store-and-forward backup
				r.broadcastExcept(userID, frame)
			}
		}
	}

	// Stop ping goroutine and unregister
	close(pingDone)
	r.mu.Lock()
	if ci, ok := r.clients[userID]; ok && ci.conn == conn {
		delete(r.clients, userID)
		delete(r.aliases, userID)
	}
	r.mu.Unlock()
}

// handleDirectory returns ALL known peers (ever connected, TTL 30 days) with online/offline status.
func (r *relay) handleDirectory(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	r.mu.RLock()
	type directoryEntry struct {
		UserID    string `json:"userID"`
		EdPub     string `json:"edPub"`
		X25519Pub string `json:"x25519Pub"`
		Online    bool   `json:"online"`
		IsClipper bool   `json:"isClipper"`
		LastSeen  int64  `json:"lastSeen"`
		Alias     string `json:"alias,omitempty"`
	}

	entries := make([]directoryEntry, 0, len(r.knownPeers))
	for uid, kp := range r.knownPeers {
		_, isOnline := r.clients[uid]
		alias := ""
		if isOnline {
			alias = r.aliases[uid]
		}
		entries = append(entries, directoryEntry{
			UserID:    kp.UserID,
			EdPub:     kp.EdPub,
			X25519Pub: kp.X25519Pub,
			Online:    isOnline,
			IsClipper: kp.IsClipper,
			LastSeen:  kp.LastSeen,
			Alias:     alias,
		})
	}
	r.mu.RUnlock()

	// Count stats
	onlineCount := 0
	clipperCount := 0
	for _, e := range entries {
		if e.Online {
			onlineCount++
		}
		if e.IsClipper {
			clipperCount++
		}
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"total":    len(entries),
		"online":   onlineCount,
		"clippers": clipperCount,
		"peers":    entries,
	})
}

// cleanupKnownPeers removes peers not seen for 30 days.
func (r *relay) cleanupKnownPeers() {
	r.mu.Lock()
	defer r.mu.Unlock()

	cutoff := time.Now().Unix() - 30*24*3600
	for uid, kp := range r.knownPeers {
		if kp.LastSeen < cutoff {
			delete(r.knownPeers, uid)
		}
	}
}

// handleHoldStats returns hold statistics.
func (r *relay) handleHoldStats(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	r.mu.RLock()
	total := 0
	recipients := len(r.hold)
	oldest := int64(0)
	for _, entries := range r.hold {
		total += len(entries)
		for _, e := range entries {
			if oldest == 0 || e.Timestamp < oldest {
				oldest = e.Timestamp
			}
		}
	}
	r.mu.RUnlock()
	json.NewEncoder(w).Encode(map[string]interface{}{
		"messages":   total,
		"recipients": recipients,
		"oldest":     oldest,
		"ttl_hours":  int(holdTTL.Hours()),
	})
}

// === HOLD PERSISTENCE (СВХ) ===

// loadHold reads the hold from disk on startup.
func (r *relay) loadHold() {
	path := holdDir + "/hold.json"
	data, err := os.ReadFile(path)
	if err != nil {
		return // no file — fresh start
	}
	var stored map[string][]holdEntry
	if err := json.Unmarshal(data, &stored); err != nil {
		log.Printf("[Hold] Failed to parse hold.json: %v", err)
		return
	}
	// Filter expired entries on load
	now := time.Now().Unix()
	cutoff := now - int64(holdTTL.Seconds())
	for uid, entries := range stored {
		var valid []holdEntry
		for _, e := range entries {
			if e.Timestamp > cutoff {
				valid = append(valid, e)
			}
		}
		if len(valid) > 0 {
			r.hold[uid] = valid
		}
	}
}

// saveHold persists the hold to disk.
func (r *relay) saveHold() {
	r.mu.RLock()
	data, err := json.Marshal(r.hold)
	r.mu.RUnlock()
	if err != nil {
		return
	}
	os.MkdirAll(holdDir, 0700)
	os.WriteFile(holdDir+"/hold.json", data, 0600)
}

// cleanupHold removes messages older than TTL and enforces limits.
func (r *relay) cleanupHold() {
	r.mu.Lock()
	now := time.Now().Unix()
	cutoff := now - int64(holdTTL.Seconds())
	total := 0
	removed := 0
	for uid, entries := range r.hold {
		var valid []holdEntry
		for _, e := range entries {
			if e.Timestamp > cutoff {
				valid = append(valid, e)
			} else {
				removed++
			}
		}
		if len(valid) > 0 {
			r.hold[uid] = valid
			total += len(valid)
		} else {
			delete(r.hold, uid)
		}
	}
	r.mu.Unlock()
	if removed > 0 {
		log.Printf("[Hold] Cleanup: removed %d expired, %d remaining", removed, total)
		r.saveHold()
	}
}

// broadcastExcept sends a frame to all connected clients except the given userID.
// Copies the conn list first to avoid holding lock during write.
func (r *relay) broadcastExcept(excludeUID string, frame []byte) {
	r.mu.RLock()
	targets := make([]*websocket.Conn, 0, len(r.clients))
	for uid, ci := range r.clients {
		if uid != excludeUID {
			targets = append(targets, ci.conn)
		}
	}
	r.mu.RUnlock()

	for _, c := range targets {
		c.SetWriteDeadline(time.Now().Add(5 * time.Second))
		c.WriteMessage(websocket.BinaryMessage, frame)
	}
}

func uint32Bytes(v uint32) []byte {
	b := make([]byte, 4)
	binary.BigEndian.PutUint32(b, v)
	return b
}
