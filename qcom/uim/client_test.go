package uim

import (
	"bytes"
	"context"
	"encoding/binary"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/damonto/uicc-go/qcom"
	"github.com/damonto/uicc-go/qcom/tlv"
)

type fakeTransport struct {
	mu         sync.Mutex
	t          *testing.T
	calls      []transportCall
	idx        int
	closeCalls int
	closeErr   error
}

type transportCall struct {
	check func(qcom.Request)
	resp  qcom.Response
	err   error
}

func (t *fakeTransport) Do(_ context.Context, req qcom.Request) (qcom.Response, error) {
	t.t.Helper()
	t.mu.Lock()
	if t.idx >= len(t.calls) {
		t.mu.Unlock()
		t.t.Fatalf("Do() got unexpected request: %+v", req)
	}

	call := t.calls[t.idx]
	t.idx++
	t.mu.Unlock()

	if call.check != nil {
		call.check(req)
	}
	if call.err != nil {
		return qcom.Response{}, call.err
	}
	return call.resp, nil
}

func (t *fakeTransport) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.closeCalls++
	return t.closeErr
}

func (t *fakeTransport) callCount() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.idx
}

func TestReaderUIMMessages(t *testing.T) {
	isimAID := []byte{0xA0, 0x00, 0x00, 0x00, 0x87, 0x10, 0x04}
	reader := &Reader{
		transport: &fakeTransport{
			t: t,
			calls: []transportCall{
				{
					check: func(req qcom.Request) {
						if req.MessageID != qcom.MessageGetFileAttributes {
							t.Fatalf("messageID = 0x%04X, want 0x%04X", req.MessageID, qcom.MessageGetFileAttributes)
						}
						assertTLV(t, req.TLVs, 0x01, []byte{byte(SessionPrimaryGWProvisioning), 0x00})
						assertTLV(t, req.TLVs, 0x02, []byte{0xE2, 0x2F, 0x02, 0x00, 0x3F})
					},
					resp: successResponse(qcom.MessageGetFileAttributes,
						tlv.Bytes(0x10, []byte{0x90, 0x00}),
						tlv.Bytes(0x11, encodeFileAttributes(10, 0x2FE2, 0, 0, 0, []byte{0x62, 0x08, 0x82, 0x02, 0x41, 0x21, 0x80, 0x02, 0x00, 0x0A})),
					),
				},
				{
					check: func(req qcom.Request) {
						if req.MessageID != qcom.MessageReadTransparent {
							t.Fatalf("messageID = 0x%04X, want 0x%04X", req.MessageID, qcom.MessageReadTransparent)
						}
						assertTLV(t, req.TLVs, 0x01, []byte{byte(SessionPrimaryGWProvisioning), 0x00})
						assertTLV(t, req.TLVs, 0x02, []byte{0x07, 0x6F, 0x04, 0x00, 0x3F, 0xFF, 0x7F})
						assertTLV(t, req.TLVs, 0x03, []byte{0x00, 0x00, 0x09, 0x00})
					},
					resp: successResponse(qcom.MessageReadTransparent,
						tlv.Bytes(0x10, []byte{0x90, 0x00}),
						tlv.Bytes(0x11, encodeLengthPrefixed([]byte{0x08, 0x09, 0x10, 0x10, 0x10, 0x32, 0x54, 0x76, 0x98})),
					),
				},
				{
					check: func(req qcom.Request) {
						assertTLV(t, req.TLVs, 0x01, append([]byte{byte(SessionNonProvisioningSlot1), byte(len(isimAID))}, isimAID...))
						assertTLV(t, req.TLVs, 0x02, []byte{0x04, 0x6F, 0x00})
						assertTLV(t, req.TLVs, 0x03, []byte{0x01, 0x00, 0x20, 0x00})
					},
					resp: successResponse(qcom.MessageReadRecord,
						tlv.Bytes(0x10, []byte{0x90, 0x00}),
						tlv.Bytes(0x11, encodeLengthPrefixed(tlvTextRecord("sip:alice@ims.example.com", 32))),
					),
				},
				{
					check: func(req qcom.Request) {
						if req.MessageID != qcom.MessageAuthenticate {
							t.Fatalf("messageID = 0x%04X, want 0x%04X", req.MessageID, qcom.MessageAuthenticate)
						}
						assertTLV(t, req.TLVs, 0x01, []byte{byte(SessionPrimaryGWProvisioning), 0x00})
						assertTLV(t, req.TLVs, 0x02, []byte{
							byte(AuthContext3G),
							0x22, 0x00,
							0x10, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01,
							0x10, 0x02, 0x02, 0x02, 0x02, 0x02, 0x02, 0x02, 0x02, 0x02, 0x02, 0x02, 0x02, 0x02, 0x02, 0x02, 0x02,
						})
					},
					resp: successResponse(qcom.MessageAuthenticate,
						tlv.Bytes(0x10, []byte{0x90, 0x00}),
						tlv.Bytes(0x11, encodeLengthPrefixed([]byte{0xDC, 0x00})),
					),
				},
			},
		},
		slot:     1,
		clientID: 7,
	}

	attrs, err := reader.FileAttributes(context.Background(), File{
		Session: SessionPrimaryGWProvisioning,
		Path:    []byte{0x3F, 0x00, 0x2F, 0xE2},
	})
	if err != nil {
		t.Fatalf("FileAttributes() error = %v", err)
	}
	if attrs.FileStructure != FileStructureTransparent || attrs.FileSize != 10 {
		t.Fatalf("FileAttributes() = %+v", attrs)
	}

	imsiRaw, err := reader.ReadTransparent(context.Background(), TransparentRead{
		File:   File{Session: SessionPrimaryGWProvisioning, Path: []byte{0x3F, 0x00, 0x7F, 0xFF, 0x6F, 0x07}},
		Length: 9,
	})
	if err != nil {
		t.Fatalf("ReadTransparent() error = %v", err)
	}
	if !bytes.Equal(imsiRaw, []byte{0x08, 0x09, 0x10, 0x10, 0x10, 0x32, 0x54, 0x76, 0x98}) {
		t.Fatalf("ReadTransparent() = %X", imsiRaw)
	}

	impuRaw, err := reader.ReadRecord(context.Background(), RecordRead{
		File:   File{Session: SessionNonProvisioningSlot1, AID: isimAID, Path: []byte{0x6F, 0x04}},
		Record: 1,
		Length: 32,
	})
	if err != nil {
		t.Fatalf("ReadRecord() error = %v", err)
	}
	if !bytes.Equal(impuRaw, tlvTextRecord("sip:alice@ims.example.com", 32)) {
		t.Fatalf("ReadRecord() = %X", impuRaw)
	}

	auth, err := reader.Authenticate(context.Background(), AuthenticateRequest{
		Session: SessionPrimaryGWProvisioning,
		Context: AuthContext3G,
		Rand:    bytes.Repeat([]byte{0x01}, 16),
		AUTN:    bytes.Repeat([]byte{0x02}, 16),
	})
	if err != nil {
		t.Fatalf("Authenticate() error = %v", err)
	}
	if !bytes.Equal(auth, []byte{0xDC, 0x00}) {
		t.Fatalf("Authenticate() = %X, want DC00", auth)
	}
}

