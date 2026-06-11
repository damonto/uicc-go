package qrtr

import (
	"bytes"
	"context"
	"encoding"
	"io"
	"testing"
	"time"

	"github.com/damonto/uicc-go/qcom"
	"github.com/damonto/uicc-go/qcom/tlv"
)

func TestResponseImplementsStandardInterfaces(t *testing.T) {
	var _ encoding.BinaryMarshaler = Request{}
	var _ encoding.BinaryUnmarshaler = (*Response)(nil)
}

func TestMarshalRequest(t *testing.T) {
	req := qcom.Request{
		TransactionID: 3,
		MessageID:     qcom.MessageReadTransparent,
		TLVs: tlv.TLVs{
			tlv.Bytes(0x01, []byte{0x06, 0x00}),
			tlv.Bytes(0x02, []byte{0x07, 0x6F, 0x00}),
			tlv.Bytes(0x03, []byte{0x00, 0x00, 0x09, 0x00}),
		},
	}
	want := []byte{
		0x00, 0x03, 0x00, 0x20, 0x00, 0x12, 0x00,
		0x01, 0x02, 0x00, 0x06, 0x00,
		0x02, 0x03, 0x00, 0x07, 0x6F, 0x00,
		0x03, 0x04, 0x00, 0x00, 0x00, 0x09, 0x00,
	}
	got, err := MarshalRequest(req)
	if err != nil {
		t.Fatalf("MarshalRequest() error = %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("MarshalRequest() = % X, want % X", got, want)
	}
}

func TestMarshalRequestReturnsTLVError(t *testing.T) {
	req := qcom.Request{
		TransactionID: 3,
		MessageID:     qcom.MessageReadTransparent,
		TLVs:          tlv.TLVs{{Type: 0x01, Len: 2, Value: []byte{0x01}}},
	}

	if _, err := MarshalRequest(req); err == nil {
		t.Fatal("MarshalRequest() error = nil, want TLV error")
	}
}

func TestResponseUnmarshalBinary(t *testing.T) {
	frame := []byte{
		0x02, 0x03, 0x00, 0x20, 0x00, 0x0C, 0x00,
		0x02, 0x04, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x10, 0x02, 0x00, 0x90, 0x00,
	}

	var wire Response
	if err := wire.UnmarshalBinary(frame); err != nil {
		t.Fatalf("UnmarshalBinary() error = %v", err)
	}
	resp := wire.QCOM()
	if resp.TransactionID != 3 || resp.MessageID != qcom.MessageReadTransparent {
		t.Fatalf("UnmarshalBinary() = %+v", resp)
	}
	if err := qcom.ResultError(resp.TLVs); err != nil {
		t.Fatalf("Result error = %v", err)
	}
}

func TestTransportClearsReadDeadlineOnce(t *testing.T) {
	mismatch := []byte{
		0x02, 0x09, 0x00, 0x20, 0x00, 0x0C, 0x00,
		0x02, 0x04, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x10, 0x02, 0x00, 0x90, 0x00,
	}
	match := []byte{
		0x02, 0x03, 0x00, 0x20, 0x00, 0x0C, 0x00,
		0x02, 0x04, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x10, 0x02, 0x00, 0x90, 0x00,
	}
	conn := &deadlinePacketConn{frames: [][]byte{mismatch, match}}
	transport := New(conn)

	_, err := transport.Do(context.Background(), qcom.Request{
		TransactionID: 3,
		MessageID:     qcom.MessageReadTransparent,
		Timeout:       time.Second,
	})
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	if len(conn.readDeadlines) != 2 {
		t.Fatalf("read deadlines = %d, want set and clear", len(conn.readDeadlines))
	}
	if conn.readDeadlines[0].IsZero() || !conn.readDeadlines[1].IsZero() {
		t.Fatalf("read deadlines = %+v, want non-zero then zero", conn.readDeadlines)
	}
}

type deadlinePacketConn struct {
	frames        [][]byte
	readDeadlines []time.Time
}

func (c *deadlinePacketConn) Read(p []byte) (int, error) {
	if len(c.frames) == 0 {
		return 0, io.EOF
	}
	frame := c.frames[0]
	c.frames = c.frames[1:]
	return copy(p, frame), nil
}

func (c *deadlinePacketConn) Write(p []byte) (int, error) { return len(p), nil }
func (c *deadlinePacketConn) Close() error                { return nil }

func (c *deadlinePacketConn) SetReadDeadline(t time.Time) error {
	c.readDeadlines = append(c.readDeadlines, t)
	return nil
}
