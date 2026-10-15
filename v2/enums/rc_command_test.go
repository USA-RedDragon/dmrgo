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
	name := (enums.RCCommand(0)).String()
	if name == "Cease Transmission Command" || name == "Cease Transmission Request" {
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

func TestRCCommand_Cast_Reserved(t *testing.T) {
	t.Parallel()
	rc := enums.RCCommand(0)
	if int(rc) != 0 {
		t.Errorf("expected 0, got %d", rc)
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
