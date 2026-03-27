package web

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	iskraCrypto "github.com/iskra-messenger/iskra/internal/crypto"
	"github.com/iskra-messenger/iskra/internal/filetransfer"
	"github.com/iskra-messenger/iskra/internal/identity"
	"github.com/iskra-messenger/iskra/internal/message"
	"github.com/iskra-messenger/iskra/internal/mesh"
	"github.com/iskra-messenger/iskra/internal/security"
	"github.com/iskra-messenger/iskra/internal/store"
)

// Build number — major.minor: major = feature builds, minor = polish/fix builds
const BuildNumber = "19"

// API handles REST API requests.
type API struct {
	Keypair      *identity.Keypair
	Mnemonic     []string
	Contacts     *store.Contacts
	Inbox        *store.Inbox
	Hold         *store.Hold
	Bloom        *store.SimpleBloom
	Peers        *mesh.PeerList
	Transport    *mesh.Transport
	RelayClient  *mesh.RelayClient
	DNSTransport *mesh.DNSTransport
	Groups       *store.Groups
	Channels     *store.Channels
	FileMgr      *filetransfer.Manager
	Mode         string // "lan", "relay", "offline"
	DataDir      string // For restore functionality
	InboxPath    string // Dynamic path to inbox file (per-identity)
	Seed         [32]byte
	Locked       bool     // true if PIN required and not yet verified
	VaultKey     *[32]byte
	UnlockCh     chan struct{} // closed when PIN verified / setup complete
}

// InboxFilePath returns the per-identity inbox file path.
func (a *API) InboxFilePath() string {
	if a.InboxPath != "" {
		return a.InboxPath
	}
	return filepath.Join(a.DataDir, "inbox.json")
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
	Mode     string `json:"mode"` // "solntse", "inferno", "lan", "offline"
	Peers    int    `json:"peers"`
	Relay    bool   `json:"relay"`
	DNS      bool   `json:"dns"`
	HoldSize int    `json:"holdSize"`
	Clippers int    `json:"clippers"` // silent mesh nodes (Pixel Classics blockade runners)
	Version  string `json:"version"`
	Build    string `json:"build"`
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

		// Auto-save inbox on send
		if a.DataDir != "" {
			a.Inbox.Save(a.InboxFilePath())
		}

		// Add to bloom
		a.Bloom.Add(msg.ID)

		// Store in hold for forwarding
		a.Hold.Store(msg)

		// Broadcast to connected peers
		if a.Transport != nil {
			a.Transport.BroadcastMessage(msg)
			log.Printf("[Send] Broadcast to %d LAN peers", a.Peers.Count())
		}

		// Send via relay if connected
		if a.RelayClient != nil && a.RelayClient.IsConnected() {
			if err := a.RelayClient.SendMessage(msg); err != nil {
				log.Printf("[Send] Relay send failed: %v", err)
			} else {
				log.Printf("[Send] Sent via relay, recipientID=%x", msg.RecipientID[:])
			}
		} else if a.DNSTransport != nil && a.DNSTransport.IsConnected() {
			// Fallback: DNS tunnel when relay is down
			if err := a.DNSTransport.SendMessage(msg); err != nil {
				log.Printf("[Send] DNS tunnel send failed: %v", err)
			} else {
				log.Printf("[Send] Sent via DNS tunnel, recipientID=%x", msg.RecipientID[:])
			}
		} else {
			log.Printf("[Send] No relay/DNS connected, message stored in hold")
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

	relayConnected := a.RelayClient != nil && a.RelayClient.IsConnected()
	dnsConnected := a.DNSTransport != nil && a.DNSTransport.IsConnected()

	// Mode logic: relay up = solntse, relay down + dns up = inferno, else lan/offline
	mode := "offline"
	if relayConnected {
		mode = "solntse"
	} else if dnsConnected {
		mode = "inferno"
	} else if a.Peers.Count() > 0 {
		mode = "lan"
	}

	// Fetch clipper count from relay (non-blocking, cached)
	clippers := 0
	if a.RelayClient != nil {
		clippers = a.RelayClient.GetClipperCount()
	}

	resp := statusResponse{
		Mode:     mode,
		Peers:    a.Peers.Count(),
		Relay:    relayConnected,
		DNS:      dnsConnected,
		HoldSize: a.Hold.Count(),
		Clippers: clippers,
		Version:  "0.6.0-alpha",
		Build:    BuildNumber,
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
			log.Printf("[Recv] Decryption failed: %v", err)
			return
		}
		log.Printf("[Recv] Decrypted message, type=%d, len=%d", msg.ContentType, len(plaintext))

		// Handle by content type
		switch msg.ContentType {
		case message.ContentText:
			senderID := identity.UserID(msg.AuthorPub)
			log.Printf("[Recv] Text from %s: %q", senderID[:8], string(plaintext))
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

			// Auto-save inbox on receive
			if a.DataDir != "" {
				a.Inbox.Save(a.InboxFilePath())
			}

			// Send delivery confirmation back to sender
			go a.sendDeliveryConfirm(msg)

		case message.ContentDeliveryConfirm:
			if len(plaintext) == 32 {
				msgID := hex.EncodeToString(plaintext)
				a.Inbox.MarkDelivered(msgID)
				var id [32]byte
				copy(id[:], plaintext)
				a.Hold.Delete(id)
			}

		case message.ContentGroupText:
			if a.Groups != nil {
				// Payload: groupID + "|" + text
				parts := strings.SplitN(string(plaintext), "|", 2)
				if len(parts) == 2 {
					groupID := parts[0]
					text := parts[1]
					senderID := identity.UserID(msg.AuthorPub)

					// Resolve sender name
					senderName := senderID[:8]
					if contact := a.Contacts.GetByUserID(senderID); contact != nil {
						senderName = contact.Name
					} else if group := a.Groups.Get(groupID); group != nil {
						for _, m := range group.Members {
							if m.UserID == senderID {
								senderName = m.Name
								break
							}
						}
					}

					log.Printf("[Group] Text in %s from %s: %q", groupID[:8], senderName, text)
					a.Groups.AddMessage(store.GroupMessage{
						ID:        hex.EncodeToString(msg.ID[:]),
						GroupID:   groupID,
						From:      senderID,
						FromName:  senderName,
						Text:      text,
						Timestamp: msg.Timestamp,
						Outgoing:  false,
					})
					a.Groups.Save()
				}
			}

		case message.ContentGroupInvite:
			if a.Groups != nil {
				var group store.Group
				if err := json.Unmarshal(plaintext, &group); err == nil {
					log.Printf("[Group] Received invite for group %q (%s)", group.Name, group.ID[:8])
					a.Groups.AddByInvite(group)
				}
			}

		case message.ContentFileChunk:
			if a.FileMgr != nil {
				senderID := identity.UserID(msg.AuthorPub)
				filePath, complete := a.FileMgr.ReceiveChunk(plaintext)
				if complete {
					log.Printf("[File] Received complete file: %s from %s", filePath, senderID[:8])
					a.Inbox.AddMessage(senderID, store.InboxMessage{
						ID:        hex.EncodeToString(msg.ID[:]),
						From:      senderID,
						Text:      fmt.Sprintf("[File received: %s]", filepath.Base(filePath)),
						Timestamp: msg.Timestamp,
						Status:    "delivered",
						Outgoing:  false,
					})
					a.Inbox.Save(a.InboxFilePath())
					a.Contacts.UpdateLastSeen(senderID, time.Now().Unix())
				}
			}
		}
	}

	// Handle broadcast channel posts (plaintext payload, signed)
	if msg.IsBroadcast() && msg.ContentType == message.ContentChannelPost {
		if a.Channels != nil {
			payload := msg.Payload
			if msg.VerifySignature() {
				parts := strings.SplitN(string(payload), "|", 3)
				if len(parts) == 3 {
					chID := parts[0]
					chTitle := parts[1]
					text := parts[2]
					authorID := identity.UserID(msg.AuthorPub)

					// Auto-subscribe if not subscribed
					if _, ok := a.Channels.Get(chID); !ok {
						a.Channels.Subscribe(store.Channel{
							ID:        chID,
							AuthorPub: identity.ToBase58(msg.AuthorPub[:]),
							Title:     chTitle,
							CreatedAt: time.Now().Unix(),
							IsOwner:   false,
						})
					}

					log.Printf("[Channel] Post in %q from %s: %q", chTitle, authorID[:8], text)
					a.Channels.AddPost(store.ChannelPost{
						ID:        hex.EncodeToString(msg.ID[:]),
						ChannelID: chID,
						From:      authorID,
						FromName:  chTitle,
						Text:      text,
						Timestamp: msg.Timestamp,
						Outgoing:  false,
					})
					a.Channels.Save()
				}
			}
		}
	}

	// Handle broadcast delivery confirms
	if msg.IsBroadcast() && msg.ContentType == message.ContentDeliveryConfirm {
		// Try to read the original message ID from payload (it's encrypted for broadcast sender)
		// For broadcasts, just store and forward
	}
}

