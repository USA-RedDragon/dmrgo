package shortdata_test

import (
	"bytes"
	"encoding/hex"
	"errors"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/internal/appcapture"
	"github.com/USA-RedDragon/dmrgo/v2/shortdata"
)

func unhex(t testing.TB, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(s)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestHyteraTextCaptures(t *testing.T) {
	t.Parallel()

	tests := []struct {
		data  string
		field uint16
		text  string
	}{
		{appcapture.HSMS, 0x4500, "HYT TEST"},
		{"000041000000", 0, "A"},
	}
	for _, tt := range tests {
		data := unhex(t, tt.data)
		h, err := shortdata.DecodeHyteraText(data)
		if err != nil {
			t.Fatal(err)
		}
		if h.Field != tt.field || string(h.TextUTF8(nil)) != tt.text {
			t.Errorf("%s: field %04x text %q", tt.data, h.Field, h.TextUTF8(nil))
		}
		var out shortdata.HyteraText
		out.Field = h.Field
		buf := out.SetTextUTF8(nil, []byte(tt.text))
		if !bytes.Equal(out.Text, buf) {
			t.Errorf("SetTextUTF8 set %x, returned %x", out.Text, buf)
		}
		enc, err := out.AppendBinary(nil)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.HasPrefix(data, enc) {
			t.Errorf("AppendBinary = %x, want a prefix of %x", enc, data)
		}
	}
	if _, err := shortdata.DecodeHyteraText([]byte{0}); !errors.Is(err, shortdata.ErrShort) {
		t.Errorf("one octet: %v", err)
	}
}

func TestText(t *testing.T) {
	t.Parallel()

	tests := []struct {
		format enums.DDFormat
		data   []byte
		want   string
	}{
		{enums.DDUTF8, []byte("Grüße\x00\x00"), "Grüße"},
		{enums.DDUTF8, []byte{0xFF, 'a'}, "�a"},
		{enums.DDISO8859Part1, []byte{'G', 'r', 0xFC, 0xDF, 'e', 0, 0}, "Grüße"},
		{enums.DDUTF16BE, []byte{0, 'H', 0, 'i', 0, 0, 0, 0}, "Hi"},
		{enums.DDUTF16LE, []byte{'H', 0, 'i', 0, 0, 0}, "Hi"},
		{enums.DDUTF16, []byte{0, 'H', 0, 'i'}, "Hi"},
		{enums.DDUTF16, []byte{0xFF, 0xFE, 'H', 0, 'i', 0}, "Hi"},
		{enums.DDUTF16, []byte{0xFE, 0xFF, 0, 'H', 0, 'i'}, "Hi"},
	}
	for _, tt := range tests {
		got, err := shortdata.Text([]byte("x"), tt.format, tt.data)
		if err != nil || string(got) != "x"+tt.want {
			t.Errorf("Text(%s, %x) = %q, %v, want %q", tt.format, tt.data, got, err, tt.want)
		}
	}
	for _, f := range []enums.DDFormat{enums.DDBinary, enums.DDBCD, enums.DD7Bit, enums.DDISO8859Part2, enums.DDUTF32LE, 0x3F} {
		if got, err := shortdata.Text([]byte("x"), f, []byte("a")); !errors.Is(err, shortdata.ErrFormat) || string(got) != "x" {
			t.Errorf("Text(%s) = %q, %v", f, got, err)
		}
		if got, err := shortdata.AppendText([]byte("x"), f, []byte("a")); !errors.Is(err, shortdata.ErrFormat) || string(got) != "x" {
			t.Errorf("AppendText(%s) = %q, %v", f, got, err)
		}
	}
}

func TestAppendText(t *testing.T) {
	t.Parallel()

	in := []byte("Grüße €")
	for _, f := range []enums.DDFormat{enums.DDUTF8, enums.DDUTF16, enums.DDUTF16BE, enums.DDUTF16LE} {
		enc, err := shortdata.AppendText(nil, f, in)
		if err != nil {
			t.Fatal(err)
		}
		back, err := shortdata.Text(nil, f, enc)
		if err != nil || !bytes.Equal(back, in) {
			t.Errorf("%s: %x gave %q, %v", f, enc, back, err)
		}
	}
	enc, err := shortdata.AppendText(nil, enums.DDISO8859Part1, []byte("Grüße"))
	if err != nil || !bytes.Equal(enc, []byte{'G', 'r', 0xFC, 0xDF, 'e'}) {
		t.Errorf("ISO 8859-1: %x, %v", enc, err)
	}
	if got, err := shortdata.AppendText([]byte("x"), enums.DDISO8859Part1, in); !errors.Is(err, shortdata.ErrCharacter) || string(got) != "x" {
		t.Errorf("ISO 8859-1 of €: %q, %v", got, err)
	}
}
