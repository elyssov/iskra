package mesh

import (
	"encoding/base32"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"log"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/iskra-messenger/iskra/internal/message"
	mdns "github.com/miekg/dns"
)

// DNS tunnel transport — fallback when WebSocket and UDP are blocked.
// Data is encoded in subdomains of DNS TXT queries.
// DPI sees: ordinary DNS lookups to a legitimate domain.

var b32 = base32.HexEncoding.WithPadding(base32.NoPadding)

const (
	dnsMaxLabel    = 63  // max bytes per DNS label
	dnsDataLabels  = 3   // number of data labels per query
	dnsChunkSize   = 55  // raw bytes per label (base32: 55 -> 88 chars, fits in 63 after case)
	dnsPollInterval = 3 * time.Second
	dnsKeepalive   = 25 * time.Second
	dnsTimeout     = 5 * time.Second
)

// DNSTransport sends/receives messages through DNS TXT queries to a relay domain.
type DNSTransport struct {
	pubKey      [32]byte
	myID        [idSize]byte
	relayDomain string // e.g. "tun.iskra-dns.example.com"
	dnsServer   string // e.g. "1.2.3.4:53" — direct IP of our DNS relay
	sessionID   string // random 8-char hex per session
	onMessage   func(*message.Message)
	mu          sync.RWMutex
	stop        chan struct{}
	connected   int32 // atomic
	seqSend     uint32

	// Reassembly buffer for incoming fragmented messages
	reassembly   map[string]*dnsReassembly
	reassemblyMu sync.Mutex
}

type dnsReassembly struct {
	chunks   map[int][]byte
	total    int
	lastSeen time.Time
}

// NewDNSTransport creates a DNS tunnel transport.
// relayDomain: the domain our DNS relay is authoritative for (e.g. "tun.iskra-dns.example.com")
// dnsServer: IP:port of the DNS relay server (e.g. "1.2.3.4:53")
func NewDNSTransport(pubKey [32]byte, relayDomain, dnsServer string) *DNSTransport {
	var myID [idSize]byte
	copy(myID[:], pubKey[:idSize])

	// Ensure domain ends with dot for DNS
	if !strings.HasSuffix(relayDomain, ".") {
		relayDomain = relayDomain + "."
	}

	return &DNSTransport{
		pubKey:      pubKey,
		myID:        myID,
		relayDomain: relayDomain,
		dnsServer:   dnsServer,
		sessionID:   fmt.Sprintf("%x", pubKey[:4]), // 8 hex chars from pubkey
		stop:        make(chan struct{}),
		reassembly:  make(map[string]*dnsReassembly),
	}
}

func (d *DNSTransport) SetOnMessage(fn func(*message.Message)) {
	d.mu.Lock()
	d.onMessage = fn
	d.mu.Unlock()
}

func (d *DNSTransport) Start() error {
	log.Printf("[DNS] Starting tunnel via %s (server %s)", d.relayDomain, d.dnsServer)

	// Register with relay
	if err := d.register(); err != nil {
		log.Printf("[DNS] Initial registration failed: %v (will retry)", err)
	} else {
		atomic.StoreInt32(&d.connected, 1)
	}

	go d.keepaliveLoop()
	go d.pollLoop()
	go d.cleanupLoop()

	return nil
}

func (d *DNSTransport) Stop() {
	select {
	case <-d.stop:
	default:
		close(d.stop)
	}
	atomic.StoreInt32(&d.connected, 0)
	log.Println("[DNS] Tunnel stopped")
}

func (d *DNSTransport) IsConnected() bool {
	return atomic.LoadInt32(&d.connected) == 1
}

// SendMessage sends a message through the DNS tunnel.
func (d *DNSTransport) SendMessage(msg *message.Message) error {
	data := msg.Serialize()

	// Build frame: [senderID:20][recipientID:20][serialized]
	frame := make([]byte, 0, idSize+idSize+len(data))
	frame = append(frame, d.myID[:]...)
	frame = append(frame, msg.RecipientID[:]...)
	frame = append(frame, data...)

	return d.sendFrame(frame)
}

// BroadcastMessage broadcasts to all peers via DNS relay.
func (d *DNSTransport) BroadcastMessage(msg *message.Message) {
	data := msg.Serialize()
	frame := make([]byte, 0, idSize+idSize+len(data))
	frame = append(frame, d.myID[:]...)
	frame = append(frame, make([]byte, idSize)...) // zeros = broadcast
	frame = append(frame, data...)

	if err := d.sendFrame(frame); err != nil {
		log.Printf("[DNS] Broadcast error: %v", err)
	}
}

// sendFrame splits data into chunks and sends each as a DNS TXT query.
func (d *DNSTransport) sendFrame(frame []byte) error {
	encoded := b32.EncodeToString(frame)
	chunks := splitString(encoded, dnsChunkSize*dnsDataLabels) // chars per query
	total := len(chunks)
	seq := atomic.AddUint32(&d.seqSend, 1)

	for i, chunk := range chunks {
		// Build labels from chunk data (split into sub-labels)
		labels := splitString(chunk, dnsChunkSize)

		// Query: {label1}.{label2}.{label3}.s{seq}.{i}.{total}.{sessionID}.{relayDomain}
		qname := fmt.Sprintf("%s.s%d.%d.%d.%s.%s",
			strings.Join(labels, "."),
			seq, i, total,
			d.sessionID,
			d.relayDomain,
		)

		if err := d.sendTXTQuery(qname); err != nil {
			return fmt.Errorf("chunk %d/%d: %w", i+1, total, err)
		}
	}

	return nil
}

