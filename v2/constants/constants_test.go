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

// ── ETSI TS 102 361-3 Annex A: PDP Timers and Constants ──

func TestPDPTimers_AnnexA1(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		got      time.Duration
		expected time.Duration
	}{
		{"TDataTxLmt", constants.TDataTxLmt, 60 * time.Second},
		{"TRspnsWait", constants.TRspnsWait, 180 * time.Millisecond},
		{"TRspnsWaitSimulcast", constants.TRspnsWaitSimulcast, 2 * time.Second},
		{"THoldoffMax", constants.THoldoffMax, 2 * time.Second},
		{"TDataHngtime", constants.TDataHngtime, 180 * time.Millisecond},
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

func TestPDPConstants_AnnexA2(t *testing.T) {
	t.Parallel()
	if constants.NRtryLmt != 8 {
		t.Errorf("NRtryLmt = %d, want 8", constants.NRtryLmt)
	}
}

// ── ETSI TS 102 361-4 Annex A: Trunking Timers and Constants ──

func TestTrunkingTimers_AnnexA1(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		got      time.Duration
		expected time.Duration
	}{
		{"TRandTCDefault", constants.TRandTCDefault, 10 * time.Second},
		{"TNosigDefault", constants.TNosigDefault, 5 * time.Second},
		{"TEmergTimerDefault", constants.TEmergTimerDefault, 120 * time.Second},
		{"TPacketTimerDefault", constants.TPacketTimerDefault, 10 * time.Second},
		{"TPTimerDefault", constants.TPTimerDefault, 10 * time.Second},
		{"TNPTimerDefault", constants.TNPTimerDefault, 5 * time.Second},
		{"TAwakeDefault", constants.TAwakeDefault, 1 * time.Second},
		{"TVHangtimeDefault", constants.TVHangtimeDefault, 4 * time.Second},
		{"TVItemDefault", constants.TVItemDefault, 30 * time.Second},
		{"TVInactiveDefault", constants.TVInactiveDefault, 5 * time.Second},
		{"TDInactiveDefault", constants.TDInactiveDefault, 5 * time.Second},
		{"TDItemDefault", constants.TDItemDefault, 30 * time.Second},
		{"TDHangtimeDefault", constants.TDHangtimeDefault, 4 * time.Second},
		{"TAnswerCallDefault", constants.TAnswerCallDefault, 15 * time.Second},
		{"TPendingDefault", constants.TPendingDefault, 5 * time.Second},
		{"TDeregDefault", constants.TDeregDefault, 500 * time.Millisecond},
		{"TBSInactiveDefault", constants.TBSInactiveDefault, 60 * time.Second},
		{"TDENREGDefault", constants.TDENREGDefault, 60 * time.Second},
		{"TALSDefault", constants.TALSDefault, 60 * time.Second},
		{"TALSEDefault", constants.TALSEDefault, 300 * time.Second},
		{"TALSRequestLifeSpanDefault", constants.TALSRequestLifeSpanDefault, 5 * time.Second},
		{"TALSRetransmitDelayDefault", constants.TALSRetransmitDelayDefault, 1 * time.Second},
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

func TestTrunkingConstants_AnnexA6(t *testing.T) {
	t.Parallel()
	if constants.NDefaultNW != 5 {
		t.Errorf("NDefaultNW = %d, want 5", constants.NDefaultNW)
	}
	if constants.NRandNR != 6 {
		t.Errorf("NRandNR = %d, want 6", constants.NRandNR)
	}
	if constants.NRandNE != 10 {
		t.Errorf("NRandNE = %d, want 10", constants.NRandNE)
	}
	if constants.NMaint != 4 {
		t.Errorf("NMaint = %d, want 4", constants.NMaint)
	}
	if constants.NmaxCh != 50 {
		t.Errorf("NmaxCh = %d, want 50", constants.NmaxCh)
	}
	if constants.ChPref != 50 {
		t.Errorf("ChPref = %d, want 50", constants.ChPref)
	}
	if constants.NSYSerrDefault != 2 {
		t.Errorf("NSYSerrDefault = %d, want 2", constants.NSYSerrDefault)
	}
	if constants.DMRLADefault != 1 {
		t.Errorf("DMRLADefault = %d, want 1", constants.DMRLADefault)
	}
	if constants.VoteBlkDefault != 3 {
		t.Errorf("VoteBlkDefault = %d, want 3", constants.VoteBlkDefault)
	}
}

func TestTrunkingGatewayAddresses_AnnexA8(t *testing.T) {
	t.Parallel()
	// Verify key gateway addresses are correctly defined
	if constants.GatewayPSTNI != 0xFFFEC0 {
		t.Errorf("GatewayPSTNI = 0x%06X, want 0xFFFEC0", constants.GatewayPSTNI)
	}
	if constants.GatewayREGI != 0xFFFEC6 {
		t.Errorf("GatewayREGI = 0x%06X, want 0xFFFEC6", constants.GatewayREGI)
	}
	if constants.GatewayKILLI != 0xFFFECF {
		t.Errorf("GatewayKILLI = 0x%06X, want 0xFFFECF", constants.GatewayKILLI)
	}
	if constants.GatewayDGNAI != 0xFFFED6 {
		t.Errorf("GatewayDGNAI = 0x%06X, want 0xFFFED6", constants.GatewayDGNAI)
	}
	if constants.AllMSID != 0xFFFFFF {
		t.Errorf("AllMSID = 0x%06X, want 0xFFFFFF", constants.AllMSID)
	}
	if constants.AllMSIDLocal != 0xFFFFFD {
		t.Errorf("AllMSIDLocal = 0x%06X, want 0xFFFFFD", constants.AllMSIDLocal)
	}
	if constants.AllMSIDZone != 0xFFFFFE {
		t.Errorf("AllMSIDZone = 0x%06X, want 0xFFFFFE", constants.AllMSIDZone)
	}
	if constants.AddressNull != 0x000000 {
		t.Errorf("AddressNull = 0x%06X, want 0x000000", constants.AddressNull)
	}
}
