package trunk_test

import (
	"bytes"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/trunk"
)

func appendedSeeds() [][]byte {
	return [][]byte{
		{0x00, 0x00, 0x01, 0xA5, 0x5A, 0x7F, 0xFF},
		{0x01, 0x00, 0x02, 0x01, 0x00, 0x00, 0x01, 0x00, 0x00, 0x02},
		{0x02, 0x08, 0x01, 0x01, 0x23, 0x45, 0x67, 0x89, 0xAB, 0xFF},
		{0x03, 0x10, 0x01, 0x83, 0x0B, 0xFF},
		{0x05, 0x00, 0x02, 0x58, 0x29, 0x34, 0xD2, 0x00},
		{0x07, 0x08, 0x01, 0x00, 0x61, 0xD8, 0x3D, 0xDE, 0x00},
		{0x0A, 0x04, 0x01, 0x01, 0x00, 0x23, 0x29, 0x00, 0x71},
		{0x0B, 0x00, 0x01, 0x10, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
		{0x06, 0x00, 0x02, 0x20, 0x01, 0x0D, 0xB8},
	}
}

func runAppended(t *testing.T, in []byte) {
	if len(in) < 3 {
		return
	}
	a := trunk.Appended{Format: enums.UDTFormat(in[0] & 0xF), PadNibble: in[1] & 0x1F, Blocks: int(in[2] % 6)}
	copy(a.Data[:], in[3:])
	var b trunk.Appended
	if bits, n, err := a.Binary(nil); err == nil {
		if err := b.SetBinary(bits, n); err != nil {
			t.Fatalf("Binary %d bits: %v", n, err)
		}
		if back, m, err := b.Binary(nil); err != nil || m != n || !bytes.Equal(back, bits) {
			t.Fatalf("Binary %d bits back as %d %v", n, m, err)
		}
	}
	if l, err := a.Addresses(); err == nil {
		if err := b.SetAddresses(l.OneKey, l.Addresses[:l.Len]); err != nil {
			t.Fatalf("Addresses %+v: %v", l, err)
		}
		if back, err := b.Addresses(); err != nil || back != l {
			t.Fatalf("Addresses %+v back as %+v %v", l, back, err)
		}
	}
	if d, err := a.Digits(nil); err == nil && len(d) > 0 {
		if err := b.SetDigits(d); err != nil {
			t.Fatalf("Digits %q: %v", d, err)
		}
		if back, err := b.Digits(nil); err != nil || !bytes.Equal(back, d) {
			t.Fatalf("Digits %q back as %q %v", d, back, err)
		}
	}
	if text, err := a.Text(nil); err == nil && len(text) > 0 && a.Format != enums.UDTMixed {
		if err := b.SetText(a.Format, text); err == nil {
			if back, err := b.Text(nil); err != nil || !bytes.Equal(back, text) {
				t.Fatalf("Text %q back as %q %v", text, back, err)
			}
		}
	}
	if addr, oneKey, err := a.Mixed(); err == nil {
		text, _ := a.Text(nil)
		if err := b.SetMixed(oneKey, addr, text); err == nil {
			back, _ := b.Text(nil)
			if ba, bk, err := b.Mixed(); err != nil || ba != addr || bk != oneKey || !bytes.Equal(back, text) {
				t.Fatalf("Mixed %d %q back as %d %q %v", addr, text, ba, back, err)
			}
		}
	}
	runAppendedFormats(t, &a)
}

func runAppendedFormats(t *testing.T, a *trunk.Appended) {
	var b trunk.Appended
	if n, err := a.NMEAShort(); err == nil {
		if err := b.SetNMEAShort(&n); err == nil {
			if back, _ := b.NMEAShort(); back != n {
				t.Fatalf("NMEAShort %+v back as %+v", n, back)
			}
		}
		_ = n.String()
	}
	if n, err := a.NMEALong(); err == nil {
		if err := b.SetNMEALong(&n); err == nil {
			if back, _ := b.NMEALong(); back != n {
				t.Fatalf("NMEALong %+v back as %+v", n, back)
			}
		}
		_ = n.String()
	}
	if ip, v6, err := a.IP(); err == nil {
		b.SetIP(ip, v6)
		if back, bv6, err := b.IP(); err != nil || back != ip || bv6 != v6 {
			t.Fatalf("IP %v back as %v %v", ip, back, err)
		}
	}
	if lip, err := a.LIP(nil); err == nil && len(lip) > 0 {
		if err := b.SetLIP(lip); err != nil {
			t.Fatalf("LIP %d octets: %v", len(lip), err)
		}
		if back, err := b.LIP(nil); err != nil || !bytes.Equal(back, lip) {
			t.Fatalf("LIP back as %x %v", back, err)
		}
	}
	p := [10]byte(a.Data[:10])
	r := trunk.DecodeLIPShortReport(p)
	_ = r.String()
	if e, err := r.Encode(); err == nil && trunk.DecodeLIPShortReport(e) != r {
		t.Fatalf("LIPShortReport %+v", r)
	}
	q := trunk.DecodeUSBDPollRequest(p)
	_ = q.String()
	if e, err := q.Encode(); err != nil || trunk.DecodeUSBDPollRequest(e) != q {
		t.Fatalf("USBDPollRequest %+v %v", q, err)
	}
	s := trunk.DecodeUSBDLIPResponse(p)
	_ = s.String()
	if e, err := s.Encode(); err == nil && trunk.DecodeUSBDLIPResponse(e) != s {
		t.Fatalf("USBDLIPResponse %+v", s)
	}
	_ = a.Format.String()
}

func FuzzAppended(f *testing.F) {
	for _, s := range appendedSeeds() {
		f.Add(s)
	}
	f.Fuzz(runAppended)
}
