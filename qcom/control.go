package qcom

import (
	"fmt"

	"github.com/damonto/uicc-go/qcom/tlv"
)

type InternalOpenRequest struct {
	TransactionID uint16
	DevicePath    []byte
}

func (r InternalOpenRequest) Request() Request {
	return Request{
		TransactionID: r.TransactionID,
		MessageID:     MessageInternalProxyOpen,
		Service:       ServiceControl,
		TLVs: tlv.TLVs{
			tlv.Bytes(0x01, r.DevicePath),
		},
	}
}

type AllocateClientIDRequest struct {
	TransactionID uint16
	ServiceType   ServiceType
}

func (r AllocateClientIDRequest) Request() Request {
	return Request{
		TransactionID: r.TransactionID,
		MessageID:     MessageAllocateClientID,
		Service:       ServiceControl,
		TLVs: tlv.TLVs{
			tlv.Uint(0x01, r.ServiceType),
		},
	}
}

type AllocateClientIDResponse struct {
	ClientID uint8
}

func (r *AllocateClientIDResponse) UnmarshalResponse(tlvs tlv.TLVs) error {
	if value, ok := tlvs.Find(0x01); ok && len(value.Value) >= 2 {
		r.ClientID = value.Value[1]
		return nil
	}
	return fmt.Errorf("could not find allocated client ID in response")
}

type ReleaseClientIDRequest struct {
	ClientID      uint8
	TransactionID uint16
	ServiceType   ServiceType
}

func (r ReleaseClientIDRequest) Request() Request {
	return Request{
		TransactionID: r.TransactionID,
		MessageID:     MessageReleaseClientID,
		Service:       ServiceControl,
		TLVs: tlv.TLVs{
			tlv.Bytes(0x01, []byte{byte(r.ServiceType), r.ClientID}),
		},
	}
}

type ReleaseClientIDResponse struct {
	ClientID uint8
}

func (r *ReleaseClientIDResponse) UnmarshalResponse(tlvs tlv.TLVs) error {
	if value, ok := tlvs.Find(0x01); ok && len(value.Value) >= 2 {
		r.ClientID = value.Value[1]
		return nil
	}
	return fmt.Errorf("could not find released client ID in response")
}
