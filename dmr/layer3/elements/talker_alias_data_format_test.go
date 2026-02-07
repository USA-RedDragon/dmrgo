package elements

import "testing"

func TestNewTalkerAliasDataFormatFromBits(t *testing.T) {
	tests := []struct {
		name     string
		bits     [2]byte
		expected TalkerAliasDataFormat
	}{
		{"SevenBit_00", [2]byte{0, 0}, SevenBitCharacters},
		{"ISO_10", [2]byte{1, 0}, ISOEightBitCharacters},
		{"UTF8_01", [2]byte{0, 1}, UTF8Characters},
		{"UTF16_11", [2]byte{1, 1}, UTF16Characters},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NewTalkerAliasDataFormatFromBits(tt.bits)
			if got != tt.expected {
				t.Errorf("NewTalkerAliasDataFormatFromBits(%v) = %d, want %d", tt.bits, got, tt.expected)
			}
		})
	}
}

func TestTalkerAliasDataFormatToName(t *testing.T) {
	tests := []struct {
		format   TalkerAliasDataFormat
		expected string
	}{
		{SevenBitCharacters, "646 (7-bit)"},
		{ISOEightBitCharacters, "ISO (Latin)"},
		{UTF8Characters, "UTF-8"},
		{UTF16Characters, "UTF-16 LE"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			got := TalkerAliasDataFormatToName(tt.format)
			if got != tt.expected {
				t.Errorf("TalkerAliasDataFormatToName(%d) = %q, want %q", tt.format, got, tt.expected)
			}
		})
	}
}

func TestTalkerAliasDataFormatToName_Unknown(t *testing.T) {
	got := TalkerAliasDataFormatToName(TalkerAliasDataFormat(99))
	if got != "Unknown" {
		t.Errorf("TalkerAliasDataFormatToName(99) = %q, want %q", got, "Unknown")
	}
}

func TestTalkerAliasDataFormat_Constants(t *testing.T) {
	// Verify iota ordering
	if SevenBitCharacters != 0 {
		t.Errorf("SevenBitCharacters = %d, want 0", SevenBitCharacters)
	}
	if ISOEightBitCharacters != 1 {
		t.Errorf("ISOEightBitCharacters = %d, want 1", ISOEightBitCharacters)
	}
	if UTF8Characters != 2 {
		t.Errorf("UTF8Characters = %d, want 2", UTF8Characters)
	}
	if UTF16Characters != 3 {
		t.Errorf("UTF16Characters = %d, want 3", UTF16Characters)
	}
}
