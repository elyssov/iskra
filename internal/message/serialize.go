package message

import (
	"encoding/binary"
	"fmt"
)

// Serialize converts a Message to binary format.
// v1 format: [version:1][id:32][recipientID:20][ttl:4][timestamp:8][contentType:1]
//            [ephemeralPub:32][nonce:24][payloadLen:4][payload:variable]
//            [authorPub:32][signature:64][powNonce:8]                              (230 + payload)
// v2 format (b11): all of the above + [authorX25519:32]                            (262 + payload)
//   v2 is always emitted now; v1 deserialization remains supported on the wire.
func (m *Message) Serialize() []byte {
	// Fixed size: 230 + payload + 32 (authorX25519) for v2; v1 omits the trailing field.
	buf := make([]byte, 0, 262+len(m.Payload))

	buf = append(buf, m.Version)
	buf = append(buf, m.ID[:]...)
	buf = append(buf, m.RecipientID[:]...)
	buf = append(buf, uint32Bytes(m.TTL)...)
	buf = append(buf, int64Bytes(m.Timestamp)...)
	buf = append(buf, m.ContentType)
	buf = append(buf, m.EphemeralPub[:]...)
	buf = append(buf, m.Nonce[:]...)
	buf = append(buf, uint32Bytes(uint32(len(m.Payload)))...)
	buf = append(buf, m.Payload...)
	buf = append(buf, m.AuthorPub[:]...)
	buf = append(buf, m.Signature[:]...)
	buf = append(buf, uint64Bytes(m.PoWNonce)...)

	// v2+: always emit AuthorX25519 (32 bytes). For v1 messages constructed manually
	// without AuthorX25519, the field stays zero and is still emitted — but only when
	// the message itself declares v2 or higher. This keeps strict v1 emitters byte-exact.
	if m.Version >= 2 {
		buf = append(buf, m.AuthorX25519[:]...)
	}

	return buf
}

// Deserialize parses binary data into a Message.
// Accepts both v1 (no trailing AuthorX25519) and v2+ (with AuthorX25519) on the wire.
// b11 note: v1 messages are still readable for backward compat, but cannot benefit from
// the reply-key MITM protection — recipients should treat their AuthorX25519 as zero
// and fall back to legacy reply-via-online.
func Deserialize(data []byte) (*Message, error) {
	// Minimum size without payload: 230
	if len(data) < 230 {
		return nil, fmt.Errorf("data too short: %d bytes, need at least 230", len(data))
	}

	m := &Message{}
	offset := 0

	m.Version = data[offset]
	offset++

	copy(m.ID[:], data[offset:offset+32])
	offset += 32

	copy(m.RecipientID[:], data[offset:offset+20])
	offset += 20

	m.TTL = binary.BigEndian.Uint32(data[offset : offset+4])
	offset += 4

	m.Timestamp = int64(binary.BigEndian.Uint64(data[offset : offset+8]))
	offset += 8

	m.ContentType = data[offset]
	offset++

	copy(m.EphemeralPub[:], data[offset:offset+32])
	offset += 32

	copy(m.Nonce[:], data[offset:offset+24])
	offset += 24

	payloadLen := binary.BigEndian.Uint32(data[offset : offset+4])
	offset += 4

	if uint32(len(data)-offset) < payloadLen+32+64+8 {
		return nil, fmt.Errorf("data too short for payload length %d", payloadLen)
	}

	m.Payload = make([]byte, payloadLen)
	copy(m.Payload, data[offset:offset+int(payloadLen)])
	offset += int(payloadLen)

	copy(m.AuthorPub[:], data[offset:offset+32])
	offset += 32

	copy(m.Signature[:], data[offset:offset+64])
	offset += 64

	m.PoWNonce = binary.BigEndian.Uint64(data[offset : offset+8])
	offset += 8

	// v2 extension: AuthorX25519 (32 bytes after PoWNonce, present whenever m.Version >= 2).
	// For compatibility with bytes-on-wire from v1 senders we also accept trailing 32 bytes
	// even when Version == 1 (some pre-b11 builds emitted the field optionally).
	if len(data) >= offset+32 {
		copy(m.AuthorX25519[:], data[offset:offset+32])
	}

	return m, nil
}

func uint32Bytes(v uint32) []byte {
	b := make([]byte, 4)
	binary.BigEndian.PutUint32(b, v)
	return b
}

func int64Bytes(v int64) []byte {
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, uint64(v))
	return b
}

func uint64Bytes(v uint64) []byte {
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, v)
	return b
}
