package mesh

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"log"
	"net"
	"sync"
	"time"

	"github.com/iskra-messenger/iskra/internal/identity"
	"github.com/iskra-messenger/iskra/internal/message"
)

// Network-wide obfuscation key. NOT for crypto security (messages already encrypted).
// Only to fool DPI pattern matching — makes UDP datagrams look like random noise.
var networkObfKey = []byte("iskra-udp-v1-обфускация-сети")

const (
	maxUDPPacket = 65507 // max UDP payload
	nonceSize    = 8
	idSize       = 20
)

// UDPTransport sends/receives obfuscated UDP datagrams through a UDP relay.
type UDPTransport struct {
	pubKey    [32]byte
	myID      [idSize]byte // first 20 bytes of pubKey (RecipientID format)
	relayAddr *net.UDPAddr
	conn      *net.UDPConn
	onMessage func(*message.Message)
	mu        sync.RWMutex
	stop      chan struct{}
	connected bool
}

// NewUDPTransport creates a new obfuscated UDP transport.
// relayAddress is "host:port" of the UDP relay server.
func NewUDPTransport(pubKey [32]byte, relayAddress string) (*UDPTransport, error) {
	addr, err := net.ResolveUDPAddr("udp", relayAddress)
	if err != nil {
		return nil, fmt.Errorf("resolve relay address: %w", err)
	}

	var myID [idSize]byte
	copy(myID[:], pubKey[:idSize])

	return &UDPTransport{
		pubKey:    pubKey,
		myID:      myID,
		relayAddr: addr,
		stop:      make(chan struct{}),
	}, nil
}

// SetOnMessage sets the callback for received messages.
func (u *UDPTransport) SetOnMessage(fn func(*message.Message)) {
	u.onMessage = fn
}

// Start begins listening for incoming UDP packets and registers with relay.
func (u *UDPTransport) Start() error {
	conn, err := net.ListenUDP("udp", &net.UDPAddr{Port: 0})
	if err != nil {
		return fmt.Errorf("udp listen: %w", err)
	}
	u.conn = conn
	u.connected = true

	// Register with relay by sending a registration packet
	u.sendRegistration()

	go u.readLoop()
	go u.keepaliveLoop()

	log.Printf("[UDP] Started on %s, relay=%s", conn.LocalAddr(), u.relayAddr)
	return nil
}

// Stop shuts down the UDP transport.
func (u *UDPTransport) Stop() {
	close(u.stop)
	if u.conn != nil {
		u.conn.Close()
	}
	u.connected = false
}

// IsConnected returns whether the transport is active.
func (u *UDPTransport) IsConnected() bool {
	u.mu.RLock()
	defer u.mu.RUnlock()
	return u.connected
}

// SendMessage sends an obfuscated message through the UDP relay.
func (u *UDPTransport) SendMessage(msg *message.Message) error {
	serialized := msg.Serialize()
	recipientID := msg.RecipientID[:idSize]

	// Packet: sender_id(20) + recipient_id(20) + serialized_message
	packet := make([]byte, idSize+idSize+len(serialized))
	copy(packet[:idSize], u.myID[:])
	copy(packet[idSize:idSize*2], recipientID)
	copy(packet[idSize*2:], serialized)

	obfuscated := obfuscate(packet)

	_, err := u.conn.WriteToUDP(obfuscated, u.relayAddr)
	if err != nil {
		log.Printf("[UDP] Send error: %v", err)
		return err
	}
	return nil
}

// BroadcastMessage sends message via relay (relay routes by recipient_id).
func (u *UDPTransport) BroadcastMessage(msg *message.Message) {
	u.SendMessage(msg)
}

func (u *UDPTransport) sendRegistration() {
	// Registration: sender_id + recipient_id(zeros) + empty payload
	// Relay sees sender_id and maps it to our address
	packet := make([]byte, idSize*2)
	copy(packet[:idSize], u.myID[:])
	// recipient_id is all zeros = registration signal

	obfuscated := obfuscate(packet)
	u.conn.WriteToUDP(obfuscated, u.relayAddr)
	log.Printf("[UDP] Registered with relay as %s", identity.UserID(u.pubKey))
}

func (u *UDPTransport) readLoop() {
	buf := make([]byte, maxUDPPacket)
	for {
		select {
		case <-u.stop:
			return
		default:
		}

		n, _, err := u.conn.ReadFromUDP(buf)
		if err != nil {
			select {
			case <-u.stop:
				return
			default:
				log.Printf("[UDP] Read error: %v", err)
				continue
			}
		}

		if n < nonceSize+idSize*2 {
			continue // too short
		}

		data, err := deobfuscate(buf[:n])
		if err != nil {
			continue // corrupt packet, ignore
		}

		// data = sender_id(20) + recipient_id(20) + serialized_message
		if len(data) < idSize*2+230 { // 230 = min message size
			continue
		}

		// Skip sender_id and recipient_id, deserialize message
		msgData := data[idSize*2:]
		msg, err := message.Deserialize(msgData)
		if err != nil {
			log.Printf("[UDP] Deserialize error: %v", err)
			continue
		}

		if u.onMessage != nil {
			u.onMessage(msg)
		}
	}
}

func (u *UDPTransport) keepaliveLoop() {
	ticker := time.NewTicker(25 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-u.stop:
			return
		case <-ticker.C:
			u.sendRegistration()
		}
	}
}

// obfuscate wraps data with a random nonce and XOR key stream.
// Output: [nonce(8)] [XOR'd data]
// DPI sees: random bytes, varying lengths, no patterns.
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

// deobfuscate extracts data from an obfuscated packet.
func deobfuscate(packet []byte) ([]byte, error) {
	if len(packet) < nonceSize+1 {
		return nil, fmt.Errorf("packet too short")
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

// generateKeyStream creates a XOR key stream from nonce using HMAC-SHA256.
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
