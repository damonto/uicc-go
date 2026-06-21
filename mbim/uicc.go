package mbim

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"slices"

	"github.com/damonto/uicc-go/apdu"
)

func (r *Reader) ListApplications(ctx context.Context) ([]Application, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed {
		return nil, errors.New("listing MBIM applications: reader is closed")
	}

	request := ApplicationListRequest{TransactionID: r.nextTransactionID()}
	if err := request.Request().Transmit(ctx, r.conn); err != nil {
		return nil, fmt.Errorf("listing MBIM applications: %w", err)
	}

	apps := make([]Application, 0, len(request.Response.Applications))
	for _, app := range request.Response.Applications {
		if len(app.AID) == 0 {
			continue
		}
		apps = append(apps, Application{
			AID:   slices.Clone(app.AID),
			Label: app.Label,
		})
	}
	return apps, nil
}

func (r *Reader) AuthenticateAKA(ctx context.Context, rand, autn []byte) (*AuthAKAResponse, error) {
	if len(rand) != 16 {
		return nil, fmt.Errorf("authenticating MBIM AKA: RAND length %d, want 16", len(rand))
	}
	if len(autn) != 16 {
		return nil, fmt.Errorf("authenticating MBIM AKA: AUTN length %d, want 16", len(autn))
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed {
		return nil, errors.New("authenticating MBIM AKA: reader is closed")
	}

	request := AuthAKARequest{
		TransactionID: r.nextTransactionID(),
		Rand:          slices.Clone(rand),
		AUTN:          slices.Clone(autn),
	}
	if err := request.Request().Transmit(ctx, r.conn); err != nil {
		return nil, fmt.Errorf("authenticating MBIM AKA: %w", err)
	}
	return request.Response, nil
}

func (r *Reader) QueryUiccATR(ctx context.Context) ([]byte, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed {
		return nil, errors.New("querying MBIM UICC ATR: reader is closed")
	}

	request := UiccATRQueryRequest{TransactionID: r.nextTransactionID()}
	if err := request.Request().Transmit(ctx, r.conn); err != nil {
		return nil, fmt.Errorf("querying MBIM UICC ATR: %w", err)
	}
	return slices.Clone(request.Response.ATR), nil
}

func (r *Reader) OpenChannel(ctx context.Context, aid []byte) (uint32, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed {
		return 0, errors.New("opening MBIM UICC channel: reader is closed")
	}

	request := OpenChannelRequest{
		TransactionID: r.nextTransactionID(),
		ApplicationID: slices.Clone(aid),
		ChannelGroup:  uiccChannelGroupDefault,
	}
	if err := request.Request().Transmit(ctx, r.conn); err != nil {
		return 0, fmt.Errorf("opening MBIM UICC channel: %w", err)
	}
	if err := uiccStatusError("opening MBIM UICC channel", request.Response.Status); err != nil {
		return 0, err
	}
	return request.Response.Channel, nil
}

func (r *Reader) TransmitAPDU(ctx context.Context, channel uint32, command []byte) ([]byte, uint32, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed {
		return nil, 0, errors.New("transmitting MBIM UICC APDU: reader is closed")
	}

	request := APDURequest{
		TransactionID:   r.nextTransactionID(),
		Channel:         channel,
		SecureMessaging: UiccSecureMessagingNone,
		ClassByteType:   UiccClassByteTypeInterIndustry,
		Command:         slices.Clone(command),
	}
	if err := request.Request().Transmit(ctx, r.conn); err != nil {
		return nil, 0, fmt.Errorf("transmitting MBIM UICC APDU: %w", err)
	}
	return slices.Clone(request.Response.Response), request.Response.Status, nil
}

func (r *Reader) SetUiccReset(ctx context.Context, action UiccPassThroughAction) (UiccPassThroughStatus, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed {
		return 0, errors.New("setting MBIM UICC reset: reader is closed")
	}

	request := UiccResetSetRequest{
		TransactionID: r.nextTransactionID(),
		Action:        action,
	}
	if err := request.Request().Transmit(ctx, r.conn); err != nil {
		return 0, fmt.Errorf("setting MBIM UICC reset: %w", err)
	}
	return request.Response.PassThroughStatus, nil
}

func (r *Reader) QueryUiccReset(ctx context.Context) (UiccPassThroughStatus, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed {
		return 0, errors.New("querying MBIM UICC reset: reader is closed")
	}

	request := UiccResetQueryRequest{TransactionID: r.nextTransactionID()}
	if err := request.Request().Transmit(ctx, r.conn); err != nil {
		return 0, fmt.Errorf("querying MBIM UICC reset: %w", err)
	}
	return request.Response.PassThroughStatus, nil
}

func (r *Reader) SetUiccTerminalCapability(ctx context.Context, capabilities [][]byte) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed {
		return errors.New("setting MBIM UICC terminal capability: reader is closed")
	}

	request := UiccTerminalCapabilitySetRequest{
		TransactionID: r.nextTransactionID(),
		Capabilities:  cloneByteSlices(capabilities),
	}
	if err := request.Request().Transmit(ctx, r.conn); err != nil {
		return fmt.Errorf("setting MBIM UICC terminal capability: %w", err)
	}
	return nil
}