// HandleOnline proxies the relay /online endpoint, converting hex keys to base58.
func (a *API) HandleOnline(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	emptyResp := map[string]interface{}{"count": 0, "peers": []interface{}{}}

	if a.RelayClient == nil {
		writeJSON(w, emptyResp)
		return
	}

	relayHTTP := a.RelayClient.HTTPBaseURL()
	if relayHTTP == "" {
		writeJSON(w, emptyResp)
		return
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(relayHTTP + "/online")
	if err != nil {
		writeJSON(w, emptyResp)
		return
	}
	defer resp.Body.Close()

	var relayData struct {
		Count int `json:"count"`
		Peers []struct {
			Alias  string `json:"alias"`
			EdPub  string `json:"edPub"`
			X25519 string `json:"x25519"`
		} `json:"peers"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&relayData); err != nil {
		writeJSON(w, emptyResp)
		return
	}

	// Convert hex keys to base58 and compute userID, filter out self
	myID := identity.UserID(a.Keypair.Ed25519Pub)
	type peerOut struct {
		Alias  string `json:"alias"`
		UserID string `json:"userID"`
		EdPub  string `json:"edPub"`
		X25519 string `json:"x25519"`
	}
	peers := make([]peerOut, 0, len(relayData.Peers))
	for _, p := range relayData.Peers {
		edBytes, err := hex.DecodeString(p.EdPub)
		if err != nil || len(edBytes) != 32 {
			continue
		}
		var edPub [32]byte
		copy(edPub[:], edBytes)
		uid := identity.UserID(edPub)
		if uid == myID {
			continue // Don't show self
		}
		x25519B58 := ""
		if xBytes, err := hex.DecodeString(p.X25519); err == nil && len(xBytes) == 32 {
			x25519B58 = identity.ToBase58(xBytes)
		}
		peers = append(peers, peerOut{
			Alias:  p.Alias,
			UserID: uid,
			EdPub:  identity.ToBase58(edBytes),
			X25519: x25519B58,
		})
	}

	writeJSON(w, map[string]interface{}{
		"count": len(peers),
		"peers": peers,
	})
}

// HandleDeleteChat deletes all messages in a chat.
func (a *API) HandleDeleteChat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	userID := strings.TrimPrefix(r.URL.Path, "/api/chat/delete/")
	userID = strings.TrimRight(userID, "/")
	if userID == "" {
		http.Error(w, "userID required", http.StatusBadRequest)
		return
	}
	a.Inbox.DeleteChat(userID)
	writeJSON(w, map[string]string{"status": "ok"})
}

// HandleRenameContact renames a contact locally.
func (a *API) HandleRenameContact(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	userID := strings.TrimPrefix(r.URL.Path, "/api/contacts/rename/")
	userID = strings.TrimRight(userID, "/")
	if userID == "" {
		http.Error(w, "userID required", http.StatusBadRequest)
		return
	}
	var req struct {
		Name string `json:"name"`
	}
	if err := readJSON(r, &req); err != nil || req.Name == "" {
		http.Error(w, "name required", http.StatusBadRequest)
		return
	}
	if a.Contacts.Rename(userID, req.Name) {
		writeJSON(w, map[string]string{"status": "ok"})
	} else {
		http.Error(w, "contact not found", http.StatusNotFound)
	}
}

// HandleRestore restores identity from mnemonic words.
func (a *API) HandleRestore(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Words string `json:"words"`
	}
	if err := readJSON(r, &req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	words := strings.Fields(req.Words)
	if len(words) != 24 {
		writeJSON(w, map[string]string{"error": "Нужно ровно 24 слова"})
		return
	}

	if !identity.ValidateMnemonic(words) {
		writeJSON(w, map[string]string{"error": "Невалидная мнемоника. Проверьте слова."})
		return
	}

	seed, err := identity.MnemonicToSeed(words)
	if err != nil {
		writeJSON(w, map[string]string{"error": "Ошибка восстановления: " + err.Error()})
		return
	}

	// Save seed
	seedPath := a.DataDir + "/seed.key"
	if err := os.WriteFile(seedPath, seed[:], 0600); err != nil {
		writeJSON(w, map[string]string{"error": "Не удалось сохранить ключ"})
		return
	}

	kp := identity.KeypairFromSeed(seed)
	writeJSON(w, map[string]string{
		"status": "ok",
		"userID": identity.UserID(kp.Ed25519Pub),
		"message": "Ключ восстановлен. Перезапустите приложение.",
	})
}

// sendDeliveryConfirm sends a delivery receipt back to the message sender.
func (a *API) sendDeliveryConfirm(origMsg *message.Message) {
	// Build recipient keys from sender's AuthorPub
	var recipientKeys message.RecipientKeys
	recipientKeys.Ed25519Pub = origMsg.AuthorPub

	// Try to find sender's X25519 key from contacts
	senderID := identity.UserID(origMsg.AuthorPub)
	if contact := a.Contacts.GetByUserID(senderID); contact != nil && contact.X25519Pub != "" {
		if xBytes, err := identity.FromBase58(contact.X25519Pub); err == nil && len(xBytes) == 32 {
			copy(recipientKeys.X25519Pub[:], xBytes)
		}
	}

	// Payload = original message ID (32 bytes)
	confirmMsg, err := message.NewWithType(a.Keypair, recipientKeys, message.ContentDeliveryConfirm, origMsg.ID[:])
	if err != nil {
		log.Printf("[Confirm] Failed to create: %v", err)
		return
	}

	// Send via relay
	if a.RelayClient != nil && a.RelayClient.IsConnected() {
		if err := a.RelayClient.SendMessage(confirmMsg); err != nil {
			log.Printf("[Confirm] Relay send failed: %v", err)
		} else {
			log.Printf("[Confirm] Sent delivery confirm for %x", origMsg.ID[:4])
		}
	}

	// Broadcast to LAN peers
	if a.Transport != nil {
		a.Transport.BroadcastMessage(confirmMsg)
	}
}

// HandleGroups handles GET (list) and POST (create) for groups.
func (a *API) HandleGroups(w http.ResponseWriter, r *http.Request) {
	if a.Groups == nil {
		http.Error(w, "groups not initialized", http.StatusInternalServerError)
		return
	}

	switch r.Method {
	case http.MethodGet:
		writeJSON(w, a.Groups.List())

	case http.MethodPost:
		var req struct {
			Name    string   `json:"name"`
			Members []string `json:"members"` // list of UserIDs
		}
		if err := readJSON(r, &req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if req.Name == "" || len(req.Members) < 1 {
			http.Error(w, "name and at least 1 member required", http.StatusBadRequest)
			return
		}

		// Build member list from contacts
		myID := identity.UserID(a.Keypair.Ed25519Pub)
		members := []store.GroupMember{{
			UserID:    myID,
			Name:      "Я",
			PubKey:    identity.ToBase58(a.Keypair.Ed25519Pub[:]),
			X25519Pub: identity.ToBase58(a.Keypair.X25519Pub[:]),
		}}

		for _, uid := range req.Members {
			contact := a.Contacts.GetByUserID(uid)
			if contact == nil {
				http.Error(w, "contact not found: "+uid, http.StatusBadRequest)
				return
			}
			members = append(members, store.GroupMember{
				UserID:    contact.UserID,
				Name:      contact.Name,
				PubKey:    contact.PubKey,
				X25519Pub: contact.X25519Pub,
			})
		}

		group := a.Groups.Create(req.Name, myID, members)

		// Send group invite to all members
		go a.sendGroupInvites(group)

		writeJSON(w, group)

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// HandleGroupMessages handles GET (history) and POST (send) for group messages.
func (a *API) HandleGroupMessages(w http.ResponseWriter, r *http.Request) {
	if a.Groups == nil {
		http.Error(w, "groups not initialized", http.StatusInternalServerError)
		return
	}

	groupID := strings.TrimPrefix(r.URL.Path, "/api/groups/messages/")
	groupID = strings.TrimRight(groupID, "/")
	if groupID == "" {
		http.Error(w, "groupID required", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodGet:
		msgs := a.Groups.GetMessages(groupID)
		writeJSON(w, msgs)

	case http.MethodPost:
		var req struct {
			Text      string `json:"text"`
			ReplyTo   string `json:"replyTo"`
			ReplyText string `json:"replyText"`
			ReplyFrom string `json:"replyFrom"`
		}
		if err := readJSON(r, &req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		group := a.Groups.Get(groupID)
		if group == nil {
			http.Error(w, "group not found", http.StatusNotFound)
			return
		}

		myID := identity.UserID(a.Keypair.Ed25519Pub)

		// Store in group messages
		msgIDBytes := make([]byte, 32)
		for i := range msgIDBytes {
			msgIDBytes[i] = byte(i) ^ byte(time.Now().UnixNano()>>(i%8))
		}
		msgID := hex.EncodeToString(msgIDBytes)

		a.Groups.AddMessage(store.GroupMessage{
			ID:        msgID,
			GroupID:   groupID,
			From:      myID,
			FromName:  "Я",
			Text:      req.Text,
			Timestamp: time.Now().Unix(),
			Outgoing:  true,
			ReplyTo:   req.ReplyTo,
			ReplyText: req.ReplyText,
			ReplyFrom: req.ReplyFrom,
		})

		// Payload: groupID (hex, 64 chars) + "|" + text
		payload := []byte(groupID + "|" + req.Text)

		// Send to each member (except self)
		for _, member := range group.Members {
			if member.UserID == myID {
				continue
			}

			edPubBytes, err := identity.FromBase58(member.PubKey)
			if err != nil || len(edPubBytes) != 32 {
				continue
			}
			var recipientKeys message.RecipientKeys
			copy(recipientKeys.Ed25519Pub[:], edPubBytes)

			if member.X25519Pub != "" {
				x25519Bytes, err := identity.FromBase58(member.X25519Pub)
				if err == nil && len(x25519Bytes) == 32 {
					copy(recipientKeys.X25519Pub[:], x25519Bytes)
				}
			}

			msg, err := message.NewWithType(a.Keypair, recipientKeys, message.ContentGroupText, payload)
			if err != nil {
				log.Printf("[Group] Failed to create message for %s: %v", member.UserID[:8], err)
				continue
			}

			a.Bloom.Add(msg.ID)
			a.Hold.Store(msg)

			if a.RelayClient != nil && a.RelayClient.IsConnected() {
				a.RelayClient.SendMessage(msg)
			}
			if a.Transport != nil {
				a.Transport.BroadcastMessage(msg)
			}
		}

		a.Groups.Save()
		writeJSON(w, map[string]string{"status": "sent", "id": msgID})

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// HandleDeleteGroup deletes a group chat.
func (a *API) HandleDeleteGroup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	groupID := strings.TrimPrefix(r.URL.Path, "/api/groups/delete/")
	groupID = strings.TrimRight(groupID, "/")
	if groupID == "" {
		http.Error(w, "groupID required", http.StatusBadRequest)
		return
	}
	if a.Groups.Delete(groupID) {
		writeJSON(w, map[string]string{"status": "ok"})
	} else {
		http.Error(w, "group not found", http.StatusNotFound)
	}
}

// sendGroupInvites sends group invite to all members.
func (a *API) sendGroupInvites(group *store.Group) {
	myID := identity.UserID(a.Keypair.Ed25519Pub)

	// Invite payload: JSON with group info
	inviteData, err := json.Marshal(group)
	if err != nil {
		log.Printf("[Group] Failed to marshal invite: %v", err)
		return
	}

	for _, member := range group.Members {
		if member.UserID == myID {
			continue
		}

		edPubBytes, err := identity.FromBase58(member.PubKey)
		if err != nil || len(edPubBytes) != 32 {
			continue
		}
		var recipientKeys message.RecipientKeys
		copy(recipientKeys.Ed25519Pub[:], edPubBytes)

		if member.X25519Pub != "" {
			x25519Bytes, err := identity.FromBase58(member.X25519Pub)
			if err == nil && len(x25519Bytes) == 32 {
				copy(recipientKeys.X25519Pub[:], x25519Bytes)
			}
		}

		msg, err := message.NewWithType(a.Keypair, recipientKeys, message.ContentGroupInvite, inviteData)
		if err != nil {
			log.Printf("[Group] Failed to create invite for %s: %v", member.UserID[:8], err)
			continue
		}

		a.Bloom.Add(msg.ID)
		a.Hold.Store(msg)

		if a.RelayClient != nil && a.RelayClient.IsConnected() {
			a.RelayClient.SendMessage(msg)
		}
		if a.Transport != nil {
			a.Transport.BroadcastMessage(msg)
		}

		log.Printf("[Group] Sent invite to %s for group %s", member.UserID[:8], group.Name)
	}
}

// HandleUnread returns unread counts for all contacts and groups in one call.
func (a *API) HandleUnread(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		LastRead map[string]int64 `json:"lastRead"` // key -> timestamp
	}
	if err := readJSON(r, &req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	counts := make(map[string]int)
	lastMsg := make(map[string]string)
	lastTs := make(map[string]int64) // last message timestamp for sorting

	// All inbox keys (contacts + anyone who messaged us)
	checkedKeys := make(map[string]bool)
	for _, c := range a.Contacts.List() {
		checkedKeys[c.UserID] = true
	}
	// Also check inbox keys not in contacts (new senders)
	for _, k := range a.Inbox.Keys() {
		checkedKeys[k] = true
	}

	for uid := range checkedKeys {
		msgs := a.Inbox.GetMessages(uid)
		lr := req.LastRead[uid]
		unread := 0
		last := ""
		var maxTs int64
		for _, m := range msgs {
			if !m.Outgoing && m.Timestamp > lr {
				unread++
			}
			last = m.Text
			if m.Timestamp > maxTs {
				maxTs = m.Timestamp
			}
		}
		if unread > 0 {
			counts[uid] = unread
		}
		if last != "" {
			if len(last) > 40 {
				last = last[:40] + "..."
			}
			lastMsg[uid] = last
		}
		if maxTs > 0 {
			lastTs[uid] = maxTs
		}
	}

	// Groups
	if a.Groups != nil {
		for _, g := range a.Groups.List() {
			msgs := a.Groups.GetMessages(g.ID)
			lr := req.LastRead["g:"+g.ID]
			unread := 0
			last := ""
			var maxTs int64
			for _, m := range msgs {
				if !m.Outgoing && m.Timestamp > lr {
					unread++
				}
				if m.Text != "" {
					if m.Outgoing {
						last = "Вы: " + m.Text
					} else {
						last = m.FromName + ": " + m.Text
					}
				}
				if m.Timestamp > maxTs {
					maxTs = m.Timestamp
				}
			}
			key := "g:" + g.ID
			if unread > 0 {
				counts[key] = unread
			}
			if last != "" {
				if len(last) > 40 {
					last = last[:40] + "..."
				}
				lastMsg[key] = last
			}
			if maxTs > 0 {
				lastTs[key] = maxTs
			}
		}
	}

	writeJSON(w, map[string]interface{}{
		"counts":  counts,
		"lastMsg": lastMsg,
		"lastTs":  lastTs,
	})
}

// --- FOTA (update check) ---

var (
	updateCacheMu     sync.Mutex
	updateCacheResult *updateCheckResponse
	updateCacheTime   time.Time
)

type updateAsset struct {
	Name string `json:"name"`
	URL  string `json:"url"`
	Size int64  `json:"size"`
}

type updateCheckResponse struct {
	Available   bool          `json:"available"`
	Version     string        `json:"version"`
	RemoteBuild string        `json:"remoteBuild,omitempty"`
	Changelog   string        `json:"changelog"`
	Assets      []updateAsset `json:"assets"`
}

// HandleCheckUpdate checks GitHub Releases for a newer version.
func (a *API) HandleCheckUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	updateCacheMu.Lock()
	if updateCacheResult != nil && time.Since(updateCacheTime) < 1*time.Hour {
		cached := *updateCacheResult
		updateCacheMu.Unlock()
		writeJSON(w, cached)
		return
	}
	updateCacheMu.Unlock()

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get("https://api.github.com/repos/elyssov/iskra/releases/latest")
	if err != nil {
		log.Printf("[Update] GitHub API error: %v", err)
		writeJSON(w, updateCheckResponse{Available: false})
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Printf("[Update] GitHub API status: %d", resp.StatusCode)
		writeJSON(w, updateCheckResponse{Available: false})
		return
	}

	var release struct {
		TagName string `json:"tag_name"`
		Body    string `json:"body"`
		Assets  []struct {
			Name               string `json:"name"`
			BrowserDownloadURL string `json:"browser_download_url"`
			Size               int64  `json:"size"`
		} `json:"assets"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		log.Printf("[Update] JSON parse error: %v", err)
		writeJSON(w, updateCheckResponse{Available: false})
		return
	}

	// Compare: if remote tag differs from local version, update is available
	remoteVer := strings.TrimPrefix(release.TagName, "v")
	localVer := "0.6.0-alpha"

	// Extract remote build number from APK asset name (iskra-buildXX.apk)
	remoteBuild := ""
	for _, a := range release.Assets {
		name := strings.ToLower(a.Name)
		if strings.HasPrefix(name, "iskra-build") && strings.HasSuffix(name, ".apk") {
			remoteBuild = strings.TrimSuffix(strings.TrimPrefix(name, "iskra-build"), ".apk")
			break
		}
	}

	// Update available if version differs OR remote build number is strictly higher
	available := remoteVer != localVer
	if !available && remoteBuild != "" {
		localF, errL := strconv.ParseFloat(BuildNumber, 64)
		remoteF, errR := strconv.ParseFloat(remoteBuild, 64)
		if errL == nil && errR == nil && remoteF > localF {
			available = true
		}
	}
	log.Printf("[Update] Local=%s (build %s) Remote=%s (build %s) Available=%v", localVer, BuildNumber, remoteVer, remoteBuild, available)

	assets := make([]updateAsset, 0, len(release.Assets))
	for _, a := range release.Assets {
		assets = append(assets, updateAsset{
			Name: a.Name,
			URL:  a.BrowserDownloadURL,
			Size: a.Size,
		})
	}

	result := &updateCheckResponse{
		Available:   available,
		Version:     remoteVer,
		RemoteBuild: remoteBuild,
		Changelog:   release.Body,
		Assets:      assets,
	}

	updateCacheMu.Lock()
	updateCacheResult = result
	updateCacheTime = time.Now()
	updateCacheMu.Unlock()

	writeJSON(w, result)
}

// HandleUpdateDownload proxies download of an update asset from GitHub and saves locally.
// POST /api/update/download with {"url":"https://...","filename":"iskra-build10.apk"}
func (a *API) HandleUpdateDownload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		URL      string `json:"url"`
		Filename string `json:"filename"`
	}
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, map[string]interface{}{"error": "bad request"})
		return
	}
	if req.URL == "" || req.Filename == "" {
		writeJSON(w, map[string]interface{}{"error": "url and filename required"})
		return
	}

	log.Printf("[Update] Downloading %s from %s", req.Filename, req.URL)

	client := &http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Get(req.URL)
	if err != nil {
		log.Printf("[Update] Download error: %v", err)
		writeJSON(w, map[string]interface{}{"error": "download failed: " + err.Error()})
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Printf("[Update] Download HTTP %d", resp.StatusCode)
		writeJSON(w, map[string]interface{}{"error": fmt.Sprintf("HTTP %d", resp.StatusCode)})
		return
	}

	// Determine save path
	savePath := filepath.Join(a.DataDir, req.Filename)

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("[Update] Read error: %v", err)
		writeJSON(w, map[string]interface{}{"error": "read failed"})
		return
	}

	if err := os.WriteFile(savePath, data, 0644); err != nil {
		log.Printf("[Update] Save error: %v", err)
		writeJSON(w, map[string]interface{}{"error": "save failed: " + err.Error()})
		return
	}

	log.Printf("[Update] Saved %s (%d bytes) to %s", req.Filename, len(data), savePath)

	// For Windows exe: if this is an exe, save next to current binary
	exeSavePath := savePath
	if strings.HasSuffix(req.Filename, ".exe") {
		exePath, err := os.Executable()
		if err == nil {
			dir := filepath.Dir(exePath)
			exeSavePath = filepath.Join(dir, req.Filename)
			if exeSavePath != savePath {
				os.WriteFile(exeSavePath, data, 0755)
				log.Printf("[Update] Also saved exe to %s", exeSavePath)
			}
		}
	}

	writeJSON(w, map[string]interface{}{
		"ok":   true,
		"path": savePath,
		"size": len(data),
	})
}

