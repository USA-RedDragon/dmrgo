package layer3_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/layer3"
)

func TestSourceType_Constants(t *testing.T) {
	t.Parallel()
	if uint8(layer3.SourceTypeBS) != 0 {
		t.Errorf("SourceTypeBS should be 0, got %d", layer3.SourceTypeBS)
	}
	if uint8(layer3.SourceTypeMS) != 1 {
		t.Errorf("SourceTypeMS should be 1, got %d", layer3.SourceTypeMS)
	}
}

func TestSourceTypeToName(t *testing.T) {
	t.Parallel()
	if name := layer3.SourceTypeToName(layer3.SourceTypeBS); name != "BS" {
		t.Errorf("expected 'BS', got %q", name)
	}
	if name := layer3.SourceTypeToName(layer3.SourceTypeMS); name != "MS" {
		t.Errorf("expected 'MS', got %q", name)
	}
}

func TestSourceTypeToName_Unknown(t *testing.T) {
	t.Parallel()
	name := layer3.SourceTypeToName(layer3.SourceType(5))
	if name != "Unknown" {
		t.Errorf("expected 'Unknown', got %q", name)
	}
}

func TestSourceType_ToString(t *testing.T) {
	t.Parallel()
	s := layer3.SourceTypeBS.ToString()
	if s != "SourceType{ BS }" {
		t.Errorf("unexpected ToString: %q", s)
	}
	s = layer3.SourceTypeMS.ToString()
	if s != "SourceType{ MS }" {
		t.Errorf("unexpected ToString: %q", s)
	}
}
