package mesh

import (
	"fmt"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/iskra-messenger/iskra/internal/message"
)

// RelayClient connects to a relay server for cross-network message delivery.
type RelayClient struct {
	url       string
	pubKey    [32]byte
	conn      *websocket.Conn
	onMessage func(*message.Message)
	mu        sync.Mutex
	stop      chan struct{}
	connected bool
}

// NewRelayClient creates a relay client.
func NewRelayClient(url string, pubKey [32]byte) *RelayClient {
	return &RelayClient{
		url:    url,
		pubKey: pubKey,
		stop:   make(chan struct{}),
	}
}

// SetOnMessage sets the callback for messages received via relay.
func (rc *RelayClient) SetOnMessage(fn func(*message.Message)) {
	rc.onMessage = fn
}

// Start connects to the relay and begins reading messages.
func (rc *RelayClient) Start() error {
	if err := rc.connect(); err != nil {
		// Don't fail — retry in background
		go rc.reconnectLoop()
		return nil
	}
	go rc.readLoop()
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
		case <-time.After(10 * time.Second):
		}

		rc.mu.Lock()
		isConn := rc.connected
		rc.mu.Unlock()

		if !isConn {
			if err := rc.connect(); err == nil {
				go rc.readLoop()
			}
		}
	}
}
