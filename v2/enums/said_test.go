package enums_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
)

func TestSAIDString(t *testing.T) {
	t.Parallel()
	tests := []struct {
		said     enums.SAID
		expected string
	}{
		{enums.SAIDRadioNetwork, "Radio Network"},
		{enums.SAIDUSBEthernetNetwork, "USB/Ethernet Network"},
		{enums.SAID(5), "Reserved"},
		{enums.SAIDManufacturerSpecific, "Manufacturer Specific"},
		{enums.SAID(15), "Manufacturer Specific"},
	}
	for _, tt := range tests {
		if got := tt.said.String(); got != tt.expected {
			t.Errorf("(%d).String() = %q, want %q", tt.said, got, tt.expected)
		}
	}
}

func TestSAID_Valid(t *testing.T) {
	t.Parallel()
	if !enums.SAID(0).Valid() {
		t.Error("SAID(0) is not valid")
	}
	if enums.SAID(16).Valid() {
		t.Error("SAID(16) is valid")
	}
}
