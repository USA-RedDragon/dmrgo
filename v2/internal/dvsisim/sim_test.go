package dvsisim_test

import (
	"bytes"
	"encoding/hex"
	"errors"
	"strings"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/internal/dvsisim"
)

type timeout interface{ Timeout() bool }

func reply(t *testing.T, d *dvsisim.Device) []byte {
	t.Helper()
	var buf [1024]byte
	n, err := d.Read(buf[:])
	if err != nil {
		var to timeout
		if !errors.As(err, &to) || !to.Timeout() {
			t.Fatalf("Read error %v has no Timeout method", err)
		}
	}
	return buf[:n]
}

func packet(t *testing.T, typ byte, fields string, parity bool) []byte {
	t.Helper()
	f, err := hexBytes(fields)
	if err != nil {
		t.Fatal(err)
	}
	n := len(f)
	if parity {
		n += 2
	}
	b := append([]byte{0x61, byte(n >> 8), byte(n), typ}, f...)
	if parity {
		b = append(b, 0x2F)
		var x byte
		for _, c := range b[1:] {
			x ^= c
		}
		b = append(b, x)
	}
	return b
}

func TestDeviceRejects(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   []byte
		err  string
	}{
		{"bad parity", []byte{0x61, 0x00, 0x03, 0x00, 0x33, 0x2F, 0x00}, "parity"},
		{"no parity field", packet(t, 0, "33", false), "parity"},
		{"packet type 7", packet(t, 7, "", true), "packet type"},
		{"unknown control field", packet(t, 0, "77", true), "control field"},
		{"channel 1 of an AMBE-3000R", packet(t, 0, "41 0b 03", true), "channel 1"},
		{"other RATEP", packet(t, 0, "0a 0431 0754 0000 0000 0000 7031", true), "RATEP"},
		{"INIT of the encoder only", packet(t, 0, "0b 01", true), "INIT"},
		{"decode before the rate is set", packet(t, 1, "01 48 000000000000000000", true), "configured"},
		{"encode before the rate is set", packet(t, 2, "00 a0", true), "configured"},
		{"cut RATEP", packet(t, 0, "0a 0431", true), "short"},
		{"bad parity mode", packet(t, 0, "3f 02", true), "parity mode"},
	}
	for _, tt := range tests {
		d := dvsisim.New(nil, dvsisim.Options{})
		if _, err := d.Write(tt.in); err != nil {
			t.Fatal(err)
		}
		if len(d.Errs) == 0 || !strings.Contains(d.Errs[0], tt.err) {
			t.Errorf("%s: errors %q, want %q", tt.name, d.Errs, tt.err)
		}
	}
	d := dvsisim.New(nil, dvsisim.Options{})
	_, _ = d.Write(packet(t, 0, "0a 0431 0754 2400 0000 0000 6f48 0b 03", true))
	for _, in := range [][]byte{packet(t, 1, "01 47 000000000000000000", true), packet(t, 2, "00 9f", true)} {
		d.Errs = nil
		_, _ = d.Write(in)
		if len(d.Errs) != 1 {
			t.Errorf("%x: errors %q", in, d.Errs)
		}
	}
}

func TestDeviceReplies(t *testing.T) {
	t.Parallel()

	pairs := dvsisim.Pairs(1)
	d := dvsisim.New(pairs, dvsisim.Options{Datagram: true})
	write := func(s string) {
		t.Helper()
		b, err := hexBytes(s)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = d.Write(b)
	}
	write("00000000 610003 00 33 2f 1f")
	if got := reply(t, d); !bytes.Equal(got, []byte{0x61, 0x00, 0x03, 0x00, 0x39, 0x2F, 0x15}) {
		t.Errorf("reset reply %x", got)
	}
	write("610004 00 3f 00 2f 14")
	if got := reply(t, d); !bytes.Equal(got, []byte{0x61, 0x00, 0x02, 0x00, 0x3F, 0x00}) {
		t.Errorf("parity mode reply %x", got)
	}
	write("61000f 00 0a 0431 0754 2400 0000 0000 6f48 0b 03")
	if got := reply(t, d); !bytes.Equal(got, []byte{0x61, 0x00, 0x04, 0x00, 0x0A, 0x00, 0x0B, 0x00}) {
		t.Errorf("rate reply %x", got)
	}
	s := pairs[0].Slot
	write("61000b 01 01 48")
	_, _ = d.Write(s[:])
	got := reply(t, d)
	if len(got) != 4+2+320 || got[3] != 0x02 || got[4] != 0x00 || got[5] != 160 {
		t.Errorf("decode reply % x", got[:min(len(got), 8)])
	}
	if len(d.Errs) > 0 {
		t.Errorf("errors %q", d.Errs)
	}
	if got := reply(t, d); len(got) != 0 {
		t.Errorf("extra reply %x", got)
	}
	var small [8]byte
	write("610001 00 30")
	if n, _ := d.Read(small[:]); n != len(small) || len(d.Errs) != 1 {
		t.Errorf("datagram read into 8 bytes: %d bytes, errors %q", n, d.Errs)
	}
}

