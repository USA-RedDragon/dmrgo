package enums_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
)

func TestSPIDToName(t *testing.T) {
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
		if got := enums.SPIDToName(tt.spid); got != tt.expected {
			t.Errorf("SPIDToName(%d) = %q, want %q", tt.spid, got, tt.expected)
		}
	}
}

func TestSPIDFromInt(t *testing.T) {
	t.Parallel()
	if _, err := enums.SPIDFromInt(0); err != nil {
		t.Errorf("SPIDFromInt(0) returned error: %v", err)
	}
	if _, err := enums.SPIDFromInt(128); err == nil {
		t.Error("SPIDFromInt(128) should return error")
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
