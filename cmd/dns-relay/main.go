// DNS relay server for Iskra.
// Accepts DNS TXT queries, extracts message data from subdomains, routes to recipients.
// DPI sees: ordinary DNS traffic to a legitimate domain. Blocking DNS = killing the internet.
package main

import (
	"encoding/base32"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/miekg/dns"
)

var b32 = base32.HexEncoding.WithPadding(base32.NoPadding)

const (
	idSize         = 20
	maxPendingMsgs = 200
	maxPendingAge  = 5 * time.Minute
)

type client struct {
	sessionID string
	lastSeen  time.Time
	pending   []pendingMsg // messages waiting for poll
}

type pendingMsg struct {
	data      string // base32-encoded frame chunks, formatted for TXT response
	createdAt time.Time
}

// reassembly tracks incoming chunked messages being assembled
type reassembly struct {
	chunks   map[int]string
	total    int
	lastSeen time.Time
}

type relay struct {
	domain string // authoritative domain (with trailing dot)

	mu      sync.RWMutex
	clients map[string]*client // senderID_hex -> client

	reassemblyMu sync.Mutex
	reassembly    map[string]*reassembly // "{sessionID}-s{seq}" -> reassembly
}

func main() {
	port := flag.Int("port", 5353, "DNS port to listen on")
	domain := flag.String("domain", "tun.iskra.local.", "Authoritative domain (with trailing dot)")
	flag.Parse()

	if !strings.HasSuffix(*domain, ".") {
		*domain = *domain + "."
	}

	r := &relay{
		domain:     *domain,
		clients:    make(map[string]*client),
		reassembly: make(map[string]*reassembly),
	}

	// Register DNS handler
	dns.HandleFunc(*domain, r.handleDNS)

	server := &dns.Server{
		Addr: fmt.Sprintf(":%d", *port),
		Net:  "udp",
	}

	fmt.Printf("🔥 Искра DNS Relay\n")
	fmt.Printf("   Порт: %d\n", *port)
	fmt.Printf("   Домен: %s\n", *domain)
	fmt.Printf("   DPI видит: обычный DNS-трафик\n\n")

	go r.cleanupLoop()

	go func() {
		if err := server.ListenAndServe(); err != nil {
			log.Fatalf("DNS server failed: %v", err)
		}
	}()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan
	fmt.Println("\nОстановка...")
	server.Shutdown()
}

func (r *relay) handleDNS(w dns.ResponseWriter, req *dns.Msg) {
	resp := new(dns.Msg)
	resp.SetReply(req)
	resp.Authoritative = true

	if len(req.Question) == 0 {
		w.WriteMsg(resp)
		return
	}

	qname := req.Question[0].Name
	// Strip the relay domain suffix to get the command part
	if !strings.HasSuffix(qname, r.domain) {
		w.WriteMsg(resp)
		return
	}

	cmdPart := strings.TrimSuffix(qname, "."+r.domain)
	if strings.HasSuffix(cmdPart, ".") {
		cmdPart = strings.TrimSuffix(cmdPart, ".")
	}
	labels := strings.Split(cmdPart, ".")

	if len(labels) < 1 {
		w.WriteMsg(resp)
		return
	}

	switch labels[0] {
	case "reg":
		r.handleRegister(labels, resp)
	case "poll":
		r.handlePoll(labels, resp)
	default:
		// Data send query
		r.handleSend(labels, resp)
	}

	w.WriteMsg(resp)
}

// handleRegister: reg.{senderID_hex}.{sessionID}
func (r *relay) handleRegister(labels []string, resp *dns.Msg) {
	if len(labels) < 3 {
		return
	}
	senderID := labels[1]
	sessionID := labels[2]

	r.mu.Lock()
	c, exists := r.clients[senderID]
	if !exists {
		c = &client{sessionID: sessionID}
		r.clients[senderID] = c
	}
	c.sessionID = sessionID
	c.lastSeen = time.Now()
	r.mu.Unlock()

	log.Printf("[REG] %s session=%s (%d clients)", truncID(senderID), sessionID, r.clientCount())

	// Respond with OK TXT
	resp.Answer = append(resp.Answer, &dns.TXT{
		Hdr: dns.RR_Header{Name: resp.Question[0].Name, Rrtype: dns.TypeTXT, Class: dns.ClassINET, Ttl: 0},
		Txt: []string{"ok"},
	})
}

// handlePoll: poll.{sessionID}
// Returns pending messages as TXT records
func (r *relay) handlePoll(labels []string, resp *dns.Msg) {
	if len(labels) < 2 {
		return
	}
	sessionID := labels[1]

	// Find client by session ID
	r.mu.Lock()
	var found *client
	for _, c := range r.clients {
		if c.sessionID == sessionID {
			found = c
			break
		}
	}

	if found == nil {
		r.mu.Unlock()
		return
	}

	found.lastSeen = time.Now()

	// Grab pending messages (up to 10 per poll to fit DNS response)
	maxPerPoll := 10
	if len(found.pending) < maxPerPoll {
		maxPerPoll = len(found.pending)
	}
	toSend := make([]pendingMsg, maxPerPoll)
	copy(toSend, found.pending[:maxPerPoll])
	found.pending = found.pending[maxPerPoll:]
	r.mu.Unlock()

	// Each pending message becomes a TXT record
	for _, pm := range toSend {
		resp.Answer = append(resp.Answer, &dns.TXT{
			Hdr: dns.RR_Header{Name: resp.Question[0].Name, Rrtype: dns.TypeTXT, Class: dns.ClassINET, Ttl: 0},
			Txt: []string{pm.data},
		})
	}
}

