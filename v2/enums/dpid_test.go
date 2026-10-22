package enums_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
)

func TestDPIDString(t *testing.T) {
	t.Parallel()
	tests := []struct {
		dpid     enums.DPID
		expected string
	}{
		{enums.DPIDExtendedHeader, "Extended Header"},
		{enums.DPIDTextMessage, "Text Message (5016)"},
		{enums.DPIDLocationProtocol, "Location Protocol (5017)"},
		{enums.DPID(50), "DPID(50)"},
		{enums.DPID(95), "DPID(95)"},
		{enums.DPID(127), "DPID(127)"},
	}
	for _, tt := range tests {
		if got := tt.dpid.String(); got != tt.expected {
			t.Errorf("(%d).String() = %q, want %q", tt.dpid, got, tt.expected)
		}
	}
}
