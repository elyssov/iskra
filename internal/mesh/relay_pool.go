package mesh

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"sync"
	"time"
)

// RelayEntry is a known relay server.
type RelayEntry struct {
	URL      string `json:"url"`
	LastSeen int64  `json:"last_seen"` // unix timestamp of last successful connection
	Source   string `json:"source"`    // "builtin", "manual", "mesh"
	AddedAt  int64  `json:"added_at"`
	Alive    bool   `json:"alive"` // last check result
}

const relayTTL = 90 * 24 * 3600 // 90 days in seconds

// RelayPool manages a list of known relay servers.
type RelayPool struct {
	entries  []RelayEntry
	path     string // persistence file
	mu       sync.RWMutex
}

// NewRelayPool creates a pool with builtin relays and loads saved ones from disk.
func NewRelayPool(path string, builtinURLs []string) *RelayPool {
	rp := &RelayPool{path: path}
	rp.load()

	// Ensure builtins are always present
	now := time.Now().Unix()
	for _, url := range builtinURLs {
		if !rp.has(url) {
			rp.entries = append(rp.entries, RelayEntry{
				URL: url, LastSeen: now, Source: "builtin", AddedAt: now, Alive: true,
			})
		}
	}
	rp.cleanup()
	rp.save()
	return rp
}

// Add adds a relay from manual input or mesh exchange. Returns true if new.
func (rp *RelayPool) Add(url, source string) bool {
	rp.mu.Lock()
	defer rp.mu.Unlock()

	for i, e := range rp.entries {
		if e.URL == url {
			// Already known — update last_seen if mesh confirms it
			if source == "mesh" {
				rp.entries[i].LastSeen = time.Now().Unix()
			}
			return false
		}
	}

	rp.entries = append(rp.entries, RelayEntry{
		URL:     url,
		Source:  source,
		AddedAt: time.Now().Unix(),
		Alive:   false, // not verified yet
	})
	rp.save()
	return true
}

// Remove removes a relay by URL.
func (rp *RelayPool) Remove(url string) {
	rp.mu.Lock()
	defer rp.mu.Unlock()
	var kept []RelayEntry
	for _, e := range rp.entries {
		if e.URL != url {
			kept = append(kept, e)
		}
	}
	rp.entries = kept
	rp.save()
}

// List returns all known relays.
func (rp *RelayPool) List() []RelayEntry {
	rp.mu.RLock()
	defer rp.mu.RUnlock()
	out := make([]RelayEntry, len(rp.entries))
	copy(out, rp.entries)
	return out
}

// AliveURLs returns URLs of relays that were alive at last check.
func (rp *RelayPool) AliveURLs() []string {
	rp.mu.RLock()
	defer rp.mu.RUnlock()
	var urls []string
	for _, e := range rp.entries {
		if e.Alive {
			urls = append(urls, e.URL)
		}
	}
	return urls
}

// ForExchange returns compact relay list for mesh sync exchange.
func (rp *RelayPool) ForExchange() []string {
	rp.mu.RLock()
	defer rp.mu.RUnlock()
	var urls []string
	for _, e := range rp.entries {
		if e.Alive || time.Now().Unix()-e.LastSeen < 7*24*3600 {
			urls = append(urls, e.URL)
		}
	}
	return urls
}

// MergeFromMesh adds relays received during mesh sync.
func (rp *RelayPool) MergeFromMesh(urls []string) int {
	added := 0
	for _, url := range urls {
		if rp.Add(url, "mesh") {
			added++
			log.Printf("[RelayPool] Discovered relay via mesh: %s", url)
		}
	}
	// Verify new relays in background
	if added > 0 {
		go rp.CheckAll()
	}
	return added
}

// MarkAlive updates a relay's alive status and last_seen.
func (rp *RelayPool) MarkAlive(url string, alive bool) {
	rp.mu.Lock()
	defer rp.mu.Unlock()
	for i, e := range rp.entries {
		if e.URL == url {
			rp.entries[i].Alive = alive
			if alive {
				rp.entries[i].LastSeen = time.Now().Unix()
			}
			break
		}
	}
	rp.save()
}

// CheckAll pings all relays to verify they're alive.
func (rp *RelayPool) CheckAll() {
	entries := rp.List()
	for _, e := range entries {
		alive := checkRelay(e.URL)
		rp.MarkAlive(e.URL, alive)
	}
}

// checkRelay pings a relay URL to see if it responds.
func checkRelay(wsURL string) bool {
	// Convert wss://host/ws → https://host/
	httpURL := "https" + wsURL[3:]
	if len(httpURL) > 3 && httpURL[len(httpURL)-3:] == "/ws" {
		httpURL = httpURL[:len(httpURL)-3]
	}
	httpURL += "/"

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(httpURL)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == 200
}

func (rp *RelayPool) has(url string) bool {
	for _, e := range rp.entries {
		if e.URL == url {
			return true
		}
	}
	return false
}

func (rp *RelayPool) cleanup() {
	now := time.Now().Unix()
	var kept []RelayEntry
	for _, e := range rp.entries {
		if e.Source == "builtin" || now-e.LastSeen < relayTTL {
			kept = append(kept, e)
		} else {
			log.Printf("[RelayPool] Expired relay: %s (last seen %d days ago)", e.URL, (now-e.LastSeen)/86400)
		}
	}
	rp.entries = kept
}

func (rp *RelayPool) load() {
	data, err := os.ReadFile(rp.path)
	if err != nil {
		return
	}
	json.Unmarshal(data, &rp.entries)
}

func (rp *RelayPool) save() {
	data, _ := json.MarshalIndent(rp.entries, "", "  ")
	os.WriteFile(rp.path, data, 0600)
}
