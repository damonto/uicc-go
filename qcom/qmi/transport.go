package qmi

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

type QMUXHeader struct {
	IfType       uint8
	Length       uint16
	ControlFlags uint8
	ServiceType  qcom.ServiceType
	ClientID     uint8
}

type Header[T uint8 | uint16] struct {
	MessageType   qcom.MessageType
	TransactionID T
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
	conn Conn

	writeMu  sync.Mutex
	readOnce sync.Once
	mu       sync.Mutex
	pending  map[messageKey]chan responseResult
	subs     map[uint64]subscription
	nextSub  uint64
	readErr  error
	closed   bool
}

func New(conn Conn) *Transport {
	return &Transport{
		conn:    conn,
		pending: make(map[messageKey]chan responseResult),
		subs:    make(map[uint64]subscription),
	}
}

func (t *Transport) Close() error {
	err := t.conn.Close()
	t.failAll(errors.New("QMI transport is closed"))
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
		service: qcom.ServiceControl,
		client:  req.ClientID,
		txn:     req.TransactionID,
		message: req.MessageID,
	}
	if req.Service != qcom.ServiceControl {
		key.service = req.Service
	}
	result := make(chan responseResult, 1)
	if err := t.addPending(key, result); err != nil {
		return qcom.Response{}, err
	}
	t.startReader()

	deadline, hasDeadline := qcom.RequestDeadline(ctx, req.Timeout)
	t.writeMu.Lock()
	if hasDeadline {
		if err := t.conn.SetWriteDeadline(deadline); err != nil {
			t.writeMu.Unlock()
			t.removePending(key)
			return qcom.Response{}, fmt.Errorf("setting QMI write deadline: %w", err)
		}
	}
	writeErr := writeFull(t.conn, packet)
	if hasDeadline {
		_ = t.conn.SetWriteDeadline(time.Time{})
	}
	t.writeMu.Unlock()
	if writeErr != nil {
		t.removePending(key)
		return qcom.Response{}, fmt.Errorf("writing QMI request: %w", writeErr)
	}

	select {
	case result := <-result:
		return result.resp, result.err
	case <-waitCtx.Done():
		t.removePending(key)
		return qcom.Response{}, waitCtx.Err()
	}
}

func (t *Transport) Indications(ctx context.Context, service qcom.ServiceType, clientID uint8, id qcom.MessageID) (<-chan qcom.Indication, error) {
	ch := make(chan qcom.Indication, 16)
	sub := subscription{service: service, client: clientID, message: id, ch: ch}

	t.mu.Lock()
	if t.readErr != nil {
		t.mu.Unlock()
		close(ch)
		return nil, t.readErr
	}
	if t.closed {
		t.mu.Unlock()
		close(ch)
		return nil, errors.New("QMI transport is closed")
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
	service qcom.ServiceType
	client  uint8
	txn     uint16
	message qcom.MessageID
}

type responseResult struct {
	resp qcom.Response
	err  error
}

type subscription struct {
	service qcom.ServiceType
	client  uint8
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
		return errors.New("QMI transport is closed")
	}
	if _, ok := t.pending[key]; ok {
		return errors.New("QMI request is already pending")
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
		frame, err := ReadFrame(t.conn)
		if err != nil {
			t.failAll(fmt.Errorf("reading QMI message: %w", err))
			return
		}

		var wire Response
		if err := wire.UnmarshalBinary(frame); err != nil {
			t.failAll(err)
			return
		}
		switch wire.MessageType {
		case qcom.MessageTypeResponse, 0x01:
			t.deliverResponse(wire.QCOM())
		case qcom.MessageTypeIndication:
			t.deliverIndication(wire.QCOMIndication())
		}
	}
}

func (t *Transport) deliverResponse(resp qcom.Response) {
	key := messageKey{
		service: resp.Service,
		client:  resp.ClientID,
		txn:     resp.TransactionID,
		message: resp.MessageID,
	}
	if resp.Service == qcom.ServiceControl {
		key.client = 0
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
		if sub.service == ind.Service && sub.client == ind.ClientID && sub.message == ind.MessageID {
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
		return nil, fmt.Errorf("marshal QMI TLVs: %w", err)
	}
	maxPayloadLength := qcom.MaxQMUXServiceTLVLength
	if req.Service == qcom.ServiceControl {
		maxPayloadLength = qcom.MaxQMUXControlTLVLength
	}
	if len(payload) > maxPayloadLength {
		return nil, fmt.Errorf("QMI message TLVs length %d exceeds limit %d", len(payload), maxPayloadLength)
	}

	sdu := new(bytes.Buffer)
	if req.Service == qcom.ServiceControl {
		if err := binary.Write(sdu, binary.LittleEndian, Header[uint8]{
			MessageType:   qcom.MessageTypeRequest,
			TransactionID: uint8(req.TransactionID),
			MessageID:     req.MessageID,
			MessageLength: uint16(len(payload)),
		}); err != nil {
			return nil, fmt.Errorf("write control QMI header: %w", err)
		}
	} else {
		if err := binary.Write(sdu, binary.LittleEndian, Header[uint16]{
			MessageType:   qcom.MessageTypeRequest,
			TransactionID: req.TransactionID,
			MessageID:     req.MessageID,
			MessageLength: uint16(len(payload)),
		}); err != nil {
			return nil, fmt.Errorf("write service QMI header: %w", err)
		}
	}
	if _, err := sdu.Write(payload); err != nil {
		return nil, fmt.Errorf("write QMI payload: %w", err)
	}

	out := new(bytes.Buffer)
	if err := binary.Write(out, binary.LittleEndian, QMUXHeader{
		IfType:       qcom.QMUXIfType,
		Length:       uint16(sdu.Len() + 5),
		ControlFlags: qcom.QMUXControlFlagRequest,
		ServiceType:  req.Service,
		ClientID:     req.ClientID,
	}); err != nil {
		return nil, fmt.Errorf("write QMUX header: %w", err)
	}
	if _, err := out.Write(sdu.Bytes()); err != nil {
		return nil, fmt.Errorf("write QMUX payload: %w", err)
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
