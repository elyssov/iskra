package crypto

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"fmt"

	"golang.org/x/crypto/curve25519"
	"golang.org/x/crypto/nacl/secretbox"
)

// EncryptedPayload contains all data needed for decryption.
type EncryptedPayload struct {
	EphemeralPub [32]byte // X25519 ephemeral public key
	Nonce        [24]byte // XSalsa20 nonce
	Ciphertext   []byte   // Obfuscated ciphertext
}

// Encrypt encrypts plaintext for a recipient using ephemeral X25519 key exchange,
// XSalsa20-Poly1305, and obfuscation layer on top.
func Encrypt(senderX25519Priv [32]byte, recipientX25519Pub [32]byte, plaintext []byte) (*EncryptedPayload, error) {
	// Step 1: Generate ephemeral keypair
	var ephPriv [32]byte
	if _, err := rand.Read(ephPriv[:]); err != nil {
		return nil, fmt.Errorf("failed to generate ephemeral key: %w", err)
	}
	ephPub, err := curve25519.X25519(ephPriv[:], curve25519.Basepoint)
	if err != nil {
		return nil, fmt.Errorf("ephemeral pubkey derivation failed: %w", err)
	}

	// Step 2: Key agreement
	sharedSecret, err := curve25519.X25519(ephPriv[:], recipientX25519Pub[:])
	if err != nil {
		return nil, fmt.Errorf("X25519 key agreement failed: %w", err)
	}

	// Derive symmetric key
	symmetricKey := deriveKey(sharedSecret, "iskra-v1-message-key")

	// Step 3: Generate nonce and encrypt
	var nonce [24]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return nil, fmt.Errorf("failed to generate nonce: %w", err)
	}

	ciphertext := secretbox.Seal(nil, plaintext, &nonce, &symmetricKey)

	// Step 4: Obfuscate
	obfStream := generateObfuscationStream(sharedSecret, nonce[:], len(ciphertext))
	obfuscated := xorBytes(ciphertext, obfStream)

	result := &EncryptedPayload{Nonce: nonce, Ciphertext: obfuscated}
	copy(result.EphemeralPub[:], ephPub)

	return result, nil
}

// Decrypt decrypts an EncryptedPayload using the recipient's X25519 private key.
func Decrypt(recipientX25519Priv [32]byte, payload *EncryptedPayload) ([]byte, error) {
	// Key agreement
	sharedSecret, err := curve25519.X25519(recipientX25519Priv[:], payload.EphemeralPub[:])
	if err != nil {
		return nil, fmt.Errorf("X25519 key agreement failed: %w", err)
	}

	// Derive symmetric key
	symmetricKey := deriveKey(sharedSecret, "iskra-v1-message-key")

	// De-obfuscate
	obfStream := generateObfuscationStream(sharedSecret, payload.Nonce[:], len(payload.Ciphertext))
	ciphertext := xorBytes(payload.Ciphertext, obfStream)

	// Decrypt
	plaintext, ok := secretbox.Open(nil, ciphertext, &payload.Nonce, &symmetricKey)
	if !ok {
		return nil, fmt.Errorf("decryption failed: authentication error")
	}

	return plaintext, nil
}

// deriveKey derives a 32-byte symmetric key from shared secret using HMAC-SHA256.
func deriveKey(sharedSecret []byte, info string) [32]byte {
	mac := hmac.New(sha256.New, sharedSecret)
	mac.Write([]byte(info))
	var key [32]byte
	copy(key[:], mac.Sum(nil))
	return key
}

// generateObfuscationStream creates a keystream for obfuscation using repeated HMAC.
func generateObfuscationStream(sharedSecret []byte, nonce []byte, length int) []byte {
	stream := make([]byte, 0, length)
	info := append(nonce, []byte("iskra-obf")...)

	counter := byte(0)
	for len(stream) < length {
		mac := hmac.New(sha256.New, sharedSecret)
		mac.Write(info)
		mac.Write([]byte{counter})
		stream = append(stream, mac.Sum(nil)...)
		counter++
	}

	return stream[:length]
}

// xorBytes XORs two byte slices of the same length.
func xorBytes(a, b []byte) []byte {
	result := make([]byte, len(a))
	for i := range a {
		result[i] = a[i] ^ b[i]
	}
	return result
}
