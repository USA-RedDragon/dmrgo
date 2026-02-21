package enums

import "testing"

func TestFLCOToName(t *testing.T) {
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
			got := FLCOToName(tt.flco)
			if got != tt.expected {
				t.Errorf("FLCOToName(%d) = %q, want %q", tt.flco, got, tt.expected)
			}
		})
	}
}

func TestFLCOFromInt_ValidValues(t *testing.T) {
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
		flco, err := FLCOFromInt(tt.input)
		if err != nil {
			t.Errorf("FLCOFromInt(%d) unexpected error: %v", tt.input, err)
		}
		if flco != tt.expected {
			t.Errorf("FLCOFromInt(%d) = %d, want %d", tt.input, flco, tt.expected)
		}
	}
}

func TestFLCOFromInt_InvalidValue(t *testing.T) {
	invalidValues := []int{1, 2, 9, 100, 255}
	for _, v := range invalidValues {
		_, err := FLCOFromInt(v)
		if err == nil {
			t.Errorf("FLCOFromInt(%d) should have returned error", v)
		}
	}
}
