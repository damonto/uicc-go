package qrtr

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/damonto/uicc-go/qcom"
)

type fakeDialer struct {
	conn    packetConn
	err     error
	service qcom.ServiceType
}

func (d *fakeDialer) Dial(_ context.Context, service qcom.ServiceType) (packetConn, error) {
	d.service = service
	return d.conn, d.err
}

type fakeConn struct{}

func (fakeConn) Read([]byte) (int, error)        { return 0, io.EOF }
func (fakeConn) Write(p []byte) (int, error)     { return len(p), nil }
func (fakeConn) Close() error                    { return nil }
func (fakeConn) SetReadDeadline(time.Time) error { return nil }

func TestOpenUsesDialer(t *testing.T) {
	tests := []struct {
		name    string
		service qcom.ServiceType
		dialer  *fakeDialer
		wantErr bool
	}{
		{"default service", 0, &fakeDialer{conn: fakeConn{}}, false},
		{"custom service", qcom.ServiceCAT, &fakeDialer{conn: fakeConn{}}, false},
		{"dial error", 0, &fakeDialer{err: errors.New("boom")}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := []Option{WithDialer(tt.dialer)}
			if tt.service != 0 {
				opts = append(opts, WithService(tt.service))
			}
			got, err := Open(context.Background(), opts...)
			if tt.wantErr {
				if err == nil {
					t.Fatal("Open() error = nil, want error")
				}
				return
			}
			if err != nil {
				t.Fatalf("Open() error = %v", err)
			}
			if got == nil {
				t.Fatal("Open() = nil, want transport")
			}
			wantService := tt.service
			if wantService == 0 {
				wantService = qcom.ServiceUIM
			}
			if tt.dialer.service != wantService {
				t.Fatalf("service = %d, want %d", tt.dialer.service, wantService)
			}
		})
	}
}

func TestOpenRejectsNilDialer(t *testing.T) {
	if _, err := Open(context.Background(), WithDialer(nil)); err == nil {
		t.Fatal("Open() error = nil, want error")
	}
}
