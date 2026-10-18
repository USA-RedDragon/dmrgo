package rf

// options holds the stage settings of a Receiver. tierOptions returns the
// settings of each tier; a sweep of the settings measures what each one
// costs and gains.
type options struct {
	// Coherent selects the coherent trellis detector instead of the FM
	// discriminator.
	Coherent bool
	// Window is the number of symbols on each side of a symbol that the
	// trellis models, 1 or 2.
	Window int
	// TrellisSPS is the number of samples per symbol that the trellis
	// correlates, 5 or 10.
	TrellisSPS int
	// ExactPhase carries the carrier phase in the trellis state, so that
	// paths merge only with the same phase, instead of with each survivor.
	ExactPhase bool
	// PhaseStates is the number of carrier phases of the ExactPhase
	// trellis, 52 or 200; 0 means 200, the exact count for h = 0.27. Fewer
	// phases round the phase turn of each symbol and cost proportionally
	// less.
	PhaseStates int
	// PSPGain is the gain of the phase tracking of each trellis survivor.
	PSPGain float32
	// KnownPrior is the log prior, in nats, of the expected symbols of the
	// SYNC of an anchor.
	KnownPrior float32
	// SoftFEC rewrites the FEC fields of each burst with the codeword that
	// soft-decision decoding picks.
	SoftFEC bool
	// BPTCOSD decodes BPTC(196,96) by ordered statistics; otherwise by
	// Chase-II around the hard decoder.
	BPTCOSD bool
	// GridBPTC decodes a BPTC(196,96) block whose CRC or Reed-Solomon
	// check fails again by iterative row and column decoding of its grid.
	GridBPTC bool
	// DataPasses is the number of times a coherent tier detects a
	// BPTC(196,96) data burst that fails its check again, with the
	// extrinsic information of its grid decode as priors weighed by
	// DataPrior. DataUnchecked does so also for a burst without a check
	// whose decode corrected bits.
	DataPasses    int
	DataPrior     float32
	DataUnchecked bool
	// RefinePrior, when above 0, detects each voice burst again with the
	// bits that soft-decision decoding fixed as priors of that many nats.
	RefinePrior float32
	// LLRScale scales the log-likelihood ratios of the coherent detection
	// so that they match the error rates of the bits: the trellis takes
	// the noise as white, but the channel filter colors it, and that makes
	// its ratios too confident. The priors that a detection again adds
	// enter the trellis divided by it. 0 means 1.
	LLRScale float32
	// WarmUp detects the first slots of a stream twice, the first time
	// only to settle the carrier, timing and modulation index estimates.
	WarmUp bool
	// TrackCarrier follows the carrier between anchors with a
	// decision-directed estimate, after the warm pass.
	TrackCarrier bool
	// TrackH estimates the modulation index of each stream.
	TrackH bool
	// ChanDecimation is the factor, 1 or 2, by which the channel ring of
	// the coherent tiers runs below basebandRate; 0 means 1.
	ChanDecimation int
	// CompactChannel holds the channel ring of the coherent tiers as 8-bit
	// block floating point and keeps no copy of the wide input: a retune
	// rotates the samples it holds instead of filtering the input again.
	CompactChannel bool
	// DiscOutlier is the share of discriminator samples that a click
	// throws anywhere in the band. It puts a floor under the symbol
	// log-likelihoods, so that a click does not make a confident error.
	DiscOutlier float64
	// Sources is the number of SYNC sources the search tries: 2 for BS
	// and MS, 4 with both direct mode slots.
	Sources int
	// ClickSpan, when above 0, sets the floor of the discriminator symbol
	// likelihoods from the share of clicks that the bursts show, spread
	// over ClickSpan Hz, in place of DiscOutlier.
	ClickSpan float64
	// SoftExt adds softExt after the streaming core: the stages that
	// ExtAMBE, ExtOSD and ExtELC select, and the ExtLookback slots before
	// the SYNC of a lock.
	SoftExt bool
	// ExtAMBE decodes C0 and C1 of each voice frame by soft ML.
	ExtAMBE bool
	// ExtOSD decodes a BPTC(196,96) block whose check fails after the
	// decode of the core by ordered statistics, and again over its grid.
	ExtOSD bool
	// ExtELC decodes the embedded LC of each superframe over its four
	// fragments, summed with the earlier superframes of the call.
	ExtELC bool
	// ExtLookback is the number of slots before the SYNC of a lock that
	// softExt decodes.
	ExtLookback int
	// ExtNPML detects the bursts that softExt decodes from the symbols of
	// the core by npml, with the correlation of their noise.
	ExtNPML bool
	// IQDisc runs the streaming receiver on I/Q as iqRx, iqDisc before a
	// DiscReceiver at IQHz, 12000 or 24000, in place of leanRx. 0 means
	// 24000.
	IQDisc bool
	IQHz   int
	// CohExt adds to softExt on iqRx, or on a DiscReceiver, the coherent
	// detection of each burst again, with the trellis settings PSPGain and
	// LLRScale.
	CohExt bool
	// CohSPS is the number of samples per symbol that the coherent
	// detection correlates, 2 or 5; 0 means 5.
	CohSPS int
	// CohLazy runs the core of a DiscReceiver with CohExt on its input,
	// and builds the phase of each sample that the coherent detection
	// reads from the input as it reads it, instead of a ring of channel
	// filter samples for both.
	CohLazy bool
}

// discOptions returns the settings of the FM discriminator core alone:
// those of Tier0 on discriminator audio and with the complex front end.
func discOptions() options {
	o := options{TrellisSPS: 10, KnownPrior: 4, PSPGain: 0.02, WarmUp: true, TrackCarrier: true, DiscOutlier: 0.01, ClickSpan: 500, Sources: 2}
	o.SoftFEC, o.GridBPTC = true, true
	return o
}

// softOptions adds to o the soft stages of the tiers above Tier0: the
// ordered statistics decode of BPTC(196,96) and the embedded LC over each
// superframe.
func softOptions(o options) options {
	o.SoftExt, o.ExtOSD, o.ExtELC = true, true, true
	return o
}

// tier1Options adds to o the stages of Tier1: the soft stages, the decode
// of C0 and C1 of each voice frame by soft ML, and the detection of each
// burst that they decode with the correlation of the noise of its symbols.
func tier1Options(o options) options {
	o = softOptions(o)
	o.ExtAMBE, o.ExtNPML = true, true
	return o
}

// discTierOptions returns the settings of tier t on discriminator audio:
// Tier1 adds the stages of tier1Options, and Tier2 the soft stages, the
// decode of C0 and C1 by soft ML and the coherent stage on the phase that
// discPhase rebuilds.
func discTierOptions(t Tier) options {
	o := discOptions()
	switch t {
	case Tier1:
		o = tier1Options(o)
		o.ExtLookback = 8
	case Tier2:
		o = softOptions(o)
		o.ExtLookback, o.ExtAMBE, o.CohExt, o.PSPGain, o.LLRScale = 8, true, true, 0.2, 0.5
	case TierDefault, Tier0, Tier3, Tier4:
	}
	return o
}

// tierOptions returns the settings of tier t on I/Q at BasebandRate; New
// sets the lookback of Tier1 and Tier2 from the Config.
func tierOptions(t Tier) options {
	o := discOptions()
	switch t {
	case TierDefault, Tier0:
		o.IQDisc, o.IQHz = true, 12000
	case Tier1:
		o = tier1Options(o)
		o.IQDisc, o.IQHz = true, 12000
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

// tabled reports whether tables_gen.go holds the tables that the settings
// select, as it does for the settings that the comments of options give.
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