func TestReaderAuthenticateUsesISIMContext(t *testing.T) {
	isimAID := []byte{0xA0, 0x00, 0x00, 0x00, 0x87, 0x10, 0x04}
	reader := &Reader{
		transport: &fakeTransport{
			t: t,
			calls: []transportCall{{
				check: func(req qcom.Request) {
					if req.MessageID != qcom.MessageAuthenticate {
						t.Fatalf("messageID = 0x%04X, want 0x%04X", req.MessageID, qcom.MessageAuthenticate)
					}
					assertTLV(t, req.TLVs, 0x01, append([]byte{byte(SessionCardSlot1), byte(len(isimAID))}, isimAID...))
					assertTLV(t, req.TLVs, 0x02, []byte{
						byte(AuthContextIMSAKA),
						0x22, 0x00,
						0x10, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01,
						0x10, 0x02, 0x02, 0x02, 0x02, 0x02, 0x02, 0x02, 0x02, 0x02, 0x02, 0x02, 0x02, 0x02, 0x02, 0x02, 0x02,
					})
				},
				resp: successResponse(qcom.MessageAuthenticate,
					tlv.Bytes(0x10, []byte{0x90, 0x00}),
					tlv.Bytes(0x11, encodeLengthPrefixed([]byte{0xDC, 0x00})),
				),
			}},
		},
		slot:     1,
		clientID: 7,
	}

	auth, err := reader.Authenticate(context.Background(), AuthenticateRequest{
		Session: SessionCardSlot1,
		AID:     isimAID,
		Context: AuthContextIMSAKA,
		Rand:    bytes.Repeat([]byte{0x01}, 16),
		AUTN:    bytes.Repeat([]byte{0x02}, 16),
	})
	if err != nil {
		t.Fatalf("Authenticate() error = %v", err)
	}
	if !bytes.Equal(auth, []byte{0xDC, 0x00}) {
		t.Fatalf("Authenticate() = %X, want DC00", auth)
	}
}

