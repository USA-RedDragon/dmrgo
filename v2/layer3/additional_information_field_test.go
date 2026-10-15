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

func TestAdditionalInformationFieldString(t *testing.T) {
	t.Parallel()
	if name := layer3.AdditionalInfoIgnore.String(); name != "Ignore" {
		t.Errorf("expected 'Ignore', got %q", name)
	}
	if name := layer3.AdditionalInfoValid.String(); name != "Valid" {
		t.Errorf("expected 'Valid', got %q", name)
	}
}

func TestAdditionalInformationFieldString_Unknown(t *testing.T) {
	t.Parallel()
	name := (layer3.AdditionalInformationField(5)).String()
	if name != "Unknown" {
		t.Errorf("expected 'Unknown', got %q", name)
	}
}

func TestAdditionalInformationField_String(t *testing.T) {
	t.Parallel()
	s := layer3.AdditionalInfoIgnore.String()
	if s != "Ignore" {
		t.Errorf("unexpected String: %q", s)
	}
	s = layer3.AdditionalInfoValid.String()
	if s != "Valid" {
		t.Errorf("unexpected String: %q", s)
	}
}
