package store

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/iskra-messenger/iskra/internal/identity"
	"github.com/iskra-messenger/iskra/internal/message"
)

func makeTestMsg(t *testing.T) *message.Message {
	t.Helper()
	seed1 := [32]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16,
		17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32}
	seed2 := [32]byte{32, 31, 30, 29, 28, 27, 26, 25, 24, 23, 22, 21, 20, 19, 18, 17,
		16, 15, 14, 13, 12, 11, 10, 9, 8, 7, 6, 5, 4, 3, 2, 1}
	author := identity.KeypairFromSeed(seed1)
	recipient := identity.KeypairFromSeed(seed2)
	rk := message.RecipientKeys{Ed25519Pub: recipient.Ed25519Pub, X25519Pub: recipient.X25519Pub}
	msg, err := message.New(author, rk, "test message")
	if err != nil {
		t.Fatalf("failed to create message: %v", err)
	}
	return msg
}

func TestHold_StoreAndGetAll(t *testing.T) {
	dir := filepath.Join(os.TempDir(), "iskra-test-hold-1")
	defer os.RemoveAll(dir)

	hold, err := NewHold(dir)
	if err != nil {
		t.Fatalf("NewHold failed: %v", err)
	}

	msg := makeTestMsg(t)
	if err := hold.Store(msg); err != nil {
		t.Fatalf("Store failed: %v", err)
	}

	msgs, err := hold.GetAll()
	if err != nil {
		t.Fatalf("GetAll failed: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	if msgs[0].ID != msg.ID {
		t.Fatal("retrieved message ID doesn't match")
	}
}

func TestHold_Delete(t *testing.T) {
	dir := filepath.Join(os.TempDir(), "iskra-test-hold-2")
	defer os.RemoveAll(dir)

	hold, _ := NewHold(dir)
	msg := makeTestMsg(t)
	hold.Store(msg)

	if err := hold.Delete(msg.ID); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	msgs, _ := hold.GetAll()
	if len(msgs) != 0 {
		t.Fatalf("expected 0 messages after delete, got %d", len(msgs))
	}
}

func TestHold_Has(t *testing.T) {
	dir := filepath.Join(os.TempDir(), "iskra-test-hold-3")
	defer os.RemoveAll(dir)

	hold, _ := NewHold(dir)
	msg := makeTestMsg(t)
	hold.Store(msg)

	if !hold.Has(msg.ID) {
		t.Fatal("Has returned false for stored message")
	}

	var unknownID [32]byte
	if hold.Has(unknownID) {
		t.Fatal("Has returned true for unknown message")
	}
}

func TestHold_Count(t *testing.T) {
	dir := filepath.Join(os.TempDir(), "iskra-test-hold-4")
	defer os.RemoveAll(dir)

	hold, _ := NewHold(dir)
	if hold.Count() != 0 {
		t.Fatal("empty hold should have count 0")
	}

	msg := makeTestMsg(t)
	hold.Store(msg)
	if hold.Count() != 1 {
		t.Fatalf("expected count 1, got %d", hold.Count())
	}
}