func TestReaderSMSPPDownloadUsesCATEnvelope(t *testing.T) {
	reader := &Reader{
		transport: &fakeTransport{
			t: t,
			calls: []transportCall{
				{
					check: func(req qcom.Request) {
						if req.Service != qcom.ServiceControl || req.MessageID != qcom.MessageGetVersionInfo {
							t.Fatalf("request = service %#x message 0x%04X, want service version info", req.Service, req.MessageID)
						}
					},
					resp: successResponse(qcom.MessageGetVersionInfo, tlv.Bytes(0x01, encodeServiceVersions(
						serviceVersion{Service: qcom.ServiceCAT2, Major: 2, Minor: 24},
					))),
				},
				{
					check: func(req qcom.Request) {
						if req.Service != qcom.ServiceControl || req.MessageID != qcom.MessageAllocateClientID {
							t.Fatalf("request = service %#x message 0x%04X, want CAT2 client allocation", req.Service, req.MessageID)
						}
						assertTLV(t, req.TLVs, 0x01, []byte{byte(qcom.ServiceCAT2)})
					},
					resp: successResponse(qcom.MessageAllocateClientID, tlv.Bytes(0x01, []byte{byte(qcom.ServiceCAT2), 9})),
				},
				{
					check: func(req qcom.Request) {
						if req.Service != qcom.ServiceCAT2 || req.ClientID != 9 || req.MessageID != qcom.MessageSendEnvelope {
							t.Fatalf("request = service %#x client %d message 0x%04X, want CAT2 envelope", req.Service, req.ClientID, req.MessageID)
						}
						assertTLV(t, req.TLVs, 0x01, []byte{
							0x09, 0x00,
							0x10, 0x00,
							0xD1, 0x0E,
							0x82, 0x02, 0x83, 0x81,
							0x86, 0x03, 0x91, 0x21, 0x43,
							0x8B, 0x03, 0x00, 0x7F, 0xF6,
						})
						assertTLV(t, req.TLVs, 0x10, []byte{0x01})
					},
					resp: successResponse(qcom.MessageSendEnvelope, tlv.Bytes(0x10, []byte{0x90, 0x00, 0x00})),
				},
			},
		},
		slot:     1,
		clientID: 7,
	}

	got, err := reader.SendEnvelope(context.Background(), smsPPEnvelope())
	if err != nil {
		t.Fatalf("SendEnvelope() error = %v", err)
	}
	if got.SW1 != 0x90 || got.SW2 != 0x00 {
		t.Fatalf("SendEnvelope() status = %02X%02X, want 9000", got.SW1, got.SW2)
	}
}

