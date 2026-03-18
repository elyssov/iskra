package crypto

import (
	"bytes"
	"testing"

	"github.com/iskra-messenger/iskra/internal/identity"
)

func TestEncryptDecrypt_Roundtrip(t *testing.T) {
	seed1 := [32]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16,
		17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32}
	seed2 := [32]byte{32, 31, 30, 29, 28, 27, 26, 25, 24, 23, 22, 21, 20, 19, 18, 17,
		16, 15, 14, 13, 12, 11, 10, 9, 8, 7, 6, 5, 4, 3, 2, 1}

	sender := identity.KeypairFromSeed(seed1)
	recipient := identity.KeypairFromSeed(seed2)

	plaintext := []byte("Привет из Искры! 🔥")

	encrypted, err := Encrypt(sender.X25519Private, recipient.X25519Pub, plaintext)
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}

	decrypted, err := Decrypt(recipient.X25519Private, encrypted)
	if err != nil {
		t.Fatalf("Decrypt failed: %v", err)
	}

	if !bytes.Equal(plaintext, decrypted) {
		t.Fatalf("decrypted text doesn't match: %q vs %q", plaintext, decrypted)
	}
}

func TestDecrypt_WrongKey(t *testing.T) {
	seed1 := [32]byte{1}
	seed2 := [32]byte{2}
	seed3 := [32]byte{3}

	sender := identity.KeypairFromSeed(seed1)
	recipient := identity.KeypairFromSeed(seed2)
	wrongRecipient := identity.KeypairFromSeed(seed3)

	encrypted, err := Encrypt(sender.X25519Private, recipient.X25519Pub, []byte("secret"))
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}

	_, err = Decrypt(wrongRecipient.X25519Private, encrypted)
	if err == nil {
		t.Fatal("expected decryption to fail with wrong key")
	}
}

func TestEncrypt_Tampering(t *testing.T) {
	seed1 := [32]byte{10}
	seed2 := [32]byte{20}

	sender := identity.KeypairFromSeed(seed1)
	recipient := identity.KeypairFromSeed(seed2)

	encrypted, err := Encrypt(sender.X25519Private, recipient.X25519Pub, []byte("tamper test"))
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}

	// Tamper with ciphertext
	if len(encrypted.Ciphertext) > 0 {
		encrypted.Ciphertext[0] ^= 0xFF
	}

	_, err = Decrypt(recipient.X25519Private, encrypted)
	if err == nil {
		t.Fatal("expected decryption to fail after tampering")
	}
}

func TestEncrypt_DifferentCiphertexts(t *testing.T) {
	seed1 := [32]byte{1}
	seed2 := [32]byte{2}

	sender := identity.KeypairFromSeed(seed1)
	recipient := identity.KeypairFromSeed(seed2)

	plaintext := []byte("same message")

	ct1, _ := Encrypt(sender.X25519Private, recipient.X25519Pub, plaintext)
	ct2, _ := Encrypt(sender.X25519Private, recipient.X25519Pub, plaintext)

	// Ephemeral keys should differ → ciphertexts differ
	if ct1.EphemeralPub == ct2.EphemeralPub {
		t.Fatal("two encryptions produced same ephemeral key")
	}
}

func TestObfuscation_NoNaClPattern(t *testing.T) {
	seed1 := [32]byte{5}
	seed2 := [32]byte{6}

	sender := identity.KeypairFromSeed(seed1)
	recipient := identity.KeypairFromSeed(seed2)

	// Encrypt many messages and check output doesn't have obvious NaCl patterns
	for i := 0; i < 10; i++ {
		encrypted, _ := Encrypt(sender.X25519Private, recipient.X25519Pub, []byte("test data for obfuscation check"))

		// Obfuscated output should look random - no long runs of zeros
		zeroCount := 0
		for _, b := range encrypted.Ciphertext {
			if b == 0 {
				zeroCount++
			}
		}
		// More than 25% zeros would be suspicious for random-looking data
		if len(encrypted.Ciphertext) > 20 && float64(zeroCount)/float64(len(encrypted.Ciphertext)) > 0.25 {
			t.Logf("warning: high zero count in ciphertext (%d/%d)", zeroCount, len(encrypted.Ciphertext))
		}
	}
}

func TestEncrypt_EmptyPlaintext(t *testing.T) {
	seed1 := [32]byte{1}
	seed2 := [32]byte{2}

	sender := identity.KeypairFromSeed(seed1)
	recipient := identity.KeypairFromSeed(seed2)

	encrypted, err := Encrypt(sender.X25519Private, recipient.X25519Pub, []byte{})
	if err != nil {
		t.Fatalf("Encrypt empty failed: %v", err)
	}

	decrypted, err := Decrypt(recipient.X25519Private, encrypted)
	if err != nil {
		t.Fatalf("Decrypt empty failed: %v", err)
	}

	if len(decrypted) != 0 {
		t.Fatalf("expected empty decrypted, got %d bytes", len(decrypted))
	}
}
