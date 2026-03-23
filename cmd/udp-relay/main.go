// UDP relay server for Iskra.
// Receives obfuscated UDP datagrams, extracts recipient_id, forwards to registered clients.
// DPI sees: random UDP traffic on a single port. No patterns, no handshakes.
package main

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

var networkObfKey = []byte("iskra-udp-v1-обфускация-сети")

const (
	nonceSize    = 8
	idSize       = 20
	maxUDPPacket = 65507
)

type client struct {
	addr     *net.UDPAddr
	lastSeen time.Time
}

type relay struct {
	conn    *net.UDPConn
	clients map[string]*client // hex(id) -> client
	mu      sync.RWMutex
}

func main() {
	port := flag.Int("port", 4243, "UDP port to listen on")
	flag.Parse()

	addr := &net.UDPAddr{Port: *port}
	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}

	r := &relay{
		conn:    conn,
		clients: make(map[string]*client),
	}

	fmt.Printf("🔥 Искра UDP Relay\n")
	fmt.Printf("   Порт: %d\n", *port)
	fmt.Printf("   Обфускация: включена\n\n")

	// Clean stale clients periodically
	go r.cleanupLoop()

	// Main packet processing loop
	go r.readLoop()

	// Wait for shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan
	fmt.Println("\nОстановка...")
	conn.Close()
}

func (r *relay) readLoop() {
	buf := make([]byte, maxUDPPacket)
	for {
		n, addr, err := r.conn.ReadFromUDP(buf)
		if err != nil {
			log.Printf("Read error: %v", err)
			return
		}

		if n < nonceSize+idSize*2 {
			continue // too short, ignore
		}

		// Deobfuscate
		data, err := deobfuscate(buf[:n])
		if err != nil || len(data) < idSize*2 {
			continue
		}

		senderID := fmt.Sprintf("%x", data[:idSize])
		recipientID := fmt.Sprintf("%x", data[idSize:idSize*2])

		// Register sender
		r.mu.Lock()
		r.clients[senderID] = &client{addr: addr, lastSeen: time.Now()}
		r.mu.Unlock()

		// Check if this is a registration packet (recipient_id = all zeros)
		allZeros := true
		for _, b := range data[idSize : idSize*2] {
			if b != 0 {
				allZeros = false
				break
			}
		}
		if allZeros || len(data) <= idSize*2 {
			log.Printf("[REG] %s from %s (%d clients)", senderID[:8], addr, r.clientCount())
			continue
		}

		// Find recipient and forward
		r.mu.RLock()
		dest, ok := r.clients[recipientID]
		r.mu.RUnlock()

		if ok {
			// Re-obfuscate with new nonce and forward
			forwarded := obfuscate(data)
			r.conn.WriteToUDP(forwarded, dest.addr)
			log.Printf("[FWD] %s -> %s (%d bytes)", senderID[:8], recipientID[:8], n)
		} else {
			log.Printf("[MISS] %s -> %s (recipient offline)", senderID[:8], recipientID[:8])
		}
	}
}

func (r *relay) cleanupLoop() {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		r.mu.Lock()
		for id, c := range r.clients {
			if time.Since(c.lastSeen) > 5*time.Minute {
				delete(r.clients, id)
				log.Printf("[CLEANUP] Removed stale client %s", id[:8])
			}
		}
		r.mu.Unlock()
	}
}

func (r *relay) clientCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.clients)
}

func obfuscate(data []byte) []byte {
	nonce := make([]byte, nonceSize)
	rand.Read(nonce)
	stream := generateKeyStream(nonce, len(data))
	result := make([]byte, nonceSize+len(data))
	copy(result[:nonceSize], nonce)
	for i := range data {
		result[nonceSize+i] = data[i] ^ stream[i]
	}
	return result
}

func deobfuscate(packet []byte) ([]byte, error) {
	if len(packet) < nonceSize+1 {
		return nil, fmt.Errorf("too short")
	}
	nonce := packet[:nonceSize]
	encrypted := packet[nonceSize:]
	stream := generateKeyStream(nonce, len(encrypted))
	data := make([]byte, len(encrypted))
	for i := range encrypted {
		data[i] = encrypted[i] ^ stream[i]
	}
	return data, nil
}

func generateKeyStream(nonce []byte, length int) []byte {
	stream := make([]byte, 0, length)
	counter := byte(0)
	for len(stream) < length {
		mac := hmac.New(sha256.New, networkObfKey)
		mac.Write(nonce)
		mac.Write([]byte{counter})
		stream = append(stream, mac.Sum(nil)...)
		counter++
	}
	return stream[:length]
}
