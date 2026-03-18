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

type relay struct {
	clients map[string]*websocket.Conn // UserID → connection
	aliases map[string]string          // UserID → current alias
	pending map[string][][]byte        // UserID → queued messages
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
		clients: make(map[string]*websocket.Conn),
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

// handleOnline returns list of currently connected aliases.
func (r *relay) handleOnline(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	r.mu.RLock()
	online := make([]string, 0, len(r.aliases))
	for _, alias := range r.aliases {
		online = append(online, alias)
	}
	r.mu.RUnlock()

	json.NewEncoder(w).Encode(map[string]interface{}{
		"count":   len(online),
		"aliases": online,
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

	userID := fmt.Sprintf("%x", pubkeyMsg[:20])

	// Register client with alias
	r.mu.Lock()
	oldConn, existed := r.clients[userID]
	if existed && oldConn != nil {
		oldConn.Close()
	}
	r.clients[userID] = conn
	r.aliases[userID] = r.pickAlias()

	pending := r.pending[userID]
	delete(r.pending, userID)
	r.mu.Unlock()

	for _, msg := range pending {
		conn.WriteMessage(websocket.BinaryMessage, msg)
	}

	// Read loop
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

		frame := make([]byte, 20+len(msgData))
		copy(frame[:20], pubkeyMsg[:20])
		copy(frame[20:], msgData)

		r.mu.RLock()
		target, online := r.clients[recipID]
		r.mu.RUnlock()

		if online {
			target.WriteMessage(websocket.BinaryMessage, frame)
		} else {
			r.mu.Lock()
			if len(r.pending[recipID]) < 1000 {
				r.pending[recipID] = append(r.pending[recipID], frame)
			}
			r.mu.Unlock()
		}
	}

	// Unregister — alias удаляется, при следующем входе будет новая
	r.mu.Lock()
	if r.clients[userID] == conn {
		delete(r.clients, userID)
		delete(r.aliases, userID)
	}
	r.mu.Unlock()
}

func uint32Bytes(v uint32) []byte {
	b := make([]byte, 4)
	binary.BigEndian.PutUint32(b, v)
	return b
}
