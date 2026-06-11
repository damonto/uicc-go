package uim

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/damonto/uicc-go/qcom"
	"github.com/damonto/uicc-go/qcom/tlv"
)

func (r *Reader) activateSlot(ctx context.Context) error {
	status, err := r.SlotStatus(ctx)
	if err != nil {
		if errors.Is(err, qcom.QMIErrorNotSupported) {
			return nil
		}
		return fmt.Errorf("activating slot %d: %w", r.slot, err)
	}
	if status.ActiveSlot == r.slot {
		return nil
	}
	if err := r.SwitchSlot(ctx, 1, uint32(r.slot)); err != nil {
		return fmt.Errorf("activating slot %d: %w", r.slot, err)
	}
	if err := r.waitForSlotReady(ctx); err != nil {
		return fmt.Errorf("activating slot %d: %w", r.slot, err)
	}
	return nil
}

func (r *Reader) SlotStatus(ctx context.Context) (SlotStatus, error) {
	resp, err := r.requestWithTimeout(ctx, qcom.MessageGetSlotStatus, nil, slotStatusTimeout)
	if err != nil {
		return SlotStatus{}, err
	}
	if err := resultOK(resp); err != nil {
		return SlotStatus{}, err
	}
	return decodeSlotStatus(resp)
}

func (r *Reader) SwitchSlot(ctx context.Context, logicalSlot uint8, physicalSlot uint32) error {
	resp, err := r.request(ctx, qcom.MessageSwitchSlot, tlv.TLVs{
		tlv.Uint(0x01, logicalSlot),
		tlv.Uint(0x02, physicalSlot),
	})
	if err != nil {
		return err
	}
	return resultOK(resp)
}

func (r *Reader) waitForSlotReady(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, slotReadyTimeout)
	defer cancel()

	for {
		status, err := r.CardStatus(ctx)
		if err == nil && status.Ready() {
			return nil
		}
		if ctx.Err() != nil {
			if err != nil {
				return fmt.Errorf("waiting for card readiness: %w", err)
			}
			return errors.New("waiting for card readiness: timeout")
		}

		timer := time.NewTimer(slotPollInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			if err != nil {
				return fmt.Errorf("waiting for card readiness: %w", err)
			}
			return errors.New("waiting for card readiness: timeout")
		case <-timer.C:
		}
	}
}

func (r *Reader) CardStatus(ctx context.Context) (CardStatus, error) {
	resp, err := r.request(ctx, qcom.MessageGetCardStatus, nil)
	if err != nil {
		return CardStatus{}, err
	}
	if err := resultOK(resp); err != nil {
		return CardStatus{}, err
	}
	return decodeCardStatus(resp)
}
