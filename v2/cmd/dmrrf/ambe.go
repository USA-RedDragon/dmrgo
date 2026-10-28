package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/vocoder"
	"github.com/USA-RedDragon/dmrgo/v2/vocoder/dvsi"
)

const (
	defaultBaud    = 460800
	ambeserverPort = "2460"
)

const ambeUsage = `AMBE+2 decoder for -pcm: "software" for the software codec of the build (only in a build that adds one), "serial:<device>[@<baud>]" for a DVSI chip on a serial port (default 460800 baud), or "ambeserver:<host>[:<port>]" for an AMBEserver (default port 2460)`

type ambeBackend struct {
	codec vocoder.Codec
	close func() error
	desc  string
}

var errNoSoftware = errors.New("-ambe software: this build of dmrrf has no software AMBE+2 codec")

func openAMBE(spec string, software func() (vocoder.Codec, error)) (*ambeBackend, error) {
	kind, arg, _ := strings.Cut(spec, ":")
	switch kind {
	case "software":
		if arg != "" {
			return nil, fmt.Errorf("-ambe %q: software takes no argument", spec)
		}
		if software == nil {
			return nil, errNoSoftware
		}
		c, err := software()
		if err != nil {
			return nil, err
		}
		return &ambeBackend{codec: c, close: c.Close, desc: "software codec"}, nil
	case "serial":
		path, b, hasBaud := strings.Cut(arg, "@")
		baud := defaultBaud
		if hasBaud {
			var err error
			if baud, err = strconv.Atoi(b); err != nil {
				return nil, fmt.Errorf("-ambe %q: baud: %w", spec, err)
			}
		}
		if path == "" {
			return nil, fmt.Errorf("-ambe %q: no device", spec)
		}
		f, err := openSerial(path, baud)
		if err != nil {
			return nil, err
		}
		return newDVSI(f, fmt.Sprintf("%s at %d baud", path, baud))
	case "ambeserver":
		addr := arg
		if _, _, err := net.SplitHostPort(addr); err != nil {
			addr = net.JoinHostPort(arg, ambeserverPort)
		}
		var d net.Dialer
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		conn, err := d.DialContext(ctx, "udp", addr)
		if err != nil {
			return nil, err
		}
		return newDVSI(conn, "AMBEserver "+addr)
	}
	return nil, fmt.Errorf("-ambe %q: expected software, serial:<device>[@<baud>] or ambeserver:<host>[:<port>]", spec)
}

func newDVSI(port io.ReadWriteCloser, where string) (*ambeBackend, error) {
	c, err := dvsi.New(port, &dvsi.Config{Timeout: time.Second})
	if err != nil {
		_ = port.Close()
		return nil, fmt.Errorf("%s: %w", where, err)
	}
	return &ambeBackend{
		codec: c,
		close: func() error { return errors.Join(c.Close(), port.Close()) },
		desc:  fmt.Sprintf("%s %s on %s", c.Product(), c.Version(), where),
	}, nil
}

type audioOut struct {
	codec  vocoder.Codec
	w      io.Writer
	frames int
	err    error
	pcm    []int16
	buf    []byte
}

func (a *audioOut) decode(slot [9]byte) {
	if a.err != nil {
		return
	}
	pcm, _, err := a.codec.Decode(a.pcm[:0], slot)
	if err != nil {
		a.err = err
		return
	}
	a.pcm = pcm
	a.buf = a.buf[:0]
	for _, s := range pcm {
		v := uint16(int(s) & 0xFFFF)
		a.buf = append(a.buf, byte(v&0xFF), byte(v>>8))
	}
	if _, err := a.w.Write(a.buf); err != nil {
		a.err = err
		return
	}
	a.frames++
}
