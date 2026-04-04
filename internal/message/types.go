package message

// Content types
const (
	ContentText            uint8 = 0
	ContentDeliveryConfirm uint8 = 1
	ContentContactsSync    uint8 = 2
	ContentGroupText       uint8 = 3
	ContentGroupInvite     uint8 = 4
	ContentChannelPost     uint8 = 5
	ContentFileChunk       uint8 = 6
	ContentLetter          uint8 = 0x20 // Mail/letter (long TTL)
	ContentFOTA            uint8 = 0x30 // Firmware over-the-air update chunk
	ContentAppUpdate       uint8 = 255
)

// Protocol version
const ProtocolVersion uint8 = 1

// TTL by content type
const (
	TTLChat    uint32 = 15 * 24 * 3600  // 15 days — chat messages
	TTLLetter  uint32 = 60 * 24 * 3600  // 60 days — letters (reliable delivery)
	TTLChannel uint32 = 30 * 24 * 3600  // 30 days — channel posts
	TTLFOTA    uint32 = 7 * 24 * 3600   // 7 days  — firmware updates
)

// DefaultTTL for backwards compatibility
const DefaultTTL uint32 = TTLChat

// TTLForContentType returns the appropriate TTL for a given content type.
func TTLForContentType(ct uint8) uint32 {
	switch ct {
	case ContentLetter:
		return TTLLetter
	case ContentChannelPost:
		return TTLChannel
	case ContentFOTA:
		return TTLFOTA
	default:
		return TTLChat
	}
}

// ShouldStoreInHold returns whether this content type should be stored
// in the hold for store-and-forward. File chunks should NOT be forwarded.
func ShouldStoreInHold(ct uint8) bool {
	return ct != ContentFileChunk && ct != ContentDeliveryConfirm
}
