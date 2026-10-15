package enums_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
)

func TestActivityID_String_NoActivity(t *testing.T) {
	t.Parallel()
	name := enums.ActivityNoActivity.String()
	if name != "No Activity" {
		t.Errorf("expected 'No Activity', got %q", name)
	}
}

func TestActivityID_String_GroupCSBK(t *testing.T) {
	t.Parallel()
	name := enums.ActivityGroupCSBK.String()
	if name != "Group CSBK" {
		t.Errorf("expected 'Group CSBK', got %q", name)
	}
}

func TestActivityID_String_IndividualCSBK(t *testing.T) {
	t.Parallel()
	name := enums.ActivityIndividualCSBK.String()
	if name != "Individual CSBK" {
		t.Errorf("expected 'Individual CSBK', got %q", name)
	}
}

func TestActivityID_String_GroupVoice(t *testing.T) {
	t.Parallel()
	name := enums.ActivityGroupVoice.String()
	if name != "Group Voice" {
		t.Errorf("expected 'Group Voice', got %q", name)
	}
}

func TestActivityID_String_IndividualVoice(t *testing.T) {
	t.Parallel()
	name := enums.ActivityIndividualVoice.String()
	if name != "Individual Voice" {
		t.Errorf("expected 'Individual Voice', got %q", name)
	}
}

func TestActivityID_String_GroupData(t *testing.T) {
	t.Parallel()
	name := enums.ActivityGroupData.String()
	if name != "Group Data" {
		t.Errorf("expected 'Group Data', got %q", name)
	}
}

func TestActivityID_String_IndividualData(t *testing.T) {
	t.Parallel()
	name := enums.ActivityIndividualData.String()
	if name != "Individual Data" {
		t.Errorf("expected 'Individual Data', got %q", name)
	}
}

func TestActivityID_String_EmergencyGroupVoice(t *testing.T) {
	t.Parallel()
	name := enums.ActivityEmergencyGroupVoice.String()
	if name != "Emergency Group Voice" {
		t.Errorf("expected 'Emergency Group Voice', got %q", name)
	}
}

func TestActivityID_String_EmergencyIndividualVoice(t *testing.T) {
	t.Parallel()
	name := enums.ActivityEmergencyIndividualVoice.String()
	if name != "Emergency Individual Voice" {
		t.Errorf("expected 'Emergency Individual Voice', got %q", name)
	}
}

func TestActivityID_String_Reserved(t *testing.T) {
	t.Parallel()
	name := (enums.ActivityID(0b0001)).String()
	if name == "No Activity" || name == "Group Voice" {
		t.Errorf("reserved value should not match known names, got %q", name)
	}
}

func TestActivityID_Cast_NoActivity(t *testing.T) {
	t.Parallel()
	a := enums.ActivityID(0)
	if a != enums.ActivityNoActivity {
		t.Errorf("expected ActivityNoActivity, got %d", a)
	}
}

func TestActivityID_Cast_GroupVoice(t *testing.T) {
	t.Parallel()
	a := enums.ActivityID(0b1000)
	if a != enums.ActivityGroupVoice {
		t.Errorf("expected ActivityGroupVoice, got %d", a)
	}
}

func TestActivityID_Cast_Reserved(t *testing.T) {
	t.Parallel()
	a := enums.ActivityID(0b0001)
	if int(a) != 1 {
		t.Errorf("expected 1, got %d", a)
	}
}

func TestActivityID_Values(t *testing.T) {
	t.Parallel()
	if int(enums.ActivityNoActivity) != 0 {
		t.Errorf("ActivityNoActivity should be 0, got %d", enums.ActivityNoActivity)
	}
	if int(enums.ActivityGroupVoice) != 8 {
		t.Errorf("ActivityGroupVoice should be 8, got %d", enums.ActivityGroupVoice)
	}
	if int(enums.ActivityIndividualVoice) != 9 {
		t.Errorf("ActivityIndividualVoice should be 9, got %d", enums.ActivityIndividualVoice)
	}
	if int(enums.ActivityIndividualData) != 0b1010 {
		t.Errorf("ActivityIndividualData should be 0b1010, got %d", enums.ActivityIndividualData)
	}
	if int(enums.ActivityGroupData) != 0b1011 {
		t.Errorf("ActivityGroupData should be 0b1011, got %d", enums.ActivityGroupData)
	}
	if int(enums.ActivityEmergencyGroupVoice) != 0b1100 {
		t.Errorf("ActivityEmergencyGroupVoice should be 0b1100, got %d", enums.ActivityEmergencyGroupVoice)
	}
	if int(enums.ActivityEmergencyIndividualVoice) != 0b1101 {
		t.Errorf("ActivityEmergencyIndividualVoice should be 0b1101, got %d", enums.ActivityEmergencyIndividualVoice)
	}
}