// HandlePINStatus returns the current PIN/lock state.
func (a *API) HandlePINStatus(w http.ResponseWriter, r *http.Request) {
	hasPin := security.HasPIN(a.DataDir)
	attempts := security.GetAttempts(a.DataDir)
	writeJSON(w, map[string]interface{}{
		"locked":      a.Locked,
		"needsSetup":  !hasPin,
		"attempts":    attempts,
		"maxAttempts":  security.MaxAttempts,
	})
}

// HandlePINSetup sets up a new PIN (first time or after wipe).
func (a *API) HandlePINSetup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		PIN string `json:"pin"`
	}
	if err := readJSON(r, &req); err != nil || len(req.PIN) < 4 || len(req.PIN) > 6 {
		writeJSON(w, map[string]interface{}{"ok": false, "error": "PIN должен быть от 4 до 6 цифр"})
		return
	}

	if err := security.SetPIN(a.DataDir, req.PIN); err != nil {
		writeJSON(w, map[string]interface{}{"ok": false, "error": err.Error()})
		return
	}

	// Set per-identity inbox path
	userID := identity.UserID(a.Keypair.Ed25519Pub)
	a.InboxPath = filepath.Join(a.DataDir, "inbox-"+userID+".json")

	// Derive storage key and unlock
	key := security.DeriveStorageKey(a.Seed, req.PIN)
	a.VaultKey = &key

	// Re-save existing stores encrypted (migration)
	if a.Contacts != nil {
		a.Contacts.VaultKey = &key
	}
	if a.Inbox != nil {
		a.Inbox.VaultKey = &key
	}
	if a.Groups != nil {
		a.Groups.VaultKey = &key
	}

	a.Locked = false
	if a.UnlockCh != nil {
		select {
		case <-a.UnlockCh:
		default:
			close(a.UnlockCh)
		}
	}

	log.Println("[PIN] PIN set, stores encrypted")
	writeJSON(w, map[string]interface{}{"ok": true})
}

