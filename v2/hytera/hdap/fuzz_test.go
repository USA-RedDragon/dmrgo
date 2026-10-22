package hdap_test

import (
	"bytes"
	"errors"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/hytera/hdap"
)

func FuzzDecode(f *testing.F) {
	decodeSeeds(func(data []byte) { f.Add(data) })
	f.Fuzz(fuzzDecode)
}

func decodeSeeds(add func(data []byte)) {
	add([]byte{0x11, 0x00, 0x03, 0x00, 0x04, 0x0a, 0x00, 0x00, 0x50, 0xd1, 0x03})
	add(append([]byte{0x08, 0xa0, 0x02, 0x00, 0x32, 0, 0, 0, 7, 10, 0, 0, 80, 0, 0}, append([]byte(gps), 0xa5, 0x03)...))
	add([]byte("\x08\xa0\x02\x00\x32\x00\x00\x00\x03\x00\x23\x37\xfb\x00\x00A\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00N5003.8771E01426.5302\x00\x00\x00\x00\x00\x00\x70\x03"))
	add([]byte{0x89, 0x00, 0xa1, 0x00, 0x0e, 0, 0, 0, 1, 10, 0, 0, 2, 10, 0, 0, 1, 'H', 0, 0x00, 0x03})
}

func fuzzDecode(t *testing.T, data []byte) {
	m, err := hdap.DecodeMessage(data)
	if err != nil {
		if !errors.Is(err, hdap.ErrShort) && !errors.Is(err, hdap.ErrLength) && !errors.Is(err, hdap.ErrSignature) {
			t.Fatalf("DecodeMessage(%x): unexpected error %v", data, err)
		}
		return
	}
	out, err := m.AppendBinary(nil)
	if err != nil || !bytes.Equal(out, data) {
		t.Fatalf("DecodeMessage(%x) encodes to %x, %v", data, out, err)
	}
	p := m.Payload
	if a, err := hdap.DecodeLPAnswer(p); err == nil {
		b, err := a.AppendBinary(nil)
		if err != nil {
			t.Fatalf("LPAnswer of %x does not encode: %v", p, err)
		}
		if again, err := hdap.DecodeLPAnswer(b); err != nil || again != a || !nulZeroEqual(b, p) {
			t.Fatalf("LPAnswer of %x encodes to %x, %v", p, b, err)
		}
		_, _ = a.GPS.Latitude(), a.GPS.Longitude()
	} else if !errors.Is(err, hdap.ErrShort) && !errors.Is(err, hdap.ErrLength) && !errors.Is(err, hdap.ErrSignature) && !errors.Is(err, hdap.ErrText) {
		t.Fatalf("DecodeLPAnswer(%x): %v", p, err)
	}
	if tm, err := hdap.DecodeTMPMessage(p); err == nil {
		_ = tm.TextUTF8(nil)
	}
}
