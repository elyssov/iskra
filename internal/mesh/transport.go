package mesh

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"net"
	"sync"
	"time"

	"github.com/iskra-messenger/iskra/internal/message"
)

// bloomContains checks if a message ID exists in a remote bloom filter.
func bloomContains(bloomData []byte, id [32]byte) bool {
	remoteBits := uint64(len(bloomData) * 8)
	if remoteBits == 0 {
		return false
	}
	// Use same hash as store.SimpleBloom (double hashing)
	numHash := uint8(10) // default for our bloom config

	for i := uint8(0); i < numHash; i++ {
		data := make([]byte, 33)
		copy(data[:32], id[:])
		data[32] = 0
		h := sha256.Sum256(data)
		h1 := binary.BigEndian.Uint64(h[:8])

		data[32] = 1
		h = sha256.Sum256(data)
		h2 := binary.BigEndian.Uint64(h[:8])

		pos := (h1 + uint64(i)*h2) % remoteBits
		byteIdx := pos / 8
		bitIdx := pos % 8
		if byteIdx >= uint64(len(bloomData)) {
			return false
		}
		if bloomData[byteIdx]&(1<<bitIdx) == 0 {
			return false
		}
	}
	return true
}

// Protocol command types
const (
	CmdHello byte = 1
	CmdHave  byte = 2
	CmdWant  byte = 3
	CmdMsg   byte = 4
	CmdAck   byte = 5
)

// Transport manages TCP connections between nodes.
// Using TCP instead of KCP for alpha simplicity — KCP can be added in v0.2.
// HoldReader allows transport to read/update hold storage.
type HoldReader interface {
	Get(id [32]byte) (*message.Message, error)
	MarkForwarded(id [32]byte)
}

type Transport struct {
	pubKey     [32]byte
	listenPort uint16
	listener   net.Listener
	peers      *PeerList
	onMessage  func(*message.Message)
	onAck      func([32]byte) // called when peer ACKs a message
	hold       HoldReader     // fallback for WANT when message not in snapshot
	sessions   map[[32]byte]net.Conn
	mu         sync.RWMutex
	stop       chan struct{}
}

// NewTransport creates a new transport layer.
func NewTransport(pubKey [32]byte, listenPort uint16, peers *PeerList) *Transport {
	return &Transport{
		pubKey:     pubKey,
		listenPort: listenPort,
		peers:      peers,
		sessions:   make(map[[32]byte]net.Conn),
		stop:       make(chan struct{}),
	}
}

// SetOnMessage sets the callback for received messages.
func (t *Transport) SetOnMessage(fn func(*message.Message)) {
	t.onMessage = fn
}

// SetHold sets the hold storage for WANT fallback reads.
func (t *Transport) SetHold(h HoldReader) {
	t.hold = h
}

// SetOnAck sets the callback for received ACKs (delivery confirmation via LAN).
func (t *Transport) SetOnAck(fn func([32]byte)) {
	t.onAck = fn
}

// Start begins listening for connections.
func (t *Transport) Start() error {
	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", t.listenPort))
	if err != nil {
		return fmt.Errorf("failed to listen: %w", err)
	}
	t.listener = ln
	t.listenPort = uint16(ln.Addr().(*net.TCPAddr).Port)

	go t.acceptLoop()
	return nil
}

// Port returns the actual listening port.
func (t *Transport) Port() uint16 {
	return t.listenPort
}

// Stop stops the transport.
func (t *Transport) Stop() {
	close(t.stop)
	if t.listener != nil {
		t.listener.Close()
	}
	t.mu.Lock()
	for _, conn := range t.sessions {
		conn.Close()
	}
	t.mu.Unlock()
}

// ConnectAndSync connects to a peer and performs handshake + message sync.
func (t *Transport) ConnectAndSync(ip string, port uint16, bloomData []byte, holdMsgs []*message.Message) error {
	addr := fmt.Sprintf("%s:%d", ip, port)
	conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		return fmt.Errorf("connect failed: %w", err)
	}

	// Handshake
	peerPub, err := t.handshake(conn)
	if err != nil {
		conn.Close()
		return err
	}

	t.mu.Lock()
	t.sessions[peerPub] = conn
	t.mu.Unlock()
	t.peers.SetConnected(peerPub, true)

	// Send HAVE with bloom filter
	if err := t.sendHave(conn, bloomData); err != nil {
		return err
	}

	// Read WANT response and send requested messages
	go t.handleConnection(conn, peerPub, holdMsgs)

	return nil
}

// SendMessage sends a single message to a specific peer.
func (t *Transport) SendMessage(peerPub [32]byte, msg *message.Message) error {
	t.mu.RLock()
	conn, ok := t.sessions[peerPub]
	t.mu.RUnlock()

	if !ok {
		return fmt.Errorf("no connection to peer")
	}

	return t.sendMsg(conn, msg)
}

// BroadcastMessage sends a message to all connected peers.
func (t *Transport) BroadcastMessage(msg *message.Message) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	for _, conn := range t.sessions {
		t.sendMsg(conn, msg)
	}
}

func (t *Transport) acceptLoop() {
	for {
		select {
		case <-t.stop:
			return
		default:
		}

		conn, err := t.listener.Accept()
		if err != nil {
			select {
			case <-t.stop:
				return
			default:
				continue
			}
		}

		go func(c net.Conn) {
			peerPub, err := t.handshake(c)
			if err != nil {
				c.Close()
				return
			}
			t.mu.Lock()
			t.sessions[peerPub] = c
			t.mu.Unlock()
			t.peers.SetConnected(peerPub, true)

			t.handleConnection(c, peerPub, nil)
		}(conn)
	}
}

