package uim

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/damonto/uicc-go/qcom"
	"github.com/damonto/uicc-go/qcom/tlv"
)

func TestReaderPowerPrimitives(t *testing.T) {
	tests := []struct {
		name    string
		run     func(context.Context, *Reader) error
		check   func(*testing.T, qcom.Request)
		resp    qcom.Response
		wantErr string
	}{
		{
			name: "reset",
			run: func(ctx context.Context, r *Reader) error {
				return r.Reset(ctx)
			},
			check: func(t *testing.T, req qcom.Request) {
				t.Helper()
				if req.MessageID != qcom.MessageReset {
					t.Fatalf("MessageID = 0x%04X, want 0x%04X", req.MessageID, qcom.MessageReset)
				}
				if len(req.TLVs) != 0 {
					t.Fatalf("TLVs = %+v, want empty", req.TLVs)
				}
			},
			resp: successResponse(qcom.MessageReset),
		},
		{
			name: "power off sim",
			run: func(ctx context.Context, r *Reader) error {
				return r.PowerOffSIM(ctx, 2)
			},
			check: func(t *testing.T, req qcom.Request) {
				t.Helper()
				if req.MessageID != qcom.MessagePowerOffSIM {
					t.Fatalf("MessageID = 0x%04X, want 0x%04X", req.MessageID, qcom.MessagePowerOffSIM)
				}
				assertTLV(t, req.TLVs, 0x01, []byte{0x02})
			},
			resp: successResponse(qcom.MessagePowerOffSIM),
		},
		{
			name: "power on sim",
			run: func(ctx context.Context, r *Reader) error {
				return r.PowerOnSIM(ctx, PowerOnSIMRequest{Slot: 2})
			},
			check: func(t *testing.T, req qcom.Request) {
				t.Helper()
				if req.MessageID != qcom.MessagePowerOnSIM {
					t.Fatalf("MessageID = 0x%04X, want 0x%04X", req.MessageID, qcom.MessagePowerOnSIM)
				}
				assertTLV(t, req.TLVs, 0x01, []byte{0x02})
				if _, ok := tlv.Value(req.TLVs, 0x10); ok {
					t.Fatal("PowerOnSIM() includes ignore hot-swap TLV, want omitted")
				}
			},
			resp: successResponse(qcom.MessagePowerOnSIM),
		},
		{
			name: "power on sim ignoring hot-swap",
			run: func(ctx context.Context, r *Reader) error {
				return r.PowerOnSIM(ctx, PowerOnSIMRequest{Slot: 2, IgnoreHotSwapSwitch: true})
			},
			check: func(t *testing.T, req qcom.Request) {
				t.Helper()
				if req.MessageID != qcom.MessagePowerOnSIM {
					t.Fatalf("MessageID = 0x%04X, want 0x%04X", req.MessageID, qcom.MessagePowerOnSIM)
				}
				assertTLV(t, req.TLVs, 0x01, []byte{0x02})
				assertTLV(t, req.TLVs, 0x10, []byte{0x01})
			},
			resp: successResponse(qcom.MessagePowerOnSIM),
		},
		{
			name: "power off sim rejects zero slot",
			run: func(ctx context.Context, r *Reader) error {
				return r.PowerOffSIM(ctx, 0)
			},
			wantErr: "slot is zero",
		},
		{
			name: "power on sim rejects zero slot",
			run: func(ctx context.Context, r *Reader) error {
				return r.PowerOnSIM(ctx, PowerOnSIMRequest{})
			},
			wantErr: "slot is zero",
		},
		{
			name: "change provisioning session",
			run: func(ctx context.Context, r *Reader) error {
				return r.ChangeProvisioningSession(ctx, ChangeProvisioningSessionRequest{
					Session:  SessionPrimaryGWProvisioning,
					Activate: true,
				})
			},
			check: func(t *testing.T, req qcom.Request) {
				t.Helper()
				if req.MessageID != qcom.MessageChangeProvisioningSession {
					t.Fatalf("MessageID = 0x%04X, want 0x%04X", req.MessageID, qcom.MessageChangeProvisioningSession)
				}
				assertTLV(t, req.TLVs, 0x01, []byte{byte(SessionPrimaryGWProvisioning), 0x01})
				if _, ok := tlv.Value(req.TLVs, 0x10); ok {
					t.Fatal("ChangeProvisioningSession() includes application information TLV, want omitted")
				}
			},
			resp: successResponse(qcom.MessageChangeProvisioningSession),
		},
		{
			name: "change provisioning session with application information",
			run: func(ctx context.Context, r *Reader) error {
				return r.ChangeProvisioningSession(ctx, ChangeProvisioningSessionRequest{
					Session:  SessionPrimaryGWProvisioning,
					Activate: true,
					Slot:     2,
					AID:      []byte{0xA0, 0x00, 0x00, 0x00, 0x87, 0x10, 0x02},
				})
			},
			check: func(t *testing.T, req qcom.Request) {
				t.Helper()
				if req.MessageID != qcom.MessageChangeProvisioningSession {
					t.Fatalf("MessageID = 0x%04X, want 0x%04X", req.MessageID, qcom.MessageChangeProvisioningSession)
				}
				assertTLV(t, req.TLVs, 0x01, []byte{byte(SessionPrimaryGWProvisioning), 0x01})
				assertTLV(t, req.TLVs, 0x10, []byte{0x02, 0x07, 0xA0, 0x00, 0x00, 0x00, 0x87, 0x10, 0x02})
			},
			resp: successResponse(qcom.MessageChangeProvisioningSession),
		},
		{
			name: "change provisioning session rejects long aid",
			run: func(ctx context.Context, r *Reader) error {
				return r.ChangeProvisioningSession(ctx, ChangeProvisioningSessionRequest{
					Session:  SessionPrimaryGWProvisioning,
					Activate: true,
					Slot:     2,
					AID:      bytes.Repeat([]byte{0xA0}, 256),
				})
			},
			wantErr: "AID length 256 exceeds 255",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := &fakeTransport{t: t}
			if tt.check != nil {
				transport.calls = []transportCall{{
					check: func(req qcom.Request) {
						if req.Service != qcom.ServiceUIM || req.ClientID != 7 {
							t.Fatalf("request = service %#x client %d, want UIM client 7", req.Service, req.ClientID)
						}
						assertRequestTimeout(t, req, DefaultRequestTimeout)
						tt.check(t, req)
					},
					resp: tt.resp,
				}}
			}
			reader := &Reader{
				transport: transport,
				slot:      1,
				clientID:  7,
			}

			err := tt.run(context.Background(), reader)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("%s error = %v, want text %q", tt.name, err, tt.wantErr)
				}
				if transport.idx != 0 {
					t.Fatalf("Do() calls = %d, want 0", transport.idx)
				}
				return
			}
			if err != nil {
				t.Fatalf("%s error = %v", tt.name, err)
			}
			if transport.idx != len(transport.calls) {
				t.Fatalf("Do() calls = %d, want %d", transport.idx, len(transport.calls))
			}
		})
	}
}
