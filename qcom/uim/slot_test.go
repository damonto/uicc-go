package uim

import (
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
