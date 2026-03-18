package message

import (
	"crypto/sha256"
	"time"

	iskraCrypto "github.com/iskra-messenger/iskra/internal/crypto"
	"github.com/iskra-messenger/iskra/internal/identity"
)

// Message represents an Iskra protocol message.
type Message struct {
	// Header (not encrypted, needed for routing)
	Version     uint8
	ID          [32]byte
	RecipientID [20]byte // First 20 bytes of recipient's Ed25519 pubkey
	TTL         uint32
	Timestamp   int64
	ContentType uint8

	// Crypto block
	EphemeralPub [32]byte
	Nonce        [24]byte
	Payload      []byte // Obfuscated ciphertext

	// Signature
	AuthorPub [32]byte // Ed25519 pubkey of author
	Signature [64]byte

	// PoW
	PoWNonce uint64
}

// RecipientKeys holds both keys needed to send a message to someone.
type RecipientKeys struct {
	Ed25519Pub [32]byte
	X25519Pub  [32]byte
}

// New creates a new text message, encrypts it, signs it, and solves PoW.
func New(author *identity.Keypair, recipient RecipientKeys, text string) (*Message, error) {
	return NewWithType(author, recipient, ContentText, []byte(text))
}

// NewWithType creates a message with a specific content type.
func NewWithType(author *identity.Keypair, recipient RecipientKeys, contentType uint8, plaintext []byte) (*Message, error) {
	msg := &Message{
		Version:     ProtocolVersion,
		TTL:         DefaultTTL,
		Timestamp:   time.Now().Unix(),
		ContentType: contentType,
		AuthorPub:   author.Ed25519Pub,
	}

	// RecipientID = first 20 bytes of Ed25519 pubkey
	copy(msg.RecipientID[:], recipient.Ed25519Pub[:20])

	// Encrypt with X25519 keys
	encrypted, err := iskraCrypto.Encrypt(author.X25519Private, recipient.X25519Pub, plaintext)
	if err != nil {
		return nil, err
	}
	msg.EphemeralPub = encrypted.EphemeralPub
	msg.Nonce = encrypted.Nonce
	msg.Payload = encrypted.Ciphertext

	// Compute ID = SHA256 of all fields except ID, Signature, PoWNonce
	msg.ID = msg.computeID()

	// Sign
	msg.Signature = iskraCrypto.Sign(author.Ed25519Private, msg.signableBytes())

	// PoW (difficulty 16 for text)
	msg.PoWNonce = iskraCrypto.SolvePoW(msg.ID, msg.Timestamp, 16)

	return msg, nil
}

// NewBroadcast creates a broadcast message (RecipientID = all zeros).
func NewBroadcast(author *identity.Keypair, contentType uint8, plaintext []byte) (*Message, error) {
	// For broadcast: use author's own X25519 pub as "recipient" (payload is signed, not encrypted for specific recipient)
	recipient := RecipientKeys{} // Zero keys = broadcast
	msg := &Message{
		Version:     ProtocolVersion,
		TTL:         DefaultTTL,
		Timestamp:   time.Now().Unix(),
		ContentType: contentType,
		AuthorPub:   author.Ed25519Pub,
	}
	// RecipientID stays all zeros for broadcast

	// For broadcast messages (like delivery confirm), payload is not encrypted
	// Just obfuscate with author's key for consistency
	encrypted, err := iskraCrypto.Encrypt(author.X25519Private, recipient.X25519Pub, plaintext)
	if err != nil {
		return nil, err
	}
	msg.EphemeralPub = encrypted.EphemeralPub
	msg.Nonce = encrypted.Nonce
	msg.Payload = encrypted.Ciphertext

	msg.ID = msg.computeID()
	msg.Signature = iskraCrypto.Sign(author.Ed25519Private, msg.signableBytes())
	msg.PoWNonce = iskraCrypto.SolvePoW(msg.ID, msg.Timestamp, 16)

	return msg, nil
}

// computeID returns SHA256 of all message fields except ID, Signature, PoWNonce.
func (m *Message) computeID() [32]byte {
	h := sha256.New()
	h.Write([]byte{m.Version})
	h.Write(m.RecipientID[:])
	h.Write(uint32Bytes(m.TTL))
	h.Write(int64Bytes(m.Timestamp))
	h.Write([]byte{m.ContentType})
	h.Write(m.EphemeralPub[:])
	h.Write(m.Nonce[:])
	h.Write(m.Payload)
	h.Write(m.AuthorPub[:])
	var id [32]byte
	copy(id[:], h.Sum(nil))
	return id
}

// signableBytes returns bytes that are signed.
func (m *Message) signableBytes() []byte {
	var data []byte
	data = append(data, m.Version)
	data = append(data, m.ID[:]...)
	data = append(data, m.RecipientID[:]...)
	data = append(data, uint32Bytes(m.TTL)...)
	data = append(data, int64Bytes(m.Timestamp)...)
	data = append(data, m.ContentType)
	data = append(data, m.EphemeralPub[:]...)
	data = append(data, m.Nonce[:]...)
	data = append(data, m.Payload...)
	return data
}

// VerifySignature checks the Ed25519 signature.
func (m *Message) VerifySignature() bool {
	return iskraCrypto.Verify(m.AuthorPub, m.signableBytes(), m.Signature)
}

// VerifyPoW checks the proof-of-work.
func (m *Message) VerifyPoW(difficulty uint8) bool {
	return iskraCrypto.VerifyPoW(m.ID, m.Timestamp, m.PoWNonce, difficulty)
}

// IsForRecipient checks if the message is addressed to the given Ed25519 public key.
func (m *Message) IsForRecipient(edPub [32]byte) bool {
	var recipientID [20]byte
	copy(recipientID[:], edPub[:20])
	return m.RecipientID == recipientID
}

// IsBroadcast checks if the message is a broadcast (RecipientID all zeros).
func (m *Message) IsBroadcast() bool {
	var zero [20]byte
	return m.RecipientID == zero
}
