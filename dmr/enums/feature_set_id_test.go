package enums

import (
	"strings"
	"testing"
)

func TestFeatureSetIDToName_KnownIDs(t *testing.T) {
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
		name := FeatureSetIDToName(tt.fsid)
		if len(name) == 0 {
			t.Errorf("FeatureSetIDToName(%d) returned empty string", tt.fsid)
		}
		if !strings.Contains(name, tt.contains) {
			t.Errorf("FeatureSetIDToName(%d) = %q, want to contain %q", tt.fsid, name, tt.contains)
		}
	}
}

func TestFeatureSetIDToName_UnknownID(t *testing.T) {
	name := FeatureSetIDToName(FeatureSetID(0xFF))
	if name == "" {
		t.Error("FeatureSetIDToName for unknown ID should return non-empty string")
	}
	if !strings.Contains(name, "Unknown") {
		t.Errorf("FeatureSetIDToName(0xFF) = %q, want to contain 'Unknown'", name)
	}
}

func TestFeatureSetIDFromInt_ValidValues(t *testing.T) {
	validIDs := []int{
		0x0, 0x4, 0x5, 0x6, 0x7, 0x8, 0x9, 0xA, 0xB,
		0x10, 0x13, 0x1C, 0x20, 0x33, 0x3C, 0x58, 0x68, 0x77,
	}
	for _, id := range validIDs {
		fsid, err := FeatureSetIDFromInt(id)
		if err != nil {
			t.Errorf("FeatureSetIDFromInt(%d) unexpected error: %v", id, err)
		}
		if int(fsid) != id {
			t.Errorf("FeatureSetIDFromInt(%d) = %d, want %d", id, fsid, id)
		}
	}
}

func TestFeatureSetIDFromInt_InvalidValue(t *testing.T) {
	_, err := FeatureSetIDFromInt(0xFF)
	if err == nil {
		t.Error("FeatureSetIDFromInt(0xFF) should return error")
	}
}

func TestFeatureSetIDFromInt_RoundTrip(t *testing.T) {
	validIDs := []int{0x0, 0x10, 0x20, 0x58, 0x77}
	for _, id := range validIDs {
		fsid, err := FeatureSetIDFromInt(id)
		if err != nil {
			t.Fatalf("FeatureSetIDFromInt(%d) failed: %v", id, err)
		}
		name := FeatureSetIDToName(fsid)
		if strings.Contains(name, "Unknown") {
			t.Errorf("Valid FeatureSetID %d maps to 'Unknown' name", id)
		}
	}
}
