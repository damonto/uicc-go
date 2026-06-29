package at

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"
)

const defaultBaudRate = 115200

var errIOTimedOut = errors.New("I/O timeout")

type Reader struct {
	port   io.ReadWriteCloser
	reader *bufio.Reader

	mu     sync.Mutex
	closed bool
}

type readDeadliner interface {
	SetReadDeadline(time.Time) error
}

type writeDeadliner interface {
	SetWriteDeadline(time.Time) error
}

func Open(device string, baudRate int) (*Reader, error) {
	device = strings.TrimSpace(device)
	if device == "" {
		return nil, errors.New("opening AT reader: device is empty")
	}

	port, err := openSerialPort(device, baudRateOrDefault(baudRate))
	if err != nil {
		return nil, fmt.Errorf("opening serial port %s: %w", device, err)
	}

	return newReader(port), nil
}

func newReader(port io.ReadWriteCloser) *Reader {
	return &Reader{
		port:   port,
		reader: bufio.NewReader(port),
	}
}

func newCSIMCommand(req []byte) (CSIMCommand, error) {
	if req == nil {
		return nil, errors.New("building AT+CSIM command: request is nil")
	}
	return CSIMCommand(req), nil
}

func baudRateOrDefault(baudRate int) int {
	if baudRate == 0 {
		return defaultBaudRate
	}
	return baudRate
}

func (d *Reader) Transmit(ctx context.Context, req []byte) ([]byte, error) {
	command, err := newCSIMCommand(req)
	if err != nil {
		return nil, fmt.Errorf("transmitting APDU: %w", err)
	}

	textCommand, err := command.MarshalText()
	if err != nil {
		return nil, fmt.Errorf("transmitting APDU %X: %w", req, err)
	}

	text, err := d.run(ctx, string(textCommand))
	if err != nil {
		return nil, fmt.Errorf("transmitting APDU %X: %w", req, err)
	}

	var response CSIMResponse
	if err := response.UnmarshalText([]byte(text)); err != nil {
		return nil, fmt.Errorf("transmitting APDU %X: %w", req, err)
	}
	if len(response) < 2 {
		return nil, fmt.Errorf("transmitting APDU %X: invalid response status word", req)
	}
	return append([]byte(nil), response...), nil
}

func (d *Reader) run(ctx context.Context, command string) (string, error) {
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	default:
	}

	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return "", errors.New("using serial reader: reader is closed")
	}
	if err := d.setWriteDeadline(ctx); err != nil {
		return "", err
	}
	if err := writeFull(d.port, []byte(command+"\r\n")); err != nil {
		if err := contextIOError(ctx, err); err != nil {
			return "", err
		}
		return "", fmt.Errorf("writing AT command %q: %w", command, err)
	}

	var builder strings.Builder
	for {
		if err := d.setReadDeadline(ctx); err != nil {
			return "", err
		}
		line, err := d.reader.ReadString('\n')
		if err != nil {
			if err := contextIOError(ctx, err); err != nil {
				return "", err
			}
			return "", fmt.Errorf("reading AT response for %q: %w", command, err)
		}

		select {
		case <-ctx.Done():
			return "", ctx.Err()
		default:
		}

		line = strings.TrimSpace(line)
		if line == "" || line == command {
			continue
		}

		upper := strings.ToUpper(line)
		switch {
		case upper == "OK":
			return strings.TrimSpace(builder.String()), nil
		case upper == "ERROR", strings.HasPrefix(upper, "+CME ERROR:"), strings.HasPrefix(upper, "+CMS ERROR:"):
			return "", errors.New(line)
		default:
			if builder.Len() > 0 {
				builder.WriteByte('\n')
			}
			builder.WriteString(line)
		}
	}
}

func contextIOError(ctx context.Context, err error) error {
	if ctxErr := ctx.Err(); ctxErr != nil {
		return ctxErr
	}
	deadline, ok := ctx.Deadline()
	if ok && errors.Is(err, errIOTimedOut) && !time.Now().Before(deadline) {
		return context.DeadlineExceeded
	}
	return nil
}

func (d *Reader) setWriteDeadline(ctx context.Context) error {
	port, ok := d.port.(writeDeadliner)
	if !ok {
		return nil
	}
	deadline, _ := ctx.Deadline()
	if err := port.SetWriteDeadline(deadline); err != nil {
		return fmt.Errorf("setting AT write deadline: %w", err)
	}
	return nil
}

func (d *Reader) setReadDeadline(ctx context.Context) error {
	port, ok := d.port.(readDeadliner)
	if !ok {
		return nil
	}
	deadline, _ := ctx.Deadline()
	if err := port.SetReadDeadline(deadline); err != nil {
		return fmt.Errorf("setting AT read deadline: %w", err)
	}
	return nil
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

func (d *Reader) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return nil
	}
	d.closed = true
	return d.port.Close()
}
