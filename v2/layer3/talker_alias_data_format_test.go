package layer3

import (
	"testing"
)

func TestTalkerAliasDataFormatString(t *testing.T) {
	tests := []struct {
		format   TalkerAliasDataFormat
		expected string
	}{
		{TalkerAliasSevenBit, "646 (7-bit)"},
		{TalkerAliasISOEightBit, "ISO (Latin)"},
		{TalkerAliasUTF8, "UTF-8"},
		{TalkerAliasUTF16, "UTF-16 BE"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			got := tt.format.String()
			if got != tt.expected {
				t.Errorf("(%d).String() = %q, want %q", tt.format, got, tt.expected)
			}
		})
	}
}

func TestTalkerAliasDataFormatString_Unknown(t *testing.T) {
	got := (TalkerAliasDataFormat(99)).String()
	if got != "Unknown" {
		t.Errorf("(99).String() = %q, want %q", got, "Unknown")
	}
}

func TestTalkerAliasDataFormat_Constants(t *testing.T) {
	if TalkerAliasSevenBit != 0 {
		t.Errorf("SevenBitCharacters = %d, want 0", TalkerAliasSevenBit)
	}
	if TalkerAliasISOEightBit != 1 {
		t.Errorf("ISOEightBitCharacters = %d, want 1", TalkerAliasISOEightBit)
	}
	if TalkerAliasUTF8 != 2 {
		t.Errorf("UTF8Characters = %d, want 2", TalkerAliasUTF8)
	}
	if TalkerAliasUTF16 != 3 {
		t.Errorf("UTF16Characters = %d, want 3", TalkerAliasUTF16)
	}
}
