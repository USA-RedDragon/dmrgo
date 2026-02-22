package elements_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/layer3/elements"
)

func TestAdditionalInformationField_Constants(t *testing.T) {
	t.Parallel()
	if uint8(elements.AdditionalInfoIgnore) != 0 {
		t.Errorf("AdditionalInfoIgnore should be 0, got %d", elements.AdditionalInfoIgnore)
	}
	if uint8(elements.AdditionalInfoValid) != 1 {
		t.Errorf("AdditionalInfoValid should be 1, got %d", elements.AdditionalInfoValid)
	}
}

func TestAdditionalInformationFieldToName(t *testing.T) {
	t.Parallel()
	if name := elements.AdditionalInformationFieldToName(elements.AdditionalInfoIgnore); name != "Ignore" {
		t.Errorf("expected 'Ignore', got %q", name)
	}
	if name := elements.AdditionalInformationFieldToName(elements.AdditionalInfoValid); name != "Valid" {
		t.Errorf("expected 'Valid', got %q", name)
	}
}

func TestAdditionalInformationFieldToName_Unknown(t *testing.T) {
	t.Parallel()
	name := elements.AdditionalInformationFieldToName(elements.AdditionalInformationField(5))
	if name != "Unknown" {
		t.Errorf("expected 'Unknown', got %q", name)
	}
}

func TestAdditionalInformationField_ToString(t *testing.T) {
	t.Parallel()
	s := elements.AdditionalInfoIgnore.ToString()
	if s != "AdditionalInformationField{ Ignore }" {
		t.Errorf("unexpected ToString: %q", s)
	}
	s = elements.AdditionalInfoValid.ToString()
	if s != "AdditionalInformationField{ Valid }" {
		t.Errorf("unexpected ToString: %q", s)
	}
}