func TestReaderSMSPPDownloadUsesCATWhenOnlyCATIsExposed(t *testing.T) {
	reader := &Reader{
		transport: &fakeTransport{
			t: t,
			calls: []transportCall{
				{
					check: func(req qcom.Request) {
						if req.Service != qcom.ServiceControl || req.MessageID != qcom.MessageGetVersionInfo {
							t.Fatalf("request = service %#x message 0x%04X, want service version info", req.Service, req.MessageID)
						}
					},
					resp: successResponse(qcom.MessageGetVersionInfo, tlv.Bytes(0x01, encodeServiceVersions(
						serviceVersion{Service: qcom.ServiceCAT, Major: 1, Minor: 0},
					))),
				},
				{
					check: func(req qcom.Request) {
						if req.Service != qcom.ServiceControl || req.MessageID != qcom.MessageAllocateClientID {
							t.Fatalf("request = service %#x message 0x%04X, want CAT client allocation", req.Service, req.MessageID)
						}
						assertTLV(t, req.TLVs, 0x01, []byte{byte(qcom.ServiceCAT)})
					},
					resp: successResponse(qcom.MessageAllocateClientID, tlv.Bytes(0x01, []byte{byte(qcom.ServiceCAT), 10})),
				},
				{
					check: func(req qcom.Request) {
						if req.Service != qcom.ServiceCAT || req.ClientID != 10 || req.MessageID != qcom.MessageSendEnvelope {
							t.Fatalf("request = service %#x client %d message 0x%04X, want CAT envelope", req.Service, req.ClientID, req.MessageID)
						}
						assertTLV(t, req.TLVs, 0x01, []byte{
							0x09, 0x00,
							0x10, 0x00,
							0xD1, 0x0E,
							0x82, 0x02, 0x83, 0x81,
							0x86, 0x03, 0x91, 0x21, 0x43,
							0x8B, 0x03, 0x00, 0x7F, 0xF6,
						})
						if _, ok := tlv.Value(req.TLVs, 0x10); ok {
							t.Fatal("CAT envelope request includes slot TLV 0x10, want CAT1 request without slot")
						}
					},
					resp: successResponse(qcom.MessageSendEnvelope, tlv.Bytes(0x10, []byte{0x90, 0x00, 0x00})),
				},
			},
		},
		slot:     1,
		clientID: 7,
	}

	got, err := reader.SendEnvelope(context.Background(), smsPPEnvelope())
	if err != nil {
		t.Fatalf("SendEnvelope() error = %v", err)
	}
	if got.SW1 != 0x90 || got.SW2 != 0x00 {
		t.Fatalf("SendEnvelope() status = %02X%02X, want 9000", got.SW1, got.SW2)
	}
	if reader.catService != qcom.ServiceCAT || reader.catClientID != 10 {
		t.Fatalf("CAT client = service %#x client %d, want CAT client 10", reader.catService, reader.catClientID)
	}
}

func TestReaderSMSPPDownloadDoesNotFallbackAfterCAT2EnvelopeError(t *testing.T) {
	reader := &Reader{
		transport: &fakeTransport{
			t: t,
			calls: []transportCall{
				{
					check: func(req qcom.Request) {
						if req.Service != qcom.ServiceControl || req.MessageID != qcom.MessageGetVersionInfo {
							t.Fatalf("request = service %#x message 0x%04X, want service version info", req.Service, req.MessageID)
						}
					},
					resp: successResponse(qcom.MessageGetVersionInfo, tlv.Bytes(0x01, encodeServiceVersions(
						serviceVersion{Service: qcom.ServiceCAT2, Major: 2, Minor: 24},
						serviceVersion{Service: qcom.ServiceCAT, Major: 1, Minor: 0},
					))),
				},
				{
					check: func(req qcom.Request) {
						if req.Service != qcom.ServiceControl || req.MessageID != qcom.MessageAllocateClientID {
							t.Fatalf("request = service %#x message 0x%04X, want CAT2 client allocation", req.Service, req.MessageID)
						}
						assertTLV(t, req.TLVs, 0x01, []byte{byte(qcom.ServiceCAT2)})
					},
					resp: successResponse(qcom.MessageAllocateClientID, tlv.Bytes(0x01, []byte{byte(qcom.ServiceCAT2), 9})),
				},
				{
					check: func(req qcom.Request) {
						if req.Service != qcom.ServiceCAT2 || req.ClientID != 9 || req.MessageID != qcom.MessageSendEnvelope {
							t.Fatalf("request = service %#x client %d message 0x%04X, want CAT2 envelope", req.Service, req.ClientID, req.MessageID)
						}
					},
					resp: errorResponse(qcom.MessageSendEnvelope, qcom.QMIErrorInvalidOperation),
				},
			},
		},
		slot:     1,
		clientID: 7,
	}

	if _, err := reader.SendEnvelope(context.Background(), smsPPEnvelope()); err == nil || !strings.Contains(err.Error(), "Invalid operation") {
		t.Fatalf("SendEnvelope() error = %v, want Invalid operation", err)
	}
}

