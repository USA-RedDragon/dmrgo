package enums_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
)

func TestDAIDToName(t *testing.T) {
	t.Parallel()
	tests := []struct {
		daid     enums.DAID
		expected string
	}{
		{enums.DAIDRadioNetwork, "Radio Network"},
		{enums.DAIDUSBEthernetNetwork, "USB/Ethernet Network"},
		{enums.DAIDGroupNetwork, "Group Network"},
		{enums.DAID(5), "Reserved"},
		{enums.DAIDManufacturerSpecific, "Manufacturer Specific"},
		{enums.DAID(15), "Manufacturer Specific"},
	}
	for _, tt := range tests {
		if got := enums.DAIDToName(tt.daid); got != tt.expected {
			t.Errorf("DAIDToName(%d) = %q, want %q", tt.daid, got, tt.expected)
		}
	}
}

func TestDAIDFromInt(t *testing.T) {
	t.Parallel()
	if _, err := enums.DAIDFromInt(0); err != nil {
		t.Errorf("DAIDFromInt(0) returned error: %v", err)
	}
	if _, err := enums.DAIDFromInt(16); err == nil {
		t.Error("DAIDFromInt(16) should return error")
	}
}
