package mesh

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"github.com/iskra-messenger/iskra/internal/message"
)

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
type Transport struct {
	pubKey     [32]byte
	listenPort uint16
	listener   net.Listener
	peers      *PeerList
	onMessage  func(*message.Message)
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
			// Send messages that the peer doesn't have
			// For now, send all hold messages (proper bloom check in v0.2)
			for _, msg := range holdMsgs {
				t.sendMsg(conn, msg)
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
			// ACK received — could update delivery status

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
				// Find and send the message from hold
				for _, msg := range holdMsgs {
					if msg.ID == id {
						t.sendMsg(conn, msg)
						break
					}
				}
			}

		default:
			return // Unknown command
		}
	}
}