func TestPairsDistinct(t *testing.T) {
	t.Parallel()

	p := dvsisim.Pairs(32)
	for i := range p {
		for j := range i {
			if p[i].Slot == p[j].Slot || p[i].PCM == p[j].PCM {
				t.Fatalf("pairs %d and %d are not distinct", i, j)
			}
		}
	}
}

func hexBytes(s string) ([]byte, error) {
	s = strings.ReplaceAll(s, " ", "")
	b := make([]byte, len(s)/2)
	for i := range b {
		var v byte
		for _, c := range s[2*i : 2*i+2] {
			switch {
			case c >= '0' && c <= '9':
				v = v<<4 | byte(c-'0')
			case c >= 'a' && c <= 'f':
				v = v<<4 | byte(c-'a'+10)
			default:
				return nil, errors.New("bad hex")
			}
		}
		b[i] = v
	}
	return b, nil
}

func TestDeviceStatus(t *testing.T) {
	t.Parallel()

	pairs := dvsisim.Pairs(1)
	d := dvsisim.New(pairs, dvsisim.Options{NoParityPin: true, Datagram: true})
	_, _ = d.Write(packet(t, 0, "0a 0431 0754 2400 0000 0000 6f48 0b 03", false))
	_ = reply(t, d)
	_, _ = d.Write(packet(t, 0, "16 0001", false))
	if got := reply(t, d); !bytes.Equal(got, []byte{0x61, 0x00, 0x02, 0x00, 0x16, 0x00}) {
		t.Errorf("SPCHFMT reply %x", got)
	}
	s := pairs[0].Slot
	_, _ = d.Write(packet(t, 1, "01 48 "+hex.EncodeToString(s[:]), false))
	got := reply(t, d)
	if len(got) != 4+2+320+3 || !bytes.Equal(got[len(got)-3:], []byte{0x02, 0x00, 0x02}) {
		t.Errorf("decode reply ends % x", got[max(0, len(got)-3):])
	}
	for i, want := range []byte{0x22, 0x22, 0x20} {
		_, _ = d.Write(packet(t, 1, "02 0004", false))
		got := reply(t, d)
		if len(got) != 329 || !bytes.Equal(got[len(got)-3:], []byte{0x02, 0x00, want}) || !bytes.Equal(got[6:8], []byte{byte(uint16(pairs[0].PCM[0]) >> 8), byte(pairs[0].PCM[0])}) {
			t.Errorf("lost frame %d: reply ends % x", i, got[max(0, len(got)-3):])
		}
	}
	if d.Counts.Lost != 3 || d.Counts.CMode != 3 || len(d.Errs) > 0 {
		t.Errorf("counts %+v, errors %q", d.Counts, d.Errs)
	}
	for _, tt := range []struct {
		in, err string
	}{
		{"16 0002", "SPCHFMT is not"},
		{"16 00", "short SPCHFMT"},
	} {
		d.Errs = nil
		_, _ = d.Write(packet(t, 0, tt.in, false))
		if len(d.Errs) == 0 || !strings.Contains(d.Errs[0], tt.err) {
			t.Errorf("%s: errors %q, want %q", tt.in, d.Errs, tt.err)
		}
		_ = reply(t, d)
	}
	for _, tt := range []struct {
		in, err string
	}{
		{"02 0008", "other than LOST_FRAME"},
		{"02 0000", "no CHAND"},
		{"03 a0", "fields other than"},
	} {
		d.Errs = nil
		_, _ = d.Write(packet(t, 1, tt.in, false))
		if len(d.Errs) == 0 || !strings.Contains(d.Errs[0], tt.err) {
			t.Errorf("%s: errors %q, want %q", tt.in, d.Errs, tt.err)
		}
		_ = reply(t, d)
	}
	d = dvsisim.New(pairs, dvsisim.Options{RejectSpeechFormat: true, NoParityPin: true})
	_, _ = d.Write(packet(t, 0, "16 0001", false))
	if got := reply(t, d); !bytes.Equal(got, []byte{0x61, 0x00, 0x02, 0x00, 0x16, 0x01}) {
		t.Errorf("rejected SPCHFMT reply %x", got)
	}
}
