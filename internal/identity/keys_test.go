package identity

import (
	"testing"
)

func TestGenerateSeed(t *testing.T) {
	seed1, err := GenerateSeed()
	if err != nil {
		t.Fatalf("GenerateSeed failed: %v", err)
	}
	seed2, err := GenerateSeed()
	if err != nil {
		t.Fatalf("GenerateSeed failed: %v", err)
	}
	if seed1 == seed2 {
		t.Fatal("two GenerateSeed calls returned identical seeds")
	}
}

func TestKeypairFromSeed_Deterministic(t *testing.T) {
	seed := [32]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16,
		17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32}

	kp1 := KeypairFromSeed(seed)
	kp2 := KeypairFromSeed(seed)

	if kp1.Ed25519Pub != kp2.Ed25519Pub {
		t.Fatal("same seed produced different Ed25519 public keys")
	}
	if kp1.X25519Pub != kp2.X25519Pub {
		t.Fatal("same seed produced different X25519 public keys")
	}
}

func TestKeypairFromSeed_DifferentSeeds(t *testing.T) {
	seed1 := [32]byte{1}
	seed2 := [32]byte{2}

	kp1 := KeypairFromSeed(seed1)
	kp2 := KeypairFromSeed(seed2)

	if kp1.Ed25519Pub == kp2.Ed25519Pub {
		t.Fatal("different seeds produced same Ed25519 public keys")
	}
}

func TestUserID_Length20(t *testing.T) {
	seed := [32]byte{42, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15,
		16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31}
	kp := KeypairFromSeed(seed)
	uid := UserID(kp.Ed25519Pub)
	if len(uid) != 20 {
		t.Fatalf("UserID length = %d, want 20", len(uid))
	}
}

func TestUserID_Deterministic(t *testing.T) {
	seed := [32]byte{99}
	kp := KeypairFromSeed(seed)
	uid1 := UserID(kp.Ed25519Pub)
	uid2 := UserID(kp.Ed25519Pub)
	if uid1 != uid2 {
		t.Fatal("UserID not deterministic")
	}
}

func TestBase58_Roundtrip(t *testing.T) {
	data := []byte{0, 0, 1, 2, 3, 255, 128, 64}
	encoded := ToBase58(data)
	decoded, err := FromBase58(encoded)
	if err != nil {
		t.Fatalf("FromBase58 failed: %v", err)
	}
	if len(data) != len(decoded) {
		t.Fatalf("roundtrip length mismatch: %d vs %d", len(data), len(decoded))
	}
	for i := range data {
		if data[i] != decoded[i] {
			t.Fatalf("roundtrip mismatch at byte %d: %d vs %d", i, data[i], decoded[i])
		}
	}
}

func TestBase58_PubkeyRoundtrip(t *testing.T) {
	seed := [32]byte{7, 8, 9}
	kp := KeypairFromSeed(seed)
	encoded := ToBase58(kp.Ed25519Pub[:])
	decoded, err := FromBase58(encoded)
	if err != nil {
		t.Fatalf("FromBase58 failed: %v", err)
	}
	var pub [32]byte
	copy(pub[:], decoded)
	if pub != kp.Ed25519Pub {
		t.Fatal("base58 roundtrip of pubkey failed")
	}
}
