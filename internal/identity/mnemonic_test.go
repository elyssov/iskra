package identity

import (
	"testing"
)

func TestMnemonic_Roundtrip(t *testing.T) {
	seed, err := GenerateMnemonicSeed()
	if err != nil {
		t.Fatalf("GenerateMnemonicSeed failed: %v", err)
	}

	words := SeedToMnemonic(seed)
	if len(words) != 24 {
		t.Fatalf("expected 24 words, got %d", len(words))
	}

	recovered, err := MnemonicToSeed(words)
	if err != nil {
		t.Fatalf("MnemonicToSeed failed: %v", err)
	}

	if seed != recovered {
		t.Fatal("mnemonic roundtrip failed: seeds don't match")
	}
}

func TestMnemonic_24Words(t *testing.T) {
	seed, _ := GenerateMnemonicSeed()
	words := SeedToMnemonic(seed)
	if len(words) != 24 {
		t.Fatalf("expected 24 words, got %d", len(words))
	}
	for i, w := range words {
		if w == "" {
			t.Fatalf("word %d is empty", i)
		}
	}
}

func TestMnemonic_ValidateGood(t *testing.T) {
	seed, _ := GenerateMnemonicSeed()
	words := SeedToMnemonic(seed)
	if !ValidateMnemonic(words) {
		t.Fatal("valid mnemonic rejected")
	}
}

func TestMnemonic_InvalidWord(t *testing.T) {
	words := make([]string, 24)
	for i := range words {
		words[i] = "берёза"
	}
	words[5] = "несуществующее"

	_, err := MnemonicToSeed(words)
	if err == nil {
		t.Fatal("expected error for invalid word")
	}
}

func TestMnemonic_WrongCount(t *testing.T) {
	_, err := MnemonicToSeed([]string{"берёза", "молоко"})
	if err == nil {
		t.Fatal("expected error for wrong word count")
	}
}

func TestMnemonic_DifferentSeeds(t *testing.T) {
	seed1, _ := GenerateMnemonicSeed()
	seed2, _ := GenerateMnemonicSeed()
	words1 := SeedToMnemonic(seed1)
	words2 := SeedToMnemonic(seed2)

	same := true
	for i := range words1 {
		if words1[i] != words2[i] {
			same = false
			break
		}
	}
	if same {
		t.Fatal("two different seeds produced identical mnemonics")
	}
}

func TestMnemonic_KeypairFromMnemonicSeed(t *testing.T) {
	seed, _ := GenerateMnemonicSeed()
	kp1 := KeypairFromSeed(seed)

	words := SeedToMnemonic(seed)
	recovered, _ := MnemonicToSeed(words)
	kp2 := KeypairFromSeed(recovered)

	if kp1.Ed25519Pub != kp2.Ed25519Pub {
		t.Fatal("keypair from original seed != keypair from recovered seed")
	}
}
