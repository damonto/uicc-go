package uim

import (
	"bytes"
	"context"
	"encoding/binary"
	"testing"
	"time"

	"github.com/damonto/uicc-go/qcom"
	"github.com/damonto/uicc-go/qcom/tlv"
)

func TestMonitorTLVEncoding(t *testing.T) {
	tests := []struct {
		name    string
		build   func() (tlv.TLVs, error)
		check   func(*testing.T, tlv.TLVs)
		wantErr bool
	}{
		{
			name: "slot register",
			build: func() (tlv.TLVs, error) {
				return registerEventsTLVs(eventRegistrationPhysicalSlotStatus), nil
			},
			check: func(t *testing.T, tlvs tlv.TLVs) {
				t.Helper()
				assertTLV(t, tlvs, 0x01, binary.LittleEndian.AppendUint32(nil, eventRegistrationPhysicalSlotStatus))
			},
		},
		{
			name: "refresh files",
			build: func() (tlv.TLVs, error) {
				return refreshRegisterTLVs(RefreshRegisterRequest{
					Session:     SessionCardSlot1,
					AID:         []byte{0xA0, 0x00},
					VoteForInit: true,
					Files: []RefreshFile{{
						Path: []byte{0x3F, 0x00, 0x7F, 0xFF, 0x6F, 0xAD},
					}},
				}, true)
			},
			check: func(t *testing.T, tlvs tlv.TLVs) {
				t.Helper()
				assertTLV(t, tlvs, 0x01, []byte{byte(SessionCardSlot1), 0x02, 0xA0, 0x00})
				assertTLV(t, tlvs, 0x02, []byte{
					0x01, 0x01,
					0x01, 0x00,
					0xAD, 0x6F,
					0x04, 0x00, 0x3F, 0xFF, 0x7F,
				})
			},
		},
		{
			name: "refresh files unregister",
			build: func() (tlv.TLVs, error) {
				return refreshRegisterTLVs(RefreshRegisterRequest{
					Session: SessionCardSlot1,
					Files:   []RefreshFile{{Path: []byte{0x3F, 0x00, 0x2F, 0xE2}}},
				}, false)
			},
			check: func(t *testing.T, tlvs tlv.TLVs) {
				t.Helper()
				assertTLV(t, tlvs, 0x02, []byte{
					0x00, 0x00,
					0x01, 0x00,
					0xE2, 0x2F,
					0x02, 0x00, 0x3F,
				})
			},
		},
		{
			name: "refresh all",
			build: func() (tlv.TLVs, error) {
				return refreshRegisterAllTLVs(SessionCardSlot1, nil, true)
			},
			check: func(t *testing.T, tlvs tlv.TLVs) {
				t.Helper()
				assertTLV(t, tlvs, 0x01, []byte{byte(SessionCardSlot1), 0x00})
				assertTLV(t, tlvs, 0x02, []byte{0x01})
			},
		},
		{
			name: "refresh complete",
			build: func() (tlv.TLVs, error) {
				return refreshCompleteTLVs(SessionCardSlot1, []byte{0xA0, 0x00}, true), nil
			},
			check: func(t *testing.T, tlvs tlv.TLVs) {
				t.Helper()
				assertTLV(t, tlvs, 0x01, []byte{byte(SessionCardSlot1), 0x02, 0xA0, 0x00})
				assertTLV(t, tlvs, 0x02, []byte{0x01})
			},
		},
		{
			name: "reject odd path",
			build: func() (tlv.TLVs, error) {
				return refreshRegisterTLVs(RefreshRegisterRequest{
					Files: []RefreshFile{{Path: []byte{0x3F}}},
				}, true)
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.build()
			if tt.wantErr {
				if err == nil {
					t.Fatal("build() error = nil, want error")
				}
				return
			}
			if err != nil {
				t.Fatalf("build() error = %v", err)
			}
			tt.check(t, got)
		})
	}
}

