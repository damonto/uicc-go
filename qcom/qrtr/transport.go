package qrtr

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/damonto/uicc-go/qcom"
)

type packetConn interface {
	io.ReadWriter
	SetReadDeadline(time.Time) error
	Close() error
}

type Header struct {
	MessageType   qcom.MessageType
	TransactionID uint16
	MessageID     qcom.MessageID
	MessageLength uint16
}

type Request struct {
	qcom.Request
}

func (r Request) MarshalBinary() ([]byte, error) {
	return marshalRequest(r.Request)
}

type Transport struct {
	conn packetConn

	writeMu  sync.Mutex
	readOnce sync.Once
	mu       sync.Mutex
	pending  map[messageKey]chan responseResult
	subs     map[uint64]subscription
	nextSub  uint64
	readErr  error
	closed   bool
}

func New(conn packetConn) *Transport {
	return &Transport{
		conn:    conn,
		pending: make(map[messageKey]chan responseResult),
		subs:    make(map[uint64]subscription),
	}
}

func (t *Transport) Close() error {
	err := t.conn.Close()
	t.failAll(errors.New("QRTR transport is closed"))
	return err
}

func (t *Transport) Do(ctx context.Context, req qcom.Request) (qcom.Response, error) {
	packet, err := (Request{Request: req}).MarshalBinary()
	if err != nil {
		return qcom.Response{}, err
	}

	waitCtx, cancel := requestContext(ctx, req.Timeout)
	defer cancel()

	key := messageKey{
		txn:     req.TransactionID,
		message: req.MessageID,
	}
	result := make(chan responseResult, 1)
	if err := t.addPending(key, result); err != nil {
		return qcom.Response{}, err
	}
	t.startReader()

	t.writeMu.Lock()
	if err := writeFull(t.conn, packet); err != nil {
		t.writeMu.Unlock()
		t.removePending(key)
		return qcom.Response{}, fmt.Errorf("writing QRTR request: %w", err)
	}
	t.writeMu.Unlock()

	select {
	case result := <-result:
		return result.resp, result.err
	case <-waitCtx.Done():
		t.removePending(key)
		return qcom.Response{}, waitCtx.Err()
	}
}

func (t *Transport) Indications(ctx context.Context, _ qcom.ServiceType, _ uint8, id qcom.MessageID) (<-chan qcom.Indication, error) {
	ch := make(chan qcom.Indication, 16)
	sub := subscription{message: id, ch: ch}

	t.mu.Lock()
	if t.readErr != nil {
		t.mu.Unlock()
		close(ch)
		return nil, t.readErr
	}
	if t.closed {
		t.mu.Unlock()
		close(ch)
		return nil, errors.New("QRTR transport is closed")
	}
	t.nextSub++
	idn := t.nextSub
	t.subs[idn] = sub
	t.mu.Unlock()

	t.startReader()
	go func() {
		<-ctx.Done()
		t.removeSubscription(idn)
	}()
	return ch, nil
}

type messageKey struct {
	txn     uint16
	message qcom.MessageID
}

type responseResult struct {
	resp qcom.Response
	err  error
}

type subscription struct {
	message qcom.MessageID
	ch      chan qcom.Indication
}

func requestContext(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	deadline, ok := qcom.RequestDeadline(ctx, timeout)
	if !ok {
		return ctx, func() {}
	}
	return context.WithDeadline(ctx, deadline)
}

func (t *Transport) addPending(key messageKey, ch chan responseResult) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.readErr != nil {
		return t.readErr
	}
	if t.closed {
		return errors.New("QRTR transport is closed")
	}
	if _, ok := t.pending[key]; ok {
		return errors.New("QRTR request is already pending")
	}
	t.pending[key] = ch
	return nil
}

func (t *Transport) removePending(key messageKey) {
	t.mu.Lock()
	delete(t.pending, key)
	t.mu.Unlock()
}

func (t *Transport) removeSubscription(id uint64) {
	t.mu.Lock()
	sub, ok := t.subs[id]
	if ok {
		delete(t.subs, id)
	}
	t.mu.Unlock()
	if ok {
		close(sub.ch)
	}
}

func (t *Transport) startReader() {
	t.readOnce.Do(func() {
		go t.readLoop()
	})
}

func (t *Transport) readLoop() {
	for {
		buf := make([]byte, qcom.MaxQRTRQMIMessageLength)
		n, err := t.conn.Read(buf)
		if err != nil {
			t.failAll(fmt.Errorf("reading QRTR QMI message: %w", err))
			return
		}

		var wire Response
		if err := wire.UnmarshalBinary(buf[:n]); err != nil {
			t.failAll(err)
			return
		}
		switch wire.MessageType {
		case qcom.MessageTypeResponse:
			t.deliverResponse(wire.QCOM())
		case qcom.MessageTypeIndication:
			t.deliverIndication(wire.QCOMIndication())
		}
	}
}

func (t *Transport) deliverResponse(resp qcom.Response) {
	key := messageKey{
		txn:     resp.TransactionID,
		message: resp.MessageID,
	}

	t.mu.Lock()
	ch, ok := t.pending[key]
	if ok {
		delete(t.pending, key)
	}
	t.mu.Unlock()
	if ok {
		ch <- responseResult{resp: resp}
	}
}

func (t *Transport) deliverIndication(ind qcom.Indication) {
	t.mu.Lock()
	defer t.mu.Unlock()

	for _, sub := range t.subs {
		if sub.message == ind.MessageID {
			trySendIndication(sub.ch, ind)
		}
	}
}

func trySendIndication(ch chan qcom.Indication, ind qcom.Indication) {
	select {
	case ch <- ind:
	default:
	}
}

func (t *Transport) failAll(err error) {
	t.mu.Lock()
	if t.closed {
		t.mu.Unlock()
		return
	}
	t.closed = true
	t.readErr = err
	pending := t.pending
	t.pending = make(map[messageKey]chan responseResult)
	subs := t.subs
	t.subs = make(map[uint64]subscription)
	t.mu.Unlock()

	for _, ch := range pending {
		ch <- responseResult{err: err}
	}
	for _, sub := range subs {
		close(sub.ch)
	}
}

func MarshalRequest(req qcom.Request) ([]byte, error) {
	return (Request{Request: req}).MarshalBinary()
}

func marshalRequest(req qcom.Request) ([]byte, error) {
	payload, err := req.TLVs.MarshalBinary()
	if err != nil {
		return nil, fmt.Errorf("marshal QRTR QMI TLVs: %w", err)
	}
	if len(payload) > qcom.MaxQRTRServiceTLVLength {
		return nil, fmt.Errorf("QRTR QMI message TLVs length %d exceeds limit %d", len(payload), qcom.MaxQRTRServiceTLVLength)
	}

	out := new(bytes.Buffer)
	if err := binary.Write(out, binary.LittleEndian, Header{
		MessageType:   qcom.MessageTypeRequest,
		TransactionID: req.TransactionID,
		MessageID:     req.MessageID,
		MessageLength: uint16(len(payload)),
	}); err != nil {
		return nil, fmt.Errorf("write QRTR QMI header: %w", err)
	}
	if _, err := out.Write(payload); err != nil {
		return nil, fmt.Errorf("write QRTR QMI payload: %w", err)
	}
	return out.Bytes(), nil
}

func writeFull(w io.Writer, p []byte) error {
	for len(p) > 0 {
		n, err := w.Write(p)
		if err != nil {
			return err
		}
		if n <= 0 {
			return io.ErrShortWrite
		}
		p = p[n:]
	}
	return nil
}
