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

func TestTransportDispatchesIndications(t *testing.T) {
	mismatch := []byte{
		0x02, 0x09, 0x00, 0x20, 0x00, 0x0C, 0x00,
		0x02, 0x04, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x10, 0x02, 0x00, 0x90, 0x00,
	}
	indication := []byte{
		0x04, 0x00, 0x00, 0x48, 0x00, 0x00, 0x00,
	}
	match := []byte{
		0x02, 0x03, 0x00, 0x20, 0x00, 0x0C, 0x00,
		0x02, 0x04, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x10, 0x02, 0x00, 0x90, 0x00,
	}
	conn := &deadlinePacketConn{frames: [][]byte{mismatch, indication, match}}
	transport := New(conn)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	indications, err := transport.Indications(ctx, qcom.ServiceUIM, 0, qcom.MessageSlotStatus)
	if err != nil {
		t.Fatalf("Indications() error = %v", err)
	}

	_, err = transport.Do(context.Background(), qcom.Request{
		TransactionID: 3,
		MessageID:     qcom.MessageReadTransparent,
		Timeout:       time.Second,
	})
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}

	select {
	case ind := <-indications:
		if ind.Service != qcom.ServiceUIM || ind.MessageID != qcom.MessageSlotStatus {
			t.Fatalf("indication = %+v, want slot status", ind)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for indication")
	}
}

func TestTransportCanUnsubscribeWhileDeliveringIndication(t *testing.T) {
	transport := New(&deadlinePacketConn{})
	ind := qcom.Indication{
		Service:   qcom.ServiceUIM,
		MessageID: qcom.MessageSlotStatus,
	}

	for range 1000 {
		ch := make(chan qcom.Indication, 1)
		transport.mu.Lock()
		transport.nextSub++
		id := transport.nextSub
		transport.subs[id] = subscription{
			message: qcom.MessageSlotStatus,
			ch:      ch,
		}
		transport.mu.Unlock()

		done := make(chan struct{})
		go func() {
			defer close(done)
			transport.deliverIndication(ind)
		}()
		transport.removeSubscription(id)

		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for indication delivery")
		}
	}
}

type deadlinePacketConn struct {
	frames [][]byte
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
func (c *deadlinePacketConn) SetReadDeadline(time.Time) error {
	return nil
}