// handleSend: {data_labels...}.s{seq}.{index}.{total}.{sessionID}
// Data labels contain base32-encoded message chunks
func (r *relay) handleSend(labels []string, resp *dns.Msg) {
	// Find the sequence marker "s{N}" to identify structure
	seqIdx := -1
	for i, l := range labels {
		if len(l) > 1 && l[0] == 's' {
			// Check if rest is numeric
			isNum := true
			for _, ch := range l[1:] {
				if ch < '0' || ch > '9' {
					isNum = false
					break
				}
			}
			if isNum {
				seqIdx = i
				break
			}
		}
	}

	if seqIdx < 1 || seqIdx+3 >= len(labels) {
		return
	}

	// Data is all labels before seqIdx
	dataLabels := labels[:seqIdx]
	seqStr := labels[seqIdx][1:] // strip 's' prefix
	idxStr := labels[seqIdx+1]
	totalStr := labels[seqIdx+2]
	sessionID := labels[seqIdx+3]

	var seq, idx, total int
	fmt.Sscanf(seqStr, "%d", &seq)
	fmt.Sscanf(idxStr, "%d", &idx)
	fmt.Sscanf(totalStr, "%d", &total)

	if total <= 0 || total > 100 || idx < 0 || idx >= total {
		return
	}

	chunkData := strings.Join(dataLabels, "")
	key := fmt.Sprintf("%s-s%d", sessionID, seq)

	r.reassemblyMu.Lock()
	buf, exists := r.reassembly[key]
	if !exists {
		buf = &reassembly{
			chunks: make(map[int]string),
			total:  total,
		}
		r.reassembly[key] = buf
	}
	buf.chunks[idx] = chunkData
	buf.lastSeen = time.Now()

	// Check if complete
	if len(buf.chunks) < buf.total {
		r.reassemblyMu.Unlock()
		// ACK the chunk
		resp.Answer = append(resp.Answer, &dns.TXT{
			Hdr: dns.RR_Header{Name: resp.Question[0].Name, Rrtype: dns.TypeTXT, Class: dns.ClassINET, Ttl: 0},
			Txt: []string{"ack"},
		})
		return
	}

	// Reassemble full base32 string
	var fullB32 strings.Builder
	for i := 0; i < buf.total; i++ {
		fullB32.WriteString(buf.chunks[i])
	}
	delete(r.reassembly, key)
	r.reassemblyMu.Unlock()

	// Decode the frame
	frame, err := b32.DecodeString(strings.ToUpper(fullB32.String()))
	if err != nil || len(frame) < idSize*2 {
		log.Printf("[SEND] Decode error: %v", err)
		return
	}

	senderID := fmt.Sprintf("%x", frame[:idSize])
	recipientID := fmt.Sprintf("%x", frame[idSize:idSize*2])

	// Check for broadcast (all zeros)
	allZeros := true
	for _, b := range frame[idSize : idSize*2] {
		if b != 0 {
			allZeros = false
			break
		}
	}

	// Format for delivery: "{seq}.{idx}.{total}.{senderSession}.{base32data}"
	deliveryData := fmt.Sprintf("%d.0.1.%s.%s", seq, sessionID, fullB32.String())

	if allZeros {
		// Broadcast to all
		r.mu.RLock()
		for id, c := range r.clients {
			if id != senderID {
				c.pending = append(c.pending, pendingMsg{data: deliveryData, createdAt: time.Now()})
				trimPending(c)
			}
		}
		r.mu.RUnlock()
		log.Printf("[BCAST] %s -> all (%d bytes frame)", truncID(senderID), len(frame))
	} else {
		// Direct delivery
		r.mu.RLock()
		dest, ok := r.clients[recipientID]
		r.mu.RUnlock()

		if ok {
			r.mu.Lock()
			dest.pending = append(dest.pending, pendingMsg{data: deliveryData, createdAt: time.Now()})
			trimPending(dest)
			r.mu.Unlock()
			log.Printf("[FWD] %s -> %s (%d bytes)", truncID(senderID), truncID(recipientID), len(frame))
		} else {
			log.Printf("[MISS] %s -> %s (offline)", truncID(senderID), truncID(recipientID))
		}
	}

	resp.Answer = append(resp.Answer, &dns.TXT{
		Hdr: dns.RR_Header{Name: resp.Question[0].Name, Rrtype: dns.TypeTXT, Class: dns.ClassINET, Ttl: 0},
		Txt: []string{"ok"},
	})
}

func trimPending(c *client) {
	if len(c.pending) > maxPendingMsgs {
		c.pending = c.pending[len(c.pending)-maxPendingMsgs:]
	}
}

func (r *relay) cleanupLoop() {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		now := time.Now()

		r.mu.Lock()
		for id, c := range r.clients {
			if now.Sub(c.lastSeen) > 5*time.Minute {
				delete(r.clients, id)
				log.Printf("[CLEANUP] Removed stale client %s", truncID(id))
			}
			// Clean old pending messages
			fresh := c.pending[:0]
			for _, pm := range c.pending {
				if now.Sub(pm.createdAt) < maxPendingAge {
					fresh = append(fresh, pm)
				}
			}
			c.pending = fresh
		}
		r.mu.Unlock()

		r.reassemblyMu.Lock()
		for k, buf := range r.reassembly {
			if now.Sub(buf.lastSeen) > 60*time.Second {
				delete(r.reassembly, k)
			}
		}
		r.reassemblyMu.Unlock()
	}
}

func (r *relay) clientCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.clients)
}

func truncID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}
