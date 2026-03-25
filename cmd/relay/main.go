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

// Relay — минимальный WebSocket relay для Искры.
// Не логирует. Не расшифровывает. Не хранит на диск.
// Просто передаёт зашифрованные блобы между клиентами.

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
}

type relay struct {
	clients map[string]*clientInfo   // UserID → client info
	aliases map[string]string        // UserID → current alias
	pending map[string][][]byte      // UserID → queued messages
	mu      sync.RWMutex
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
		clients: make(map[string]*clientInfo),
		aliases: make(map[string]string),
		pending: make(map[string][][]byte),
	}

	http.HandleFunc("/ws", r.handleWS)
	http.HandleFunc("/online", r.handleOnline)
	http.HandleFunc("/", func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		fmt.Fprintf(w, "Iskra Relay v0.1\n")
	})

	addr := fmt.Sprintf(":%d", *port)
	fmt.Printf("🔥 Искра Relay\n")
	fmt.Printf("   Порт: %d\n", *port)
	fmt.Printf("   WebSocket: ws://0.0.0.0:%d/ws\n", *port)
	fmt.Println("   Не логирует. Не расшифровывает. Просто передаёт.")
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
	for uid, alias := range r.aliases {
		if ci, ok := r.clients[uid]; ok {
			peers = append(peers, onlinePeer{
				Alias:  alias,
				EdPub:  fmt.Sprintf("%x", ci.edPub),
				X25519: fmt.Sprintf("%x", ci.x25519Pub),
			})
		}
	}
	r.mu.RUnlock()

	json.NewEncoder(w).Encode(map[string]interface{}{
		"count": len(peers),
		"peers": peers,
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

	// Register client with alias
	r.mu.Lock()
	oldCI, existed := r.clients[userID]
	if existed && oldCI != nil {
		oldCI.conn.Close()
	}
	r.clients[userID] = &clientInfo{conn: conn, edPub: edPub, x25519Pub: x25519Pub}
	r.aliases[userID] = r.pickAlias()

	pending := r.pending[userID]
	delete(r.pending, userID)
	r.mu.Unlock()

	for _, msg := range pending {
		conn.WriteMessage(websocket.BinaryMessage, msg)
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
				targetCI.conn.WriteMessage(websocket.BinaryMessage, frame)
			} else {
				// Recipient offline — broadcast to all (store in their holds)
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
