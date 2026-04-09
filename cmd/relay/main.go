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

type relayStats struct {
	msgRelayed    int64  // messages forwarded this hour
	bytesRelayed  int64  // bytes forwarded this hour
	connections   int64  // new connections this hour
	filesBlocked  int64  // file chunks dropped this hour
	holdStored    int64  // messages stored in hold this hour
	holdDelivered int64  // messages delivered from hold this hour
}

type relay struct {
	clients      map[string]*clientInfo   // UserID → client info (online only)
	aliases      map[string]string        // UserID → current alias
	hold         map[string][]holdEntry   // UserID → queued messages (СВХ)
	knownPeers   map[string]*knownPeer   // UserID → ever-connected peer (TTL 30 days)
	lastSync     map[string]time.Time     // UserID → last NEWSYNC time (cooldown)
	knownRelays  []string                 // known relay URLs (federation)
	stats        relayStats               // hourly stats
	mu           sync.RWMutex
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
		lastSync:   make(map[string]time.Time),
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

	// Cleanup + hourly stats log
	go func() {
		for range time.Tick(1 * time.Hour) {
			r.cleanupHold()
			r.cleanupKnownPeers()
			r.logHourlyStats()
		}
	}()

	// Self-ping to prevent hosting platform sleep (HF Spaces, Render free tier)
	go func() {
		selfURL := fmt.Sprintf("http://localhost:%d/", *port)
		for range time.Tick(10 * time.Minute) {
			http.Get(selfURL)
		}
	}()

	http.HandleFunc("/ws", r.handleWS)
	http.HandleFunc("/online", r.handleOnline)
	http.HandleFunc("/directory", r.handleDirectory)
	http.HandleFunc("/hold-stats", r.handleHoldStats)
	http.HandleFunc("/api/telemetry", r.handleTelemetry)
	http.HandleFunc("/api/stats", r.handleStats)
	http.HandleFunc("/api/federation", r.handleFederation)
	http.HandleFunc("/", func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		fmt.Fprintf(w, "Iskra Relay v0.4 (СВХ + Telemetry)\n")
	})

	addr := fmt.Sprintf(":%d", *port)
	fmt.Printf("🔥 Искра Relay v0.4 (СВХ + Telemetry)\n")
	fmt.Printf("   Порт: %d\n", *port)
	fmt.Printf("   WebSocket: ws://0.0.0.0:%d/ws\n", *port)
	fmt.Printf("   Трюм: TTL %v, макс %d/получатель, %d всего\n", holdTTL, holdMaxPerUID, holdMaxTotal)
	fmt.Printf("   Телеметрия: %d уникальных устройств\n", len(telemetry.Devices))
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
		r.mu.Lock()
		r.stats.holdDelivered += int64(len(held))
		r.mu.Unlock()
		r.saveHold()
	}

	// Notify peers — but with cooldown (max once per 5 min per client)
	r.mu.Lock()
	lastSyncTime := r.lastSync[userID]
	shouldSync := time.Since(lastSyncTime) > 5*time.Minute
	if shouldSync {
		r.lastSync[userID] = time.Now()
	}
	r.stats.connections++
	r.mu.Unlock()
	if shouldSync {
		syncNotify := make([]byte, 20+7)
		copy(syncNotify[:20], edPub[:20])
		copy(syncNotify[20:], []byte("NEWSYNC"))
		r.broadcastExcept(userID, syncNotify)
	}

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

		// Block file chunks through relay — too expensive for bandwidth
		// ContentType is at offset 65 in serialized message (version:1 + id:32 + recipientID:20 + ttl:4 + timestamp:8 = 65)
		if len(msgData) > 65 && msgData[65] == 6 { // 6 = ContentFileChunk
			r.mu.Lock()
			r.stats.filesBlocked++
			r.mu.Unlock()
			continue // silently drop — files only via LAN/direct
		}

		// Track traffic stats
		r.mu.Lock()
		r.stats.msgRelayed++
		r.stats.bytesRelayed += int64(len(data))
		r.mu.Unlock()

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
					r.stats.holdStored++
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