// HandlePINVerify verifies the PIN and unlocks the app.
func (a *API) HandlePINVerify(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		PIN string `json:"pin"`
	}
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, map[string]interface{}{"ok": false, "error": "bad request"})
		return
	}

	// Check attempts
	attempts := security.GetAttempts(a.DataDir)
	if attempts >= security.MaxAttempts {
		log.Println("[PIN] Max attempts reached, wiping")
		security.WipeAll(a.DataDir)
		security.GenerateDecoy(a.DataDir)
		writeJSON(w, map[string]interface{}{"ok": false, "wiped": true, "error": "Данные уничтожены"})
		return
	}

	if !security.VerifyPIN(a.DataDir, req.PIN) {
		newAttempts := security.IncrementAttempts(a.DataDir)
		remaining := security.MaxAttempts - newAttempts
		if remaining <= 0 {
			log.Println("[PIN] Max attempts reached after verify, wiping")
			security.WipeAll(a.DataDir)
			security.GenerateDecoy(a.DataDir)
			writeJSON(w, map[string]interface{}{"ok": false, "wiped": true, "error": "Данные уничтожены"})
			return
		}
		writeJSON(w, map[string]interface{}{"ok": false, "remaining": remaining})
		return
	}

	security.ResetAttempts(a.DataDir)

	// Set per-identity inbox path
	userID := identity.UserID(a.Keypair.Ed25519Pub)
	a.InboxPath = filepath.Join(a.DataDir, "inbox-"+userID+".json")

	// Derive storage key and set on stores
	key := security.DeriveStorageKey(a.Seed, req.PIN)
	a.VaultKey = &key

	if a.Contacts != nil {
		a.Contacts.SetVaultKey(&key)
	}
	if a.Inbox != nil {
		a.Inbox.VaultKey = &key
		a.Inbox.Load(a.InboxFilePath())
	}
	if a.Groups != nil {
		a.Groups.SetVaultKey(&key)
	}

	a.Locked = false
	if a.UnlockCh != nil {
		select {
		case <-a.UnlockCh:
		default:
			close(a.UnlockCh)
		}
	}

	log.Println("[PIN] Unlocked successfully")
	writeJSON(w, map[string]interface{}{"ok": true})
}

