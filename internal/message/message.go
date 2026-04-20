package message

import (
	"bytes"
	"compress/flate"
	"crypto/sha256"
	"io"
	"time"

	iskraCrypto "github.com/iskra-messenger/iskra/internal/crypto"
	"github.com/iskra-messenger/iskra/internal/identity"
)

// Compression prefix — first byte of plaintext signals if it's compressed.
// 0x00 = uncompressed, 0x01 = deflate compressed.
const (
	compNone    byte = 0x00
	compDeflate byte = 0x01
)

// compressPayload tries to deflate the plaintext. Returns compressed data with prefix.
// Only compresses text-like content types. If compression doesn't help, returns original.
func compressPayload(plaintext []byte, contentType uint8) []byte {
	// Only compress text, letters, group text — not file chunks or binary
	if contentType != ContentText && contentType != ContentGroupText &&
		contentType != ContentLetter && contentType != ContentChannelPost {
		// Prepend "uncompressed" marker
		return append([]byte{compNone}, plaintext...)
	}

	// Try deflate
	var buf bytes.Buffer
	w, err := flate.NewWriter(&buf, flate.BestCompression)
	if err != nil {
		return append([]byte{compNone}, plaintext...)
	}
	w.Write(plaintext)
	w.Close()

	compressed := buf.Bytes()
	// Only use compressed if it actually saves space
	if len(compressed) < len(plaintext) {
		return append([]byte{compDeflate}, compressed...)
	}
	return append([]byte{compNone}, plaintext...)
}

// DecompressPayload decompresses payload based on prefix byte.
// Handles both new (compressed) and legacy (uncompressed) messages.
func DecompressPayload(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return data, nil
	}
	switch data[0] {
	case compDeflate:
		r := flate.NewReader(bytes.NewReader(data[1:]))
		defer r.Close()
		return io.ReadAll(r)
	case compNone:
		return data[1:], nil
	default:
		// No prefix — legacy message (pre-compression), return as-is
		return data, nil
	}
}

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
	AuthorPub    [32]byte // Ed25519 pubkey of author
	AuthorX25519 [32]byte // X25519 pubkey of author (for reply without /online)
	Signature    [64]byte

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
		TTL:         TTLForContentType(contentType),
		Timestamp:   time.Now().Unix(),
		ContentType: contentType,
		AuthorPub:    author.Ed25519Pub,
		AuthorX25519: author.X25519Pub,
	}

	// RecipientID = first 20 bytes of Ed25519 pubkey
	copy(msg.RecipientID[:], recipient.Ed25519Pub[:20])

	// Compress before encryption (encrypted data doesn't compress)
	compressedPayload := compressPayload(plaintext, contentType)

	// Encrypt with X25519 keys
	encrypted, err := iskraCrypto.Encrypt(author.X25519Private, recipient.X25519Pub, compressedPayload)
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
		TTL:         TTLForContentType(contentType),
		Timestamp:   time.Now().Unix(),
		ContentType: contentType,
		AuthorPub:    author.Ed25519Pub,
		AuthorX25519: author.X25519Pub,
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

// NewPlainBroadcast creates a signed but unencrypted broadcast message.
// Anyone can read the payload, but the signature proves authorship.
func NewPlainBroadcast(author *identity.Keypair, contentType uint8, plaintext []byte) (*Message, error) {
	msg := &Message{
		Version:     ProtocolVersion,
		TTL:         TTLForContentType(contentType),
		Timestamp:   time.Now().Unix(),
		ContentType: contentType,
		AuthorPub:    author.Ed25519Pub,
		AuthorX25519: author.X25519Pub,
		Payload:     plaintext, // Not encrypted — readable by all
	}
	// RecipientID stays all zeros for broadcast

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
