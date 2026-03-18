package update

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
)

// DeveloperPubKey is the hardcoded public key for verifying updates.
// Replace with the actual developer key before release.
var DeveloperPubKey [32]byte

// UpdateInfo represents a parsed app update message.
type UpdateInfo struct {
	Version   uint32
	APKHash   [32]byte
	APKURL    string
	Changelog string
}

// ParseUpdatePayload parses the decrypted payload of a ContentType=255 message.
// Format: [version:4][apk_sha256:32][url_len:2][url:variable][changelog:remaining]
func ParseUpdatePayload(payload []byte) (*UpdateInfo, error) {
	if len(payload) < 38 { // 4 + 32 + 2 minimum
		return nil, fmt.Errorf("update payload too short: %d bytes", len(payload))
	}

	info := &UpdateInfo{}
	info.Version = binary.BigEndian.Uint32(payload[0:4])
	copy(info.APKHash[:], payload[4:36])

	urlLen := binary.BigEndian.Uint16(payload[36:38])
	if len(payload) < 38+int(urlLen) {
		return nil, fmt.Errorf("payload too short for URL length %d", urlLen)
	}
	info.APKURL = string(payload[38 : 38+urlLen])
	info.Changelog = string(payload[38+urlLen:])

	return info, nil
}

// CreateUpdatePayload creates the payload for an update message.
func CreateUpdatePayload(version uint32, apkHash [32]byte, url, changelog string) []byte {
	urlBytes := []byte(url)
	changelogBytes := []byte(changelog)

	payload := make([]byte, 4+32+2+len(urlBytes)+len(changelogBytes))
	binary.BigEndian.PutUint32(payload[0:4], version)
	copy(payload[4:36], apkHash[:])
	binary.BigEndian.PutUint16(payload[36:38], uint16(len(urlBytes)))
	copy(payload[38:], urlBytes)
	copy(payload[38+len(urlBytes):], changelogBytes)

	return payload
}

// VerifyUpdateSignature checks that the update message is signed by the developer key.
func VerifyUpdateSignature(authorPub [32]byte, data []byte, signature [64]byte) bool {
	// Must be from developer key
	if authorPub != DeveloperPubKey {
		return false
	}
	return ed25519.Verify(authorPub[:], data, signature[:])
}

// VerifyAPKHash checks the SHA256 hash of a downloaded APK.
func VerifyAPKHash(apkData []byte, expectedHash [32]byte) bool {
	actual := sha256.Sum256(apkData)
	return actual == expectedHash
}
