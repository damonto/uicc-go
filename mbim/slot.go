package mbim

import (
	"context"
	"errors"
	"fmt"
	"time"
)

const (
	slotPollInterval = 500 * time.Millisecond
	slotReadyTimeout = 5 * time.Second
)

func (r *Reader) ensureSlotActivated(ctx context.Context) error {
	slot, err := r.currentActivatedSlot(ctx)
	if err != nil {
		if errors.Is(err, StatusNoDeviceSupport) {
			return nil
		}
		return fmt.Errorf("activating MBIM slot %d: %w", r.slot+1, err)
	}
	if slot == r.slot {
		return nil
	}
	if err := r.activateSlot(ctx, r.slot); err != nil {
		return fmt.Errorf("activating MBIM slot %d: %w", r.slot+1, err)
	}
	if err := r.waitForSlotReady(ctx); err != nil {
		return fmt.Errorf("activating MBIM slot %d: %w", r.slot+1, err)
	}
	return nil
}

func (r *Reader) currentActivatedSlot(ctx context.Context) (uint32, error) {
	request := DeviceSlotMappingsRequest{TransactionID: r.nextTransactionID()}
	if err := request.Request().Transmit(ctx, r.conn); err != nil {
		return 0, err
	}
	if len(request.Response.SlotMappings) == 0 {
		return 0, errors.New("reading MBIM slot mappings: mapping is empty")
	}
	return request.Response.SlotMappings[0].Slot, nil
}

func (r *Reader) activateSlot(ctx context.Context, slot uint32) error {
	request := DeviceSlotMappingsRequest{
		TransactionID: r.nextTransactionID(),
		SlotMappings:  []SlotMapping{{Slot: slot}},
	}
	return request.Request().Transmit(ctx, r.conn)
}

func (r *Reader) waitForSlotReady(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, slotReadyTimeout)
	defer cancel()

	var lastReadyState SubscriberReadyState
	var sawReadyState bool
	for {
		request := SubscriberReadyStatusRequest{TransactionID: r.nextTransactionID()}
		err := request.Request().Transmit(ctx, r.conn)
		if err == nil {
			sawReadyState = true
			lastReadyState = request.Response.ReadyState
			if lastReadyState == SubscriberReadyStateInitialized || lastReadyState == SubscriberReadyStateNoESIMProfile {
				return nil
			}
		}
		if ctx.Err() != nil {
			if err != nil {
				return fmt.Errorf("waiting for MBIM SIM readiness: %w", err)
			}
			if sawReadyState {
				return fmt.Errorf("waiting for MBIM SIM readiness: last ready state %#x", lastReadyState)
			}
			return errors.New("waiting for MBIM SIM readiness: timeout")
		}

		timer := time.NewTimer(slotPollInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
		case <-timer.C:
		}
	}
}
