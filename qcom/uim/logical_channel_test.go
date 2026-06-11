package uim

import (
	"bytes"
	"context"
	"encoding"
	"strings"
	"testing"

	"github.com/damonto/uicc-go/qcom"
	"github.com/damonto/uicc-go/qcom/tlv"
)

var (
	_ encoding.BinaryMarshaler   = OpenLogicalChannelRequest{}
	_ encoding.BinaryUnmarshaler = (*OpenLogicalChannelRequest)(nil)
	_ encoding.BinaryUnmarshaler = (*OpenLogicalChannelResponse)(nil)
	_ encoding.BinaryMarshaler   = CloseLogicalChannelRequest{}
	_ encoding.BinaryUnmarshaler = (*CloseLogicalChannelRequest)(nil)
	_ encoding.BinaryUnmarshaler = (*CloseLogicalChannelResponse)(nil)
	_ encoding.BinaryMarshaler   = SendAPDURequest{}
	_ encoding.BinaryUnmarshaler = (*SendAPDURequest)(nil)
	_ encoding.BinaryUnmarshaler = (*SendAPDUResponse)(nil)
)

func TestReaderLogicalChannelPrimitives(t *testing.T) {
	aid := []byte{0xA0, 0x00, 0x00, 0x00, 0x87, 0x10, 0x04}
	command := []byte{0x00, 0xA4, 0x04, 0x00, byte(len(aid))}
	command = append(command, aid...)

	transport := &fakeTransport{
		t: t,
		calls: []transportCall{
			{
				check: func(req qcom.Request) {
					if req.Service != qcom.ServiceUIM || req.ClientID != 7 || req.MessageID != qcom.MessageOpenLogicalChannel {
						t.Fatalf("request = service %#x client %d message 0x%04X, want open logical channel", req.Service, req.ClientID, req.MessageID)
					}
					assertTLV(t, req.TLVs, 0x10, append([]byte{byte(len(aid))}, aid...))
					assertTLV(t, req.TLVs, 0x01, []byte{0x02})
				},
				resp: successResponse(qcom.MessageOpenLogicalChannel, tlv.Bytes(0x10, []byte{0x03})),
			},
			{
				check: func(req qcom.Request) {
					if req.Service != qcom.ServiceUIM || req.ClientID != 7 || req.MessageID != qcom.MessageSendAPDU {
						t.Fatalf("request = service %#x client %d message 0x%04X, want send APDU", req.Service, req.ClientID, req.MessageID)
					}
					assertTLV(t, req.TLVs, 0x10, []byte{0x03})
					assertTLV(t, req.TLVs, 0x02, encodeLengthPrefixed(command))
					assertTLV(t, req.TLVs, 0x01, []byte{0x02})
				},
				resp: successResponse(qcom.MessageSendAPDU, tlv.Bytes(0x10, encodeLengthPrefixed([]byte{0x90, 0x00}))),
			},
			{
				check: func(req qcom.Request) {
					if req.Service != qcom.ServiceUIM || req.ClientID != 7 || req.MessageID != qcom.MessageCloseLogicalChannel {
						t.Fatalf("request = service %#x client %d message 0x%04X, want close logical channel", req.Service, req.ClientID, req.MessageID)
					}
					assertTLV(t, req.TLVs, 0x01, []byte{0x02})
					assertTLV(t, req.TLVs, 0x11, []byte{0x03})
				},
				resp: successResponse(qcom.MessageCloseLogicalChannel),
			},
		},
	}
	reader := &Reader{
		transport: transport,
		slot:      2,
		clientID:  7,
	}

	channel, err := reader.OpenLogicalChannel(context.Background(), aid)
	if err != nil {
		t.Fatalf("OpenLogicalChannel() error = %v", err)
	}
	if channel != 3 {
		t.Fatalf("OpenLogicalChannel() = %d, want 3", channel)
	}

	got, err := reader.SendAPDU(context.Background(), channel, command)
	if err != nil {
		t.Fatalf("SendAPDU() error = %v", err)
	}
	if !bytes.Equal(got, []byte{0x90, 0x00}) {
		t.Fatalf("SendAPDU() = % X, want 90 00", got)
	}

	if err := reader.CloseLogicalChannel(context.Background(), channel); err != nil {
		t.Fatalf("CloseLogicalChannel() error = %v", err)
	}
	if transport.idx != len(transport.calls) {
		t.Fatalf("Do() calls = %d, want %d", transport.idx, len(transport.calls))
	}
}

func TestOpenLogicalChannelRequestMarshalBinary(t *testing.T) {
	tests := []struct {
		name    string
		req     OpenLogicalChannelRequest
		want    []byte
		wantErr string
	}{
		{
			name: "aid",
			req:  OpenLogicalChannelRequest{AID: []byte{0xA0, 0x00}},
			want: []byte{0x02, 0xA0, 0x00},
		},
		{
			name:    "aid too long",
			req:     OpenLogicalChannelRequest{AID: bytes.Repeat([]byte{0x01}, maxLogicalChannelAIDLength+1)},
			wantErr: "exceeds",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.req.MarshalBinary()
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("MarshalBinary() error = %v, want text %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("MarshalBinary() error = %v", err)
			}
			if !bytes.Equal(got, tt.want) {
				t.Fatalf("MarshalBinary() = % X, want % X", got, tt.want)
			}
		})
	}
}