// HandlePanic triggers secure data wipe.
func (a *API) HandlePanic(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Code string `json:"code"`
	}
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, map[string]interface{}{"ok": false})
		return
	}

	// Default panic code: "159" — can be made configurable later
	if req.Code != "159" {
		writeJSON(w, map[string]interface{}{"ok": false, "error": "неверный код"})
		return
	}

	log.Println("[PANIC] Panic mode triggered!")
	security.WipeAll(a.DataDir)
	security.GenerateDecoy(a.DataDir)
	log.Println("[PANIC] Wipe complete, decoy generated")

	writeJSON(w, map[string]interface{}{"ok": true, "wiped": true})
}

// ─── Master developer account (temporary support contact) ───────────

// HandleMasterLogin handles the developer login flow.
func (a *API) HandleMasterLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", 405)
		return
	}
	var req struct {
		Login    string `json:"login"`
		Password string `json:"password"`
	}
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, map[string]interface{}{"ok": false})
		return
	}

	seed, ok := VerifyMasterCredentials(req.Login, req.Password)
	if !ok {
		writeJSON(w, map[string]interface{}{"ok": false, "error": "invalid"})
		return
	}

	// Switch identity to master account
	kp := MasterKeypairFromSeed(*seed)
	a.Keypair = kp
	a.Mnemonic = nil // Master has no mnemonic

	// Save seed for this session
	copy(a.Seed[:], seed[:])

	// Switch inbox to master-specific file
	masterUID := identity.UserID(kp.Ed25519Pub)
	a.InboxPath = filepath.Join(a.DataDir, "inbox-"+masterUID+".json")

	// Master uses separate inbox, but shares contacts/groups (don't clear their VaultKey)
	a.VaultKey = nil
	if a.Inbox != nil {
		a.Inbox.VaultKey = nil
		a.Inbox.Load(a.InboxFilePath())
	}

	// Unlock the app (same as PIN verify)
	a.Locked = false
	if a.UnlockCh != nil {
		select {
		case <-a.UnlockCh:
		default:
			close(a.UnlockCh)
		}
	}

	log.Println("[Master] Developer logged in as Мастер")
	writeJSON(w, map[string]interface{}{
		"ok":     true,
		"userID": identity.UserID(kp.Ed25519Pub),
	})
}

