package mbim

import (
	"encoding"
	"testing"
)

func TestProtocolTypesImplementStandardInterfaces(t *testing.T) {
	var _ encoding.BinaryMarshaler = (*Request)(nil)
	var _ encoding.BinaryMarshaler = (*Command)(nil)
	var _ encoding.BinaryMarshaler = (*OpenDeviceRequest)(nil)
	var _ encoding.BinaryMarshaler = (*CloseRequest)(nil)

	var _ encoding.BinaryUnmarshaler = (*CommandResponse)(nil)
	var _ encoding.BinaryUnmarshaler = (*ProxyConfigResponse)(nil)
	var _ encoding.BinaryUnmarshaler = (*OpenDeviceResponse)(nil)
	var _ encoding.BinaryUnmarshaler = (*CloseResponse)(nil)
	var _ encoding.BinaryUnmarshaler = (*DeviceSlotMappingsResponse)(nil)
	var _ encoding.BinaryUnmarshaler = (*SubscriberReadyStatusResponse)(nil)
	var _ encoding.BinaryUnmarshaler = (*ApplicationListResponse)(nil)
	var _ encoding.BinaryUnmarshaler = (*UICCApplication)(nil)
	var _ encoding.BinaryUnmarshaler = (*FileStatusResponse)(nil)
	var _ encoding.BinaryUnmarshaler = (*ReadBinaryResponse)(nil)
	var _ encoding.BinaryUnmarshaler = (*ReadRecordResponse)(nil)
	var _ encoding.BinaryUnmarshaler = (*AuthAKAResponse)(nil)
	var _ encoding.BinaryUnmarshaler = (*UiccATRResponse)(nil)
	var _ encoding.BinaryUnmarshaler = (*OpenChannelResponse)(nil)
	var _ encoding.BinaryUnmarshaler = (*CloseChannelResponse)(nil)
	var _ encoding.BinaryUnmarshaler = (*APDUResponse)(nil)
	var _ encoding.BinaryUnmarshaler = (*STKEnvelopeResponse)(nil)
}
