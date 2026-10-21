package rf

type options struct {
	DiscOutlier                          float64
	ClickSpan                            float64
	CohSkipSNR                           float64
	CohBudget                            float64
	CombBudget                           float64
	CohVerifyScan                        float64
	HGate                                float64
	IQKaiser                             float64
	IQCutHz                              float64
	IQTaps                               int
	CombMode                             int
	TrellisSPS                           int
	PhaseStates                          int
	HGateMin                             int
	DriftPenalty                         float32
	PhaseJump                            float32
	PhaseJumpBelow                       float32
	PSPGain                              float32
	KnownPrior                           float32
	RefinePrior                          float32
	LLRScale                             float32
	Sources                              int
	ExtLookback                          int
	ExtAMBECandidates                    int
	ExtOSDOrder                          int
	ExtGridIters                         int
	IQHalfband                           int
	IQTuneMaxHz                          float32
	IQAFCAvg                             int
	IQFront                              int
	IQPoly                               int
	IQBuf                                int
	CohSPS                               int
	CohDataPasses                        int
	CohRing                              int
	CohWin                               int
	DataPasses                           int
	DataPrior                            float32
	IQHz                                 int
	TwoPassSlots                         int
	TwoPassSmooth                        int
	TwoPassLevels                        int
	TwoPass                              bool
	TwoPassKeepTiming                    bool
	TwoPassKeepCarrier                   bool
	Coherent                             bool
	Comb                                 bool
	ExactPhase                           bool
	LogMAP                               bool
	SoftFEC                              bool
	BPTCOSD                              bool
	GridBPTC                             bool
	OSDBytes                             bool
	WarmUp                               bool
	TrackCarrier                         bool
	TrackH                               bool
	ExactH                               bool
	PhaseJumpFade                        bool
	CompactChannel                       bool
	SoftExt                              bool
	ExtAMBE                              bool
	ExtOSD                               bool
	ExtELC                               bool
	ExtNPML                              bool
	IQTuneSyncOnly                       bool
	IQPullAgree                          bool
	IQSpecHunt                           bool
	LevelUp                              bool
	CohExt                               bool
	CohLazy                              bool
	CohLocked                            bool
	CohNoNext, CohNoCross, CohCrossVoice bool
	IQCutAuto                            bool
	CohVoiceOnly                         bool
	CohSignMiss                          uint8
	CohSymNoise                          uint8
	MidTiming                            bool
	HuntLinMid                           bool
	StrobeTab                            bool
	Front96                              bool
	BSSuspectRun                         uint8
	LinearMid                            bool
	RawSearch                            uint8
	DataUnchecked                        bool
	IQDisc                               bool
	Window                               uint8
	ChanDecimation                       uint8
	DriftSteps                           uint8
	IQLow                                uint8
	LeanChase                            uint8
	NoMidSearch                          bool
}

func discOptions() options {
	o := options{TrellisSPS: 10, KnownPrior: 4, PSPGain: 0.02, WarmUp: true, TrackCarrier: true, DiscOutlier: 0.01, ClickSpan: 500, Sources: 2}
	o.SoftFEC, o.GridBPTC = true, true
	return o
}

func softOptions(o options) options {
	o.SoftExt, o.ExtOSD, o.ExtELC = true, true, true
	return o
}

func tier1Options(o options) options {
	o = softOptions(o)
	o.ExtAMBE, o.ExtNPML = true, true
	o.ExtLookback, o.ExtOSDOrder, o.ExtAMBECandidates = 4, 2, 1
	return o
}

func tier2Options(o options) options {
	o = softOptions(o)
	o.ExtLookback, o.ExtAMBE, o.CohExt, o.PSPGain, o.LLRScale = 8, true, true, 0.2, 0.5
	return o
}

func disc0Options() options {
	o := discOptions()
	o.MidTiming, o.Front96, o.BSSuspectRun, o.RawSearch = true, true, 3, 2
	return o
}

func iqLowOptions() options {
	o := disc0Options()
	o.HuntLinMid, o.StrobeTab = true, true
	o.IQDisc, o.IQHz, o.IQFront, o.IQLow, o.IQBuf = true, iq0LowHz, 4, 1, 32
	o.LeanChase, o.NoMidSearch = 1, true
	return o
}

func discTierOptions(t Tier) options {
	switch t {
	case Tier1:
		return tier1Options(disc0Options())
	case Tier2:
		o := tier2Options(discOptions())
		o.CohNoCross, o.CohNoNext, o.CohBudget, o.CohSymNoise = true, true, 400, 8
		return o
	case Tier3:
		o := tier2Options(discOptions())
		o.CohDataPasses = 4
		return o
	case TierDefault, Tier0:
	}
	return disc0Options()
}

func tierOptions(t Tier) options {
	o := discOptions()
	switch t {
	case TierDefault, Tier0:
		o.IQDisc, o.IQHz, o.IQPullAgree, o.LevelUp = true, 12000, true, true
		o.IQTuneMaxHz, o.IQTuneSyncOnly, o.IQHalfband, o.IQKaiser, o.IQAFCAvg = iqHuntMaxHz, true, 15, 6, 3
		o.IQSpecHunt = true
	case Tier1:
		o = tier1Options(o)
		o.IQDisc, o.IQHz, o.IQPullAgree, o.IQSpecHunt = true, 12000, true, true
		o.IQTuneMaxHz, o.IQTuneSyncOnly, o.IQAFCAvg = iqHuntMaxHz, true, 2
	case Tier2:
		o = tier2Options(o)
		o.IQDisc, o.IQHz, o.CohNoCross, o.CohNoNext, o.CohBudget = true, 24000, true, true, 300
		o.IQTuneMaxHz, o.IQTuneSyncOnly, o.IQAFCAvg, o.IQSpecHunt = iqHuntMaxHz, true, 5, true
	case Tier3:
		o.DataPasses, o.DataPrior, o.DataUnchecked = 2, 0.7, true
		o.Coherent, o.Window, o.ExactPhase, o.PhaseStates, o.BPTCOSD, o.RefinePrior, o.LLRScale = true, 1, true, 52, true, 7, 0.35
		o.TwoPass, o.TwoPassLevels, o.LogMAP, o.Comb, o.CombMode = true, 2, true, true, 12
		o.HGate, o.ExactH, o.HGateMin = 0.01, true, 4
		o.IQTuneMaxHz, o.IQCutHz, o.IQKaiser, o.IQCutAuto = iqTuneMaxHz, 6000, 6, true
	}
	return o
}

const (
	iqHuntMaxHz = 2500
	iqTuneMaxHz = 6250
)

func (o *options) signMiss() uint8 {
	if o.CohSignMiss > 0 {
		return o.CohSignMiss
	}
	return leanExtSignMiss
}

func (o *options) cohRing() int64 {
	if o.CohRing > 0 {
		return 1 << o.CohRing
	}
	return cohHist
}

func (o *options) tabled() bool {
	switch {
	case !o.Coherent:
		return true
	case o.ChanDecimation != 0 && o.ChanDecimation != 1 && o.ChanDecimation != 2:
		return false
	case o.TrellisSPS != 5 && o.TrellisSPS != 10:
		return false
	}
	return !o.ExactPhase || o.PhaseStates == 0 || o.PhaseStates == 52 || o.PhaseStates == phaseStates
}
