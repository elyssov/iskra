package security

import (
	"crypto/rand"
	"crypto/subtle"
	"os"
	"path/filepath"

	"golang.org/x/crypto/argon2"
)

const (
	pinFile      = "pin.dat"
	attemptsFile = "attempts.dat"
	saltSize     = 16
	hashSize     = 32
	// Argon2id parameters — balance of security and mobile performance
	argonTime    = 3
	argonMemory  = 64 * 1024 // 64 MB
	argonThreads = 2
	MaxAttempts  = 5
)

// HasPIN returns true if a PIN has been set.
func HasPIN(dataDir string) bool {
	info, err := os.Stat(filepath.Join(dataDir, pinFile))
	return err == nil && info.Size() == saltSize+hashSize
}

// SetPIN creates a new PIN hash and saves it.
func SetPIN(dataDir string, pin string) error {
	salt := make([]byte, saltSize)
	if _, err := rand.Read(salt); err != nil {
		return err
	}
	hash := argon2.IDKey([]byte(pin), salt, argonTime, argonMemory, argonThreads, hashSize)

	data := make([]byte, saltSize+hashSize)
	copy(data[:saltSize], salt)
	copy(data[saltSize:], hash)

	ResetAttempts(dataDir)
	return os.WriteFile(filepath.Join(dataDir, pinFile), data, 0600)
}

// VerifyPIN checks if the given PIN matches the stored hash.
func VerifyPIN(dataDir string, pin string) bool {
	data, err := os.ReadFile(filepath.Join(dataDir, pinFile))
	if err != nil || len(data) != saltSize+hashSize {
		return false
	}
	salt := data[:saltSize]
	storedHash := data[saltSize:]
	hash := argon2.IDKey([]byte(pin), salt, argonTime, argonMemory, argonThreads, hashSize)
	return subtle.ConstantTimeCompare(hash, storedHash) == 1
}

// GetAttempts returns the current failed attempt count.
func GetAttempts(dataDir string) int {
	data, err := os.ReadFile(filepath.Join(dataDir, attemptsFile))
	if err != nil || len(data) == 0 {
		return 0
	}
	return int(data[0])
}

// IncrementAttempts adds one failed attempt and returns the new count.
func IncrementAttempts(dataDir string) int {
	count := GetAttempts(dataDir) + 1
	os.WriteFile(filepath.Join(dataDir, attemptsFile), []byte{byte(count)}, 0600)
	return count
}

// ResetAttempts clears the failed attempt counter.
func ResetAttempts(dataDir string) {
	os.Remove(filepath.Join(dataDir, attemptsFile))
}
