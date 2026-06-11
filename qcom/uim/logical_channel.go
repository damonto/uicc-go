package uim

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"slices"

	"github.com/damonto/uicc-go/qcom"
	"github.com/damonto/uicc-go/qcom/tlv"
)

const (
	maxLogicalChannelAIDLength = 0xff

	sendAPDUFixedTLVLength   = 4 + 5 + 4
	maxSendAPDUCommandLength = qcom.MaxQMUXServiceTLVLength - sendAPDUFixedTLVLength
)

func (r *Reader) OpenLogicalChannel(ctx context.Context, aid []byte) (uint8, error) {
	request := OpenLogicalChannelRequest{AID: slices.Clone(aid)}
	value, err := request.MarshalBinary()
	if err != nil {
		return 0, err
	}

	resp, err := r.request(ctx, qcom.MessageOpenLogicalChannel, tlv.TLVs{
		tlv.Bytes(0x10, value),
		tlv.Uint(0x01, r.slot),
	})
	if err != nil {
		return 0, fmt.Errorf("opening QMI UIM logical channel: %w", err)
	}
	if err := resultOK(resp); err != nil {
		return 0, fmt.Errorf("opening QMI UIM logical channel: %w", err)
	}

	value, ok := tlv.Value(resp.TLVs, 0x10)
	if !ok {
		if err := cardError(resp.TLVs); err != nil {
			return 0, fmt.Errorf("opening QMI UIM logical channel: %w", err)
		}
		return 0, errors.New("opening QMI UIM logical channel: channel TLV missing")
	}

	var response OpenLogicalChannelResponse
	if err := response.UnmarshalBinary(value); err != nil {
		return 0, fmt.Errorf("opening QMI UIM logical channel: %w", err)
	}
	return response.Channel, nil
}

func (r *Reader) CloseLogicalChannel(ctx context.Context, channel uint8) error {
	request := CloseLogicalChannelRequest{Channel: channel}
	value, err := request.MarshalBinary()
	if err != nil {
		return err
	}

	resp, err := r.request(ctx, qcom.MessageCloseLogicalChannel, tlv.TLVs{
		tlv.Uint(0x01, r.slot),
		tlv.Bytes(0x11, value),
	})
	if err != nil {
		return fmt.Errorf("closing QMI UIM logical channel: %w", err)
	}
	if err := cardResultOK(resp); err != nil {
		return fmt.Errorf("closing QMI UIM logical channel: %w", err)
	}
	return nil
}

func (r *Reader) SendAPDU(ctx context.Context, channel uint8, command []byte) ([]byte, error) {
	request := SendAPDURequest{Command: slices.Clone(command)}
	value, err := request.MarshalBinary()
	if err != nil {
		return nil, err
	}

	resp, err := r.request(ctx, qcom.MessageSendAPDU, tlv.TLVs{
		tlv.Uint(0x10, channel),
		tlv.Bytes(0x02, value),
		tlv.Uint(0x01, r.slot),
	})
	if err != nil {
		return nil, fmt.Errorf("sending QMI UIM APDU: %w", err)
	}
	if err := resultOK(resp); err != nil {
		return nil, fmt.Errorf("sending QMI UIM APDU: %w", err)
	}

	value, ok := tlv.Value(resp.TLVs, 0x10)
	if !ok {
		if err := cardError(resp.TLVs); err != nil {
			return nil, fmt.Errorf("sending QMI UIM APDU: %w", err)
		}
		return nil, errors.New("sending QMI UIM APDU: response TLV missing")
	}

	var response SendAPDUResponse
	if err := response.UnmarshalBinary(value); err != nil {
		return nil, fmt.Errorf("sending QMI UIM APDU: %w", err)
	}
	return response.Response, nil
}

func (r OpenLogicalChannelRequest) MarshalBinary() ([]byte, error) {
	if len(r.AID) > maxLogicalChannelAIDLength {
		return nil, fmt.Errorf("marshaling QMI UIM open logical channel request: AID length %d exceeds %d", len(r.AID), maxLogicalChannelAIDLength)
	}

	data := make([]byte, 0, 1+len(r.AID))
	data = append(data, byte(len(r.AID)))
	data = append(data, r.AID...)
	return data, nil
}

func (r *OpenLogicalChannelRequest) UnmarshalBinary(data []byte) error {
	if len(data) == 0 {
		return errors.New("unmarshaling QMI UIM open logical channel request: AID length is missing")
	}

	length := int(data[0])
	if len(data) != 1+length {
		return fmt.Errorf("unmarshaling QMI UIM open logical channel request: AID length %d does not match actual length %d", length, len(data)-1)
	}
	r.AID = slices.Clone(data[1:])
	return nil
}

func (r *OpenLogicalChannelResponse) UnmarshalBinary(data []byte) error {
	if len(data) == 0 {
		return errors.New("unmarshaling QMI UIM open logical channel response: channel is missing")
	}
	r.Channel = data[0]
	return nil
}

func (r CloseLogicalChannelRequest) MarshalBinary() ([]byte, error) {
	return []byte{r.Channel}, nil
}

func (r *CloseLogicalChannelRequest) UnmarshalBinary(data []byte) error {
	if len(data) != 1 {
		return fmt.Errorf("unmarshaling QMI UIM close logical channel request: length %d, want 1", len(data))
	}
	r.Channel = data[0]
	return nil
}

func (r *CloseLogicalChannelResponse) UnmarshalBinary(data []byte) error {
	if len(data) != 0 {
		return fmt.Errorf("unmarshaling QMI UIM close logical channel response: length %d, want 0", len(data))
	}
	return nil
}

func (r SendAPDURequest) MarshalBinary() ([]byte, error) {
	if len(r.Command) > maxSendAPDUCommandLength {
		return nil, fmt.Errorf("marshaling QMI UIM APDU request: command length %d exceeds %d", len(r.Command), maxSendAPDUCommandLength)
	}

	data := binary.LittleEndian.AppendUint16(nil, uint16(len(r.Command)))
	data = append(data, r.Command...)
	return data, nil
}

func (r *SendAPDURequest) UnmarshalBinary(data []byte) error {
	command, err := decodeLengthPrefixedBytes(data)
	if err != nil {
		return fmt.Errorf("unmarshaling QMI UIM APDU request: %w", err)
	}
	r.Command = command
	return nil
}

func (r *SendAPDUResponse) UnmarshalBinary(data []byte) error {
	response, err := decodeLengthPrefixedBytes(data)
	if err != nil {
		return fmt.Errorf("unmarshaling QMI UIM APDU response: %w", err)
	}
	r.Response = response
	return nil
}
