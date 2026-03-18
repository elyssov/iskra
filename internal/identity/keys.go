package identity

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha512"
	"fmt"

	"golang.org/x/crypto/curve25519"
)

// Keypair holds both Ed25519 (signing) and X25519 (encryption) key pairs.
type Keypair struct {
	Seed           [32]byte
	Ed25519Pub     [32]byte
	Ed25519Private ed25519.PrivateKey // 64 bytes
	X25519Pub      [32]byte
	X25519Private  [32]byte
}

// GenerateSeed creates 32 cryptographically random bytes.
func GenerateSeed() ([32]byte, error) {
	var seed [32]byte
	_, err := rand.Read(seed[:])
	if err != nil {
		return seed, fmt.Errorf("failed to generate random seed: %w", err)
	}
	return seed, nil
}

// KeypairFromSeed derives Ed25519 and X25519 keypairs from a 32-byte seed.
func KeypairFromSeed(seed [32]byte) *Keypair {
	kp := &Keypair{Seed: seed}

	// Ed25519 from seed
	privKey := ed25519.NewKeyFromSeed(seed[:])
	kp.Ed25519Private = privKey
	copy(kp.Ed25519Pub[:], privKey.Public().(ed25519.PublicKey))

	// X25519 from seed via SHA-512 + clamping (RFC 7748)
	h := sha512.Sum512(seed[:])
	h[0] &= 248
	h[31] &= 127
	h[31] |= 64
	copy(kp.X25519Private[:], h[:32])

	// X25519 public key via curve25519.ScalarBaseMult
	pub, err := curve25519.X25519(kp.X25519Private[:], curve25519.Basepoint)
	if err != nil {
		panic("curve25519 scalar base mult failed: " + err.Error())
	}
	copy(kp.X25519Pub[:], pub)

	return kp
}

// UserID returns the first 20 characters of base58(Ed25519 public key).
func UserID(pub [32]byte) string {
	encoded := ToBase58(pub[:])
	if len(encoded) < 20 {
		for len(encoded) < 20 {
			encoded = "1" + encoded
		}
	}
	return encoded[:20]
}
