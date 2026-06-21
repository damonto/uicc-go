package qcom

import (
	"context"
	"time"

	"github.com/damonto/uicc-go/qcom/tlv"
)

// ServiceType represents QMI service types.
type ServiceType uint8

const (
	ServiceControl ServiceType = 0x00 // Control service
	ServiceCAT2    ServiceType = 0x0A // Card Application Toolkit service v2
	ServiceUIM     ServiceType = 0x0B // UIM service
	ServiceCAT     ServiceType = 0xE0 // Card Application Toolkit service v1
)

// MessageType represents QMI message types.
type MessageType uint8

const (
	MessageTypeRequest    MessageType = 0x00
	MessageTypeResponse   MessageType = 0x02
	MessageTypeIndication MessageType = 0x04
)

// MessageID represents QMI command message IDs.
type MessageID uint16

const (
	// CTL service commands
	MessageGetVersionInfo    MessageID = 0x0021
	MessageAllocateClientID  MessageID = 0x0022
	MessageReleaseClientID   MessageID = 0x0023
	MessageInternalProxyOpen MessageID = 0xFF00

	// UIM service commands
	MessageReset                     MessageID = 0x0000
	MessageReadTransparent           MessageID = 0x0020
	MessageReadRecord                MessageID = 0x0021
	MessageGetFileAttributes         MessageID = 0x0024
	MessageRefreshRegister           MessageID = 0x002A
	MessageRefreshComplete           MessageID = 0x002C
	MessageRegisterEvents            MessageID = 0x002E
	MessagePowerOffSIM               MessageID = 0x0030
	MessagePowerOnSIM                MessageID = 0x0031
	MessageRefresh                   MessageID = 0x0033
	MessageChangeProvisioningSession MessageID = 0x0038
	MessageSendAPDU                  MessageID = 0x003B
	MessageOpenLogicalChannel        MessageID = 0x0042
	MessageCloseLogicalChannel       MessageID = 0x003F
	MessageRefreshRegisterAll        MessageID = 0x0044
	MessageSwitchSlot                MessageID = 0x0046
	MessageGetSlotStatus             MessageID = 0x0047
	MessageSlotStatus                MessageID = 0x0048
	MessageGetCardStatus             MessageID = 0x002F
	MessageAuthenticate              MessageID = 0x0034

	// CAT/CAT2 service commands
	MessageSendEnvelope MessageID = 0x0022
)

// QMIResult represents the result code in QMI responses.
type QMIResult uint16

const (
	QMIResultSuccess QMIResult = 0x0000 // Success
	QMIResultFailure QMIResult = 0x0001 // Failure
)

type Request struct {
	Service       ServiceType
	ClientID      uint8
	TransactionID uint16
	MessageID     MessageID
	Timeout       time.Duration
	TLVs          tlv.TLVs
}

type Response struct {
	Service       ServiceType
	ClientID      uint8
	TransactionID uint16
	MessageID     MessageID
	TLVs          tlv.TLVs
}

// Indication is an unsolicited QMI message delivered outside a request/response
// transaction.
type Indication struct {
	Service       ServiceType
	ClientID      uint8
	TransactionID uint16
	MessageID     MessageID
	TLVs          tlv.TLVs
}

type Transport interface {
	Do(ctx context.Context, req Request) (Response, error)
	Close() error
}

// IndicationTransport extends Transport with best-effort indication delivery.
//
// Indications returns a channel for unsolicited messages matching service,
// clientID, and id. The channel is closed when ctx is done or the transport
// closes. Delivery is lossy: a slow subscriber may miss indications.
type IndicationTransport interface {
	Transport
	Indications(ctx context.Context, service ServiceType, clientID uint8, id MessageID) (<-chan Indication, error)
}

func RequestDeadline(ctx context.Context, timeout time.Duration) (time.Time, bool) {
	if deadline, ok := ctx.Deadline(); ok {
		if timeout <= 0 {
			return deadline, true
		}

		timeoutDeadline := time.Now().Add(timeout)
		if deadline.Before(timeoutDeadline) {
			return deadline, true
		}
		return timeoutDeadline, true
	}
	if timeout <= 0 {
		return time.Time{}, false
	}
	return time.Now().Add(timeout), true
}
