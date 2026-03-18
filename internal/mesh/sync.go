package mesh

import (
	"github.com/iskra-messenger/iskra/internal/message"
	"github.com/iskra-messenger/iskra/internal/store"
)

// Node orchestrates discovery, transport, and store-and-forward.
type Node struct {
	Keypair    [32]byte // Ed25519 public key
	Discovery  *Discovery
	Transport  *Transport
	Peers      *PeerList
	Hold       *store.Hold
	Bloom      *store.SimpleBloom
	OnMessage  func(*message.Message)
}

// ProcessIncoming handles an incoming message:
// - Check signature, PoW, bloom
// - If for us → deliver
// - If not → store in hold
func (n *Node) ProcessIncoming(msg *message.Message) {
	// Check if we already have this message
	if n.Bloom.Contains(msg.ID) {
		return
	}

	// Verify signature
	if !msg.VerifySignature() {
		return
	}

	// Verify PoW
	if !msg.VerifyPoW(16) {
		return
	}

	// Mark as seen
	n.Bloom.Add(msg.ID)

	// Deliver to callback (main app will check if it's for us and decrypt if so)
	if n.OnMessage != nil {
		n.OnMessage(msg)
	}

	// If not for us, store in hold for forwarding
	if !msg.IsForRecipient(n.Keypair) && !msg.IsBroadcast() {
		n.Hold.Store(msg)
	}

	// If broadcast (e.g., delivery confirm), also store for forwarding
	if msg.IsBroadcast() {
		n.Hold.Store(msg)
	}
}
