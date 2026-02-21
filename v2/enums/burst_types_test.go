package enums

import "testing"

func TestBurstTypeConstants(t *testing.T) {
	// Verify that burst type constants have the expected values
	if BurstTypeUnknown != -1 {
		t.Errorf("BurstTypeUnknown = %d, want -1", BurstTypeUnknown)
	}
	if BurstTypeVocoder != 0 {
		t.Errorf("BurstTypeVocoder = %d, want 0", BurstTypeVocoder)
	}
	if BurstTypeDataAndControl != 1 {
		t.Errorf("BurstTypeDataAndControl = %d, want 1", BurstTypeDataAndControl)
	}
}