func TestDecodeRefreshEvent(t *testing.T) {
	eventValue := []byte{
		byte(RefreshStageStart),
		byte(RefreshModeFCN),
		byte(SessionCardSlot1),
		0x02, 0xA0, 0x00,
		0x01, 0x00,
		0xAD, 0x6F,
		0x04, 0x00, 0x3F, 0xFF, 0x7F,
	}

	got, err := decodeRefreshEvent(tlv.TLVs{tlv.Bytes(0x10, eventValue)})
	if err != nil {
		t.Fatalf("decodeRefreshEvent() error = %v", err)
	}
	if got.Stage != RefreshStageStart || got.Mode != RefreshModeFCN || got.Session != SessionCardSlot1 {
		t.Fatalf("decodeRefreshEvent() = %+v", got)
	}
	if !bytes.Equal(got.AID, []byte{0xA0, 0x00}) {
		t.Fatalf("AID = % X, want A0 00", got.AID)
	}
	if len(got.Files) != 1 {
		t.Fatalf("Files length = %d, want 1", len(got.Files))
	}
	if got.Files[0].FileID != 0x6FAD || !bytes.Equal(got.Files[0].Path, []byte{0x3F, 0x00, 0x7F, 0xFF, 0x6F, 0xAD}) {
		t.Fatalf("Files[0] = %+v", got.Files[0])
	}
}