func TestOpenLogicalChannelRequestUnmarshalBinary(t *testing.T) {
	tests := []struct {
		name    string
		data    []byte
		want    OpenLogicalChannelRequest
		wantErr string
	}{
		{
			name: "aid",
			data: []byte{0x02, 0xA0, 0x00},
			want: OpenLogicalChannelRequest{AID: []byte{0xA0, 0x00}},
		},
		{
			name:    "missing length",
			data:    nil,
			wantErr: "missing",
		},
		{
			name:    "truncated aid",
			data:    []byte{0x02, 0xA0},
			wantErr: "does not match",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got OpenLogicalChannelRequest
			err := got.UnmarshalBinary(tt.data)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("UnmarshalBinary() error = %v, want text %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("UnmarshalBinary() error = %v", err)
			}
			if !bytes.Equal(got.AID, tt.want.AID) {
				t.Fatalf("UnmarshalBinary() = % X, want % X", got.AID, tt.want.AID)
			}
			if len(tt.data) > 1 && len(got.AID) > 0 {
				tt.data[1] ^= 0xff
				if bytes.Equal(got.AID, tt.data[1:]) {
					t.Fatal("UnmarshalBinary() kept AID backing array")
				}
			}
		})
	}
}

func TestLogicalChannelResponsesUnmarshalBinary(t *testing.T) {
	tests := []struct {
		name    string
		run     func() error
		wantErr string
	}{
		{
			name: "open channel response",
			run: func() error {
				var resp OpenLogicalChannelResponse
				if err := resp.UnmarshalBinary([]byte{0x03}); err != nil {
					return err
				}
				if resp.Channel != 3 {
					t.Fatalf("Channel = %d, want 3", resp.Channel)
				}
				return nil
			},
		},
		{
			name: "open channel missing",
			run: func() error {
				var resp OpenLogicalChannelResponse
				return resp.UnmarshalBinary(nil)
			},
			wantErr: "missing",
		},
		{
			name: "close channel response",
			run: func() error {
				var resp CloseLogicalChannelResponse
				return resp.UnmarshalBinary(nil)
			},
		},
		{
			name: "close channel unexpected data",
			run: func() error {
				var resp CloseLogicalChannelResponse
				return resp.UnmarshalBinary([]byte{0x00})
			},
			wantErr: "want 0",
		},
		{
			name: "send APDU response",
			run: func() error {
				var resp SendAPDUResponse
				if err := resp.UnmarshalBinary(encodeLengthPrefixed([]byte{0x90, 0x00})); err != nil {
					return err
				}
				if !bytes.Equal(resp.Response, []byte{0x90, 0x00}) {
					t.Fatalf("Response = % X, want 90 00", resp.Response)
				}
				return nil
			},
		},
		{
			name: "send APDU response truncated",
			run: func() error {
				var resp SendAPDUResponse
				return resp.UnmarshalBinary([]byte{0x02, 0x00, 0x90})
			},
			wantErr: "truncated",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.run()
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("UnmarshalBinary() error = %v, want text %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("UnmarshalBinary() error = %v", err)
			}
		})
	}
}

func TestCloseLogicalChannelRequestBinary(t *testing.T) {
	got, err := (CloseLogicalChannelRequest{Channel: 3}).MarshalBinary()
	if err != nil {
		t.Fatalf("MarshalBinary() error = %v", err)
	}
	if !bytes.Equal(got, []byte{0x03}) {
		t.Fatalf("MarshalBinary() = % X, want 03", got)
	}

	var req CloseLogicalChannelRequest
	if err := req.UnmarshalBinary(got); err != nil {
		t.Fatalf("UnmarshalBinary() error = %v", err)
	}
	if req.Channel != 3 {
		t.Fatalf("Channel = %d, want 3", req.Channel)
	}
}

func TestSendAPDURequestBinary(t *testing.T) {
	tests := []struct {
		name    string
		req     SendAPDURequest
		want    []byte
		wantErr string
	}{
		{
			name: "command",
			req:  SendAPDURequest{Command: []byte{0x00, 0xA4}},
			want: []byte{0x02, 0x00, 0x00, 0xA4},
		},
		{
			name:    "command too long",
			req:     SendAPDURequest{Command: bytes.Repeat([]byte{0x00}, maxSendAPDUCommandLength+1)},
			wantErr: "exceeds",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.req.MarshalBinary()
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("MarshalBinary() error = %v, want text %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("MarshalBinary() error = %v", err)
			}
			if !bytes.Equal(got, tt.want) {
				t.Fatalf("MarshalBinary() = % X, want % X", got, tt.want)
			}

			var req SendAPDURequest
			if err := req.UnmarshalBinary(got); err != nil {
				t.Fatalf("UnmarshalBinary() error = %v", err)
			}
			if !bytes.Equal(req.Command, tt.req.Command) {
				t.Fatalf("UnmarshalBinary() = % X, want % X", req.Command, tt.req.Command)
			}
		})
	}
}
