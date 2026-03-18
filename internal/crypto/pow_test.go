package crypto

import (
	"testing"
	"time"
)

func TestPoW_SolveAndVerify(t *testing.T) {
	messageID := [32]byte{1, 2, 3}
	timestamp := time.Now().Unix()
	difficulty := uint8(8) // 8 leading zero bits — fast

	nonce := SolvePoW(messageID, timestamp, difficulty)

	if !VerifyPoW(messageID, timestamp, nonce, difficulty) {
		t.Fatal("PoW solution failed verification")
	}
}

func TestPoW_WrongNonce(t *testing.T) {
	messageID := [32]byte{1, 2, 3}
	timestamp := time.Now().Unix()
	difficulty := uint8(16)

	nonce := SolvePoW(messageID, timestamp, difficulty)

	// Wrong nonce should fail
	if VerifyPoW(messageID, timestamp, nonce+1, difficulty) {
		t.Fatal("wrong nonce passed verification")
	}
}

func TestPoW_Difficulty16(t *testing.T) {
	messageID := [32]byte{10, 20, 30}
	timestamp := time.Now().Unix()
	difficulty := uint8(16) // ~65536 attempts, should be < 1 sec

	start := time.Now()
	nonce := SolvePoW(messageID, timestamp, difficulty)
	elapsed := time.Since(start)

	if !VerifyPoW(messageID, timestamp, nonce, difficulty) {
		t.Fatal("PoW solution failed verification")
	}

	t.Logf("PoW difficulty=%d solved in %v, nonce=%d", difficulty, elapsed, nonce)
}
