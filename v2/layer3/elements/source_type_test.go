package elements_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/layer3/elements"
)

func TestSourceType_Constants(t *testing.T) {
	t.Parallel()
	if uint8(elements.SourceTypeBS) != 0 {
		t.Errorf("SourceTypeBS should be 0, got %d", elements.SourceTypeBS)
	}
	if uint8(elements.SourceTypeMS) != 1 {
		t.Errorf("SourceTypeMS should be 1, got %d", elements.SourceTypeMS)
	}
}

func TestSourceTypeToName(t *testing.T) {
	t.Parallel()
	if name := elements.SourceTypeToName(elements.SourceTypeBS); name != "BS" {
		t.Errorf("expected 'BS', got %q", name)
	}
	if name := elements.SourceTypeToName(elements.SourceTypeMS); name != "MS" {
		t.Errorf("expected 'MS', got %q", name)
	}
}

func TestSourceTypeToName_Unknown(t *testing.T) {
	t.Parallel()
	name := elements.SourceTypeToName(elements.SourceType(5))
	if name != "Unknown" {
		t.Errorf("expected 'Unknown', got %q", name)
	}
}

func TestSourceType_ToString(t *testing.T) {
	t.Parallel()
	s := elements.SourceTypeBS.ToString()
	if s != "SourceType{ BS }" {
		t.Errorf("unexpected ToString: %q", s)
	}
	s = elements.SourceTypeMS.ToString()
	if s != "SourceType{ MS }" {
		t.Errorf("unexpected ToString: %q", s)
	}
}
