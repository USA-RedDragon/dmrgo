package enums_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
)

func TestRCCommand_String_CeaseTransmissionCommand(t *testing.T) {
	t.Parallel()
	name := enums.RCCeaseTransmissionCommand.String()
	if name != "Cease Transmission Command" {
		t.Errorf("expected 'Cease Transmission Command', got %q", name)
	}
}

func TestRCCommand_String_CeaseTransmissionRequest(t *testing.T) {
	t.Parallel()
	name := enums.RCCeaseTransmissionRequest.String()
	if name != "Cease Transmission Request" {
		t.Errorf("expected 'Cease Transmission Request', got %q", name)
	}
}

func TestRCCommand_String_Reserved(t *testing.T) {
	t.Parallel()
	name := (enums.RCCommand(0b0110)).String()
	if name != "Reserved RCCommand(6)" {
		t.Errorf("reserved value should not match known names, got %q", name)
	}
}

func TestRCCommand_Cast_CeaseCommand(t *testing.T) {
	t.Parallel()
	rc := enums.RCCommand(0b0100)
	if rc != enums.RCCeaseTransmissionCommand {
		t.Errorf("expected RCCeaseTransmissionCommand, got %d", rc)
	}
}

func TestRCCommand_Cast_CeaseRequest(t *testing.T) {
	t.Parallel()
	rc := enums.RCCommand(0b0101)
	if rc != enums.RCCeaseTransmissionRequest {
		t.Errorf("expected RCCeaseTransmissionRequest, got %d", rc)
	}
}

func TestRCCommand_Table6_32(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		rc   enums.RCCommand
		v    uint8
		name string
	}{
		{enums.RCIncreasePower, 0b0000, "Increase Power"},
		{enums.RCDecreasePower, 0b0001, "Decrease Power"},
		{enums.RCHighestPower, 0b0010, "Highest Power"},
		{enums.RCLowestPower, 0b0011, "Lowest Power"},
		{enums.RCCeaseTransmissionCommand, 0b0100, "Cease Transmission Command"},
		{enums.RCCeaseTransmissionRequest, 0b0101, "Cease Transmission Request"},
	} {
		if uint8(c.rc) != c.v || c.rc.String() != c.name {
			t.Errorf("%d: value %d name %q, want %d %q", c.v, uint8(c.rc), c.rc.String(), c.v, c.name)
		}
	}
}

func TestRCCommand_Values(t *testing.T) {
	t.Parallel()
	if int(enums.RCCeaseTransmissionCommand) != 4 {
		t.Errorf("RCCeaseTransmissionCommand should be 4, got %d", enums.RCCeaseTransmissionCommand)
	}
	if int(enums.RCCeaseTransmissionRequest) != 5 {
		t.Errorf("RCCeaseTransmissionRequest should be 5, got %d", enums.RCCeaseTransmissionRequest)
	}
}
