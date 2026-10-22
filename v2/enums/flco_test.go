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
