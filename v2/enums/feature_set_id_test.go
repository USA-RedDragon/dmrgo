package enums

import (
	"strings"
	"testing"
)

func TestFeatureSetIDString_KnownIDs(t *testing.T) {
	tests := []struct {
		fsid     FeatureSetID
		contains string // substring that should appear in the name
	}{
		{StandardizedFID, "Standardized"},
		{FlydeMicroLtd, "Flyde"},
		{ProdElSpa, "PROD-EL"},
		{TridentMicroSystems, "Trident"},
		{RadiodataGmbh, "RADIODATA"},
		{HytScienceTech, "HYT"},
		{AselsanElektronik, "ASELSAN"},
		{KirisunCommunications, "Kirisun"},
		{DmrAssociationLtd, "DMR Association"},
		{MotorolaLtd, "Motorola"},
		{ElectronicMarketingCompany, "EMC"},
		{ElectronicMarketingCompany2, "EMC"},
		{JvcKenwood, "Kenwood"},
		{RadioActivity, "Radio Activity"},
		{RadioActivity2, "Radio Activity"},
		{TaitElectronicsLtd, "Tait"},
		{HytScienceTech2, "Hyt"},
		{VertexStandard, "Vertex"},
	}
	for _, tt := range tests {
		name := tt.fsid.String()
		if len(name) == 0 {
			t.Errorf("(%d).String() returned empty string", tt.fsid)
		}
		if !strings.Contains(name, tt.contains) {
			t.Errorf("(%d).String() = %q, want to contain %q", tt.fsid, name, tt.contains)
		}
	}
}

func TestFeatureSetIDString_UnknownID(t *testing.T) {
	name := (FeatureSetID(0xFF)).String()
	if name == "" {
		t.Error("FeatureSetIDToName for unknown ID should return non-empty string")
	}
	if !strings.Contains(name, "Unknown") {
		t.Errorf("(0xFF).String() = %q, want to contain 'Unknown'", name)
	}
}

func TestFeatureSetID_ValidValues(t *testing.T) {
	validIDs := []int{
		0x0, 0x4, 0x5, 0x6, 0x7, 0x8, 0x9, 0xA, 0xB,
		0x10, 0x13, 0x1C, 0x20, 0x33, 0x3C, 0x58, 0x68, 0x77,
	}
	for _, id := range validIDs {
		if !FeatureSetID(id).Valid() {
			t.Errorf("FeatureSetID(%d) is not valid", id)
		}
	}
}

func TestFeatureSetID_InvalidValue(t *testing.T) {
	if FeatureSetID(0xFF).Valid() {
		t.Error("FeatureSetID(0xFF) is valid")
	}
}

func TestFeatureSetID_RoundTrip(t *testing.T) {
	validIDs := []int{0x0, 0x10, 0x20, 0x58, 0x77}
	for _, id := range validIDs {
		fsid := FeatureSetID(id)
		name := fsid.String()
		if strings.Contains(name, "Unknown") {
			t.Errorf("Valid FeatureSetID %d maps to 'Unknown' name", id)
		}
	}
}