// HandleMasterCheck returns master contact info for auto-add.
func (a *API) HandleMasterCheck(w http.ResponseWriter, r *http.Request) {
	uid, name, edPub, x25519 := MasterContact()
	writeJSON(w, map[string]interface{}{
		"userID":    uid,
		"name":      name,
		"edPub":     edPub,
		"x25519Pub": x25519,
	})
}

// ─── File transfer (chunked encrypted) ──────────────────────────────

// HandleSendFile accepts a multipart file upload and sends it as chunked messages.
func (a *API) HandleSendFile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", 405)
		return
	}

	// Parse recipient from URL
	userID := strings.TrimPrefix(r.URL.Path, "/api/file/send/")
	if userID == "" {
		http.Error(w, "recipient required", 400)
		return
	}

	// Limit body to 10MB + overhead
	r.Body = http.MaxBytesReader(w, r.Body, filetransfer.MaxFileSize+1024*1024)

	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "file required: "+err.Error(), 400)
		return
	}
	defer file.Close()

	// Save to temp file
	tmpDir := filepath.Join(a.DataDir, "tmp")
	os.MkdirAll(tmpDir, 0700)
	tmpPath := filepath.Join(tmpDir, header.Filename)
	tmpFile, err := os.Create(tmpPath)
	if err != nil {
		http.Error(w, "temp file error", 500)
		return
	}
	io.Copy(tmpFile, file)
	tmpFile.Close()
	defer os.Remove(tmpPath)

	// Prepare chunks
	payloads, err := filetransfer.PrepareChunks(tmpPath)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}

	// Get recipient keys
	contact := a.Contacts.GetByUserID(userID)
	if contact == nil {
		http.Error(w, "contact not found", 404)
		return
	}
	edPubBytes, err := identity.FromBase58(contact.PubKey)
	if err != nil || len(edPubBytes) != 32 {
		http.Error(w, "invalid contact key", 400)
		return
	}
	var recipientKeys message.RecipientKeys
	copy(recipientKeys.Ed25519Pub[:], edPubBytes)
	if contact.X25519Pub != "" {
		x25519Bytes, _ := identity.FromBase58(contact.X25519Pub)
		if len(x25519Bytes) == 32 {
			copy(recipientKeys.X25519Pub[:], x25519Bytes)
		}
	}

	// Send each chunk as a separate encrypted message
	sent := 0
	for _, payload := range payloads {
		msg, err := message.NewWithType(a.Keypair, recipientKeys, message.ContentFileChunk, payload)
		if err != nil {
			continue
		}
		a.Bloom.Add(msg.ID)
		a.Hold.StoreWithTTL(msg, filetransfer.ChunkHopTTL)

		if a.RelayClient != nil && a.RelayClient.IsConnected() {
			a.RelayClient.SendMessage(msg)
		}
		if a.Transport != nil {
			a.Transport.BroadcastMessage(msg)
		}
		sent++
	}

	// Store in inbox as file message
	a.Inbox.AddMessage(userID, store.InboxMessage{
		ID:        fmt.Sprintf("file-%d", time.Now().UnixNano()),
		From:      identity.UserID(a.Keypair.Ed25519Pub),
		Text:      fmt.Sprintf("[File: %s] (%d chunks)", header.Filename, sent),
		Timestamp: time.Now().Unix(),
		Status:    "sent",
		Outgoing:  true,
	})
	a.Inbox.Save(a.InboxFilePath())

	log.Printf("[File] Sent %q to %s: %d chunks", header.Filename, userID[:8], sent)
	writeJSON(w, map[string]interface{}{"ok": true, "chunks": sent, "filename": header.Filename})
}

