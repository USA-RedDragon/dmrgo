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
	IQLow                                int
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
	o.CohExt, o.PSPGain, o.LLRScale = true, 0.2, 0.5
	return o
}

func discTierOptions(t Tier) options {
	o := discOptions()
	switch t {
	case Tier1:
		o = tier1Options(o)
		o.ExtLookback, o.CohLazy, o.CohSPS, o.CohSkipSNR, o.CohLocked = 8, true, 2, 20, true
	case Tier2:
		o = softOptions(o)
		o.ExtLookback, o.ExtAMBE, o.CohExt, o.PSPGain, o.LLRScale = 8, true, true, 0.2, 0.5
	case TierDefault, Tier0, Tier3, Tier4:
	}
	return o
}

func tierOptions(t Tier) options {
	o := discOptions()
	switch t {
	case TierDefault, Tier0:
		o.IQDisc, o.IQHz, o.IQPullAgree = true, 12000, true
	case Tier1:
		o = tier1Options(o)
		o.IQDisc, o.IQHz, o.CohSkipSNR, o.CohSignMiss = true, 12000, 9, leanSignMiss
	case Tier2:
		o = softOptions(o)
		o.IQDisc, o.IQHz, o.ExtAMBE, o.CohExt, o.PSPGain, o.LLRScale = true, 24000, true, true, 0.2, 0.5
	case Tier3:
		o.DataPasses, o.DataPrior, o.DataUnchecked = 2, 0.7, true
		o.Coherent, o.Window, o.ExactPhase, o.PhaseStates, o.BPTCOSD, o.RefinePrior, o.LLRScale = true, 1, true, 52, true, 7, 0.35
	case Tier4:
		o.DataPasses, o.DataPrior, o.DataUnchecked = 2, 0.7, true
		o.Coherent, o.Window, o.ExactPhase, o.BPTCOSD, o.RefinePrior, o.LLRScale = true, 2, true, true, 7, 0.35
	}
	return o
}

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
