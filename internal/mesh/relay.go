package mesh

import (
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/iskra-messenger/iskra/internal/message"
)

const (
	relayPingInterval  = 25 * time.Second // Keep Render awake (sleeps after 50s)
	relayReconnectWait = 10 * time.Second
	relayWakeupDelay   = 2 * time.Second // Wait for cold start after wake-up HTTP ping
)

// RelayClient connects to a relay server for cross-network message delivery.
type RelayClient struct {
	url       string
	httpURL   string // HTTPS URL for wake-up pings
	pubKey    [32]byte
	conn      *websocket.Conn
	onMessage func(*message.Message)
	mu        sync.Mutex
	stop      chan struct{}
	connected bool
}

// NewRelayClient creates a relay client.
func NewRelayClient(url string, pubKey [32]byte) *RelayClient {
	// Derive HTTP URL from WebSocket URL for wake-up pings
	// wss://host/ws → https://host/
	httpURL := url
	if len(httpURL) > 0 {
		httpURL = "https" + httpURL[3:] // wss:// → https://
		// Strip /ws path
		if len(httpURL) > 4 && httpURL[len(httpURL)-3:] == "/ws" {
			httpURL = httpURL[:len(httpURL)-3]
		}
	}

	return &RelayClient{
		url:     url,
		httpURL: httpURL,
		pubKey:  pubKey,
		stop:    make(chan struct{}),
	}
}

// SetOnMessage sets the callback for messages received via relay.
func (rc *RelayClient) SetOnMessage(fn func(*message.Message)) {
	rc.onMessage = fn
}

// Start connects to the relay and begins reading messages.
func (rc *RelayClient) Start() error {
	// Wake up the relay first (cold start on Render free tier)
	rc.wakeUp()

	if err := rc.connect(); err != nil {
		go rc.reconnectLoop()
		return nil
	}
	go rc.readLoop()
	go rc.pingLoop()
	go rc.reconnectLoop()
	return nil
}

// Stop disconnects from relay.
func (rc *RelayClient) Stop() {
	close(rc.stop)
	rc.mu.Lock()
	if rc.conn != nil {
		rc.conn.Close()
	}
	rc.mu.Unlock()
}

// IsConnected returns whether the relay is connected.
func (rc *RelayClient) IsConnected() bool {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	return rc.connected
}

// SendMessage sends a message to a recipient via relay.
// Frame format: [recipientID:20][serialized message]
func (rc *RelayClient) SendMessage(msg *message.Message) error {
	rc.mu.Lock()
	defer rc.mu.Unlock()

	if rc.conn == nil {
		return fmt.Errorf("relay not connected")
	}

	serialized := msg.Serialize()
	frame := make([]byte, 20+len(serialized))
	copy(frame[:20], msg.RecipientID[:])
	copy(frame[20:], serialized)

	return rc.conn.WriteMessage(websocket.BinaryMessage, frame)
}

// BroadcastMessage sends a message to all — for relay, send with zero recipient.
func (rc *RelayClient) BroadcastMessage(msg *message.Message) error {
	return rc.SendMessage(msg)
}

// wakeUp sends an HTTP GET to the relay to wake it from Render's sleep.
// First call wakes, we wait, then the WebSocket connect will succeed.
func (rc *RelayClient) wakeUp() {
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(rc.httpURL)
	if err != nil {
		log.Printf("Relay wake-up ping failed (will retry): %v", err)
		return
	}
	resp.Body.Close()
	// Give the service time to fully start after cold boot
	time.Sleep(relayWakeupDelay)
}

func (rc *RelayClient) connect() error {
	rc.mu.Lock()
	defer rc.mu.Unlock()

	conn, _, err := websocket.DefaultDialer.Dial(rc.url, nil)
	if err != nil {
		return fmt.Errorf("relay connect failed: %w", err)
	}

	// Send pubkey as first message
	if err := conn.WriteMessage(websocket.BinaryMessage, rc.pubKey[:]); err != nil {
		conn.Close()
		return fmt.Errorf("relay handshake failed: %w", err)
	}

	rc.conn = conn
	rc.connected = true
	return nil
}

// pingLoop sends WebSocket pings every 25s to keep Render awake and detect dead connections.
func (rc *RelayClient) pingLoop() {
	ticker := time.NewTicker(relayPingInterval)
	defer ticker.Stop()

	for {
		select {
		case <-rc.stop:
			return
		case <-ticker.C:
		}

		rc.mu.Lock()
		conn := rc.conn
		isConn := rc.connected
		rc.mu.Unlock()

		if !isConn || conn == nil {
			return
		}

		rc.mu.Lock()
		err := conn.WriteMessage(websocket.PingMessage, []byte("iskra"))
		rc.mu.Unlock()

		if err != nil {
			// Connection dead — mark disconnected, reconnectLoop will handle it
			rc.mu.Lock()
			rc.connected = false
			if rc.conn != nil {
				rc.conn.Close()
				rc.conn = nil
			}
			rc.mu.Unlock()
			return
		}
	}
}

func (rc *RelayClient) readLoop() {
	for {
		select {
		case <-rc.stop:
			return
		default:
		}

		rc.mu.Lock()
		conn := rc.conn
		rc.mu.Unlock()

		if conn == nil {
			time.Sleep(time.Second)
			continue
		}

		_, data, err := conn.ReadMessage()
		if err != nil {
			rc.mu.Lock()
			rc.connected = false
			rc.conn = nil
			rc.mu.Unlock()
			return
		}

		// Frame: [senderID:20][serialized message]
		if len(data) < 20 {
			continue
		}

		msgData := data[20:]
		msg, err := message.Deserialize(msgData)
		if err != nil {
			continue
		}

		if rc.onMessage != nil {
			rc.onMessage(msg)
		}
	}
}

func (rc *RelayClient) reconnectLoop() {
	for {
		select {
		case <-rc.stop:
			return
		case <-time.After(relayReconnectWait):
		}

		rc.mu.Lock()
		isConn := rc.connected
		rc.mu.Unlock()

		if !isConn {
			// Wake up relay first (may be sleeping on Render)
			rc.wakeUp()

			if err := rc.connect(); err == nil {
				go rc.readLoop()
				go rc.pingLoop()
			}
		}
	}
}