func TestEnsureSlotActivated(t *testing.T) {
	tests := []struct {
		name    string
		slot    uint8
		ctx     func() context.Context
		calls   []transportCall
		wantErr string
	}{
		{
			name: "already active",
			slot: 2,
			ctx:  context.Background,
			calls: []transportCall{
				{resp: successResponse(qcom.MessageGetSlotStatus, tlv.Bytes(0x10, encodeSlotStatus(2)))},
			},
		},
		{
			name: "switch then ready",
			slot: 2,
			ctx:  context.Background,
			calls: []transportCall{
				{resp: successResponse(qcom.MessageGetSlotStatus, tlv.Bytes(0x10, encodeSlotStatus(1)))},
				{
					check: func(req qcom.Request) {
						assertTLV(t, req.TLVs, 0x01, []byte{0x01})
						assertTLV(t, req.TLVs, 0x02, []byte{0x02, 0x00, 0x00, 0x00})
					},
					resp: successResponse(qcom.MessageSwitchSlot),
				},
				{resp: successResponse(qcom.MessageGetCardStatus, tlv.Bytes(0x10, encodeCardStatus(false)))},
				{resp: successResponse(qcom.MessageGetCardStatus, tlv.Bytes(0x10, encodeCardStatus(true)))},
			},
		},
		{
			name: "unsupported get slot status",
			slot: 1,
			ctx:  context.Background,
			calls: []transportCall{
				{resp: errorResponse(qcom.MessageGetSlotStatus, qcom.QMIErrorNotSupported)},
			},
		},
		{
			name: "timeout waiting for app readiness",
			slot: 2,
			ctx: func() context.Context {
				ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
				t.Cleanup(cancel)
				return ctx
			},
			calls: []transportCall{
				{resp: successResponse(qcom.MessageGetSlotStatus, tlv.Bytes(0x10, encodeSlotStatus(1)))},
				{resp: successResponse(qcom.MessageSwitchSlot)},
				{resp: successResponse(qcom.MessageGetCardStatus, tlv.Bytes(0x10, encodeCardStatus(false)))},
			},
			wantErr: "waiting for card readiness",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := &Reader{
				transport: &fakeTransport{t: t, calls: tt.calls},
				slot:      tt.slot,
				clientID:  7,
			}

			err := reader.ActivateSlot(tt.ctx())
			switch {
			case tt.wantErr == "":
				if err != nil {
					t.Fatalf("ActivateSlot() error = %v", err)
				}
			case err == nil || !strings.Contains(err.Error(), tt.wantErr):
				t.Fatalf("ActivateSlot() error = %v, want text %q", err, tt.wantErr)
			}
		})
	}
}

func TestReaderCloseIsIdempotent(t *testing.T) {
	transport := &fakeTransport{
		t: t,
		calls: []transportCall{
			{
				check: func(req qcom.Request) {
					if req.Service != qcom.ServiceControl {
						t.Fatalf("Service = %v, want %v", req.Service, qcom.ServiceControl)
					}
					if req.ClientID != 0 {
						t.Fatalf("ClientID = %d, want 0", req.ClientID)
					}
					if req.MessageID != qcom.MessageReleaseClientID {
						t.Fatalf("qcom.MessageID = 0x%04X, want 0x%04X", req.MessageID, qcom.MessageReleaseClientID)
					}
					assertTLV(t, req.TLVs, 0x01, []byte{byte(qcom.ServiceUIM), 0x07})
				},
				resp: qcom.Response{
					Service:   qcom.ServiceControl,
					MessageID: qcom.MessageReleaseClientID,
					TLVs: tlv.TLVs{
						tlv.Bytes(qmiTLVResult, []byte{0x00, 0x00, 0x00, 0x00}),
					},
				},
			},
		},
	}
	reader := &Reader{
		transport: transport,
		slot:      1,
		clientID:  7,
	}

	if err := reader.Close(); err != nil {
		t.Fatalf("first Close() error = %v", err)
	}
	if err := reader.Close(); err != nil {
		t.Fatalf("second Close() error = %v", err)
	}
	if transport.idx != 1 {
		t.Fatalf("Do() calls = %d, want 1", transport.idx)
	}
	if transport.closeCalls != 1 {
		t.Fatalf("Close() calls = %d, want 1", transport.closeCalls)
	}
	if reader.clientID != 0 {
		t.Fatalf("ClientID = %d, want 0", reader.clientID)
	}
	if reader.transport != nil {
		t.Fatal("Transport was not cleared")
	}
}

func TestReaderRejectsRequestsAfterClose(t *testing.T) {
	transport := &fakeTransport{
		t: t,
		calls: []transportCall{
			{
				check: func(req qcom.Request) {
					if req.MessageID != qcom.MessageReleaseClientID {
						t.Fatalf("MessageID = 0x%04X, want release client ID", req.MessageID)
					}
				},
				resp: successResponse(qcom.MessageReleaseClientID),
			},
		},
	}
	reader := &Reader{
		transport: transport,
		slot:      1,
		clientID:  7,
	}

	if err := reader.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if _, err := reader.CardStatus(context.Background()); err == nil || !strings.Contains(err.Error(), "closed") {
		t.Fatalf("CardStatus() after Close() error = %v, want closed", err)
	}
	if transport.idx != 1 {
		t.Fatalf("Do() calls = %d, want only release client ID", transport.idx)
	}
}

