package identity

import (
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"strings"

	"github.com/iskra-messenger/iskra/wordlist"
)

// SeedToMnemonic encodes a 32-byte seed into 24 Russian words.
// First 24 bytes (192 bits) → 24 words (8 bits per word).
// Last 8 bytes used as checksum (stored in the remaining seed bytes).
// Encoding: first 24 bytes of seed map directly to word indices.
// Checksum: SHA256(seed[:24])[:8] must equal seed[24:32].
func SeedToMnemonic(seed [32]byte) []string {
	words := make([]string, 24)
	for i := 0; i < 24; i++ {
		words[i] = wordlist.RussianWordlist[seed[i]]
	}
	return words
}

// MnemonicToSeed reconstructs a 32-byte seed from 24 Russian words.
// The first 24 bytes come from word indices, last 8 from SHA256 checksum.
func MnemonicToSeed(words []string) ([32]byte, error) {
	var seed [32]byte

	if len(words) != 24 {
		return seed, fmt.Errorf("expected 24 words, got %d", len(words))
	}

	for i, word := range words {
		idx, err := wordIndex(word)
		if err != nil {
			return seed, fmt.Errorf("word %d: %w", i+1, err)
		}
		seed[i] = byte(idx)
	}

	// Generate checksum: SHA256 of first 24 bytes → last 8 bytes
	checksum := sha256.Sum256(seed[:24])
	copy(seed[24:], checksum[:8])

	return seed, nil
}

// GenerateMnemonicSeed creates a seed that is compatible with mnemonic encoding.
// First 24 bytes are random, last 8 are SHA256(first24)[:8].
func GenerateMnemonicSeed() ([32]byte, error) {
	var seed [32]byte
	_, err := rand.Read(seed[:24])
	if err != nil {
		return seed, fmt.Errorf("failed to generate random bytes: %w", err)
	}
	checksum := sha256.Sum256(seed[:24])
	copy(seed[24:], checksum[:8])
	return seed, nil
}

// ValidateMnemonic checks if the mnemonic words form a valid seed.
func ValidateMnemonic(words []string) bool {
	seed, err := MnemonicToSeed(words)
	if err != nil {
		return false
	}
	// Verify checksum
	checksum := sha256.Sum256(seed[:24])
	for i := 0; i < 8; i++ {
		if seed[24+i] != checksum[i] {
			return false
		}
	}
	return true
}

func wordIndex(word string) (int, error) {
	word = strings.TrimSpace(word)
	for i, w := range wordlist.RussianWordlist {
		if w == word {
			return i, nil
		}
	}
	return -1, fmt.Errorf("unknown word: %q", word)
}
