package rf

// Tier selects how a Receiver detects symbols, by compute budget.
type Tier int

// The tiers, from the least compute to the most. docs/rx/README.md gives
// the sensitivity, compute and RAM of each. Every tier decodes each FEC field
// with soft decisions, and Tier0 to Tier2 each need under 20 ms of CPU per
// second of signal on a PC. Tier0 to Tier2 follow the MS-sourced bursts of
// both TDMA slots, as on the inbound of a repeater or a duplex hotspot with
// a radio on each slot: each slot keeps its own SYNC timing, symbol clock,
// levels and carrier offset.
const (
	// TierDefault is the zero value: Tier1 for NewReceiver and
	// NewAudioReceiver, and Tier0 for Init and under TinyGo.
	TierDefault Tier = iota
	// Tier0 slices the output of an FM discriminator and its matched
	// filter. On I/Q from 48 kHz an FM discriminator at 12 kHz comes before
	// it, and the receiver keeps about 19 KB. On I/Q at 9.6 or 12 kHz that
	// a radio chip filtered to the channel it keeps about 1.4 KB, and Init
	// sets it up in memory of the caller, for a microcontroller. On I/Q
	// from 48 kHz Tier0 to Tier3 find a carrier up to 6.25 kHz off, half
	// the channel spacing, from the spectrum before they lock; a carrier
	// further off is taken for the next channel.
	Tier0
	// Tier1 adds to Tier0 the soft decoders: a BPTC(196,96) block whose
	// check fails by ordered statistics, C0 and C1 of each voice frame by
	// soft ML, the embedded LC of each superframe over its four fragments
	// and the earlier superframes of the call, and LookbackSlots slots
	// before the first lock decoded again. On I/Q it keeps the input
	// samples of those slots, and when it tuned to the carrier before the
	// lock it filters them again at the new tune before it decodes them. It
	// detects the bursts that they decode, and each rate 3/4 block, from
	// the symbols of the discriminator with the correlation of their noise.
	// It keeps about 35 KB on audio and 108 KB on I/Q, 58 KB of that for
	// the input samples.
	Tier1
	// Tier2 adds to the soft decoders of Tier1 the detection again of the
	// bursts with a trellis over three symbols that carries the carrier
	// phase with each survivor, from the SYNC or EMB of the burst, within a
	// compute budget per slot: on I/Q from the samples at 24 kHz, on audio
	// from the carrier phase that it sums from the discriminator samples.
	// It holds each burst one slot longer and keeps about 550 KB on audio
	// and 565 KB on I/Q, 75 KB of that for the input samples.
	Tier2
	// Tier3 on I/Q detects over a window of three symbols with one of 52
	// carrier phases in the trellis state, in two passes, detects each
	// voice burst again with the bits that its FEC fixed, and takes each
	// voice frame from it or from a Tier2 receiver beside it. It needs
	// about 70 times the compute of Tier2 and about 36 MB, for offline
	// decoding on a PC. On audio it is Tier2 with four passes over each
	// data burst, at about twice the compute of Tier2.
	Tier3
)

type options struct {
	DiscOutlier           float64
	ClickSpan             float64
	CohBudget             float64
	CombBudget            float64
	HGate                 float64
	IQKaiser              float64
	IQCutHz               float64
	IQTaps                int
	CombMode              int
	TrellisSPS            int
	PhaseStates           int
	HGateMin              int
	PSPGain               float32
	KnownPrior            float32
	RefinePrior           float32
	LLRScale              float32
	Sources               int
	ExtLookback           int
	ExtAMBECandidates     int
	ExtOSDOrder           int
	IQHalfband            int
	IQTuneMaxHz           float32
	IQAFCAvg              int
	IQBuf                 int
	CohDataPasses         int
	DataPasses            int
	DataPrior             float32
	IQHz                  int
	TwoPassSlots          int
	TwoPassSmooth         int
	TwoPassLevels         int
	TwoPass               bool
	TwoPassKeepTiming     bool
	TwoPassKeepCarrier    bool
	Coherent              bool
	Comb                  bool
	ExactPhase            bool
	LogMAP                bool
	SoftFEC               bool
	BPTCOSD               bool
	GridBPTC              bool
	OSDBytes              bool
	WarmUp                bool
	TrackCarrier          bool
	TrackH                bool
	ExactH                bool
	CompactChannel        bool
	SoftExt               bool
	ExtAMBE               bool
	ExtOSD                bool
	ExtELC                bool
	ExtNPML               bool
	IQTuneSyncOnly        bool
	IQPullAgree           bool
	IQSpecHunt            bool
	LevelUp               bool
	CohExt                bool
	CohExact              bool
	CohNoNext, CohNoCross bool
	IQCutAuto             bool
	CohSlotFit            bool
	CohWarm               bool
	CohWide               bool
	IQSpecFine            bool
	CohSymNoise           uint8
	MidTiming             bool
	HuntLinMid            bool
	StrobeTab             bool
	Front96               bool
	BSSuspectRun          uint8
	LinearMid             bool
	RawSearch             uint8
	DataUnchecked         bool
	IQDisc                bool
	Window                uint8
	ChanDecimation        uint8
	IQLow                 uint8
	LeanChase             uint8
	IQHalf                bool
	CohRing               uint8
	UnionTwins            uint8
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
	o.IQDisc, o.IQHz, o.IQLow, o.IQBuf = true, iq0LowHz, 1, 32
	o.LeanChase = 1
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
		o.CohNoCross, o.CohNoNext, o.CohBudget, o.CohSymNoise, o.CohDataPasses = true, true, 1000, 8, 4
		o.UnionTwins = 1
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
