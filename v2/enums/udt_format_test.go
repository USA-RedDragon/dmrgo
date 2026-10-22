package enums

import "testing"

func TestUDTFormat_Table7_88(t *testing.T) {
	t.Parallel()
	for f, want := range map[UDTFormat]string{
		UDTBinary: "Binary", UDTAddress: "Address", UDTBCD: "BCD", UDTISO7: "ISO 7 bit", UDTISO8: "ISO 8 bit",
		UDTNMEA: "NMEA", UDTIP: "IP", UDTUTF16BE: "UTF-16BE", UDTManufacturer1: "Manufacturer Specific",
		UDTManufacturer2: "Manufacturer Specific", UDTMixed: "Mixed", UDTLIP: "LIP", 0b1100: "UDTFormat(12)",
	} {
		if got := f.String(); got != want {
			t.Errorf("UDTFormat %d: %q, want %q", uint8(f), got, want)
		}
	}
	if UDTLIP != 0b1011 || UDTMixed != 0b1010 || UDTUTF16BE != 0b0111 {
		t.Error("UDTFormat values")
	}
}
