package constants_test

import (
	"testing"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/constants"
)

func TestTimers_AnnexA1(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		got      time.Duration
		expected time.Duration
	}{
		{"TAckWait", constants.TAckWait, 360 * time.Millisecond},
		{"TAckWaitSimulcast", constants.TAckWaitSimulcast, 2 * time.Second},
		{"TTO", constants.TTO, 180 * time.Second},
		{"TTOMax", constants.TTOMax, 180 * time.Second},
		{"CTRHOTMax", constants.CTRHOTMax, 3240 * time.Millisecond},
		{"CTRHOTIncrement", constants.CTRHOTIncrement, 60 * time.Millisecond},
		{"NoLeader", constants.NoLeader, 4*time.Minute + 30*time.Second},
		{"SyncAge", constants.SyncAge, 10 * time.Minute},
		{"SyncAgeIncrement", constants.SyncAgeIncrement, 500 * time.Millisecond},
		{"SyncAgeWarning", constants.SyncAgeWarning, 9 * time.Minute},
		{"TMSChanAuth", constants.TMSChanAuth, 180 * time.Millisecond},
		{"TMSChanAuthMultiSite", constants.TMSChanAuthMultiSite, 360 * time.Millisecond},
		{"TBSChanAuthRspVoice", constants.TBSChanAuthRspVoice, 720 * time.Millisecond},
		{"TBSChanAuthRspCSBKSingle", constants.TBSChanAuthRspCSBKSingle, 900 * time.Millisecond},
		{"TBSChanAuthRspCSBKMulti", constants.TBSChanAuthRspCSBKMulti, 1140 * time.Millisecond},
		{"TBSChanAuthSel", constants.TBSChanAuthSel, 240 * time.Millisecond},
		{"TRCtimer", constants.TRCtimer, 600 * time.Millisecond},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if tt.got != tt.expected {
				t.Errorf("%s = %v, want %v", tt.name, tt.got, tt.expected)
			}
		})
	}
}

func TestConstants_AnnexA2(t *testing.T) {
	t.Parallel()
	if constants.NCSBKRetry != 1 {
		t.Errorf("NCSBKRetry = %d, want 1", constants.NCSBKRetry)
	}
	if constants.BeaconDuration != 600*time.Millisecond {
		t.Errorf("BeaconDuration = %v, want 600ms", constants.BeaconDuration)
	}
	if constants.BeaconInterval != 4*time.Minute+30*time.Second {
		t.Errorf("BeaconInterval = %v, want 4m30s", constants.BeaconInterval)
	}
	if constants.CTDuration != 180*time.Millisecond {
		t.Errorf("CTDuration = %v, want 180ms", constants.CTDuration)
	}
}

func TestNoLeader_Equals_BeaconInterval(t *testing.T) {
	t.Parallel()
	// Per spec, NoLeader and BeaconInterval are both 4.5 minutes
	if constants.NoLeader != constants.BeaconInterval {
		t.Errorf("NoLeader (%v) should equal BeaconInterval (%v)", constants.NoLeader, constants.BeaconInterval)
	}
}

func TestSyncAgeWarning_Equals_TwoBeaconIntervals(t *testing.T) {
	t.Parallel()
	// Per spec, SyncAgeWarning = 2 × BeaconInterval = 9 min
	if constants.SyncAgeWarning != 2*constants.BeaconInterval {
		t.Errorf("SyncAgeWarning (%v) should equal 2*BeaconInterval (%v)", constants.SyncAgeWarning, 2*constants.BeaconInterval)
	}
}