// logHourlyStats logs and resets hourly statistics.
func (r *relay) logHourlyStats() {
	r.mu.Lock()
	s := r.stats
	r.stats = relayStats{} // reset
	online := len(r.clients)
	holdMsgs := 0
	for _, entries := range r.hold {
		holdMsgs += len(entries)
	}
	r.mu.Unlock()

	log.Printf("[Stats] Hour: connections=%d msgs=%d bytes=%s files_blocked=%d hold_stored=%d hold_delivered=%d online=%d hold=%d",
		s.connections, s.msgRelayed, formatBytes(s.bytesRelayed), s.filesBlocked,
		s.holdStored, s.holdDelivered, online, holdMsgs)
}

func formatBytes(b int64) string {
	if b < 1024 {
		return fmt.Sprintf("%dB", b)
	} else if b < 1024*1024 {
		return fmt.Sprintf("%.1fKB", float64(b)/1024)
	} else if b < 1024*1024*1024 {
		return fmt.Sprintf("%.1fMB", float64(b)/(1024*1024))
	}
	return fmt.Sprintf("%.2fGB", float64(b)/(1024*1024*1024))
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

// === ANONYMOUS TELEMETRY ===
// Tracks unique devices, platforms, models — no personal data.
// device_hash = SHA-256(ANDROID_ID + salt) — irreversible.

const telemetryDir = "relay-telemetry"

type deviceRecord struct {
	DeviceHash  string `json:"device_hash"`
	Platform    string `json:"platform"`     // android, windows, linux
	Model       string `json:"model"`        // "Samsung SM-S926B"
	OSVersion   string `json:"os_version"`   // "Android 15", "Win11"
	AppVersion  string `json:"app_version"`  // "2.0-b8"
	Transport   string `json:"transport"`    // relay, dns, lan, wifi_direct
	Lang        string `json:"lang"`         // ru, en
	HoldCount   int    `json:"hold_count"`   // messages in hold
	ContactCount int   `json:"contact_count"`
	UptimeMin   int    `json:"uptime_min"`
	FirstSeen   int64  `json:"first_seen"`
	LastSeen    int64  `json:"last_seen"`
	Sessions    int    `json:"sessions"`
}

type telemetryStore struct {
	Devices map[string]*deviceRecord `json:"devices"` // device_hash → record
	mu      sync.RWMutex
}

var telemetry = &telemetryStore{
	Devices: make(map[string]*deviceRecord),
}

func init() {
	telemetry.load()
}

func (ts *telemetryStore) load() {
	path := telemetryDir + "/devices.json"
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	ts.mu.Lock()
	defer ts.mu.Unlock()
	json.Unmarshal(data, &ts.Devices)
}

func (ts *telemetryStore) save() {
	ts.mu.RLock()
	data, err := json.MarshalIndent(ts.Devices, "", "  ")
	ts.mu.RUnlock()
	if err != nil {
		return
	}
	os.MkdirAll(telemetryDir, 0700)
	os.WriteFile(telemetryDir+"/devices.json", data, 0600)
}

func (ts *telemetryStore) record(dr *deviceRecord) {
	if dr.DeviceHash == "" || len(dr.DeviceHash) < 16 {
		return // invalid
	}
	now := time.Now().Unix()
	ts.mu.Lock()
	existing, ok := ts.Devices[dr.DeviceHash]
	if ok {
		// Update existing
		existing.LastSeen = now
		existing.Sessions++
		if dr.Platform != "" { existing.Platform = dr.Platform }
		if dr.Model != "" { existing.Model = dr.Model }
		if dr.OSVersion != "" { existing.OSVersion = dr.OSVersion }
		if dr.AppVersion != "" { existing.AppVersion = dr.AppVersion }
		if dr.Transport != "" { existing.Transport = dr.Transport }
		if dr.Lang != "" { existing.Lang = dr.Lang }
		existing.HoldCount = dr.HoldCount
		existing.ContactCount = dr.ContactCount
		existing.UptimeMin = dr.UptimeMin
	} else {
		// New device
		dr.FirstSeen = now
		dr.LastSeen = now
		dr.Sessions = 1
		ts.Devices[dr.DeviceHash] = dr
	}
	ts.mu.Unlock()
	ts.save()
}

func (ts *telemetryStore) stats() map[string]interface{} {
	ts.mu.RLock()
	defer ts.mu.RUnlock()

	now := time.Now().Unix()
	total := len(ts.Devices)
	active24h := 0
	active7d := 0
	platforms := make(map[string]int)
	versions := make(map[string]int)
	transports := make(map[string]int)
	models := make(map[string]int)
	langs := make(map[string]int)
	totalHold := 0
	totalContacts := 0
	totalSessions := 0
	returnUsers := 0 // sessions > 1

	for _, d := range ts.Devices {
		if now - d.LastSeen < 24*3600 { active24h++ }
		if now - d.LastSeen < 7*24*3600 { active7d++ }
		if d.Platform != "" { platforms[d.Platform]++ }
		if d.AppVersion != "" { versions[d.AppVersion]++ }
		if d.Transport != "" { transports[d.Transport]++ }
		if d.Model != "" { models[d.Model]++ }
		if d.Lang != "" { langs[d.Lang]++ }
		totalHold += d.HoldCount
		totalContacts += d.ContactCount
		totalSessions += d.Sessions
		if d.Sessions > 1 { returnUsers++ }
	}

	// Top 10 models
	type kv struct { K string; V int }
	var topModels []kv
	for k, v := range models {
		topModels = append(topModels, kv{k, v})
	}
	// Simple sort (no import needed for small list)
	for i := 0; i < len(topModels); i++ {
		for j := i + 1; j < len(topModels); j++ {
			if topModels[j].V > topModels[i].V {
				topModels[i], topModels[j] = topModels[j], topModels[i]
			}
		}
	}
	topModelList := make([]string, 0, 10)
	for i, m := range topModels {
		if i >= 10 { break }
		topModelList = append(topModelList, fmt.Sprintf("%s (%d)", m.K, m.V))
	}

	avgHold := 0.0
	retention := 0.0
	if total > 0 {
		avgHold = float64(totalHold) / float64(total)
		retention = float64(returnUsers) / float64(total) * 100
	}

	return map[string]interface{}{
		"total_unique_devices": total,
		"active_24h":          active24h,
		"active_7d":           active7d,
		"return_rate_pct":     fmt.Sprintf("%.1f", retention),
		"total_sessions":      totalSessions,
		"platforms":           platforms,
		"versions":            versions,
		"transports":          transports,
		"languages":           langs,
		"top_models":          topModelList,
		"avg_hold_count":      fmt.Sprintf("%.1f", avgHold),
	}
}

// handleTelemetry accepts anonymous device reports.
func (r *relay) handleTelemetry(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	if req.Method == "OPTIONS" {
		return
	}
	if req.Method != "POST" {
		http.Error(w, "POST only", 405)
		return
	}

	var dr deviceRecord
	if err := json.NewDecoder(req.Body).Decode(&dr); err != nil {
		http.Error(w, "bad json", 400)
		return
	}
	telemetry.record(&dr)
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"ok":true}`)
}

// handleStats returns public aggregate statistics.
func (r *relay) handleStats(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	json.NewEncoder(w).Encode(telemetry.stats())
}

// handleFederation allows relay-to-relay and client-to-relay exchange of known relay URLs.
// GET: returns known relays. POST: announce a new relay URL.
func (r *relay) handleFederation(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	if req.Method == "OPTIONS" { return }

	switch req.Method {
	case "GET":
		r.mu.RLock()
		relays := make([]string, len(r.knownRelays))
		copy(relays, r.knownRelays)
		r.mu.RUnlock()
		json.NewEncoder(w).Encode(map[string]interface{}{
			"relays": relays,
			"count":  len(relays),
		})
	case "POST":
		var req2 struct {
			URL string `json:"url"`
		}
		if err := json.NewDecoder(req.Body).Decode(&req2); err != nil || req2.URL == "" {
			http.Error(w, "url required", 400)
			return
		}
		r.mu.Lock()
		found := false
		for _, u := range r.knownRelays {
			if u == req2.URL { found = true; break }
		}
		if !found {
			r.knownRelays = append(r.knownRelays, req2.URL)
			log.Printf("[Federation] New relay announced: %s", req2.URL)
		}
		r.mu.Unlock()
		fmt.Fprintf(w, `{"ok":true,"new":%v}`, !found)
	default:
		http.Error(w, "GET or POST", 405)
	}
}

func uint32Bytes(v uint32) []byte {
	b := make([]byte, 4)
	binary.BigEndian.PutUint32(b, v)
	return b
}
