package enums

import "testing"

func TestDDFormatNames(t *testing.T) {
	t.Parallel()

	for f, want := range map[DDFormat]string{
		DDBinary: "Binary", DDISO8859Part1: "ISO 8859-1", DDISO8859Part11: "ISO 8859-11",
		DDISO8859Part13: "ISO 8859-13", DDISO8859Part16: "ISO 8859-16", DDUTF16LE: "UTF-16LE",
		DDUTF32LE: "UTF-32LE", 0x3F: "DDFormat(63)",
	} {
		if got := f.String(); got != want {
			t.Errorf("%d: %q, want %q", uint8(f), got, want)
		}
	}
}
