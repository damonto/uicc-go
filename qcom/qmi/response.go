package qmi

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"

	"github.com/damonto/uicc-go/qcom"
	"github.com/damonto/uicc-go/qcom/tlv"
)

type Response struct {
	QMUXHeader
	TransactionID uint16
	MessageID     qcom.MessageID
	MessageType   qcom.MessageType
	MessageLength uint16
	TLVs          tlv.TLVs
}

func (r Response) QCOM() qcom.Response {
	return qcom.Response{
		Service:       r.ServiceType,
		ClientID:      r.ClientID,
		TransactionID: r.TransactionID,
		MessageID:     r.MessageID,
		TLVs:          r.TLVs,
	}
}

func (r Response) QCOMIndication() qcom.Indication {
	return qcom.Indication{
		Service:       r.ServiceType,
		ClientID:      r.ClientID,
		TransactionID: r.TransactionID,
		MessageID:     r.MessageID,
		TLVs:          r.TLVs,
	}
}

func (r *Response) UnmarshalBinary(data []byte) error {
	*r = Response{}
	if len(data) < 12 {
		return fmt.Errorf("parsing QMI message: data too short: got %d bytes", len(data))
	}

	reader := bytes.NewReader(data)
	if err := binary.Read(reader, binary.LittleEndian, &r.QMUXHeader); err != nil {
		return fmt.Errorf("parsing QMI message: read QMUX header: %w", err)
	}
	if r.QMUXHeader.IfType != qcom.QMUXIfType {
		return fmt.Errorf("parsing QMI message: unexpected QMUX marker 0x%02X", r.QMUXHeader.IfType)
	}
	if got, want := len(data), int(r.QMUXHeader.Length)+1; got != want {
		return fmt.Errorf("parsing QMI message: QMUX length mismatch: got %d bytes, header declares %d", got, want)
	}

	if r.QMUXHeader.ServiceType == qcom.ServiceControl {
		var header Header[uint8]
		if err := binary.Read(reader, binary.LittleEndian, &header); err != nil {
			return fmt.Errorf("parsing QMI message: read control QMI header: %w", err)
		}
		r.MessageType = header.MessageType
		r.TransactionID = uint16(header.TransactionID)
		r.MessageID = header.MessageID
		r.MessageLength = header.MessageLength
	} else {
		var header Header[uint16]
		if err := binary.Read(reader, binary.LittleEndian, &header); err != nil {
			return fmt.Errorf("parsing QMI message: read service QMI header: %w", err)
		}
		r.MessageType = header.MessageType
		r.TransactionID = header.TransactionID
		r.MessageID = header.MessageID
		r.MessageLength = header.MessageLength
	}

	if r.QMUXHeader.ServiceType == qcom.ServiceControl {
		if r.MessageType != 0x01 {
			return fmt.Errorf("parsing QMI message: unexpected control message type 0x%02X", r.MessageType)
		}
	} else if r.MessageType != qcom.MessageTypeResponse && r.MessageType != qcom.MessageTypeIndication {
		return fmt.Errorf("parsing QMI message: unexpected message type 0x%02X", r.MessageType)
	}
	if got, want := reader.Len(), int(r.MessageLength); got != want {
		return fmt.Errorf("parsing QMI message: QMI TLV length mismatch: got %d bytes, header declares %d", got, want)
	}
	if r.MessageLength > 0 {
		if err := r.TLVs.UnmarshalBinary(data[len(data)-int(r.MessageLength):]); err != nil {
			return err
		}
	}
	return nil
}

func ReadFrame(r io.Reader) ([]byte, error) {
	header := make([]byte, 3)
	if _, err := io.ReadFull(r, header); err != nil {
		return nil, err
	}

	length := int(binary.LittleEndian.Uint16(header[1:3])) + 1
	if length < len(header) {
		return nil, errors.New("reading QMI frame: invalid length")
	}

	frame := make([]byte, length)
	copy(frame, header)
	if _, err := io.ReadFull(r, frame[len(header):]); err != nil {
		return nil, err
	}
	return frame, nil
}
