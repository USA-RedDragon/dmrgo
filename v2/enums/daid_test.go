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
		{enums.DAID(5), "DAID(5)"},
		{enums.DAID(12), "DAID(12)"},
		{enums.DAID(15), "DAID(15)"},
	}
	for _, tt := range tests {
		if got := tt.daid.String(); got != tt.expected {
			t.Errorf("(%d).String() = %q, want %q", tt.daid, got, tt.expected)
		}
	}
}
