package uim

import (
	"bytes"
	"context"
	"testing"

	"github.com/damonto/uicc-go/qcom"
	"github.com/damonto/uicc-go/qcom/tlv"
)

func TestReaderSlotPrimitives(t *testing.T) {
	transport := &fakeTransport{
		t: t,
		calls: []transportCall{
			{
				check: func(req qcom.Request) {
					if req.MessageID != qcom.MessageGetSlotStatus {
						t.Fatalf("MessageID = 0x%04X, want 0x%04X", req.MessageID, qcom.MessageGetSlotStatus)
					}
					assertRequestTimeout(t, req, slotStatusTimeout)
					if len(req.TLVs) != 0 {
						t.Fatalf("TLVs = %+v, want empty", req.TLVs)
					}
				},
				resp: successResponse(qcom.MessageGetSlotStatus, tlv.Bytes(0x10, encodeSlotStatus(2))),
			},
			{
				check: func(req qcom.Request) {
					if req.MessageID != qcom.MessageSwitchSlot {
						t.Fatalf("MessageID = 0x%04X, want 0x%04X", req.MessageID, qcom.MessageSwitchSlot)
					}
					assertRequestTimeout(t, req, DefaultRequestTimeout)
					assertTLV(t, req.TLVs, 0x01, []byte{0x01})
					assertTLV(t, req.TLVs, 0x02, []byte{0x02, 0x00, 0x00, 0x00})
				},
				resp: successResponse(qcom.MessageSwitchSlot),
			},
			{
				check: func(req qcom.Request) {
					if req.MessageID != qcom.MessageGetCardStatus {
						t.Fatalf("MessageID = 0x%04X, want 0x%04X", req.MessageID, qcom.MessageGetCardStatus)
					}
					assertRequestTimeout(t, req, DefaultRequestTimeout)
					if len(req.TLVs) != 0 {
						t.Fatalf("TLVs = %+v, want empty", req.TLVs)
					}
				},
				resp: successResponse(qcom.MessageGetCardStatus, tlv.Bytes(0x10, encodeCardStatus(true))),
			},
		},
	}
	reader := &Reader{
		transport: transport,
		slot:      2,
		clientID:  7,
	}

	slotStatus, err := reader.SlotStatus(context.Background())
	if err != nil {
		t.Fatalf("SlotStatus() error = %v", err)
	}
	if slotStatus.ActiveSlot != 2 {
		t.Fatalf("SlotStatus().ActiveSlot = %d, want 2", slotStatus.ActiveSlot)
	}
	if len(slotStatus.Slots) != 2 {
		t.Fatalf("SlotStatus().Slots length = %d, want 2", len(slotStatus.Slots))
	}
	if slotStatus.Slots[1].PhysicalSlotStatus != SlotStateActive || slotStatus.Slots[1].LogicalSlot != 1 {
		t.Fatalf("SlotStatus().Slots[1] = %+v, want active logical slot 1", slotStatus.Slots[1])
	}

	if err := reader.SwitchSlot(context.Background(), 1, 2); err != nil {
		t.Fatalf("SwitchSlot() error = %v", err)
	}

	cardStatus, err := reader.CardStatus(context.Background())
	if err != nil {
		t.Fatalf("CardStatus() error = %v", err)
	}
	if !cardStatus.Ready() {
		t.Fatal("CardStatus().Ready() = false, want true")
	}
	if len(cardStatus.Cards) != 1 {
		t.Fatalf("CardStatus().Cards length = %d, want 1", len(cardStatus.Cards))
	}
	card := cardStatus.Cards[0]
	if card.State != CardStatePresent || card.UPINState != PINStateNotInitialized || card.ErrorCode != CardErrorUnknown {
		t.Fatalf("CardStatus().Cards[0] = %+v, want present card with zero PIN/error fields", card)
	}
	if len(card.Applications) != 1 {
		t.Fatalf("CardStatus().Cards[0].Applications length = %d, want 1", len(card.Applications))
	}
	app := card.Applications[0]
	if app.Type != ApplicationTypeUSIM || app.State != ApplicationStateReady || app.PIN2State != PINStateNotInitialized {
		t.Fatalf("CardStatus().Cards[0].Applications[0] = %+v, want ready USIM app", app)
	}
	if transport.idx != len(transport.calls) {
		t.Fatalf("Do() calls = %d, want %d", transport.idx, len(transport.calls))
	}
}

func TestDecodeSlotStatusPhysicalSlotInformation(t *testing.T) {
	tests := []struct {
		name    string
		tlvs    tlv.TLVs
		check   func(*testing.T, SlotStatus)
		wantErr bool
	}{
		{
			name: "with physical slot information",
			tlvs: tlv.TLVs{
				tlv.Bytes(0x10, encodeSlotStatus(2)),
				tlv.Bytes(0x11, encodeSlotInformation()),
			},
			check: func(t *testing.T, got SlotStatus) {
				t.Helper()
				if got.ActiveSlot != 2 {
					t.Fatalf("ActiveSlot = %d, want 2", got.ActiveSlot)
				}
				slot := got.Slots[1]
				if slot.CardProtocol != CardProtocolUICC {
					t.Fatalf("CardProtocol = %d, want %d", slot.CardProtocol, CardProtocolUICC)
				}
				if slot.ValidApplications != 3 {
					t.Fatalf("ValidApplications = %d, want 3", slot.ValidApplications)
				}
				if !bytes.Equal(slot.ATR, []byte{0x3B, 0x9F}) {
					t.Fatalf("ATR = % X, want 3B 9F", slot.ATR)
				}
				if !slot.IsEUICC {
					t.Fatal("IsEUICC = false, want true")
				}
			},
		},
		{
			name: "without physical slot information",
			tlvs: tlv.TLVs{
				tlv.Bytes(0x10, encodeSlotStatus(1)),
			},
			check: func(t *testing.T, got SlotStatus) {
				t.Helper()
				slot := got.Slots[0]
				if slot.CardProtocol != CardProtocolUnknown || slot.ValidApplications != 0 || len(slot.ATR) != 0 || slot.IsEUICC {
					t.Fatalf("Slots[0] = %+v, want zero physical slot information fields", slot)
				}
			},
		},
		{
			name: "physical slot information count mismatch",
			tlvs: tlv.TLVs{
				tlv.Bytes(0x10, encodeSlotStatus(1)),
				tlv.Bytes(0x11, []byte{
					0x01,
					0x01, 0x00, 0x00, 0x00,
					0x01,
					0x01, 0x3B,
					0x00,
				}),
			},
			wantErr: true,
		},
		{
			name: "truncated physical slot information",
			tlvs: tlv.TLVs{
				tlv.Bytes(0x10, encodeSlotStatus(1)),
				tlv.Bytes(0x11, []byte{
					0x02,
					0x01, 0x00, 0x00, 0x00,
					0x01,
					0x01, 0x3B,
				}),
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := decodeSlotStatus(qcom.Response{TLVs: tt.tlvs})
			if tt.wantErr {
				if err == nil {
					t.Fatal("decodeSlotStatus() error = nil, want error")
				}
				return
			}
			if err != nil {
				t.Fatalf("decodeSlotStatus() error = %v", err)
			}
			tt.check(t, got)
		})
	}
}