func TestWatchSlotStatus(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	transport := &fakeIndicationTransport{
		fakeTransport: fakeTransport{
			t: t,
			calls: []transportCall{
				{
					check: func(req qcom.Request) {
						if req.MessageID != qcom.MessageRegisterEvents {
							t.Fatalf("MessageID = 0x%04X, want register events", req.MessageID)
						}
						assertTLV(t, req.TLVs, 0x01, binary.LittleEndian.AppendUint32(nil, eventRegistrationPhysicalSlotStatus))
					},
					resp: successResponse(qcom.MessageRegisterEvents),
				},
				{
					check: func(req qcom.Request) {
						assertTLV(t, req.TLVs, 0x01, []byte{0, 0, 0, 0})
					},
					resp: successResponse(qcom.MessageRegisterEvents),
				},
			},
		},
	}
	reader := &Reader{transport: transport, slot: 1, clientID: 7}

	statuses, err := reader.WatchSlotStatus(ctx)
	if err != nil {
		t.Fatalf("WatchSlotStatus() error = %v", err)
	}
	transport.emit(qcom.Indication{
		Service:   qcom.ServiceUIM,
		ClientID:  7,
		MessageID: qcom.MessageSlotStatus,
		TLVs: tlv.TLVs{
			tlv.Bytes(0x10, encodeSlotStatus(1)),
			tlv.Bytes(0x11, encodeSlotInformation()),
		},
	})

	select {
	case status := <-statuses:
		if status.ActiveSlot != 1 {
			t.Fatalf("ActiveSlot = %d, want 1", status.ActiveSlot)
		}
		if status.Slots[1].CardProtocol != CardProtocolUICC || !status.Slots[1].IsEUICC {
			t.Fatalf("Slots[1] = %+v, want UICC eUICC slot information", status.Slots[1])
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for slot status")
	}

	cancel()
	transport.waitCalls(t, 2)
}

func TestWatchRefreshFilesCompletesStartEvent(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	eventValue := []byte{
		byte(RefreshStageStart),
		byte(RefreshModeFCN),
		byte(SessionCardSlot1),
		0x00,
		0x01, 0x00,
		0xAD, 0x6F,
		0x04, 0x00, 0x3F, 0xFF, 0x7F,
	}
	transport := &fakeIndicationTransport{
		fakeTransport: fakeTransport{
			t: t,
			calls: []transportCall{
				{
					check: func(req qcom.Request) {
						if req.MessageID != qcom.MessageRefreshRegister {
							t.Fatalf("MessageID = 0x%04X, want refresh register", req.MessageID)
						}
						assertTLV(t, req.TLVs, 0x02, []byte{
							0x01, 0x00,
							0x01, 0x00,
							0xAD, 0x6F,
							0x04, 0x00, 0x3F, 0xFF, 0x7F,
						})
					},
					resp: successResponse(qcom.MessageRefreshRegister),
				},
				{
					check: func(req qcom.Request) {
						if req.MessageID != qcom.MessageRefreshComplete {
							t.Fatalf("MessageID = 0x%04X, want refresh complete", req.MessageID)
						}
						assertTLV(t, req.TLVs, 0x01, []byte{byte(SessionCardSlot1), 0x00})
						assertTLV(t, req.TLVs, 0x02, []byte{0x01})
					},
					resp: successResponse(qcom.MessageRefreshComplete),
				},
				{
					check: func(req qcom.Request) {
						if req.MessageID != qcom.MessageRefreshRegister {
							t.Fatalf("MessageID = 0x%04X, want refresh unregister", req.MessageID)
						}
						value, ok := tlv.Value(req.TLVs, 0x02)
						if !ok || len(value) == 0 || value[0] != 0 {
							t.Fatalf("unregister info TLV = % X, want register flag 0", value)
						}
					},
					resp: successResponse(qcom.MessageRefreshRegister),
				},
			},
		},
	}
	reader := &Reader{transport: transport, slot: 1, clientID: 7}

	events, err := reader.WatchRefreshFiles(ctx, RefreshRegisterRequest{
		Session: SessionCardSlot1,
		Files: []RefreshFile{{
			Path: []byte{0x3F, 0x00, 0x7F, 0xFF, 0x6F, 0xAD},
		}},
	})
	if err != nil {
		t.Fatalf("WatchRefreshFiles() error = %v", err)
	}
	transport.emit(qcom.Indication{
		Service:   qcom.ServiceUIM,
		ClientID:  7,
		MessageID: qcom.MessageRefresh,
		TLVs:      tlv.TLVs{tlv.Bytes(0x10, eventValue)},
	})

	select {
	case event := <-events:
		if event.Stage != RefreshStageStart || event.Mode != RefreshModeFCN {
			t.Fatalf("event = %+v", event)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for refresh event")
	}

	transport.waitCalls(t, 2)
	cancel()
	transport.waitCalls(t, 3)
}

func TestWatchRefreshFilesCompletesWhenConsumerIsSlow(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	eventValue := []byte{
		byte(RefreshStageStart),
		byte(RefreshModeFCN),
		byte(SessionCardSlot1),
		0x00,
		0x00, 0x00,
	}
	calls := []transportCall{
		{
			check: func(req qcom.Request) {
				if req.MessageID != qcom.MessageRefreshRegister {
					t.Fatalf("MessageID = 0x%04X, want refresh register", req.MessageID)
				}
			},
			resp: successResponse(qcom.MessageRefreshRegister),
		},
	}
	for range 10 {
		calls = append(calls, transportCall{
			check: func(req qcom.Request) {
				if req.MessageID != qcom.MessageRefreshComplete {
					t.Fatalf("MessageID = 0x%04X, want refresh complete", req.MessageID)
				}
			},
			resp: successResponse(qcom.MessageRefreshComplete),
		})
	}
	calls = append(calls, transportCall{
		check: func(req qcom.Request) {
			if req.MessageID != qcom.MessageRefreshRegister {
				t.Fatalf("MessageID = 0x%04X, want refresh unregister", req.MessageID)
			}
		},
		resp: successResponse(qcom.MessageRefreshRegister),
	})

	transport := &fakeIndicationTransport{
		fakeTransport: fakeTransport{
			t:     t,
			calls: calls,
		},
	}
	reader := &Reader{transport: transport, slot: 1, clientID: 7}

	_, err := reader.WatchRefreshFiles(ctx, RefreshRegisterRequest{
		Session: SessionCardSlot1,
		Files: []RefreshFile{{
			Path: []byte{0x3F, 0x00, 0x2F, 0xE2},
		}},
	})
	if err != nil {
		t.Fatalf("WatchRefreshFiles() error = %v", err)
	}
	for range 10 {
		transport.emit(qcom.Indication{
			Service:   qcom.ServiceUIM,
			ClientID:  7,
			MessageID: qcom.MessageRefresh,
			TLVs:      tlv.TLVs{tlv.Bytes(0x10, eventValue)},
		})
	}

	transport.waitCalls(t, 11)
	cancel()
	transport.waitCalls(t, 12)
}

type fakeIndicationTransport struct {
	fakeTransport
	indications chan qcom.Indication
}

func (t *fakeIndicationTransport) Indications(ctx context.Context, _ qcom.ServiceType, _ uint8, _ qcom.MessageID) (<-chan qcom.Indication, error) {
	t.indications = make(chan qcom.Indication, 8)
	go func() {
		<-ctx.Done()
		close(t.indications)
	}()
	return t.indications, nil
}

func (t *fakeIndicationTransport) emit(ind qcom.Indication) {
	t.indications <- ind
}

func (t *fakeIndicationTransport) waitCalls(tb testing.TB, want int) {
	tb.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if t.callCount() >= want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	tb.Fatalf("Do() calls = %d, want at least %d", t.callCount(), want)
}
