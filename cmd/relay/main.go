package main

import (
	"encoding/binary"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"sync"
)

// Relay is a minimal TCP relay for Iskra nodes.
// Nodes connect, send their pubkey (20 bytes UserID), then send/receive messages.
// The relay does NOT decrypt anything — it only routes encrypted blobs.

type relay struct {
	clients map[string]net.Conn // UserID hex → connection
	pending map[string][][]byte // UserID hex → queued messages
	mu      sync.RWMutex
}

func main() {
	port := flag.Int("port", 8443, "Listen port")
	flag.Parse()

	r := &relay{
		clients: make(map[string]net.Conn),
		pending: make(map[string][][]byte),
	}

	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", *port))
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}

	fmt.Printf("🔥 Искра Relay запущен на порту %d\n", *port)
	fmt.Println("   Не логирует. Не расшифровывает. Просто передаёт.")

	for {
		conn, err := ln.Accept()
		if err != nil {
			continue
		}
		go r.handleClient(conn)
	}
}

func (r *relay) handleClient(conn net.Conn) {
	defer conn.Close()

	// Read 20-byte UserID
	idBuf := make([]byte, 20)
	if _, err := io.ReadFull(conn, idBuf); err != nil {
		return
	}
	userID := fmt.Sprintf("%x", idBuf)

	// Register
	r.mu.Lock()
	r.clients[userID] = conn
	// Send pending messages
	for _, msg := range r.pending[userID] {
		conn.Write(msg)
	}
	delete(r.pending, userID)
	r.mu.Unlock()

	defer func() {
		r.mu.Lock()
		delete(r.clients, userID)
		r.mu.Unlock()
	}()

	// Read messages: [recipientID:20][msgLen:4][msgData:variable]
	for {
		// Read recipient ID
		recipBuf := make([]byte, 20)
		if _, err := io.ReadFull(conn, recipBuf); err != nil {
			return
		}
		recipID := fmt.Sprintf("%x", recipBuf)

		// Read message length
		lenBuf := make([]byte, 4)
		if _, err := io.ReadFull(conn, lenBuf); err != nil {
			return
		}
		msgLen := binary.BigEndian.Uint32(lenBuf)
		if msgLen > 1*1024*1024 { // Max 1MB
			return
		}

		// Read message data
		msgData := make([]byte, msgLen)
		if _, err := io.ReadFull(conn, msgData); err != nil {
			return
		}

		// Build frame: [msgLen:4][msgData:variable]
		frame := make([]byte, 4+len(msgData))
		binary.BigEndian.PutUint32(frame[:4], msgLen)
		copy(frame[4:], msgData)

		// Route
		r.mu.RLock()
		if target, ok := r.clients[recipID]; ok {
			target.Write(frame)
		} else {
			// Queue for later (max 100 per user)
			r.mu.RUnlock()
			r.mu.Lock()
			if len(r.pending[recipID]) < 100 {
				r.pending[recipID] = append(r.pending[recipID], frame)
			}
			r.mu.Unlock()
			continue
		}
		r.mu.RUnlock()
	}
}
