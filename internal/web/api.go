package web

import (
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	iskraCrypto "github.com/iskra-messenger/iskra/internal/crypto"
	"github.com/iskra-messenger/iskra/internal/identity"
	"github.com/iskra-messenger/iskra/internal/message"
	"github.com/iskra-messenger/iskra/internal/mesh"
	"github.com/iskra-messenger/iskra/internal/store"
)

// API handles REST API requests.
type API struct {
	Keypair     *identity.Keypair
	Mnemonic    []string
	Contacts    *store.Contacts
	Inbox       *store.Inbox
	Hold        *store.Hold
	Bloom       *store.SimpleBloom
	Peers       *mesh.PeerList
	Transport   *mesh.Transport
	RelayClient *mesh.RelayClient
	Mode        string // "lan", "relay", "offline"
}

type identityResponse struct {
	UserID   string   `json:"userID"`
	PubKey   string   `json:"pubkey"`
	X25519   string   `json:"x25519_pub"`
	Mnemonic []string `json:"mnemonic"`
}

type contactRequest struct {
	Name         string `json:"name"`
	PubKeyBase58 string `json:"pubkeyBase58"`
	X25519Base58 string `json:"x25519Base58"`
}

type messageRequest struct {
	Text string `json:"text"`
}

type statusResponse struct {
	Mode     string `json:"mode"`
	Peers    int    `json:"peers"`
	HoldSize int    `json:"holdSize"`
	Version  string `json:"version"`
}

// HandleIdentity returns the user's identity info.
func (a *API) HandleIdentity(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	resp := identityResponse{
		UserID:   identity.UserID(a.Keypair.Ed25519Pub),
		PubKey:   identity.ToBase58(a.Keypair.Ed25519Pub[:]),
		X25519:   identity.ToBase58(a.Keypair.X25519Pub[:]),
		Mnemonic: a.Mnemonic,
	}
	writeJSON(w, resp)
}

