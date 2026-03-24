// Package mesh — Wave Controller
//
// Mesh Wave Protocol: decentralized message propagation without infrastructure.
// Every phone is both client and server. No central coordinator.
//
// Two modes (auto-switching):
//   MODE_INFRA:  connected to Wi-Fi → multicast discovery → TCP sync
//   MODE_DIRECT: no Wi-Fi → Wi-Fi Direct P2P → sequential sync with cooldown
//
// After sync with each peer:
//   - Peer goes into cooldown (10 min)
//   - If NEW messages received → reset ALL cooldowns (except just-synced) → new wave
//   - If no new messages → continue to next peer
//
// Result: messages propagate in exponential waves across all nearby devices.
package mesh

import (
	"log"
	"math/rand"
	"sync"
	"time"

	"github.com/iskra-messenger/iskra/internal/message"
)

const (
	CooldownDuration = 10 * time.Minute
	SleepBetweenScans = 30 * time.Second
	SyncTimeout       = 15 * time.Second
)

// WaveState represents the current state of the wave controller.
type WaveState int

const (
	WaveCheckWifi WaveState = iota
	WaveInfra
	WaveDirect
	WaveSleep
)

func (s WaveState) String() string {
	switch s {
	case WaveCheckWifi:
		return "CHECK_WIFI"
	case WaveInfra:
		return "INFRA"
	case WaveDirect:
		return "DIRECT"
	case WaveSleep:
		return "SLEEP"
	default:
		return "UNKNOWN"
	}
}

// WavePeer represents a discovered peer for wave sync.
type WavePeer struct {
	ID   string // unique identifier (MAC, pubkey hex, etc.)
	IP   string
	Port uint16
}

// WifiDirectBridge is the interface that Kotlin implements via JNI.
// Go calls these methods to control the Wi-Fi radio.
type WifiDirectBridge interface {
	// IsWifiConnected returns true if connected to a Wi-Fi network
	IsWifiConnected() bool
	// ScanWifiDirect starts Wi-Fi Direct peer discovery
	// Returns discovered peers (blocking, up to timeout)
	ScanWifiDirect(timeoutSec int) []WavePeer
	// ConnectWifiDirect connects to a peer, returns local IP for TCP sync
	ConnectWifiDirect(peerID string) (localIP string, peerIP string, err error)
	// DisconnectWifiDirect disconnects current P2P connection
	DisconnectWifiDirect()
	// GetCurrentMode returns current mode string for UI
	GetCurrentMode() string
}

// WaveController manages the mesh wave protocol.
type WaveController struct {
	bridge    WifiDirectBridge
	transport *Transport
	bloom     interface {
		Export() []byte
	}
	holdGetter func() ([]*message.Message, error)
	onMessage  func(*message.Message)

	cooldownMap map[string]time.Time // peerID → cooldown expiry
	mu          sync.Mutex
	state       WaveState
	stop        chan struct{}
	newMsgCh    chan struct{} // signal: new messages received during sync
}

// NewWaveController creates a new wave controller.
func NewWaveController(
	bridge WifiDirectBridge,
	transport *Transport,
	bloom interface{ Export() []byte },
	holdGetter func() ([]*message.Message, error),
) *WaveController {
	return &WaveController{
		bridge:      bridge,
		transport:   transport,
		bloom:       bloom,
		holdGetter:  holdGetter,
		cooldownMap: make(map[string]time.Time),
		stop:        make(chan struct{}),
		newMsgCh:    make(chan struct{}, 1),
	}
}

// Start begins the wave protocol loop.
func (w *WaveController) Start() {
	go w.loop()
	log.Println("[WAVE] Controller started")
}

// Stop stops the wave controller.
func (w *WaveController) Stop() {
	close(w.stop)
}

// State returns current state.
func (w *WaveController) State() WaveState {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.state
}

// NotifyNewMessages signals that new messages were received.
// Called from the message handler when hold gets new data.
func (w *WaveController) NotifyNewMessages() {
	select {
	case w.newMsgCh <- struct{}{}:
	default:
	}
}

func (w *WaveController) loop() {
	for {
		select {
		case <-w.stop:
			return
		default:
		}

		w.setState(WaveCheckWifi)

		if w.bridge != nil && w.bridge.IsWifiConnected() {
			// MODE INFRA: connected to Wi-Fi, use multicast discovery
			w.setState(WaveInfra)
			w.runInfraMode()
		}

		// MODE DIRECT: Wi-Fi Direct P2P
		w.setState(WaveDirect)
		w.runDirectMode()

		// All peers in cooldown — sleep, then loop back
		w.setState(WaveSleep)
		select {
		case <-w.stop:
			return
		case <-w.newMsgCh:
			log.Println("[WAVE] New messages received during sleep, restarting scan")
			continue
		case <-time.After(SleepBetweenScans):
		}
	}
}

