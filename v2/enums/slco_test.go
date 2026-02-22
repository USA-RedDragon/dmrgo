package enums_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
)

func TestSLCO_ToName_NullMessage(t *testing.T) {
	t.Parallel()
	name := enums.SLCOToName(enums.SLCONullMessage)
	if name != "Null Message" {
		t.Errorf("expected 'Null Message', got %q", name)
	}
}

func TestSLCO_ToName_ActivityUpdate(t *testing.T) {
	t.Parallel()
	name := enums.SLCOToName(enums.SLCOActivityUpdate)
	if name != "Activity Update" {
		t.Errorf("expected 'Activity Update', got %q", name)
	}
}

func TestSLCO_ToName_Reserved(t *testing.T) {
	t.Parallel()
	name := enums.SLCOToName(enums.SLCO(15))
	if name == "Null Message" || name == "Activity Update" {
		t.Errorf("reserved value should not match known names, got %q", name)
	}
}

func TestSLCO_FromInt_NullMessage(t *testing.T) {
	t.Parallel()
	slco := enums.SLCOFromInt(0)
	if slco != enums.SLCONullMessage {
		t.Errorf("expected SLCONullMessage, got %d", slco)
	}
}

func TestSLCO_FromInt_ActivityUpdate(t *testing.T) {
	t.Parallel()
	slco := enums.SLCOFromInt(1)
	if slco != enums.SLCOActivityUpdate {
		t.Errorf("expected SLCOActivityUpdate, got %d", slco)
	}
}

func TestSLCO_FromInt_Reserved(t *testing.T) {
	t.Parallel()
	slco := enums.SLCOFromInt(5)
	// Reserved values are returned as-is
	if int(slco) != 5 {
		t.Errorf("expected 5, got %d", slco)
	}
}

func TestSLCO_Values(t *testing.T) {
	t.Parallel()
	if int(enums.SLCONullMessage) != 0 {
		t.Errorf("SLCONullMessage should be 0, got %d", enums.SLCONullMessage)
	}
	if int(enums.SLCOActivityUpdate) != 1 {
		t.Errorf("SLCOActivityUpdate should be 1, got %d", enums.SLCOActivityUpdate)
	}
}
