//go:build linux

package main

import (
	"os"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
	"unsafe"

	"github.com/USA-RedDragon/dmrgo/v2/internal/dvsisim"
)

func openPTY(t *testing.T) (*os.File, string) {
	t.Helper()
	m, err := os.OpenFile("/dev/ptmx", os.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		t.Skipf("no pseudo terminal: %v", err)
	}
	t.Cleanup(func() { _ = m.Close() })
	rc, err := m.SyscallConn()
	if err != nil {
		t.Fatal(err)
	}
	var n uint32
	var ioErr syscall.Errno
	err = rc.Control(func(fd uintptr) {
		var unlock int32
		if _, _, e := syscall.Syscall(syscall.SYS_IOCTL, fd, syscall.TIOCSPTLCK, uintptr(unsafe.Pointer(&unlock))); e != 0 {
			ioErr = e
			return
		}
		_, _, ioErr = syscall.Syscall(syscall.SYS_IOCTL, fd, syscall.TIOCGPTN, uintptr(unsafe.Pointer(&n)))
	})
	if err != nil || ioErr != 0 {
		t.Fatalf("unlock the pseudo terminal: %v %v", err, ioErr)
	}
	return m, "/dev/pts/" + strconv.FormatUint(uint64(n), 10)
}

func serveSerial(t *testing.T, m *os.File, d *dvsisim.Device) {
	t.Helper()
	stop := make(chan struct{})
	t.Cleanup(func() { close(stop) })
	go func() {
		buf := make([]byte, 512)
		for {
			n, err := m.Read(buf)
			if err != nil {
				return
			}
			_, _ = d.Write(buf[:n])
		}
	}()
	go func() {
		buf := make([]byte, 64)
		for {
			n, err := d.Read(buf)
			if err != nil || n == 0 {
				select {
				case <-stop:
					return
				case <-time.After(time.Millisecond):
				}
				continue
			}
			if _, err := m.Write(buf[:n]); err != nil {
				return
			}
		}
	}()
}

func TestAMBESerial(t *testing.T) {
	t.Parallel()

	slots := weakSlots(t)
	for _, baud := range []string{"", "@921600"} {
		m, tty := openPTY(t)
		d := dvsisim.New(nil, dvsisim.Options{Chunk: 13, Echo: true})
		serveSerial(t, m, d)
		pcm, out := runWeak(t, "serial:"+tty+baud)
		checkPCM(t, pcm, slots)
		want := tty + " at 460800 baud"
		if baud != "" {
			want = tty + " at 921600 baud"
		}
		if !strings.Contains(out, want) {
			t.Errorf("no %q in\n%s", want, out)
		}
		if counts, errs := d.Snapshot(); len(errs) > 0 || counts.Decode != len(slots) || counts.Reset != 1 {
			t.Errorf("device: %+v, errors %q", counts, errs)
		}
	}
}
