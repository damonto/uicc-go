//go:build linux

package qmi

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/damonto/uicc-go/cdcwdm"
)

type DirectDialer struct {
	Device string
}

func (d DirectDialer) Dial(ctx context.Context) (Conn, error) {
	device := strings.TrimSpace(d.Device)
	if device == "" {
		return nil, errors.New("opening QMI device: device is empty")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	conn, err := cdcwdm.Open(device)
	if err != nil {
		return nil, fmt.Errorf("opening QMI device: %w", err)
	}
	return conn, nil
}
