package shortdata_test

import (
	"bytes"
	"encoding/hex"
	"errors"
	"testing"
	"unicode/utf8"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/internal/appcapture"
	"github.com/USA-RedDragon/dmrgo/v2/shortdata"
)

func FuzzText(f *testing.F) {
	textSeeds(func(format uint8, data []byte) { f.Add(format, data) })
	f.Fuzz(fuzzText)
}

func textSeeds(add func(format uint8, data []byte)) {
	h, _ := hex.DecodeString(appcapture.HSMS)
	add(uint8(enums.DDUTF16LE), h)
	add(uint8(enums.DDBCD), h)
	add(uint8(enums.DDUTF16), []byte{0xFF, 0xFE, 0x3D, 0xD8, 0x00, 0xDE})
	add(uint8(enums.DDUTF8), []byte("Grüße"))
	add(uint8(enums.DDISO8859Part1), []byte{0xFC, 0})
}

func fuzzText(t *testing.T, format uint8, data []byte) {
	f := enums.DDFormat(format & 0x3F)
	got, err := shortdata.Text(nil, f, data)
	if err != nil {
		if !errors.Is(err, shortdata.ErrFormat) || len(got) != 0 {
			t.Fatalf("Text(%s, %x) = %q, %v", f, data, got, err)
		}
		return
	}
	if !utf8.Valid(got) || bytes.IndexByte(got, 0) >= 0 {
		t.Fatalf("Text(%s, %x) = %q: not UTF-8 without NUL", f, data, got)
	}
	enc, err := shortdata.AppendText(nil, f, got)
	if err != nil {
		t.Fatalf("AppendText(%s, %q): %v", f, got, err)
	}
	back, err := shortdata.Text(nil, f, enc)
	if err != nil || !bytes.Equal(back, got) {
		t.Fatalf("round trip of %q through %s gave %q, %v", got, f, back, err)
	}
	h, err := shortdata.DecodeHyteraText(data)
	if err != nil {
		if !errors.Is(err, shortdata.ErrShort) {
			t.Fatalf("DecodeHyteraText(%x): %v", data, err)
		}
		return
	}
	out, err := h.AppendBinary(nil)
	if err != nil || !bytes.Equal(out, data) {
		t.Fatalf("HyteraText round trip of %x gave %x, %v", data, out, err)
	}
}
