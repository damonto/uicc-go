package qrtr

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/damonto/uicc-go/qcom"
)

type packetConn interface {
	io.ReadWriter
	SetReadDeadline(time.Time) error
	Close() error
}

type Header struct {
	MessageType   qcom.MessageType
	TransactionID uint16
	MessageID     qcom.MessageID
	MessageLength uint16
}

type Request struct {
	qcom.Request
}

func (r Request) MarshalBinary() ([]byte, error) {
	return marshalRequest(r.Request)
}

type Transport struct {
	conn packetConn
	mu   sync.Mutex
}

func New(conn packetConn) *Transport {
	return &Transport{conn: conn}
}

func (t *Transport) Close() error {
	return t.conn.Close()
}

func (t *Transport) Do(ctx context.Context, req qcom.Request) (qcom.Response, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	packet, err := (Request{Request: req}).MarshalBinary()
	if err != nil {
		return qcom.Response{}, err
	}

	deadline, hasDeadline := qcom.RequestDeadline(ctx, req.Timeout)
	if err := writeFull(t.conn, packet); err != nil {
		return qcom.Response{}, fmt.Errorf("writing QRTR request: %w", err)
	}

	if hasDeadline {
		if err := t.conn.SetReadDeadline(deadline); err != nil {
			return qcom.Response{}, fmt.Errorf("setting QRTR read deadline: %w", err)
		}
		defer func() { _ = t.conn.SetReadDeadline(time.Time{}) }()
	}

	for {
		buf := make([]byte, qcom.MaxQRTRQMIMessageLength)
		n, err := t.conn.Read(buf)
		if err != nil {
			if ctx.Err() != nil {
				return qcom.Response{}, ctx.Err()
			}
			return qcom.Response{}, fmt.Errorf("reading QRTR response: %w", err)
		}

		var wire Response
		if err := wire.UnmarshalBinary(buf[:n]); err != nil {
			return qcom.Response{}, err
		}
		resp := wire.QCOM()
		if resp.TransactionID != req.TransactionID || resp.MessageID != req.MessageID {
			continue
		}
		return resp, nil
	}
}

func MarshalRequest(req qcom.Request) ([]byte, error) {
	return (Request{Request: req}).MarshalBinary()
}

func marshalRequest(req qcom.Request) ([]byte, error) {
	payload, err := req.TLVs.MarshalBinary()
	if err != nil {
		return nil, fmt.Errorf("marshal QRTR QMI TLVs: %w", err)
	}
	if len(payload) > qcom.MaxQRTRServiceTLVLength {
		return nil, fmt.Errorf("QRTR QMI message TLVs length %d exceeds limit %d", len(payload), qcom.MaxQRTRServiceTLVLength)
	}

	out := new(bytes.Buffer)
	if err := binary.Write(out, binary.LittleEndian, Header{
		MessageType:   qcom.MessageTypeRequest,
		TransactionID: req.TransactionID,
		MessageID:     req.MessageID,
		MessageLength: uint16(len(payload)),
	}); err != nil {
		return nil, fmt.Errorf("write QRTR QMI header: %w", err)
	}
	if _, err := out.Write(payload); err != nil {
		return nil, fmt.Errorf("write QRTR QMI payload: %w", err)
	}
	return out.Bytes(), nil
}

func writeFull(w io.Writer, p []byte) error {
	for len(p) > 0 {
		n, err := w.Write(p)
		if err != nil {
			return err
		}
		if n <= 0 {
			return io.ErrShortWrite
		}
		p = p[n:]
	}
	return nil
}