// ─── Mesh peer injection (WiFi Direct → TCP sync) ──────────────────

// HandleAddPeer accepts a peer IP from WiFi Direct discovery and triggers TCP sync.
func (a *API) HandleAddPeer(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", 405)
		return
	}
	var req struct {
		IP   string `json:"ip"`
		Port int    `json:"port,omitempty"`
	}
	if err := readJSON(r, &req); err != nil || req.IP == "" {
		http.Error(w, "ip required", 400)
		return
	}

	// Default mesh port = our own transport port
	port := req.Port
	if port == 0 && a.Transport != nil {
		port = int(a.Transport.Port())
	}
	if port == 0 {
		port = 4243 // fallback
	}

	log.Printf("[Mesh] WiFi Direct peer: %s:%d — initiating sync", req.IP, port)

	go func() {
		holdMsgs, _ := a.Hold.GetForSync()
		if a.Transport != nil {
			a.Transport.ConnectAndSync(req.IP, uint16(port), a.Bloom.Export(), holdMsgs)
		}
	}()

	writeJSON(w, map[string]interface{}{"ok": true, "ip": req.IP, "port": port})
}

// ─── Channels (broadcast one-to-many) ───────────────────────────────

// HandleCreateChannel creates a new broadcast channel.
func (a *API) HandleCreateChannel(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", 405)
		return
	}
	var req struct {
		Title string `json:"title"`
	}
	if err := readJSON(r, &req); err != nil || req.Title == "" {
		http.Error(w, "title required", 400)
		return
	}

	userID := identity.UserID(a.Keypair.Ed25519Pub)
	authorPub := identity.ToBase58(a.Keypair.Ed25519Pub[:])
	x25519Pub := identity.ToBase58(a.Keypair.X25519Pub[:])

	ch := a.Channels.Create(userID, authorPub, x25519Pub, req.Title)
	writeJSON(w, ch)
}

