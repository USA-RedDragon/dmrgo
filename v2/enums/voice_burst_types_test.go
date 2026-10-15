package enums

import "testing"

func TestVoiceBurstTypeString(t *testing.T) {
	tests := []struct {
		burstType VoiceBurstType
		expected  string
	}{
		{VoiceBurstUnknown, unknownName},
		{VoiceBurstA, "A"},
		{VoiceBurstB, "B"},
		{VoiceBurstC, "C"},
		{VoiceBurstD, "D"},
		{VoiceBurstE, "E"},
		{VoiceBurstF, "F"},
	}
	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			got := tt.burstType.String()
			if got != tt.expected {
				t.Errorf("(%d).String() = %q, want %q", tt.burstType, got, tt.expected)
			}
		})
	}
}

func TestVoiceBurstTypeString_InvalidReturnsUnknown(t *testing.T) {
	got := (VoiceBurstType(99)).String()
	if got != unknownName {
		t.Errorf("(99).String() = %q, want %q", got, unknownName)
	}
}
