package mbim

const (
	CIDSubscriberReadyStatus = 0x00000002

	CIDAuthAKA = 0x00000001

	CIDSTKEnvelope = 0x00000003

	CIDUiccATR                = 0x00000001
	CIDUiccOpenChannel        = 0x00000002
	CIDUiccCloseChannel       = 0x00000003
	CIDUiccAPDU               = 0x00000004
	CIDUiccTerminalCapability = 0x00000005
	CIDUiccReset              = 0x00000006
	CIDUiccApplicationList    = 0x00000007
	CIDUiccFileStatus         = 0x00000008
	CIDUiccReadBinary         = 0x00000009
	CIDUiccReadRecord         = 0x0000000A

	CIDProxyControlConfiguration = 0x00000001
	CIDDeviceSlotMappings        = 0x00000007
)

const uiccChannelGroupDefault = 1

const (
	defaultMaxControlTransfer = 4096
	maxFrameLength            = 2 * 1024 * 1024
)
