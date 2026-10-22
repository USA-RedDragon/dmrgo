package enums_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
)

func TestSPIDString(t *testing.T) {
	t.Parallel()
	tests := []struct {
		spid     enums.SPID
		expected string
	}{
		{enums.SPIDExtendedHeader, "Extended Header"},
		{enums.SPIDTextMessage, "Text Message (5016)"},
		{enums.SPIDLocationProtocol, "Location Protocol (5017)"},
		{enums.SPID(50), "SPID(50)"},
		{enums.SPID(95), "SPID(95)"},
		{enums.SPID(127), "SPID(127)"},
	}
	for _, tt := range tests {
		if got := tt.spid.String(); got != tt.expected {
			t.Errorf("(%d).String() = %q, want %q", tt.spid, got, tt.expected)
		}
	}
}
