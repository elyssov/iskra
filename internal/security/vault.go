package security

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"os"

	"golang.org/x/crypto/nacl/secretbox"
)

// DeriveStorageKey derives a 32-byte storage encryption key from seed and PIN.
// Uses HMAC-SHA256(seed, "iskra-storage-v1:" + PIN).
func DeriveStorageKey(seed [32]byte, pin string) [32]byte {
	mac := hmac.New(sha256.New, seed[:])
	mac.Write([]byte("iskra-storage-v1:" + pin))
	var key [32]byte
	copy(key[:], mac.Sum(nil))
	return key
}

// EncryptData encrypts data using XSalsa20-Poly1305.
// Returns: 24-byte nonce + ciphertext.
func EncryptData(data []byte, key *[32]byte) ([]byte, error) {
	var nonce [24]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return nil, fmt.Errorf("generate nonce: %w", err)
	}
	sealed := secretbox.Seal(nonce[:], data, &nonce, key)
	return sealed, nil
}

// DecryptData decrypts data encrypted with EncryptData.
// Input: 24-byte nonce + ciphertext.
func DecryptData(encrypted []byte, key *[32]byte) ([]byte, error) {
	if len(encrypted) < 24+secretbox.Overhead {
		return nil, fmt.Errorf("data too short")
	}
	var nonce [24]byte
	copy(nonce[:], encrypted[:24])
	plaintext, ok := secretbox.Open(nil, encrypted[24:], &nonce, key)
	if !ok {
		return nil, fmt.Errorf("decryption failed: wrong key or corrupted data")
	}
	return plaintext, nil
}

// EncryptFile encrypts data and writes to file.
func EncryptFile(path string, data []byte, key *[32]byte) error {
	encrypted, err := EncryptData(data, key)
	if err != nil {
		return err
	}
	return os.WriteFile(path, encrypted, 0600)
}

// DecryptFile reads an encrypted file and returns plaintext.
func DecryptFile(path string, key *[32]byte) ([]byte, error) {
	encrypted, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return DecryptData(encrypted, key)
}
