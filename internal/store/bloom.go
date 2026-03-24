package store

import (
	"crypto/sha256"
	"encoding/binary"
	"sync"
)

// SimpleBloom is a minimal bloom filter implementation.
// Using our own to avoid external dependency for alpha.
type SimpleBloom struct {
	bits    []uint64
	numBits uint64
	numHash uint8
	mu      sync.RWMutex
}

// NewBloom creates a bloom filter sized for expectedItems with given false positive rate.
// For 1M items at 0.1% FP rate: ~1.8MB.
func NewBloom(expectedItems uint64, fpRate float64) *SimpleBloom {
	// m = -n * ln(p) / (ln2)^2
	// k = m/n * ln2
	ln2 := 0.693147
	ln2sq := ln2 * ln2
	lnP := -6.907755 // ln(0.001)
	if fpRate > 0 && fpRate < 1 {
		// Use approximation for common rates
		switch {
		case fpRate <= 0.001:
			lnP = -6.907755
		case fpRate <= 0.01:
			lnP = -4.60517
		default:
			lnP = -2.302585
		}
	}

	m := uint64(float64(expectedItems) * (-lnP) / ln2sq)
	k := uint8(float64(m) / float64(expectedItems) * ln2)
	if k < 1 {
		k = 1
	}
	if k > 20 {
		k = 20
	}

	// Round m up to multiple of 64
	m = ((m + 63) / 64) * 64

	return &SimpleBloom{
		bits:    make([]uint64, m/64),
		numBits: m,
		numHash: k,
	}
}

// Add adds a message ID to the bloom filter.
func (b *SimpleBloom) Add(id [32]byte) {
	b.mu.Lock()
	defer b.mu.Unlock()

	for i := uint8(0); i < b.numHash; i++ {
		pos := b.hash(id, i)
		b.bits[pos/64] |= 1 << (pos % 64)
	}
}

// Contains checks if a message ID might be in the filter.
func (b *SimpleBloom) Contains(id [32]byte) bool {
	b.mu.RLock()
	defer b.mu.RUnlock()

	for i := uint8(0); i < b.numHash; i++ {
		pos := b.hash(id, i)
		if b.bits[pos/64]&(1<<(pos%64)) == 0 {
			return false
		}
	}
	return true
}

// hash computes the i-th hash position for the given ID.
// Uses double hashing: h(i) = (h1 + i*h2) mod m
func (b *SimpleBloom) hash(id [32]byte, i uint8) uint64 {
	// h1 from first 8 bytes of SHA256(id || i)
	data := make([]byte, 33)
	copy(data[:32], id[:])
	data[32] = 0
	h := sha256.Sum256(data)
	h1 := binary.BigEndian.Uint64(h[:8])

	data[32] = 1
	h = sha256.Sum256(data)
	h2 := binary.BigEndian.Uint64(h[:8])

	return (h1 + uint64(i)*h2) % b.numBits
}

// CheckInRemote checks if a message ID is in a remote bloom filter (raw bytes from Export).
// Used during SYNC to avoid sending messages the peer already has.
func (b *SimpleBloom) CheckInRemote(remoteData []byte, id [32]byte) bool {
	remoteBits := len(remoteData) * 8
	if remoteBits == 0 {
		return false
	}
	remoteNumBits := uint64(remoteBits)

	for i := uint8(0); i < b.numHash; i++ {
		// Same hash function but mod remote size
		data := make([]byte, 33)
		copy(data[:32], id[:])
		data[32] = 0
		h := sha256.Sum256(data)
		h1 := binary.BigEndian.Uint64(h[:8])

		data[32] = 1
		h = sha256.Sum256(data)
		h2 := binary.BigEndian.Uint64(h[:8])

		pos := (h1 + uint64(i)*h2) % remoteNumBits
		byteIdx := pos / 8
		bitIdx := pos % 8
		if byteIdx >= uint64(len(remoteData)) {
			return false
		}
		if remoteData[byteIdx]&(1<<bitIdx) == 0 {
			return false
		}
	}
	return true
}

// Export returns the raw bit array for sync protocol (HAVE message).
func (b *SimpleBloom) Export() []byte {
	b.mu.RLock()
	defer b.mu.RUnlock()

	out := make([]byte, len(b.bits)*8)
	for i, v := range b.bits {
		binary.LittleEndian.PutUint64(out[i*8:], v)
	}
	return out
}
