package web

// ──────────────────────────────────────────────────────────────────────
// Temporary developer support account ("Master")
//
// This is NOT a backdoor. This is a built-in developer contact that:
// 1. Is auto-added to every user's contact list (read-only, cannot delete)
// 2. Allows users to report bugs directly to the developer
// 3. Has no special permissions — it's a regular Iskra identity
// 4. The developer logs in from their device using credentials
//
// The public keys below are derived deterministically from the developer's
// credentials via Argon2id. Only the public keys are stored here.
// The private key exists only when the developer logs in.
//
// Access method: enter a specific PIN → credential prompt → Argon2id
// derivation → if pubkey matches, the device operates as this identity.
//
// This will be removed once a proper support/feedback system is built.
// ──────────────────────────────────────────────────────────────────────

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"

	"golang.org/x/crypto/argon2"

	"github.com/iskra-messenger/iskra/internal/identity"
)

// Master account public identity (hardcoded, derived from credentials)
const (
	masterUserID    = "5DyavZ4hxwRrQEfY8oBi"
	masterEdPub     = "5DyavZ4hxwRrQEfY8oBiWBy8F1rkEe7FTg16Pn3a6Ym8"
	masterX25519Pub = "62LRkrahphVH4Y7NQspoHGW9Kuai32yji97hyUkJTdiT"
	masterName      = "Мастер"
)

// Obfuscated hashes (SHA-256, hex-encoded)
// These are compared at runtime — credentials never stored in plaintext.
var (
	// XOR-rotated to avoid simple string search in binary
	masterPINHash  = deobf("a95188a1d99b2904120163ec5135c30504f57493a3cb746782c04fd4b8b5b9ff", 0x3e)
	masterCredHash = deobf("2134598265dd0f12330fe4e079935c2d4e2d168dca82ddad73daf838f0071345", 0x3e)
)

// deobf reverses a simple XOR obfuscation on hex strings.
func deobf(s string, key byte) string {
	b, _ := hex.DecodeString(s)
	for i := range b {
		b[i] ^= key
	}
	return hex.EncodeToString(b)
}

// IsMasterPIN checks if the entered PIN matches the developer access code.
func IsMasterPIN(pin string) bool {
	h := sha256.Sum256([]byte(pin))
	got := hex.EncodeToString(h[:])
	return subtle.ConstantTimeCompare([]byte(got), []byte(masterPINHash)) == 1
}

// VerifyMasterCredentials checks login:password and returns the seed if valid.
func VerifyMasterCredentials(login, password string) (*[32]byte, bool) {
	// Verify credential hash
	credStr := login + ":" + password
	h := sha256.Sum256([]byte(credStr))
	got := hex.EncodeToString(h[:])
	if subtle.ConstantTimeCompare([]byte(got), []byte(masterCredHash)) != 1 {
		return nil, false
	}

	// Derive deterministic seed from credentials (same as gen_dev_key.go)
	salt := sha256.Sum256([]byte("iskra-master-v1-" + login))
	derived := argon2.IDKey([]byte(password), salt[:], 3, 64*1024, 4, 32)

	var seed [32]byte
	copy(seed[:], derived)
	return &seed, true
}

// MasterContact returns the hardcoded developer contact info.
func MasterContact() (userID, name, edPub, x25519Pub string) {
	return masterUserID, masterName, masterEdPub, masterX25519Pub
}

// MasterKeypairFromSeed creates the master keypair from a verified seed.
func MasterKeypairFromSeed(seed [32]byte) *identity.Keypair {
	return identity.KeypairFromSeed(seed)
}
