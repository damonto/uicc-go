//go:build !linux

package qmi

import (
	"context"
	"errors"
)

type DirectDialer struct {
	Device string
}

func (d DirectDialer) Dial(context.Context) (Conn, error) {
	return nil, errors.New("opening QMI device: direct cdc-wdm access is only supported on linux")
}
