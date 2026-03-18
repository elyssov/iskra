package crypto

import (
	"crypto/ed25519"
)

// Sign creates an Ed25519 signature of data.
func Sign(privateKey ed25519.PrivateKey, data []byte) [64]byte {
	sig := ed25519.Sign(privateKey, data)
	var result [64]byte
	copy(result[:], sig)
	return result
}

// Verify checks an Ed25519 signature.
func Verify(publicKey [32]byte, data []byte, signature [64]byte) bool {
	return ed25519.Verify(publicKey[:], data, signature[:])
}
