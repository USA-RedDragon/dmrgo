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
		{enums.DPID(50), "Reserved"},
		{enums.DPIDManufacturerSpecific, "Manufacturer Specific"},
		{enums.DPID(127), "Manufacturer Specific"},
	}
	for _, tt := range tests {
		if got := tt.dpid.String(); got != tt.expected {
			t.Errorf("(%d).String() = %q, want %q", tt.dpid, got, tt.expected)
		}
	}
}

func TestDPIDToPort(t *testing.T) {
	t.Parallel()
	if p := enums.DPIDToPort(enums.DPIDTextMessage); p != 5016 {
		t.Errorf("DPIDToPort(TextMessage) = %d, want 5016", p)
	}
	if p := enums.DPIDToPort(enums.DPIDLocationProtocol); p != 5017 {
		t.Errorf("DPIDToPort(LocationProtocol) = %d, want 5017", p)
	}
	if p := enums.DPIDToPort(enums.DPIDExtendedHeader); p != 0 {
		t.Errorf("DPIDToPort(ExtendedHeader) = %d, want 0", p)
	}
}

func TestDPID_Valid(t *testing.T) {
	t.Parallel()
	if !enums.DPID(0).Valid() {
		t.Error("DPID(0) is not valid")
	}
	if enums.DPID(128).Valid() {
		t.Error("DPID(128) is valid")
	}
}
