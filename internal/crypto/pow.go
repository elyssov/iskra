package crypto

import (
	"crypto/sha256"
	"encoding/binary"
)

// SolvePoW finds a nonce such that SHA256(messageID || timestamp || nonce) has
// at least `difficulty` leading zero bits.
func SolvePoW(messageID [32]byte, timestamp int64, difficulty uint8) uint64 {
	var buf [48]byte // 32 + 8 + 8
	copy(buf[:32], messageID[:])
	binary.BigEndian.PutUint64(buf[32:40], uint64(timestamp))

	for nonce := uint64(0); ; nonce++ {
		binary.BigEndian.PutUint64(buf[40:48], nonce)
		hash := sha256.Sum256(buf[:])
		if hasLeadingZeroBits(hash[:], difficulty) {
			return nonce
		}
	}
}

// VerifyPoW checks that the given nonce satisfies the PoW difficulty.
func VerifyPoW(messageID [32]byte, timestamp int64, nonce uint64, difficulty uint8) bool {
	var buf [48]byte
	copy(buf[:32], messageID[:])
	binary.BigEndian.PutUint64(buf[32:40], uint64(timestamp))
	binary.BigEndian.PutUint64(buf[40:48], nonce)
	hash := sha256.Sum256(buf[:])
	return hasLeadingZeroBits(hash[:], difficulty)
}

// hasLeadingZeroBits checks if hash has at least n leading zero bits.
func hasLeadingZeroBits(hash []byte, n uint8) bool {
	fullBytes := n / 8
	remainBits := n % 8

	for i := uint8(0); i < fullBytes; i++ {
		if hash[i] != 0 {
			return false
		}
	}
	if remainBits > 0 {
		mask := byte(0xFF) << (8 - remainBits)
		if hash[fullBytes]&mask != 0 {
			return false
		}
	}
	return true
}
