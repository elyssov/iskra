package mesh

import (
	"bytes"
	"testing"
)

func TestObfuscateDeobfuscate(t *testing.T) {
	data := []byte("test message payload with recipient id and all that stuff here")

	obfuscated := obfuscate(data)

	// Obfuscated should be longer (nonce prefix)
	if len(obfuscated) != nonceSize+len(data) {
		t.Fatalf("expected length %d, got %d", nonceSize+len(data), len(obfuscated))
	}

	// Obfuscated should NOT contain the original data
	if bytes.Contains(obfuscated, data) {
		t.Fatal("obfuscated data should not contain plaintext")
	}

	// Deobfuscate should recover original
	recovered, err := deobfuscate(obfuscated)
	if err != nil {
		t.Fatalf("deobfuscate: %v", err)
	}
	if !bytes.Equal(recovered, data) {
		t.Fatal("recovered data doesn't match original")
	}
}

func TestObfuscateUniqueness(t *testing.T) {
	data := []byte("same payload every time")

	obf1 := obfuscate(data)
	obf2 := obfuscate(data)

	// Each obfuscation should produce different output (different nonce)
	if bytes.Equal(obf1, obf2) {
		t.Fatal("two obfuscations of same data should differ (different nonces)")
	}

	// But both should deobfuscate to the same thing
	rec1, _ := deobfuscate(obf1)
	rec2, _ := deobfuscate(obf2)
	if !bytes.Equal(rec1, rec2) {
		t.Fatal("both should deobfuscate to same data")
	}
}

func TestObfuscateLooksRandom(t *testing.T) {
	// A real message-like payload
	data := make([]byte, 300)
	for i := range data {
		data[i] = byte(i % 256)
	}

	obfuscated := obfuscate(data)

	// Check that obfuscated data has reasonable entropy (no long runs of same byte)
	maxRun := 0
	currentRun := 1
	for i := 1; i < len(obfuscated); i++ {
		if obfuscated[i] == obfuscated[i-1] {
			currentRun++
			if currentRun > maxRun {
				maxRun = currentRun
			}
		} else {
			currentRun = 1
		}
	}

	// Random data shouldn't have runs longer than ~8 in 300 bytes
	if maxRun > 12 {
		t.Fatalf("suspicious pattern: max run of %d identical bytes", maxRun)
	}
}

func TestDeobfuscateShortPacket(t *testing.T) {
	_, err := deobfuscate([]byte{1, 2, 3})
	if err == nil {
		t.Fatal("should error on short packet")
	}
}
