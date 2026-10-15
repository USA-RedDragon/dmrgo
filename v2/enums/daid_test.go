package enums_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
)

func TestDAIDString(t *testing.T) {
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
		if got := tt.daid.String(); got != tt.expected {
			t.Errorf("(%d).String() = %q, want %q", tt.daid, got, tt.expected)
		}
	}
}

func TestDAID_Valid(t *testing.T) {
	t.Parallel()
	if !enums.DAID(0).Valid() {
		t.Error("DAID(0) is not valid")
	}
	if enums.DAID(16).Valid() {
		t.Error("DAID(16) is valid")
	}
}