// register sends a registration query to the relay.
func (d *DNSTransport) register() error {
	// reg.{senderID_hex}.{sessionID}.{relayDomain}
	senderHex := fmt.Sprintf("%x", d.myID[:])
	qname := fmt.Sprintf("reg.%s.%s.%s", senderHex, d.sessionID, d.relayDomain)
	return d.sendTXTQuery(qname)
}

// poll asks the relay for pending messages.
func (d *DNSTransport) poll() {
	qname := fmt.Sprintf("poll.%s.%s", d.sessionID, d.relayDomain)

	msg := new(mdns.Msg)
	msg.SetQuestion(qname, mdns.TypeTXT)
	msg.RecursionDesired = false

	client := &mdns.Client{Timeout: dnsTimeout}
	resp, _, err := client.Exchange(msg, d.dnsServer)
	if err != nil {
		if atomic.LoadInt32(&d.connected) == 1 {
			atomic.StoreInt32(&d.connected, 0)
			log.Printf("[DNS] Poll failed (marking disconnected): %v", err)
		}
		return
	}

	if atomic.LoadInt32(&d.connected) == 0 {
		atomic.StoreInt32(&d.connected, 1)
		log.Println("[DNS] Reconnected to relay")
	}

	// Process TXT records — each contains base32-encoded chunk
	for _, rr := range resp.Answer {
		txt, ok := rr.(*mdns.TXT)
		if !ok {
			continue
		}
		for _, s := range txt.Txt {
			d.processIncoming(s)
		}
	}
}

// processIncoming handles a received TXT record string.
// Format: "{seq}.{index}.{total}.{senderSessionID}.{base32data}"
func (d *DNSTransport) processIncoming(record string) {
	parts := strings.SplitN(record, ".", 5)
	if len(parts) < 5 {
		return
	}

	var seq, idx, total int
	fmt.Sscanf(parts[0], "%d", &seq)
	fmt.Sscanf(parts[1], "%d", &idx)
	fmt.Sscanf(parts[2], "%d", &total)
	senderSession := parts[3]
	chunkData := parts[4]

	if total <= 0 || total > 100 || idx < 0 || idx >= total {
		return
	}

	decoded, err := b32.DecodeString(strings.ToUpper(chunkData))
	if err != nil {
		return
	}

	key := fmt.Sprintf("%s-%d", senderSession, seq)

	d.reassemblyMu.Lock()
	buf, exists := d.reassembly[key]
	if !exists {
		buf = &dnsReassembly{
			chunks: make(map[int][]byte),
			total:  total,
		}
		d.reassembly[key] = buf
	}
	buf.chunks[idx] = decoded
	buf.lastSeen = time.Now()

	// Check if complete
	if len(buf.chunks) == buf.total {
		// Reassemble
		var full []byte
		for i := 0; i < buf.total; i++ {
			full = append(full, buf.chunks[i]...)
		}
		delete(d.reassembly, key)
		d.reassemblyMu.Unlock()

		d.deliverFrame(full)
		return
	}
	d.reassemblyMu.Unlock()
}

// deliverFrame processes a complete frame: [senderID:20][recipientID:20][serialized]
func (d *DNSTransport) deliverFrame(frame []byte) {
	if len(frame) < idSize+idSize {
		return
	}

	// Skip sender and recipient IDs, deserialize message
	msgData := frame[idSize+idSize:]
	msg, err := message.Deserialize(msgData)
	if err != nil {
		log.Printf("[DNS] Deserialize error: %v", err)
		return
	}

	d.mu.RLock()
	fn := d.onMessage
	d.mu.RUnlock()

	if fn != nil {
		fn(msg)
	}
}

func (d *DNSTransport) sendTXTQuery(qname string) error {
	msg := new(mdns.Msg)
	msg.SetQuestion(qname, mdns.TypeTXT)
	msg.RecursionDesired = false

	client := &mdns.Client{Timeout: dnsTimeout}
	_, _, err := client.Exchange(msg, d.dnsServer)
	return err
}

// --- Loops ---

func (d *DNSTransport) keepaliveLoop() {
	for {
		jitter := randomDuration(20000, 30000)
		select {
		case <-d.stop:
			return
		case <-time.After(jitter):
			if err := d.register(); err != nil {
				if atomic.LoadInt32(&d.connected) == 1 {
					atomic.StoreInt32(&d.connected, 0)
					log.Printf("[DNS] Keepalive failed: %v", err)
				}
			} else if atomic.LoadInt32(&d.connected) == 0 {
				atomic.StoreInt32(&d.connected, 1)
				log.Println("[DNS] Reconnected via keepalive")
			}
		}
	}
}

func (d *DNSTransport) pollLoop() {
	for {
		// Randomize poll interval 2-5s to avoid DPI pattern detection
		jitter := randomDuration(2000, 5000)
		select {
		case <-d.stop:
			return
		case <-time.After(jitter):
			d.poll()
		}
	}
}

// randomDuration returns a random duration between minMs and maxMs milliseconds.
func randomDuration(minMs, maxMs int) time.Duration {
	var b [4]byte
	rand.Read(b[:])
	n := int(binary.LittleEndian.Uint32(b[:])) % (maxMs - minMs)
	return time.Duration(minMs+n) * time.Millisecond
}

func (d *DNSTransport) cleanupLoop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-d.stop:
			return
		case <-ticker.C:
			d.reassemblyMu.Lock()
			now := time.Now()
			for k, buf := range d.reassembly {
				if now.Sub(buf.lastSeen) > 60*time.Second {
					delete(d.reassembly, k)
				}
			}
			d.reassemblyMu.Unlock()
		}
	}
}

// --- Helpers ---

func splitString(s string, size int) []string {
	var parts []string
	for len(s) > 0 {
		if len(s) < size {
			size = len(s)
		}
		parts = append(parts, s[:size])
		s = s[size:]
	}
	if len(parts) == 0 {
		parts = []string{""}
	}
	return parts
}
