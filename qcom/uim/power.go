package uim

import (
	"context"
	"errors"
	"fmt"

	"github.com/damonto/uicc-go/qcom"
	"github.com/damonto/uicc-go/qcom/tlv"
)

func (r *Reader) Reset(ctx context.Context) error {
	resp, err := r.request(ctx, qcom.MessageReset, nil)
	if err != nil {
		return fmt.Errorf("resetting QMI UIM service: %w", err)
	}
	if err := resultOK(resp); err != nil {
		return fmt.Errorf("resetting QMI UIM service: %w", err)
	}
	return nil
}

func (r *Reader) PowerOffSIM(ctx context.Context, slot uint8) error {
	if slot == 0 {
		return errors.New("powering off QMI UIM SIM: slot is zero")
	}

	resp, err := r.request(ctx, qcom.MessagePowerOffSIM, tlv.TLVs{
		tlv.Uint(0x01, slot),
	})
	if err != nil {
		return fmt.Errorf("powering off QMI UIM SIM slot %d: %w", slot, err)
	}
	if err := resultOK(resp); err != nil {
		return fmt.Errorf("powering off QMI UIM SIM slot %d: %w", slot, err)
	}
	return nil
}

func (r *Reader) PowerOnSIM(ctx context.Context, req PowerOnSIMRequest) error {
	if req.Slot == 0 {
		return errors.New("powering on QMI UIM SIM: slot is zero")
	}

	tlvs := tlv.TLVs{tlv.Uint(0x01, req.Slot)}
	if req.IgnoreHotSwapSwitch {
		tlvs = append(tlvs, tlv.Uint(0x10, uint8(1)))
	}

	resp, err := r.request(ctx, qcom.MessagePowerOnSIM, tlvs)
	if err != nil {
		return fmt.Errorf("powering on QMI UIM SIM slot %d: %w", req.Slot, err)
	}
	if err := resultOK(resp); err != nil {
		return fmt.Errorf("powering on QMI UIM SIM slot %d: %w", req.Slot, err)
	}
	return nil
}

func (r *Reader) ChangeProvisioningSession(ctx context.Context, req ChangeProvisioningSessionRequest) error {
	if len(req.AID) > 0xff {
		return fmt.Errorf("changing QMI UIM provisioning session: AID length %d exceeds 255", len(req.AID))
	}

	activate := uint8(0)
	if req.Activate {
		activate = 1
	}
	tlvs := tlv.TLVs{
		tlv.Bytes(0x01, []byte{byte(req.Session), activate}),
	}
	if req.Slot != 0 || len(req.AID) > 0 {
		app := []byte{req.Slot, byte(len(req.AID))}
		app = append(app, req.AID...)
		tlvs = append(tlvs, tlv.Bytes(0x10, app))
	}

	resp, err := r.request(ctx, qcom.MessageChangeProvisioningSession, tlvs)
	if err != nil {
		return fmt.Errorf("changing QMI UIM provisioning session: %w", err)
	}
	if err := resultOK(resp); err != nil {
		return fmt.Errorf("changing QMI UIM provisioning session: %w", err)
	}
	return nil
}
