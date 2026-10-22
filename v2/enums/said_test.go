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
		{enums.SAID(5), "SAID(5)"},
		{enums.SAID(12), "SAID(12)"},
		{enums.SAID(15), "SAID(15)"},
	}
	for _, tt := range tests {
		if got := tt.said.String(); got != tt.expected {
			t.Errorf("(%d).String() = %q, want %q", tt.said, got, tt.expected)
		}
	}
}
