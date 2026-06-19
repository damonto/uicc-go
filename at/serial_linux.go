//go:build linux

package at

import (
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"golang.org/x/sys/unix"
)

const (
	defaultWriteTimeout = 5 * time.Second
)

type serialPort struct {
	mu            sync.Mutex
	file          *os.File
	oldTermios    *unix.Termios
	readDeadline  time.Time
	writeDeadline time.Time
}

func openSerialPort(name string, baudRate int) (io.ReadWriteCloser, error) {
	fd, err := unix.Open(name, unix.O_RDWR|unix.O_NOCTTY|unix.O_NONBLOCK, 0o666)
	if err != nil {
		return nil, err
	}

	oldTermios, err := setTermios(fd, baudRate)
	if err != nil {
		return nil, errors.Join(err, unix.Close(fd))
	}
	if err := flushSerial(fd); err != nil {
		return nil, errors.Join(err, unix.IoctlSetTermios(fd, unix.TCSETS, oldTermios), unix.Close(fd))
	}
	if err := unix.SetNonblock(fd, false); err != nil {
		return nil, errors.Join(err, unix.IoctlSetTermios(fd, unix.TCSETS, oldTermios), unix.Close(fd))
	}
	return &serialPort{
		file:       os.NewFile(uintptr(fd), name),
		oldTermios: oldTermios,
	}, nil
}

func setTermios(fd int, baudRate int) (*unix.Termios, error) {
	baud, err := baudConstant(baudRate)
	if err != nil {
		return nil, err
	}
	oldTermios, err := unix.IoctlGetTermios(fd, unix.TCGETS)
	if err != nil {
		return nil, fmt.Errorf("reading termios: %w", err)
	}

	termios := *oldTermios
	termios.Iflag &^= unix.IGNBRK | unix.BRKINT | unix.PARMRK | unix.ISTRIP | unix.INLCR | unix.IGNCR | unix.ICRNL | unix.IXON | unix.IXOFF | unix.IXANY
	termios.Oflag &^= unix.OPOST
	termios.Lflag &^= unix.ECHO | unix.ECHONL | unix.ICANON | unix.ISIG | unix.IEXTEN
	termios.Cflag &^= unix.CSIZE | unix.PARENB | unix.CSTOPB | unix.CRTSCTS | unix.CBAUD
	termios.Cflag |= unix.CS8 | unix.CLOCAL | unix.CREAD | baud
	termios.Ispeed = baud
	termios.Ospeed = baud
	termios.Cc[unix.VMIN] = 1
	termios.Cc[unix.VTIME] = 0
	if err := unix.IoctlSetTermios(fd, unix.TCSETS, &termios); err != nil {
		return nil, fmt.Errorf("writing termios: %w", err)
	}
	return oldTermios, nil
}

func flushSerial(fd int) error {
	return unix.IoctlSetInt(fd, unix.TCFLSH, unix.TCIOFLUSH)
}

func (p *serialPort) Read(buf []byte) (int, error) {
	if len(buf) == 0 {
		return 0, nil
	}
	for {
		deadline := p.currentReadDeadline()
		if err := waitReady(int(p.file.Fd()), unix.POLLIN, deadline); err != nil {
			return 0, err
		}
		n, err := unix.Read(int(p.file.Fd()), buf)
		if err == nil {
			if n == 0 {
				return 0, errIOTimedOut
			}
			return n, nil
		}
		if errors.Is(err, unix.EAGAIN) || errors.Is(err, unix.EWOULDBLOCK) || errors.Is(err, unix.EINTR) {
			continue
		}
		return n, err
	}
}

func (p *serialPort) Write(data []byte) (int, error) {
	if len(data) == 0 {
		return 0, nil
	}
	for {
		deadline := p.effectiveWriteDeadline()
		if err := waitReady(int(p.file.Fd()), unix.POLLOUT, deadline); err != nil {
			return 0, err
		}
		n, err := unix.Write(int(p.file.Fd()), data)
		if err == nil {
			if n == 0 {
				return 0, io.ErrShortWrite
			}
			return n, nil
		}
		if errors.Is(err, unix.EAGAIN) || errors.Is(err, unix.EWOULDBLOCK) || errors.Is(err, unix.EINTR) {
			continue
		}
		return n, err
	}
}

func (p *serialPort) Close() error {
	var errs []error
	if p.oldTermios != nil {
		if err := unix.IoctlSetTermios(int(p.file.Fd()), unix.TCSETS, p.oldTermios); err != nil {
			errs = append(errs, fmt.Errorf("restoring termios: %w", err))
		}
	}
	if err := p.file.Close(); err != nil {
		errs = append(errs, fmt.Errorf("closing serial port: %w", err))
	}
	return errors.Join(errs...)
}

func (p *serialPort) SetReadDeadline(t time.Time) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.readDeadline = t
	return nil
}

func (p *serialPort) SetWriteDeadline(t time.Time) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.writeDeadline = t
	return nil
}

func (p *serialPort) currentReadDeadline() time.Time {
	p.mu.Lock()
	deadline := p.readDeadline
	p.mu.Unlock()
	return deadline
}

func (p *serialPort) effectiveWriteDeadline() time.Time {
	p.mu.Lock()
	deadline := p.writeDeadline
	p.mu.Unlock()

	if deadline.IsZero() {
		return time.Now().Add(defaultWriteTimeout)
	}
	return deadline
}

func waitReady(fd int, events int16, deadline time.Time) error {
	for {
		timeout := -1
		if !deadline.IsZero() {
			remaining := time.Until(deadline)
			if remaining <= 0 {
				return errIOTimedOut
			}
			timeout = durationMillis(remaining)
		}
		pollFDs := []unix.PollFd{{Fd: int32(fd), Events: events}}
		n, err := unix.Poll(pollFDs, timeout)
		if errors.Is(err, unix.EINTR) {
			continue
		}
		if err != nil {
			return err
		}
		if n == 0 {
			return errIOTimedOut
		}
		revents := pollFDs[0].Revents
		if revents&unix.POLLNVAL != 0 {
			return os.ErrClosed
		}
		if revents&(unix.POLLERR|unix.POLLHUP) != 0 {
			return fmt.Errorf("serial poll failed: revents=0x%X", revents)
		}
		if revents&events != 0 {
			return nil
		}
	}
}

func durationMillis(d time.Duration) int {
	if d <= 0 {
		return 0
	}
	ms := d / time.Millisecond
	if d%time.Millisecond != 0 {
		ms++
	}
	const maxInt32 time.Duration = 1<<31 - 1
	return int(min(ms, maxInt32))
}

func baudConstant(baudRate int) (uint32, error) {
	switch baudRate {
	case 9600:
		return unix.B9600, nil
	case 19200:
		return unix.B19200, nil
	case 38400:
		return unix.B38400, nil
	case 57600:
		return unix.B57600, nil
	case 115200:
		return unix.B115200, nil
	case 230400:
		return unix.B230400, nil
	case 460800:
		return unix.B460800, nil
	default:
		return 0, fmt.Errorf("configuring serial port: unsupported baud rate %d", baudRate)
	}
}