func assertTLV(t *testing.T, tlvs tlv.TLVs, typ byte, want []byte) {
	t.Helper()
	got, ok := tlv.Value(tlvs, typ)
	if !ok {
		t.Fatalf("TLV 0x%02X is missing", typ)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("TLV 0x%02X = % X, want % X", typ, got, want)
	}
}

func assertRequestTimeout(t *testing.T, req qcom.Request, want time.Duration) {
	t.Helper()
	if req.Timeout != want {
		t.Fatalf("Timeout = %v, want %v", req.Timeout, want)
	}
}

func successResponse(id qcom.MessageID, tlvs ...tlv.TLV) qcom.Response {
	return qcom.Response{
		Service:   qcom.ServiceUIM,
		ClientID:  7,
		MessageID: id,
		TLVs: append(tlv.TLVs{
			tlv.Bytes(qmiTLVResult, []byte{0x00, 0x00, 0x00, 0x00}),
		}, tlvs...),
	}
}

func errorResponse(id qcom.MessageID, err qcom.QMIError) qcom.Response {
	return qcom.Response{
		Service:   qcom.ServiceUIM,
		ClientID:  7,
		MessageID: id,
		TLVs: tlv.TLVs{
			tlv.Bytes(qmiTLVResult, []byte{0x01, 0x00, byte(err), byte(uint16(err) >> 8)}),
		},
	}
}

func encodeLengthPrefixed(data []byte) []byte {
	return append(binary.LittleEndian.AppendUint16(nil, uint16(len(data))), data...)
}

func encodeServiceVersions(versions ...serviceVersion) []byte {
	out := []byte{byte(len(versions))}
	for _, version := range versions {
		out = append(out, byte(version.Service))
		out = binary.LittleEndian.AppendUint16(out, version.Major)
		out = binary.LittleEndian.AppendUint16(out, version.Minor)
	}
	return out
}

func encodeFileAttributes(fileSize, fileID uint16, fileType byte, recordSize, recordCount uint16, raw []byte) []byte {
	value := binary.LittleEndian.AppendUint16(nil, fileSize)
	value = binary.LittleEndian.AppendUint16(value, fileID)
	value = append(value, fileType)
	value = binary.LittleEndian.AppendUint16(value, recordSize)
	value = binary.LittleEndian.AppendUint16(value, recordCount)
	for range 5 {
		value = append(value, 0x00)
		value = binary.LittleEndian.AppendUint16(value, 0x0000)
	}
	value = binary.LittleEndian.AppendUint16(value, uint16(len(raw)))
	value = append(value, raw...)
	return value
}

func encodeSlotStatus(activeSlot uint8) []byte {
	value := []byte{0x02}
	for slot := uint8(1); slot <= 2; slot++ {
		value = binary.LittleEndian.AppendUint32(value, 2)
		slotState := uint32(0)
		if slot == activeSlot {
			slotState = 1
		}
		value = binary.LittleEndian.AppendUint32(value, slotState)
		value = append(value, 0x01, 0x00)
	}
	return value
}

func encodeCardStatus(ready bool) []byte {
	value := make([]byte, 0, 64)
	value = append(value, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00)
	value = append(value, 0x01)
	value = append(value, 0x01)
	value = append(value, 0x00, 0x00, 0x00, 0x00)
	value = append(value, 0x01)
	state := byte(0x01)
	if ready {
		state = 0x07
	}
	value = append(value, 0x02, state)
	value = append(value, make([]byte, 28)...)
	return value
}

func tlvTextRecord(value string, size int) []byte {
	record := append([]byte{0x80, byte(len(value))}, []byte(value)...)
	for len(record) < size {
		record = append(record, 0xFF)
	}
	return record
}

func smsPPEnvelope() []byte {
	return []byte{
		0xD1, 0x0E,
		0x82, 0x02, 0x83, 0x81,
		0x86, 0x03, 0x91, 0x21, 0x43,
		0x8B, 0x03, 0x00, 0x7F, 0xF6,
	}
}