func (t *Transport) handshake(conn net.Conn) ([32]byte, error) {
	var peerPub [32]byte

	// Send HELLO
	hello := make([]byte, 1+32+2)
	hello[0] = CmdHello
	copy(hello[1:33], t.pubKey[:])
	binary.BigEndian.PutUint16(hello[33:35], t.listenPort)

	conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	if _, err := conn.Write(hello); err != nil {
		return peerPub, fmt.Errorf("handshake write failed: %w", err)
	}

	// Read HELLO
	resp := make([]byte, 35)
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	if _, err := io.ReadFull(conn, resp); err != nil {
		return peerPub, fmt.Errorf("handshake read failed: %w", err)
	}

	if resp[0] != CmdHello {
		return peerPub, fmt.Errorf("expected HELLO, got %d", resp[0])
	}
	copy(peerPub[:], resp[1:33])
	port := binary.BigEndian.Uint16(resp[33:35])

	ip := conn.RemoteAddr().(*net.TCPAddr).IP.String()
	t.peers.AddOrUpdate(peerPub, ip, port)

	return peerPub, nil
}

func (t *Transport) sendHave(conn net.Conn, bloomData []byte) error {
	header := make([]byte, 5)
	header[0] = CmdHave
	binary.BigEndian.PutUint32(header[1:5], uint32(len(bloomData)))

	conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	if _, err := conn.Write(header); err != nil {
		return err
	}
	if _, err := conn.Write(bloomData); err != nil {
		return err
	}
	return nil
}

func (t *Transport) sendMsg(conn net.Conn, msg *message.Message) error {
	data := msg.Serialize()
	header := make([]byte, 5)
	header[0] = CmdMsg
	binary.BigEndian.PutUint32(header[1:5], uint32(len(data)))

	conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	if _, err := conn.Write(header); err != nil {
		return err
	}
	if _, err := conn.Write(data); err != nil {
		return err
	}
	return nil
}

func (t *Transport) sendAck(conn net.Conn, msgID [32]byte) error {
	buf := make([]byte, 33)
	buf[0] = CmdAck
	copy(buf[1:33], msgID[:])
	conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	_, err := conn.Write(buf)
	return err
}

func (t *Transport) handleConnection(conn net.Conn, peerPub [32]byte, holdMsgs []*message.Message) {
	defer func() {
		conn.Close()
		t.mu.Lock()
		delete(t.sessions, peerPub)
		t.mu.Unlock()
		t.peers.SetConnected(peerPub, false)
	}()

	for {
		select {
		case <-t.stop:
			return
		default:
		}

		// Read command byte
		cmdBuf := make([]byte, 1)
		conn.SetReadDeadline(time.Now().Add(120 * time.Second))
		if _, err := io.ReadFull(conn, cmdBuf); err != nil {
			return
		}

		switch cmdBuf[0] {
		case CmdHave:
			// Read bloom filter length + data
			lenBuf := make([]byte, 4)
			if _, err := io.ReadFull(conn, lenBuf); err != nil {
				return
			}
			bLen := binary.BigEndian.Uint32(lenBuf)
			if bLen > 10*1024*1024 { // Max 10MB
				return
			}
			bloomData := make([]byte, bLen)
			if _, err := io.ReadFull(conn, bloomData); err != nil {
				return
			}
			// Send only messages the peer doesn't have (bloom filter check)
			sent := 0
			for _, msg := range holdMsgs {
				if len(bloomData) > 0 && bloomContains(bloomData, msg.ID) {
					continue // peer already has this
				}
				t.sendMsg(conn, msg)
				sent++
			}
			if len(holdMsgs) > 0 {
				log.Printf("[SYNC] Sent %d/%d messages (bloom filtered)", sent, len(holdMsgs))
			}

		case CmdMsg:
			// Read message length + data
			lenBuf := make([]byte, 4)
			if _, err := io.ReadFull(conn, lenBuf); err != nil {
				return
			}
			mLen := binary.BigEndian.Uint32(lenBuf)
			if mLen > 1*1024*1024 { // Max 1MB per message
				return
			}
			msgData := make([]byte, mLen)
			if _, err := io.ReadFull(conn, msgData); err != nil {
				return
			}
			msg, err := message.Deserialize(msgData)
			if err != nil {
				continue
			}
			// Send ACK
			t.sendAck(conn, msg.ID)
			// Notify handler
			if t.onMessage != nil {
				t.onMessage(msg)
			}

		case CmdAck:
			// Read message ID
			idBuf := make([]byte, 32)
			if _, err := io.ReadFull(conn, idBuf); err != nil {
				return
			}
			// ACK received — notify handler to update delivery status
			if t.onAck != nil {
				var id [32]byte
				copy(id[:], idBuf)
				t.onAck(id)
			}

		case CmdWant:
			// Read list of wanted message IDs
			lenBuf := make([]byte, 4)
			if _, err := io.ReadFull(conn, lenBuf); err != nil {
				return
			}
			count := binary.BigEndian.Uint32(lenBuf)
			if count > 10000 {
				return
			}
			for i := uint32(0); i < count; i++ {
				var id [32]byte
				if _, err := io.ReadFull(conn, id[:]); err != nil {
					return
				}
				// Find and send the message from hold snapshot
				found := false
				for _, msg := range holdMsgs {
					if msg.ID == id {
						t.sendMsg(conn, msg)
						found = true
						// Mark forwarded AFTER successful send
						if t.hold != nil {
							t.hold.MarkForwarded(id)
						}
						break
					}
				}
				// Fallback: read from disk if not in snapshot (bloom race)
				if !found && t.hold != nil {
					if msg, err := t.hold.Get(id); err == nil && msg != nil {
						t.sendMsg(conn, msg)
						t.hold.MarkForwarded(id)
					}
				}
			}

		default:
			return // Unknown command
		}
	}
}
