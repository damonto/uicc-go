package uim

import (
	"context"
	"errors"
	"time"

	"github.com/damonto/uicc-go/qcom"
	"github.com/damonto/uicc-go/qcom/tlv"
)

var errReaderClosed = errors.New("QMI UIM client is closed")

func (r *Reader) allocateClientID(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed || r.transport == nil {
		return errReaderClosed
	}

	clientID, err := r.allocateServiceClientID(ctx, qcom.ServiceUIM)
	if err != nil {
		return err
	}
	r.clientID = clientID
	return nil
}

func (r *Reader) allocateServiceClientID(ctx context.Context, service qcom.ServiceType) (uint8, error) {
	resp, err := r.sendRequest(ctx, qcom.ServiceControl, 0, qcom.MessageAllocateClientID, tlv.TLVs{
		tlv.Uint(0x01, service),
	}, DefaultRequestTimeout)
	if err != nil {
		return 0, err
	}
	if err := resultOK(resp); err != nil {
		return 0, err
	}

	value, ok := tlv.Value(resp.TLVs, 0x01)
	if !ok || len(value) < 2 {
		return 0, errors.New("allocating QMI client ID: allocated client TLV missing")
	}
	return value[1], nil
}

func (r *Reader) releaseServiceClientID(ctx context.Context, service qcom.ServiceType, clientID uint8) error {
	resp, err := r.sendRequest(ctx, qcom.ServiceControl, 0, qcom.MessageReleaseClientID, tlv.TLVs{
		tlv.Bytes(0x01, []byte{byte(service), clientID}),
	}, DefaultRequestTimeout)
	if err != nil {
		return err
	}
	return resultOK(resp)
}

func (r *Reader) request(
	ctx context.Context,
	id qcom.MessageID,
	tlvs tlv.TLVs,
) (qcom.Response, error) {
	return r.requestWithTimeout(ctx, id, tlvs, DefaultRequestTimeout)
}

func (r *Reader) requestWithTimeout(
	ctx context.Context,
	id qcom.MessageID,
	tlvs tlv.TLVs,
	timeout time.Duration,
) (qcom.Response, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed || r.transport == nil {
		return qcom.Response{}, errReaderClosed
	}
	return r.sendRequest(ctx, qcom.ServiceUIM, r.clientID, id, tlvs, timeout)
}

func (r *Reader) requestService(
	ctx context.Context,
	service qcom.ServiceType,
	clientID uint8,
	id qcom.MessageID,
	tlvs tlv.TLVs,
) (qcom.Response, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed || r.transport == nil {
		return qcom.Response{}, errReaderClosed
	}
	return r.sendRequest(ctx, service, clientID, id, tlvs, DefaultRequestTimeout)
}

// sendRequest assumes r.mu is held and r.transport is live.
func (r *Reader) sendRequest(
	ctx context.Context,
	service qcom.ServiceType,
	clientID uint8,
	id qcom.MessageID,
	tlvs tlv.TLVs,
	timeout time.Duration,
) (qcom.Response, error) {
	return r.transport.Do(ctx, qcom.Request{
		Service:       service,
		ClientID:      clientID,
		TransactionID: uint16(r.txn.Add(1)),
		MessageID:     id,
		Timeout:       timeout,
		TLVs:          tlvs,
	})
}
