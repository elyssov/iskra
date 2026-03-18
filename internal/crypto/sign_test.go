package crypto

import (
	"testing"

	"github.com/iskra-messenger/iskra/internal/identity"
)

func TestSign_Verify(t *testing.T) {
	seed := [32]byte{42}
	kp := identity.KeypairFromSeed(seed)
	data := []byte("message to sign")

	sig := Sign(kp.Ed25519Private, data)
	if !Verify(kp.Ed25519Pub, data, sig) {
		t.Fatal("valid signature rejected")
	}
}

func TestSign_WrongKey(t *testing.T) {
	seed1 := [32]byte{1}
	seed2 := [32]byte{2}
	kp1 := identity.KeypairFromSeed(seed1)
	kp2 := identity.KeypairFromSeed(seed2)

	data := []byte("message")
	sig := Sign(kp1.Ed25519Private, data)

	if Verify(kp2.Ed25519Pub, data, sig) {
		t.Fatal("signature verified with wrong key")
	}
}

func TestSign_TamperedData(t *testing.T) {
	seed := [32]byte{99}
	kp := identity.KeypairFromSeed(seed)
	data := []byte("original message")

	sig := Sign(kp.Ed25519Private, data)

	tampered := []byte("tampered message")
	if Verify(kp.Ed25519Pub, tampered, sig) {
		t.Fatal("signature verified with tampered data")
	}
}
