//go:build linux

package main

import (
	"fmt"
	"os"
	"syscall"
	"unsafe"
)

const crtscts = 0x80000000

func baudFlag(baud int) (uint32, error) {
	switch baud {
	case 57600:
		return syscall.B57600, nil
	case 115200:
		return syscall.B115200, nil
	case 230400:
		return syscall.B230400, nil
	case 460800:
		return syscall.B460800, nil
	case 921600:
		return syscall.B921600, nil
	default:
		return 0, fmt.Errorf("baud %d, expected 57600, 115200, 230400, 460800 or 921600", baud)
	}
}

func openSerial(path string, baud int) (*os.File, error) {
	speed, err := baudFlag(baud)
	if err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		return nil, err
	}
	rc, err := f.SyscallConn()
	if err != nil {
		_ = f.Close()
		return nil, err
	}
	var ioErr error
	err = rc.Control(func(fd uintptr) {
		t := syscall.Termios{
			Cflag:  speed | syscall.CS8 | syscall.CREAD | syscall.CLOCAL | crtscts,
			Ispeed: speed,
			Ospeed: speed,
		}
		t.Cc[syscall.VMIN], t.Cc[syscall.VTIME] = 0, 1
		if _, _, e := syscall.Syscall(syscall.SYS_IOCTL, fd, syscall.TCSETS, uintptr(unsafe.Pointer(&t))); e != 0 {
			ioErr = e
		}
	})
	if err == nil {
		err = ioErr
	}
	if err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("%s: set serial mode: %w", path, err)
	}
	return f, nil
}
