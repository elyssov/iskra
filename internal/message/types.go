package message

// Content types
const (
	ContentText            uint8 = 0
	ContentDeliveryConfirm uint8 = 1
	ContentContactsSync    uint8 = 2
	ContentGroupText       uint8 = 3
	ContentGroupInvite     uint8 = 4
	ContentChannelPost     uint8 = 5
	ContentAppUpdate       uint8 = 255
)

// Protocol version
const ProtocolVersion uint8 = 1

// Default TTL: 14 days in seconds
const DefaultTTL uint32 = 1209600