// HandleListChannels returns all subscribed channels.
func (a *API) HandleListChannels(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, a.Channels.List())
}

// HandleChannelPosts returns posts for a channel.
func (a *API) HandleChannelPosts(w http.ResponseWriter, r *http.Request) {
	chID := strings.TrimPrefix(r.URL.Path, "/api/channels/posts/")
	if chID == "" {
		http.Error(w, "channel ID required", 400)
		return
	}
	writeJSON(w, a.Channels.Posts(chID))
}

// HandlePostToChannel publishes a broadcast post.
func (a *API) HandlePostToChannel(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", 405)
		return
	}
	chID := strings.TrimPrefix(r.URL.Path, "/api/channels/post/")
	ch, ok := a.Channels.Get(chID)
	if !ok {
		http.Error(w, "channel not found", 404)
		return
	}
	if !ch.IsOwner {
		http.Error(w, "only channel owner can post", 403)
		return
	}

	var req struct {
		Text string `json:"text"`
	}
	if err := readJSON(r, &req); err != nil || req.Text == "" {
		http.Error(w, "text required", 400)
		return
	}

	// Create broadcast message
	userID := identity.UserID(a.Keypair.Ed25519Pub)
	payload := []byte(chID + "|" + ch.Title + "|" + req.Text)

	msg, err := message.NewPlainBroadcast(a.Keypair, message.ContentChannelPost, payload)
	if err != nil {
		http.Error(w, "failed to create broadcast: "+err.Error(), 500)
		return
	}

	// Store locally
	post := store.ChannelPost{
		ID:        fmt.Sprintf("%x", msg.ID),
		ChannelID: chID,
		From:      userID,
		FromName:  "You",
		Text:      req.Text,
		Timestamp: time.Now().Unix(),
		Outgoing:  true,
	}
	a.Channels.AddPost(post)

	// Broadcast via relay and mesh
	a.Bloom.Add(msg.ID)
	a.Hold.Store(msg)
	if a.RelayClient != nil {
		a.RelayClient.SendMessage(msg)
	}
	if a.Transport != nil {
		a.Transport.BroadcastMessage(msg)
	}

	// Auto-save
	if a.DataDir != "" {
		a.Channels.Save()
	}

	writeJSON(w, post)
}

// HandleSubscribeChannel subscribes to a channel.
func (a *API) HandleSubscribeChannel(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", 405)
		return
	}
	var req struct {
		ID        string `json:"id"`
		AuthorPub string `json:"author_pub"`
		X25519Pub string `json:"x25519_pub"`
		Title     string `json:"title"`
	}
	if err := readJSON(r, &req); err != nil || req.ID == "" {
		http.Error(w, "channel info required", 400)
		return
	}

	ch := store.Channel{
		ID:        req.ID,
		AuthorPub: req.AuthorPub,
		X25519Pub: req.X25519Pub,
		Title:     req.Title,
		CreatedAt: time.Now().Unix(),
		IsOwner:   false,
	}
	a.Channels.Subscribe(ch)
	writeJSON(w, map[string]bool{"ok": true})
}

// HandleUnsubscribeChannel removes a channel subscription.
func (a *API) HandleUnsubscribeChannel(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", 405)
		return
	}
	var req struct {
		ID string `json:"id"`
	}
	if err := readJSON(r, &req); err != nil || req.ID == "" {
		http.Error(w, "channel ID required", 400)
		return
	}
	a.Channels.Unsubscribe(req.ID)
	writeJSON(w, map[string]bool{"ok": true})
}

func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func readJSON(r *http.Request, v interface{}) error {
	return json.NewDecoder(r.Body).Decode(v)
}
