package layer3_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/layer3"
)

func TestAdditionalInformationField_Constants(t *testing.T) {
	t.Parallel()
	if uint8(layer3.AdditionalInfoIgnore) != 0 {
		t.Errorf("AdditionalInfoIgnore should be 0, got %d", layer3.AdditionalInfoIgnore)
	}
	if uint8(layer3.AdditionalInfoValid) != 1 {
		t.Errorf("AdditionalInfoValid should be 1, got %d", layer3.AdditionalInfoValid)
	}
}

func TestAdditionalInformationFieldToName(t *testing.T) {
	t.Parallel()
	if name := layer3.AdditionalInformationFieldToName(layer3.AdditionalInfoIgnore); name != "Ignore" {
		t.Errorf("expected 'Ignore', got %q", name)
	}
	if name := layer3.AdditionalInformationFieldToName(layer3.AdditionalInfoValid); name != "Valid" {
		t.Errorf("expected 'Valid', got %q", name)
	}
}

func TestAdditionalInformationFieldToName_Unknown(t *testing.T) {
	t.Parallel()
	name := layer3.AdditionalInformationFieldToName(layer3.AdditionalInformationField(5))
	if name != "Unknown" {
		t.Errorf("expected 'Unknown', got %q", name)
	}
}

func TestAdditionalInformationField_ToString(t *testing.T) {
	t.Parallel()
	s := layer3.AdditionalInfoIgnore.ToString()
	if s != "AdditionalInformationField{ Ignore }" {
		t.Errorf("unexpected ToString: %q", s)
	}
	s = layer3.AdditionalInfoValid.ToString()
	if s != "AdditionalInformationField{ Valid }" {
		t.Errorf("unexpected ToString: %q", s)
	}
}
