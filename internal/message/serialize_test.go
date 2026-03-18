package message

import (
	"testing"

	"github.com/iskra-messenger/iskra/internal/identity"
)

func makeTestKeypairs() (*identity.Keypair, *identity.Keypair) {
	seed1 := [32]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16,
		17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32}
	seed2 := [32]byte{32, 31, 30, 29, 28, 27, 26, 25, 24, 23, 22, 21, 20, 19, 18, 17,
		16, 15, 14, 13, 12, 11, 10, 9, 8, 7, 6, 5, 4, 3, 2, 1}
	return identity.KeypairFromSeed(seed1), identity.KeypairFromSeed(seed2)
}

func TestSerialize_Roundtrip(t *testing.T) {
	author, recipient := makeTestKeypairs()
	rk := RecipientKeys{Ed25519Pub: recipient.Ed25519Pub, X25519Pub: recipient.X25519Pub}

	msg, err := New(author, rk, "Привет из Искры!")
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	raw := msg.Serialize()
	msg2, err := Deserialize(raw)
	if err != nil {
		t.Fatalf("Deserialize failed: %v", err)
	}

	if msg.ID != msg2.ID {
		t.Fatal("ID mismatch")
	}
	if msg.Version != msg2.Version {
		t.Fatal("Version mismatch")
	}
	if msg.RecipientID != msg2.RecipientID {
		t.Fatal("RecipientID mismatch")
	}
	if msg.TTL != msg2.TTL {
		t.Fatal("TTL mismatch")
	}
	if msg.Timestamp != msg2.Timestamp {
		t.Fatal("Timestamp mismatch")
	}
	if msg.ContentType != msg2.ContentType {
		t.Fatal("ContentType mismatch")
	}
	if msg.EphemeralPub != msg2.EphemeralPub {
		t.Fatal("EphemeralPub mismatch")
	}
	if msg.Nonce != msg2.Nonce {
		t.Fatal("Nonce mismatch")
	}
	if len(msg.Payload) != len(msg2.Payload) {
		t.Fatalf("Payload length mismatch: %d vs %d", len(msg.Payload), len(msg2.Payload))
	}
	for i := range msg.Payload {
		if msg.Payload[i] != msg2.Payload[i] {
			t.Fatalf("Payload mismatch at byte %d", i)
		}
	}
	if msg.AuthorPub != msg2.AuthorPub {
		t.Fatal("AuthorPub mismatch")
	}
	if msg.Signature != msg2.Signature {
		t.Fatal("Signature mismatch")
	}
	if msg.PoWNonce != msg2.PoWNonce {
		t.Fatal("PoWNonce mismatch")
	}
}

func TestSerialize_SignatureValid(t *testing.T) {
	author, recipient := makeTestKeypairs()
	rk := RecipientKeys{Ed25519Pub: recipient.Ed25519Pub, X25519Pub: recipient.X25519Pub}

	msg, _ := New(author, rk, "test signature")
	raw := msg.Serialize()
	msg2, _ := Deserialize(raw)

	if !msg2.VerifySignature() {
		t.Fatal("signature invalid after deserialization")
	}
}

func TestSerialize_PoWValid(t *testing.T) {
	author, recipient := makeTestKeypairs()
	rk := RecipientKeys{Ed25519Pub: recipient.Ed25519Pub, X25519Pub: recipient.X25519Pub}

	msg, _ := New(author, rk, "test pow")
	raw := msg.Serialize()
	msg2, _ := Deserialize(raw)

	if !msg2.VerifyPoW(16) {
		t.Fatal("PoW invalid after deserialization")
	}
}

func TestSerialize_CorruptData(t *testing.T) {
	_, err := Deserialize([]byte{1, 2, 3})
	if err == nil {
		t.Fatal("expected error for short data")
	}
}

func TestSerialize_IDConsistency(t *testing.T) {
	author, recipient := makeTestKeypairs()
	rk := RecipientKeys{Ed25519Pub: recipient.Ed25519Pub, X25519Pub: recipient.X25519Pub}

	msg, _ := New(author, rk, "id test")
	expectedID := msg.computeID()
	if msg.ID != expectedID {
		t.Fatal("ID doesn't match computed ID")
	}
}

func TestMessage_IsForRecipient(t *testing.T) {
	author, recipient := makeTestKeypairs()
	rk := RecipientKeys{Ed25519Pub: recipient.Ed25519Pub, X25519Pub: recipient.X25519Pub}

	msg, _ := New(author, rk, "for you")

	if !msg.IsForRecipient(recipient.Ed25519Pub) {
		t.Fatal("message should be for recipient")
	}
	if msg.IsForRecipient(author.Ed25519Pub) {
		t.Fatal("message should not be for author")
	}
}

func TestMessage_IsBroadcast(t *testing.T) {
	author, recipient := makeTestKeypairs()
	rk := RecipientKeys{Ed25519Pub: recipient.Ed25519Pub, X25519Pub: recipient.X25519Pub}

	msg, _ := New(author, rk, "not broadcast")
	if msg.IsBroadcast() {
		t.Fatal("regular message marked as broadcast")
	}
}
