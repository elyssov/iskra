// Package telemetry provides anonymous device statistics for Iskra.
// Collects: device_hash (irreversible), platform, model, OS, app version.
// Never collects: identity, keys, IP, messages, contacts names.
package telemetry

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"runtime"
	"sync"
	"time"
)

// Report is sent to the relay once per session.
type Report struct {
	DeviceHash   string `json:"device_hash"`
	Platform     string `json:"platform"`
	Model        string `json:"model"`
	OSVersion    string `json:"os_version"`
	AppVersion   string `json:"app_version"`
	Transport    string `json:"transport"`
	Lang         string `json:"lang"`
	HoldCount    int    `json:"hold_count"`
	ContactCount int    `json:"contact_count"`
	UptimeMin    int    `json:"uptime_min"`
}

// Collector gathers and sends telemetry data.
type Collector struct {
	relayHTTPBase string
	appVersion    string
	deviceHash    string
	platform      string
	model         string
	osVersion     string
	lang          string
	startTime     time.Time
	enabled       bool
	sent          bool
	mu            sync.Mutex

	// Dynamic counters (updated externally)
	HoldCount    int
	ContactCount int
	Transport    string
}

const salt = "iskra-telemetry-v1"

// DeviceHash computes SHA-256(deviceID + salt) → hex string.
// deviceID should be ANDROID_ID on Android, or hostname+MAC on desktop.
func DeviceHash(deviceID string) string {
	h := sha256.Sum256([]byte(deviceID + salt))
	return hex.EncodeToString(h[:])
}

// New creates a telemetry collector.
// deviceID: raw device identifier (will be hashed immediately, original discarded).
// enabled: whether telemetry is active (user opt-out respected).
func New(relayHTTPBase, appVersion, deviceID, model, osVersion, lang string, enabled bool) *Collector {
	plat := runtime.GOOS
	if plat == "linux" && model != "" {
		// If model is set on linux, it's probably Android via gomobile
		plat = "android"
	}

	return &Collector{
		relayHTTPBase: relayHTTPBase,
		appVersion:    appVersion,
		deviceHash:    DeviceHash(deviceID),
		platform:      plat,
		model:         model,
		osVersion:     osVersion,
		lang:          lang,
		startTime:     time.Now(),
		enabled:       enabled,
	}
}

// SetEnabled allows toggling telemetry at runtime (user opt-out).
func (c *Collector) SetEnabled(enabled bool) {
	c.mu.Lock()
	c.enabled = enabled
	c.mu.Unlock()
}

// Send transmits the report to the relay. Called once per session,
// typically after relay connection is established and initial sync done.
func (c *Collector) Send() {
	c.mu.Lock()
	if !c.enabled || c.sent || c.relayHTTPBase == "" {
		c.mu.Unlock()
		return
	}
	c.sent = true
	uptime := int(time.Since(c.startTime).Minutes())
	report := Report{
		DeviceHash:   c.deviceHash,
		Platform:     c.platform,
		Model:        c.model,
		OSVersion:    c.osVersion,
		AppVersion:   c.appVersion,
		Transport:    c.Transport,
		Lang:         c.lang,
		HoldCount:    c.HoldCount,
		ContactCount: c.ContactCount,
		UptimeMin:    uptime,
	}
	c.mu.Unlock()

	go func() {
		data, err := json.Marshal(report)
		if err != nil {
			return
		}
		url := c.relayHTTPBase + "/api/telemetry"
		resp, err := http.Post(url, "application/json", bytes.NewReader(data))
		if err != nil {
			log.Printf("[Telemetry] Failed to send: %v", err)
			return
		}
		resp.Body.Close()
		if resp.StatusCode == 200 {
			log.Printf("[Telemetry] Sent (device=%s..%s)", c.deviceHash[:8], c.deviceHash[len(c.deviceHash)-4:])
		}
	}()
}

// DesktopDeviceID generates a device identifier for desktop (no ANDROID_ID).
func DesktopDeviceID() string {
	hostname, _ := os.Hostname()
	return fmt.Sprintf("%s-%s-%s", hostname, runtime.GOOS, runtime.GOARCH)
}
