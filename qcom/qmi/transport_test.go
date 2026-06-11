package qmi

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
	tests := []struct {
		name string
		req  qcom.Request
		want []byte
	}{
		{
			name: "read transparent request",
			req: qcom.Request{
				Service:       qcom.ServiceUIM,
				ClientID:      7,
				TransactionID: 3,
				MessageID:     qcom.MessageReadTransparent,
				TLVs: tlv.TLVs{
					tlv.Bytes(0x01, []byte{0x06, 0x00}),
					tlv.Bytes(0x02, []byte{0x07, 0x6F, 0x00}),
					tlv.Bytes(0x03, []byte{0x00, 0x00, 0x09, 0x00}),
				},
			},
			want: []byte{
				0x01, 0x1E, 0x00, 0x00, 0x0B, 0x07,
				0x00, 0x03, 0x00, 0x20, 0x00, 0x12, 0x00,
				0x01, 0x02, 0x00, 0x06, 0x00,
				0x02, 0x03, 0x00, 0x07, 0x6F, 0x00,
				0x03, 0x04, 0x00, 0x00, 0x00, 0x09, 0x00,
			},
		},
		{
			name: "authenticate request",
			req: qcom.Request{
				Service:       qcom.ServiceUIM,
				ClientID:      7,
				TransactionID: 4,
				MessageID:     qcom.MessageAuthenticate,
				TLVs: tlv.TLVs{
					tlv.Bytes(0x01, []byte{0x00, 0x00}),
					tlv.Bytes(0x02, []byte{0x03, 0x04, 0x00, 0x10, 0x01, 0x10, 0x02}),
				},
			},
			want: []byte{
				0x01, 0x1B, 0x00, 0x00, 0x0B, 0x07,
				0x00, 0x04, 0x00, 0x34, 0x00, 0x0F, 0x00,
				0x01, 0x02, 0x00, 0x00, 0x00,
				0x02, 0x07, 0x00, 0x03, 0x04, 0x00, 0x10, 0x01, 0x10, 0x02,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := MarshalRequest(tt.req)
			if err != nil {
				t.Fatalf("MarshalRequest() error = %v", err)
			}
			if !bytes.Equal(got, tt.want) {
				t.Fatalf("MarshalRequest() = % X, want % X", got, tt.want)
			}
		})
	}
}

func TestMarshalRequestReturnsTLVError(t *testing.T) {
	req := qcom.Request{
		Service:       qcom.ServiceUIM,
		ClientID:      7,
		TransactionID: 3,
		MessageID:     qcom.MessageReadTransparent,
		TLVs:          tlv.TLVs{{Type: 0x01, Len: 2, Value: []byte{0x01}}},
	}

	if _, err := MarshalRequest(req); err == nil {
		t.Fatal("MarshalRequest() error = nil, want TLV error")
	}
}

func TestResponseUnmarshalBinary(t *testing.T) {
	tests := []struct {
		name    string
		frame   []byte
		service qcom.ServiceType
		client  uint8
		txn     uint16
		message qcom.MessageID
	}{
		{
			name: "service response",
			frame: []byte{
				0x01, 0x18, 0x00, 0x80, 0x0B, 0x07,
				0x02, 0x03, 0x00, 0x20, 0x00, 0x0C, 0x00,
				0x02, 0x04, 0x00, 0x00, 0x00, 0x00, 0x00,
				0x10, 0x02, 0x00, 0x90, 0x00,
			},
			service: qcom.ServiceUIM,
			client:  7,
			txn:     3,
			message: qcom.MessageReadTransparent,
		},
		{
			name: "control response",
			frame: []byte{
				0x01, 0x12, 0x00, 0x80, 0x00, 0x00,
				0x01, 0x01, 0x00, 0xFF, 0x07, 0x00,
				0x02, 0x04, 0x00, 0x00, 0x00, 0x00, 0x00,
			},
			service: qcom.ServiceControl,
			client:  0,
			txn:     1,
			message: qcom.MessageInternalProxyOpen,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var wire Response
			if err := wire.UnmarshalBinary(tt.frame); err != nil {
				t.Fatalf("UnmarshalBinary() error = %v", err)
			}
			resp := wire.QCOM()
			if resp.Service != tt.service || resp.ClientID != tt.client || resp.TransactionID != tt.txn || resp.MessageID != tt.message {
				t.Fatalf("UnmarshalBinary() = %+v", resp)
			}
			if err := qcom.ResultError(resp.TLVs); err != nil {
				t.Fatalf("Result error = %v", err)
			}
		})
	}
}

func TestTransportClearsReadDeadlineOnce(t *testing.T) {
	mismatch := []byte{
		0x01, 0x18, 0x00, 0x80, 0x0B, 0x07,
		0x02, 0x09, 0x00, 0x20, 0x00, 0x0C, 0x00,
		0x02, 0x04, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x10, 0x02, 0x00, 0x90, 0x00,
	}
	match := []byte{
		0x01, 0x18, 0x00, 0x80, 0x0B, 0x07,
		0x02, 0x03, 0x00, 0x20, 0x00, 0x0C, 0x00,
		0x02, 0x04, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x10, 0x02, 0x00, 0x90, 0x00,
	}
	conn := &deadlineConn{read: bytes.NewReader(append(mismatch, match...))}
	transport := New(conn)

	_, err := transport.Do(context.Background(), qcom.Request{
		Service:       qcom.ServiceUIM,
		ClientID:      7,
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

type deadlineConn struct {
	read          *bytes.Reader
	readDeadlines []time.Time
}

func (c *deadlineConn) Read(p []byte) (int, error) {
	if c.read == nil {
		return 0, io.EOF
	}
	return c.read.Read(p)
}

func (c *deadlineConn) Write(p []byte) (int, error) { return len(p), nil }
func (c *deadlineConn) Close() error                { return nil }

func (c *deadlineConn) SetReadDeadline(t time.Time) error {
	c.readDeadlines = append(c.readDeadlines, t)
	return nil
}

func (c *deadlineConn) SetWriteDeadline(time.Time) error { return nil }
