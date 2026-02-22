package enums_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
)

func TestActivityID_ToName_NoActivity(t *testing.T) {
	t.Parallel()
	name := enums.ActivityIDToName(enums.ActivityNoActivity)
	if name != "No Activity" {
		t.Errorf("expected 'No Activity', got %q", name)
	}
}

func TestActivityID_ToName_GroupCSBK(t *testing.T) {
	t.Parallel()
	name := enums.ActivityIDToName(enums.ActivityGroupCSBK)
	if name != "Group CSBK" {
		t.Errorf("expected 'Group CSBK', got %q", name)
	}
}

func TestActivityID_ToName_IndividualCSBK(t *testing.T) {
	t.Parallel()
	name := enums.ActivityIDToName(enums.ActivityIndividualCSBK)
	if name != "Individual CSBK" {
		t.Errorf("expected 'Individual CSBK', got %q", name)
	}
}

func TestActivityID_ToName_GroupVoice(t *testing.T) {
	t.Parallel()
	name := enums.ActivityIDToName(enums.ActivityGroupVoice)
	if name != "Group Voice" {
		t.Errorf("expected 'Group Voice', got %q", name)
	}
}

func TestActivityID_ToName_IndividualVoice(t *testing.T) {
	t.Parallel()
	name := enums.ActivityIDToName(enums.ActivityIndividualVoice)
	if name != "Individual Voice" {
		t.Errorf("expected 'Individual Voice', got %q", name)
	}
}

func TestActivityID_ToName_GroupData(t *testing.T) {
	t.Parallel()
	name := enums.ActivityIDToName(enums.ActivityGroupData)
	if name != "Group Data" {
		t.Errorf("expected 'Group Data', got %q", name)
	}
}

func TestActivityID_ToName_IndividualData(t *testing.T) {
	t.Parallel()
	name := enums.ActivityIDToName(enums.ActivityIndividualData)
	if name != "Individual Data" {
		t.Errorf("expected 'Individual Data', got %q", name)
	}
}

func TestActivityID_ToName_AllEmergency(t *testing.T) {
	t.Parallel()
	name := enums.ActivityIDToName(enums.ActivityAllEmergency)
	if name != "All/Emergency" {
		t.Errorf("expected 'All/Emergency', got %q", name)
	}
}

func TestActivityID_ToName_Reserved(t *testing.T) {
	t.Parallel()
	name := enums.ActivityIDToName(enums.ActivityID(0b0001))
	if name == "No Activity" || name == "Group Voice" {
		t.Errorf("reserved value should not match known names, got %q", name)
	}
}

func TestActivityID_FromInt_NoActivity(t *testing.T) {
	t.Parallel()
	a := enums.ActivityIDFromInt(0)
	if a != enums.ActivityNoActivity {
		t.Errorf("expected ActivityNoActivity, got %d", a)
	}
}

func TestActivityID_FromInt_GroupVoice(t *testing.T) {
	t.Parallel()
	a := enums.ActivityIDFromInt(0b1000)
	if a != enums.ActivityGroupVoice {
		t.Errorf("expected ActivityGroupVoice, got %d", a)
	}
}

func TestActivityID_FromInt_Reserved(t *testing.T) {
	t.Parallel()
	a := enums.ActivityIDFromInt(0b0001)
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
	if int(enums.ActivityAllEmergency) != 12 {
		t.Errorf("ActivityAllEmergency should be 12, got %d", enums.ActivityAllEmergency)
	}
}