package enums

import "testing"

func TestVoiceBurstTypeToName(t *testing.T) {
	tests := []struct {
		burstType VoiceBurstType
		expected  string
	}{
		{VoiceBurstUnknown, "Unknown"},
		{VoiceBurstA, "A"},
		{VoiceBurstB, "B"},
		{VoiceBurstC, "C"},
		{VoiceBurstD, "D"},
		{VoiceBurstE, "E"},
		{VoiceBurstF, "F"},
	}
	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			got := VoiceBurstTypeToName(tt.burstType)
			if got != tt.expected {
				t.Errorf("VoiceBurstTypeToName(%d) = %q, want %q", tt.burstType, got, tt.expected)
			}
		})
	}
}

func TestVoiceBurstTypeToName_InvalidReturnsUnknown(t *testing.T) {
	got := VoiceBurstTypeToName(VoiceBurstType(999))
	if got != "Unknown" {
		t.Errorf("VoiceBurstTypeToName(999) = %q, want %q", got, "Unknown")
	}
}
