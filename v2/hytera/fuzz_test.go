package hytera_test

import (
	"bytes"
	"errors"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/hytera"
)

func FuzzDecode(f *testing.F) {
	decodeSeeds(func(data []byte) { f.Add(data) })
	f.Fuzz(fuzzDecode)
}

func decodeSeeds(add func(data []byte)) {
	add([]byte{0x11, 0x00, 0x03, 0x00, 0x04, 0x0a, 0x00, 0x00, 0x50, 0xd1, 0x03})
	add(append([]byte{0x08, 0xa0, 0x02, 0x00, 0x32, 0, 0, 0, 7, 10, 0, 0, 80, 0, 0}, append([]byte(gps), 0xa5, 0x03)...))
	add([]byte{0x89, 0x00, 0xa1, 0x00, 0x0e, 0, 0, 0, 1, 10, 0, 0, 2, 10, 0, 0, 1, 'H', 0, 0x00, 0x03})
}

func fuzzDecode(t *testing.T, data []byte) {
	m, err := hytera.DecodeMessage(data)
	if err != nil {
		if !errors.Is(err, hytera.ErrShort) && !errors.Is(err, hytera.ErrLength) && !errors.Is(err, hytera.ErrSignature) {
			t.Fatalf("DecodeMessage(%x): unexpected error %v", data, err)
		}
		return
	}
	out, err := m.AppendBinary(nil)
	if err != nil || !bytes.Equal(out, data) {
		t.Fatalf("DecodeMessage(%x) encodes to %x, %v", data, out, err)
	}
	p := m.Payload
	if a, err := hytera.DecodeLPAnswer(p); err == nil {
		if b, err := a.AppendBinary(nil); err != nil || !bytes.Equal(b, p) {
			t.Fatalf("LPAnswer of %x encodes to %x, %v", p, b, err)
		}
		_, _ = a.GPS.Latitude(), a.GPS.Longitude()
	} else if !errors.Is(err, hytera.ErrShort) && !errors.Is(err, hytera.ErrLength) && !errors.Is(err, hytera.ErrSignature) && !errors.Is(err, hytera.ErrText) {
		t.Fatalf("DecodeLPAnswer(%x): %v", p, err)
	}
	if tm, err := hytera.DecodeTMPMessage(p); err == nil {
		_ = tm.TextUTF8(nil)
	}
}