func (w *WaveController) runInfraMode() {
	// In infra mode, multicast discovery + TCP sync is already handled
	// by the existing Discovery + Transport. We just manage cooldowns here.
	log.Println("[WAVE] Infra mode: using existing multicast discovery")

	// Wait a bit for multicast discovery to find peers
	select {
	case <-w.stop:
		return
	case <-time.After(10 * time.Second):
	}
}

func (w *WaveController) runDirectMode() {
	if w.bridge == nil {
		return // no Wi-Fi Direct bridge (desktop mode)
	}

	log.Println("[WAVE] Direct mode: scanning for Wi-Fi Direct peers")

	peers := w.bridge.ScanWifiDirect(10) // 10 sec scan
	if len(peers) == 0 {
		log.Println("[WAVE] No Wi-Fi Direct peers found")
		return
	}

	// Filter out cooled-down peers
	available := w.filterAvailable(peers)
	if len(available) == 0 {
		log.Println("[WAVE] All peers in cooldown")
		return
	}

	// Shuffle for random order
	rand.Shuffle(len(available), func(i, j int) {
		available[i], available[j] = available[j], available[i]
	})

	for _, peer := range available {
		select {
		case <-w.stop:
			return
		default:
		}

		if w.isCooledDown(peer.ID) {
			continue
		}

		newMsgs := w.syncWithPeer(peer)

		// Add to cooldown
		w.addCooldown(peer.ID)

		// If got new messages — reset cooldown for everyone except this peer
		if newMsgs > 0 {
			log.Printf("[WAVE] Got %d new messages from %s, resetting cooldowns", newMsgs, peer.ID)
			w.resetCooldownExcept(peer.ID)
		}
	}
}

func (w *WaveController) syncWithPeer(peer WavePeer) int {
	log.Printf("[WAVE] Connecting to %s", peer.ID)

	localIP, peerIP, err := w.bridge.ConnectWifiDirect(peer.ID)
	if err != nil {
		log.Printf("[WAVE] Connect failed: %v", err)
		return 0
	}
	defer w.bridge.DisconnectWifiDirect()

	_ = localIP // we listen on all interfaces

	// Use existing TCP transport for sync
	holdMsgs, err := w.holdGetter()
	if err != nil {
		log.Printf("[WAVE] Hold getter error: %v", err)
		return 0
	}

	bloomData := w.bloom.Export()

	// Count messages before sync
	beforeCount := len(holdMsgs)

	// TCP connect and sync (bilateral exchange)
	err = w.transport.ConnectAndSync(peerIP, peer.Port, bloomData, holdMsgs)
	if err != nil {
		log.Printf("[WAVE] Sync failed with %s: %v", peer.ID, err)
		return 0
	}

	// Wait for sync to complete (messages arrive via onMessage callback)
	time.Sleep(5 * time.Second)

	// Count new messages
	holdAfter, _ := w.holdGetter()
	newMsgs := len(holdAfter) - beforeCount
	if newMsgs < 0 {
		newMsgs = 0
	}

	log.Printf("[WAVE] Synced with %s: sent bloom(%d bytes), received %d new messages",
		peer.ID, len(bloomData), newMsgs)

	return newMsgs
}

func (w *WaveController) filterAvailable(peers []WavePeer) []WavePeer {
	w.mu.Lock()
	defer w.mu.Unlock()

	var available []WavePeer
	now := time.Now()
	for _, p := range peers {
		expiry, exists := w.cooldownMap[p.ID]
		if !exists || now.After(expiry) {
			available = append(available, p)
		}
	}
	return available
}

func (w *WaveController) isCooledDown(peerID string) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	expiry, exists := w.cooldownMap[peerID]
	return exists && time.Now().Before(expiry)
}

func (w *WaveController) addCooldown(peerID string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.cooldownMap[peerID] = time.Now().Add(CooldownDuration)
}

func (w *WaveController) resetCooldownExcept(keepPeerID string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	kept := w.cooldownMap[keepPeerID]
	w.cooldownMap = map[string]time.Time{keepPeerID: kept}
	log.Println("[WAVE] Cooldowns reset — new wave starting")
}

func (w *WaveController) setState(s WaveState) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.state != s {
		log.Printf("[WAVE] State: %s → %s", w.state, s)
		w.state = s
	}
}
