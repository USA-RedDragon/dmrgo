package enums_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
)

func TestSAIDToName(t *testing.T) {
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
		if got := enums.SAIDToName(tt.said); got != tt.expected {
			t.Errorf("SAIDToName(%d) = %q, want %q", tt.said, got, tt.expected)
		}
	}
}

func TestSAIDFromInt(t *testing.T) {
	t.Parallel()
	if _, err := enums.SAIDFromInt(0); err != nil {
		t.Errorf("SAIDFromInt(0) returned error: %v", err)
	}
	if _, err := enums.SAIDFromInt(16); err == nil {
		t.Error("SAIDFromInt(16) should return error")
	}
}
