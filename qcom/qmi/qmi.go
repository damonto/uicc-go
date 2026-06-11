package qmi

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"time"

	"github.com/damonto/uicc-go/qcom"
)

const defaultProxyOpenTimeout = 5 * time.Second

type Conn interface {
	io.ReadWriteCloser
	SetReadDeadline(time.Time) error
	SetWriteDeadline(time.Time) error
}

type Dialer interface {
	Dial(ctx context.Context) (Conn, error)
}

type Option func(*config)

type config struct {
	dialer Dialer
}

type ProxyDialer struct {
	Address string
	Device  string
}

type proxyDialer interface {
	usesProxy() bool
}

type deviceDialer interface {
	device() string
}

func WithDialer(d Dialer) Option {
	return func(c *config) {
		c.dialer = d
	}
}

func WithProxy(device string) Option {
	return func(c *config) {
		c.dialer = ProxyDialer{Device: device}
	}
}

func WithDirect(device string) Option {
	return func(c *config) {
		c.dialer = DirectDialer{Device: device}
	}
}

func Open(ctx context.Context, opts ...Option) (*Transport, error) {
	return openWithConfig(ctx, newConfig(opts))
}

func newConfig(opts []Option) config {
	cfg := config{}
	for _, opt := range opts {
		opt(&cfg)
	}
	return cfg
}

func openWithConfig(ctx context.Context, cfg config) (*Transport, error) {
	if cfg.dialer == nil {
		return nil, errors.New("opening QMI transport: dialer is nil")
	}
	proxy := dialerUsesProxy(cfg.dialer)
	device := ""
	if proxy {
		device = proxyDevice(cfg)
		if device == "" {
			return nil, errors.New("opening QMI proxy: device is empty")
		}
	}

	conn, err := cfg.dialer.Dial(ctx)
	if err != nil {
		return nil, err
	}
	transport := New(conn)
	if !proxy {
		return transport, nil
	}

	if err := transport.openProxy(ctx, device); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("opening QMI proxy for %s: %w", device, err)
	}
	return transport, nil
}

func dialerUsesProxy(d Dialer) bool {
	p, ok := d.(proxyDialer)
	return ok && p.usesProxy()
}

func (d ProxyDialer) Dial(ctx context.Context) (Conn, error) {
	address := d.Address
	if address == "" {
		address = "\x00qmi-proxy"
	}

	var dialer net.Dialer
	conn, err := dialer.DialContext(ctx, "unix", address)
	if err != nil {
		return nil, err
	}
	return conn, nil
}

func (d ProxyDialer) usesProxy() bool { return true }

func (d ProxyDialer) device() string { return strings.TrimSpace(d.Device) }

func proxyDevice(cfg config) string {
	d, ok := cfg.dialer.(deviceDialer)
	if ok {
		return d.device()
	}
	return ""
}

func (t *Transport) openProxy(ctx context.Context, device string) error {
	req := qcom.InternalOpenRequest{
		TransactionID: 1,
		DevicePath:    []byte(device),
	}.Request()
	req.Timeout = defaultProxyOpenTimeout

	resp, err := t.Do(ctx, req)
	if err != nil {
		return err
	}
	return qcom.ResultError(resp.TLVs)
}

var _ qcom.Transport = (*Transport)(nil)
