package enums

import "testing"

func TestFLCOString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		flco     FLCO
		expected string
	}{
		{FLCOGroupVoiceChannelUser, "Group Voice Channel User"},
		{FLCOUnitToUnitVoiceChannelUser, "Unit to Unit Voice Channel User"},
		{FLCOTalkerAliasHeader, "Talker Alias Header"},
		{FLCOTalkerAliasBlock1, "Talker Alias Block 1"},
		{FLCOTalkerAliasBlock2, "Talker Alias Block 2"},
		{FLCOTalkerAliasBlock3, "Talker Alias Block 3"},
		{FLCOGPSInfo, "GPS Info"},
		{FLCOTerminatorDataLinkControl, "Terminator Data Link Control"},
		{FLCO(0xFF), "Unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			t.Parallel()

			got := tt.flco.String()
			if got != tt.expected {
				t.Errorf("(%d).String() = %q, want %q", tt.flco, got, tt.expected)
			}
		})
	}
}

func TestFLCO_ValidValues(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input    int
		expected FLCO
	}{
		{0b000000, FLCOGroupVoiceChannelUser},
		{0b000011, FLCOUnitToUnitVoiceChannelUser},
		{0b000100, FLCOTalkerAliasHeader},
		{0b000101, FLCOTalkerAliasBlock1},
		{0b000110, FLCOTalkerAliasBlock2},
		{0b000111, FLCOTalkerAliasBlock3},
		{0b001000, FLCOGPSInfo},
		{0b110000, FLCOTerminatorDataLinkControl},
	}

	for _, tt := range tests {
		flco := FLCO(tt.input)
		if !flco.Valid() {
			t.Errorf("FLCO(%d) is not valid", tt.input)
		}
		if flco != tt.expected {
			t.Errorf("FLCO(%d) = %d, want %d", tt.input, flco, tt.expected)
		}
	}
}

func TestFLCO_InvalidValue(t *testing.T) {
	t.Parallel()

	invalidValues := []int{1, 2, 9, 100, 255}
	for _, v := range invalidValues {
		if FLCO(v).Valid() {
			t.Errorf("FLCO(%d) is valid", v)
		}
	}
}
