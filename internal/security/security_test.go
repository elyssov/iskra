package security

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestPINSetAndVerify(t *testing.T) {
	dir := t.TempDir()
	pin := "1234"

	if HasPIN(dir) {
		t.Fatal("should not have PIN before setup")
	}

	if err := SetPIN(dir, pin); err != nil {
		t.Fatalf("SetPIN: %v", err)
	}

	if !HasPIN(dir) {
		t.Fatal("should have PIN after setup")
	}

	if !VerifyPIN(dir, pin) {
		t.Fatal("correct PIN should verify")
	}

	if VerifyPIN(dir, "9999") {
		t.Fatal("wrong PIN should not verify")
	}
}

func TestAttempts(t *testing.T) {
	dir := t.TempDir()

	if got := GetAttempts(dir); got != 0 {
		t.Fatalf("expected 0 attempts, got %d", got)
	}

	if got := IncrementAttempts(dir); got != 1 {
		t.Fatalf("expected 1 after increment, got %d", got)
	}

	IncrementAttempts(dir)
	IncrementAttempts(dir)
	if got := GetAttempts(dir); got != 3 {
		t.Fatalf("expected 3, got %d", got)
	}

	ResetAttempts(dir)
	if got := GetAttempts(dir); got != 0 {
		t.Fatalf("expected 0 after reset, got %d", got)
	}
}

func TestVaultEncryptDecrypt(t *testing.T) {
	var key [32]byte
	copy(key[:], []byte("test-key-32-bytes-long-enough!!!"))

	plaintext := []byte("секретные данные Искры")

	encrypted, err := EncryptData(plaintext, &key)
	if err != nil {
		t.Fatalf("EncryptData: %v", err)
	}

	if len(encrypted) <= len(plaintext) {
		t.Fatal("encrypted data should be longer than plaintext")
	}

	decrypted, err := DecryptData(encrypted, &key)
	if err != nil {
		t.Fatalf("DecryptData: %v", err)
	}

	if string(decrypted) != string(plaintext) {
		t.Fatalf("decrypted != plaintext: %q vs %q", decrypted, plaintext)
	}

	// Wrong key should fail
	var wrongKey [32]byte
	copy(wrongKey[:], []byte("wrong-key-32-bytes-long-enough!!"))
	_, err = DecryptData(encrypted, &wrongKey)
	if err == nil {
		t.Fatal("wrong key should fail decryption")
	}
}

func TestDeriveStorageKey(t *testing.T) {
	var seed [32]byte
	copy(seed[:], []byte("test-seed-32-bytes-long-enough!!"))

	key1 := DeriveStorageKey(seed, "1234")
	key2 := DeriveStorageKey(seed, "1234")
	key3 := DeriveStorageKey(seed, "5678")

	if key1 != key2 {
		t.Fatal("same seed+pin should produce same key")
	}
	if key1 == key3 {
		t.Fatal("different PIN should produce different key")
	}
}

func TestVaultFileRoundtrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.enc")
	var key [32]byte
	copy(key[:], []byte("test-key-32-bytes-long-enough!!!"))

	data := []byte(`{"contacts": [{"name": "Ильич"}]}`)
	if err := EncryptFile(path, data, &key); err != nil {
		t.Fatalf("EncryptFile: %v", err)
	}

	decrypted, err := DecryptFile(path, &key)
	if err != nil {
		t.Fatalf("DecryptFile: %v", err)
	}

	if string(decrypted) != string(data) {
		t.Fatalf("roundtrip failed")
	}
}

func TestDecoyGeneration(t *testing.T) {
	dir := t.TempDir()

	if err := GenerateDecoy(dir); err != nil {
		t.Fatalf("GenerateDecoy: %v", err)
	}

	// Should have seed.key
	if _, err := os.Stat(filepath.Join(dir, "seed.key")); err != nil {
		t.Fatal("seed.key should exist")
	}

	// Should have contacts.json with 4 contacts
	data, err := os.ReadFile(filepath.Join(dir, "contacts.json"))
	if err != nil {
		t.Fatal("contacts.json should exist")
	}
	var contacts []map[string]interface{}
	if err := json.Unmarshal(data, &contacts); err != nil {
		t.Fatalf("contacts.json invalid JSON: %v", err)
	}
	if len(contacts) != 4 {
		t.Fatalf("expected 4 decoy contacts, got %d", len(contacts))
	}

	// Should have inbox.json with messages
	data, err = os.ReadFile(filepath.Join(dir, "inbox.json"))
	if err != nil {
		t.Fatal("inbox.json should exist")
	}
	var inbox map[string][]map[string]interface{}
	if err := json.Unmarshal(data, &inbox); err != nil {
		t.Fatalf("inbox.json invalid JSON: %v", err)
	}
	if len(inbox) != 4 {
		t.Fatalf("expected 4 conversations, got %d", len(inbox))
	}
	for uid, msgs := range inbox {
		if len(msgs) < 20 {
			t.Fatalf("contact %s has only %d messages, expected 20+", uid, len(msgs))
		}
	}

	// Should have hold messages
	holdEntries, err := os.ReadDir(filepath.Join(dir, "hold"))
	if err != nil {
		t.Fatal("hold dir should exist")
	}
	if len(holdEntries) < 100 {
		t.Fatalf("expected 100+ hold messages, got %d", len(holdEntries))
	}
}

func TestWipe(t *testing.T) {
	dir := t.TempDir()

	// Create some test files
	os.WriteFile(filepath.Join(dir, "seed.key"), []byte("secret-seed-data-here-32-bytes!!"), 0600)
	os.WriteFile(filepath.Join(dir, "contacts.json"), []byte(`[{"name":"test"}]`), 0600)
	os.MkdirAll(filepath.Join(dir, "hold"), 0700)
	os.WriteFile(filepath.Join(dir, "hold", "msg1.msg"), []byte("encrypted-msg"), 0600)

	if err := WipeAll(dir); err != nil {
		t.Fatalf("WipeAll: %v", err)
	}

	// All files should be gone
	entries, _ := os.ReadDir(dir)
	// Only empty directories might remain
	for _, e := range entries {
		if !e.IsDir() {
			t.Fatalf("file should be wiped: %s", e.Name())
		}
	}
}
