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
		{enums.SPID(50), "Reserved"},
		{enums.SPIDManufacturerSpecific, "Manufacturer Specific"},
		{enums.SPID(127), "Manufacturer Specific"},
	}
	for _, tt := range tests {
		if got := tt.spid.String(); got != tt.expected {
			t.Errorf("(%d).String() = %q, want %q", tt.spid, got, tt.expected)
		}
	}
}

func TestSPIDToPort(t *testing.T) {
	t.Parallel()
	if p := enums.SPIDToPort(enums.SPIDTextMessage); p != 5016 {
		t.Errorf("SPIDToPort(TextMessage) = %d, want 5016", p)
	}
	if p := enums.SPIDToPort(enums.SPIDLocationProtocol); p != 5017 {
		t.Errorf("SPIDToPort(LocationProtocol) = %d, want 5017", p)
	}
	if p := enums.SPIDToPort(enums.SPIDExtendedHeader); p != 0 {
		t.Errorf("SPIDToPort(ExtendedHeader) = %d, want 0", p)
	}
}

func TestSPID_Valid(t *testing.T) {
	t.Parallel()
	if !enums.SPID(0).Valid() {
		t.Error("SPID(0) is not valid")
	}
	if enums.SPID(128).Valid() {
		t.Error("SPID(128) is valid")
	}
}