// HandleContacts handles GET (list) and POST (add) for contacts.
func (a *API) HandleContacts(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, a.Contacts.List())

	case http.MethodPost:
		var req contactRequest
		if err := readJSON(r, &req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		edPubBytes, err := identity.FromBase58(req.PubKeyBase58)
		if err != nil || len(edPubBytes) != 32 {
			http.Error(w, "invalid public key", http.StatusBadRequest)
			return
		}
		var edPub [32]byte
		copy(edPub[:], edPubBytes)

		var x25519Pub [32]byte
		if req.X25519Base58 != "" {
			x25519Bytes, err := identity.FromBase58(req.X25519Base58)
			if err == nil && len(x25519Bytes) == 32 {
				copy(x25519Pub[:], x25519Bytes)
			}
		}

		a.Contacts.Add(req.Name, edPub, x25519Pub)
		w.WriteHeader(http.StatusCreated)
		writeJSON(w, map[string]string{"status": "ok"})

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// HandleMessages handles GET (history) and POST (send) for messages.
func (a *API) HandleMessages(w http.ResponseWriter, r *http.Request) {
	// Extract userID from path: /api/messages/{userID}
	path := strings.TrimPrefix(r.URL.Path, "/api/messages/")
	userID := strings.TrimRight(path, "/")
	if userID == "" {
		http.Error(w, "userID required", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodGet:
		msgs := a.Inbox.GetMessages(userID)
		writeJSON(w, msgs)

	case http.MethodPost:
		var req messageRequest
		if err := readJSON(r, &req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		// Find contact
		contact := a.Contacts.GetByUserID(userID)
		if contact == nil {
			http.Error(w, "contact not found", http.StatusNotFound)
			return
		}

		// Decode contact's keys
		edPubBytes, err := identity.FromBase58(contact.PubKey)
		if err != nil {
			http.Error(w, "invalid contact key", http.StatusInternalServerError)
			return
		}
		var recipientKeys message.RecipientKeys
		copy(recipientKeys.Ed25519Pub[:], edPubBytes)

		if contact.X25519Pub != "" {
			x25519Bytes, err := identity.FromBase58(contact.X25519Pub)
			if err == nil {
				copy(recipientKeys.X25519Pub[:], x25519Bytes)
			}
		}

		// Create message
		msg, err := message.New(a.Keypair, recipientKeys, req.Text)
		if err != nil {
			http.Error(w, "failed to create message: "+err.Error(), http.StatusInternalServerError)
			return
		}

		// Store in inbox
		a.Inbox.AddMessage(userID, store.InboxMessage{
			ID:        hex.EncodeToString(msg.ID[:]),
			From:      identity.UserID(a.Keypair.Ed25519Pub),
			FromPub:   identity.ToBase58(a.Keypair.Ed25519Pub[:]),
			Text:      req.Text,
			Timestamp: msg.Timestamp,
			Status:    "sent",
			Outgoing:  true,
		})

		// Add to bloom
		a.Bloom.Add(msg.ID)

		// Store in hold for forwarding
		a.Hold.Store(msg)

		// Broadcast to connected peers
		if a.Transport != nil {
			a.Transport.BroadcastMessage(msg)
		}

		// Send via relay if connected
		if a.RelayClient != nil && a.RelayClient.IsConnected() {
			a.RelayClient.SendMessage(msg)
		}

		writeJSON(w, map[string]string{
			"status": "sent",
			"id":     hex.EncodeToString(msg.ID[:]),
		})

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// HandleStatus returns node status.
func (a *API) HandleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	mode := a.Mode
	if a.RelayClient != nil && a.RelayClient.IsConnected() {
		mode = "relay"
	} else if a.Peers.Count() > 0 {
		mode = "lan"
	}

	resp := statusResponse{
		Mode:     mode,
		Peers:    a.Peers.Count(),
		HoldSize: a.Hold.Count(),
		Version:  "0.1.0-alpha",
	}
	writeJSON(w, resp)
}

// HandleImport imports contacts from JSON.
func (a *API) HandleImport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Read body as JSON contacts array
	body, err := io.ReadAll(io.LimitReader(r.Body, 10*1024*1024))
	if err != nil {
		http.Error(w, "failed to read body", http.StatusBadRequest)
		return
	}

	var contacts []struct {
		Name      string `json:"name"`
		PublicKey string `json:"publicKey"`
		X25519Key string `json:"x25519Key"`
	}
	if err := json.Unmarshal(body, &contacts); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	imported := 0
	for _, c := range contacts {
		edPubBytes, err := identity.FromBase58(c.PublicKey)
		if err != nil || len(edPubBytes) != 32 {
			continue
		}
		var edPub [32]byte
		copy(edPub[:], edPubBytes)

		var x25519Pub [32]byte
		if c.X25519Key != "" {
			x25519Bytes, err := identity.FromBase58(c.X25519Key)
			if err == nil && len(x25519Bytes) == 32 {
				copy(x25519Pub[:], x25519Bytes)
			}
		}

		a.Contacts.Add(c.Name, edPub, x25519Pub)
		imported++
	}

	writeJSON(w, map[string]interface{}{
		"status":   "ok",
		"imported": imported,
	})
}

// HandleIncomingMessage processes a message received from the mesh.
func (a *API) HandleIncomingMessage(msg *message.Message) {
	// Check if it's for us
	if msg.IsForRecipient(a.Keypair.Ed25519Pub) {
		// Decrypt
		payload := &iskraCrypto.EncryptedPayload{
			EphemeralPub: msg.EphemeralPub,
			Nonce:        msg.Nonce,
			Ciphertext:   msg.Payload,
		}
		plaintext, err := iskraCrypto.Decrypt(a.Keypair.X25519Private, payload)
		if err != nil {
			return // Can't decrypt — not really for us or corrupted
		}

		// Handle by content type
		switch msg.ContentType {
		case message.ContentText:
			senderID := identity.UserID(msg.AuthorPub)
			a.Inbox.AddMessage(senderID, store.InboxMessage{
				ID:        hex.EncodeToString(msg.ID[:]),
				From:      senderID,
				FromPub:   identity.ToBase58(msg.AuthorPub[:]),
				Text:      string(plaintext),
				Timestamp: msg.Timestamp,
				Status:    "delivered",
				Outgoing:  false,
			})

			// Update contact last seen
			a.Contacts.UpdateLastSeen(senderID, time.Now().Unix())

		case message.ContentDeliveryConfirm:
			if len(plaintext) == 32 {
				msgID := hex.EncodeToString(plaintext)
				a.Inbox.MarkDelivered(msgID)
				// Remove from hold
				var id [32]byte
				copy(id[:], plaintext)
				a.Hold.Delete(id)
			}
		}
	}

	// Handle broadcast delivery confirms
	if msg.IsBroadcast() && msg.ContentType == message.ContentDeliveryConfirm {
		// Try to read the original message ID from payload (it's encrypted for broadcast sender)
		// For broadcasts, just store and forward
	}
}

func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func readJSON(r *http.Request, v interface{}) error {
	return json.NewDecoder(r.Body).Decode(v)
}
