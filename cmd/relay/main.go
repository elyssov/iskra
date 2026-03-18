package main

import (
	"encoding/binary"
	"flag"
	"fmt"
	"log"
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

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

type relay struct {
	clients map[string]*websocket.Conn // UserID (hex of first 20 bytes pubkey) → connection
	pending map[string][][]byte        // UserID → queued messages (max 1000)
	mu      sync.RWMutex
}

func main() {
	port := flag.Int("port", 8443, "Listen port")
	flag.Parse()

	// Fly.io sets PORT env var
	if envPort := os.Getenv("PORT"); envPort != "" {
		if p, err := strconv.Atoi(envPort); err == nil {
			*port = p
		}
	}

	r := &relay{
		clients: make(map[string]*websocket.Conn),
		pending: make(map[string][][]byte),
	}

	http.HandleFunc("/ws", r.handleWS)
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

func (r *relay) handleWS(w http.ResponseWriter, req *http.Request) {
	conn, err := upgrader.Upgrade(w, req, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	// Configure timeouts: expect ping from client every 25s, allow 60s grace
	conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})
	conn.SetPingHandler(func(msg string) error {
		conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		conn.WriteControl(websocket.PongMessage, []byte(msg), time.Now().Add(5*time.Second))
		return nil
	})

	// First message: client sends their pubkey (32 bytes)
	_, pubkeyMsg, err := conn.ReadMessage()
	if err != nil || len(pubkeyMsg) != 32 {
		return
	}

	// UserID = hex of first 20 bytes
	userID := fmt.Sprintf("%x", pubkeyMsg[:20])

	// Register client
	r.mu.Lock()
	oldConn, existed := r.clients[userID]
	if existed && oldConn != nil {
		oldConn.Close() // Close old connection
	}
	r.clients[userID] = conn

	// Deliver pending messages
	pending := r.pending[userID]
	delete(r.pending, userID)
	r.mu.Unlock()

	for _, msg := range pending {
		conn.WriteMessage(websocket.BinaryMessage, msg)
	}

	// Read loop: each message is [recipientID:20][msgData:variable]
	for {
		conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		_, data, err := conn.ReadMessage()
		if err != nil {
			break
		}

		if len(data) < 20 {
			continue
		}

		recipID := fmt.Sprintf("%x", data[:20])
		msgData := data[20:]

		// Build delivery frame: [senderID:20][msgData:variable]
		frame := make([]byte, 20+len(msgData))
		copy(frame[:20], pubkeyMsg[:20])
		copy(frame[20:], msgData)

		r.mu.RLock()
		target, online := r.clients[recipID]
		r.mu.RUnlock()

		if online {
			target.WriteMessage(websocket.BinaryMessage, frame)
		} else {
			// Queue for later delivery (max 1000 messages per user)
			r.mu.Lock()
			if len(r.pending[recipID]) < 1000 {
				r.pending[recipID] = append(r.pending[recipID], frame)
			}
			r.mu.Unlock()
		}
	}

	// Unregister
	r.mu.Lock()
	if r.clients[userID] == conn {
		delete(r.clients, userID)
	}
	r.mu.Unlock()
}

// Helper for message framing (not used yet, reserved for future)
func uint32Bytes(v uint32) []byte {
	b := make([]byte, 4)
	binary.BigEndian.PutUint32(b, v)
	return b
}