func (r *Reader) QueryUiccTerminalCapability(ctx context.Context) ([][]byte, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed {
		return nil, errors.New("querying MBIM UICC terminal capability: reader is closed")
	}

	request := UiccTerminalCapabilityQueryRequest{TransactionID: r.nextTransactionID()}
	if err := request.Request().Transmit(ctx, r.conn); err != nil {
		return nil, fmt.Errorf("querying MBIM UICC terminal capability: %w", err)
	}
	return cloneByteSlices(request.Response.Capabilities), nil
}

func (r *Reader) CloseChannel(ctx context.Context, channel uint32) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed {
		return errors.New("closing MBIM UICC channel: reader is closed")
	}

	request := CloseChannelRequest{
		TransactionID: r.nextTransactionID(),
		Channel:       channel,
		ChannelGroup:  uiccChannelGroupDefault,
	}
	if err := request.Request().Transmit(ctx, r.conn); err != nil {
		return fmt.Errorf("closing MBIM UICC channel: %w", err)
	}
	return uiccStatusError("closing MBIM UICC channel", request.Response.Status)
}

func (r *Reader) STKEnvelope(ctx context.Context, data []byte) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed {
		return errors.New("running MBIM STK envelope: reader is closed")
	}

	request := STKEnvelopeRequest{
		TransactionID: r.nextTransactionID(),
		Data:          slices.Clone(data),
	}
	if err := request.Request().Transmit(ctx, r.conn); err != nil {
		return fmt.Errorf("running MBIM STK envelope: %w", err)
	}
	return nil
}

func cloneByteSlices(values [][]byte) [][]byte {
	if values == nil {
		return nil
	}
	clones := make([][]byte, len(values))
	for i, value := range values {
		clones[i] = slices.Clone(value)
	}
	return clones
}

func uiccStatusError(action string, status uint32) error {
	if uiccStatusOK(status) {
		return nil
	}
	return fmt.Errorf("%s: %w", action, apdu.StatusError{SW: uiccStatusCode(status)})
}

func uiccStatusOK(status uint32) bool {
	return status == 0 || uiccStatusCode(status) == 0x9000
}

func uiccStatusCode(status uint32) uint16 {
	var sw [2]byte
	binary.LittleEndian.PutUint16(sw[:], uint16(status&0xffff))
	return binary.BigEndian.Uint16(sw[:])
}

func cardStatusError(sw1, sw2 uint32) error {
	if sw1 == 0x90 && sw2 == 0x00 {
		return nil
	}
	return fmt.Errorf("unexpected status word 0x%02X%02X", sw1, sw2)
}
