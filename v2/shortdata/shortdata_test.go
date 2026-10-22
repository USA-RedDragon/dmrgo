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
		{enums.DDBinary, []byte{0, 0xFF, 'a'}, "\x00\xFFa"},
		{enums.DDBCD, []byte{0x12, 0x34, 0xAB, 0x5F, 0x66}, "1234*#5"},
		{enums.DDBCD, []byte{0xC1, 0x0E}, "\uFFFD10\uFFFD"},
		{enums.DD7Bit, []byte{0x91, 0xA7}, "Hi"},
		{enums.DD7Bit, []byte{0x91, 0x97, 0x66, 0xCD, 0xFF}, "Hello"},
		{enums.DD7Bit, []byte{0x83, 0x0A, 0x1C, 0x48, 0xB1, 0xA3, 0xFF}, "ABCDEFG"},
		{enums.DD7Bit, []byte{0x91, 0x80, 0x00}, "H`"},
		{enums.DD7Bit, []byte{0x91, 0xFF, 0xFF}, "H"},
		{enums.DD7Bit, []byte{0xFF, 0x20, 0x7F}, "\x7FH\x0F"},
		{enums.DDISO8859Part2, []byte{'z', 0xA1, 0xFF, 0}, "zĄ˙"},
		{enums.DDISO8859Part3, []byte{0xA5, 0xA1}, "\uFFFDĦ"},
		{enums.DDISO8859Part5, []byte{0xC0, 0xF0}, "Р№"},
		{enums.DDISO8859Part6, []byte{0xC7, 0xA1}, "ا\uFFFD"},
		{enums.DDISO8859Part7, []byte{0xC1, 0xA4}, "Α€"},
		{enums.DDISO8859Part8, []byte{0xE0, 0xFD}, "א\u200E"},
		{enums.DDISO8859Part9, []byte{0xD0, 0xFE}, "Ğş"},
		{enums.DDISO8859Part11, []byte{0xA1, 0xDA, 0xDB, 0xDF, 0xFB, 0xFC}, "ก\u0E3A\uFFFD฿๛\uFFFD"},
		{enums.DDISO8859Part13, []byte{0xA1, 0xFF}, "”’"},
		{enums.DDISO8859Part14, []byte{0xA1, 0xA2}, "Ḃḃ"},
		{enums.DDISO8859Part15, []byte{0xA4, 0xBC, 0x85}, "€Œ\u0085"},
		{enums.DDISO8859Part16, []byte{0xA1, 0xAA}, "ĄȘ"},
		{enums.DDUTF32BE, []byte{0, 0, 0, 'H', 0, 0x01, 0xF6, 0x00, 0, 0x11, 0, 0, 0, 0, 0, 0, 0, 0, 0, 'x'}, "H😀\uFFFD"},
		{enums.DDUTF32LE, []byte{'H', 0, 0, 0, 'i', 0, 0, 0, 0, 0}, "Hi"},
		{enums.DDUTF32, []byte{0xFF, 0xFE, 0, 0, 'H', 0, 0, 0}, "H"},
		{enums.DDUTF32, []byte{0, 0, 0xFE, 0xFF, 0, 0, 0, 'H'}, "H"},
		{enums.DDUTF32, []byte{0, 0, 0, 'H', 0, 0}, "H"},
		{enums.DDUTF8, []byte("Hi\x7f\x7f"), "Hi"},
		{enums.DDUTF8, []byte("H\x7fi\x7f\x00"), "H\x7fi"},
		{enums.DDISO8859Part1, []byte{'H', 'i', 0x7F}, "Hi"},
		{enums.DDUTF16BE, []byte{0, 'H', 0, 'i', 0, 0x7F, 0, 0x7F}, "Hi"},
		{enums.DDUTF16LE, []byte{'H', 0, 0x7F, 0, 0, 0}, "H"},
	}
	for _, tt := range tests {
		got, err := shortdata.DecodeText([]byte("x"), tt.format, tt.data)
		if err != nil || string(got) != "x"+tt.want {
			t.Errorf("Text(%s, %x) = %q, %v, want %q", tt.format, tt.data, got, err, tt.want)
		}
	}
	for _, f := range []enums.DDFormat{0x19, 0x3F} {
		if got, err := shortdata.DecodeText([]byte("x"), f, []byte("a")); !errors.Is(err, shortdata.ErrFormat) || string(got) != "x" {
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
		back, err := shortdata.DecodeText(nil, f, enc)
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

func TestAppendTextCodings(t *testing.T) {
	t.Parallel()

	tests := []struct {
		format enums.DDFormat
		text   string
		want   []byte
	}{
		{enums.DDBinary, "\x00\xFF", []byte{0, 0xFF}},
		{enums.DDBCD, "12*#3", []byte{0x12, 0xAB, 0x3F}},
		{enums.DDBCD, "0987", []byte{0x09, 0x87}},
		{enums.DD7Bit, "Hello", []byte{0x91, 0x97, 0x66, 0xCD, 0xFF}},
		{enums.DD7Bit, "ABCDEFGH", []byte{0x83, 0x0A, 0x1C, 0x48, 0xB1, 0xA3, 0xC8}},
		{enums.DDISO8859Part2, "Ąz˙", []byte{0xA1, 'z', 0xFF}},
		{enums.DDISO8859Part7, "Α€", []byte{0xC1, 0xA4}},
		{enums.DDISO8859Part11, "ก๛", []byte{0xA1, 0xFB}},
		{enums.DDISO8859Part15, "€\u0085", []byte{0xA4, 0x85}},
		{enums.DDUTF32, "H😀", []byte{0, 0, 0xFE, 0xFF, 0, 0, 0, 'H', 0, 0x01, 0xF6, 0x00}},
		{enums.DDUTF32LE, "H", []byte{'H', 0, 0, 0}},
		{enums.DDUTF32BE, "H", []byte{0, 0, 0, 'H'}},
	}
	for _, tt := range tests {
		got, err := shortdata.AppendText([]byte("x"), tt.format, []byte(tt.text))
		if err != nil || !bytes.Equal(got, append([]byte("x"), tt.want...)) {
			t.Errorf("AppendText(%s, %q) = %x, %v, want %x", tt.format, tt.text, got, err, tt.want)
			continue
		}
		back, err := shortdata.DecodeText(nil, tt.format, got[1:])
		if err != nil || string(back) != tt.text {
			t.Errorf("Text(%s, %x) = %q, %v, want %q", tt.format, got[1:], back, err, tt.text)
		}
	}
	for _, tt := range []struct {
		format enums.DDFormat
		text   string
	}{
		{enums.DDBCD, "12a"},
		{enums.DDBCD, "1\xFF"},
		{enums.DD7Bit, "é"},
		{enums.DDISO8859Part2, "€"},
		{enums.DDISO8859Part3, "\xFF"},
		{enums.DDISO8859Part5, "\uFFFD"},
	} {
		if got, err := shortdata.AppendText([]byte("x"), tt.format, []byte(tt.text)); !errors.Is(err, shortdata.ErrCharacter) || string(got) != "x" {
			t.Errorf("AppendText(%s, %q) = %q, %v", tt.format, tt.text, got, err)
		}
	}
}
