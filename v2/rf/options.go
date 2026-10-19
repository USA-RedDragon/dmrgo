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
	// LogMAP makes the ExactPhase trellis sum the paths into each state and
	// symbol exactly (log-sum-exp) instead of taking the best, and the bit
	// ratios of a symbol sum its two digits likewise.
	LogMAP bool
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
	// OSDBytes scores the ordered statistics patterns with byte tables in
	// place of nibble tables, 16 KiB more for each decoder. The decode is
	// the same.
	OSDBytes bool
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
	// ExtAMBECandidates, ExtOSDOrder and ExtGridIters set AMBECandidates,
	// OSDOrder and GridIters of the soft decoder of softExt; 0 keeps the
	// value of newSoftDecoder. ExtOSDOrder is the order plus 1, so that 1
	// selects order 0.
	ExtAMBECandidates int
	ExtOSDOrder       int
	ExtGridIters      int
	// IQDisc runs the streaming receiver on I/Q as iqRx, iqDisc before a
	// DiscReceiver at IQHz, 12000 or 24000, in place of leanRx. 0 means
	// 24000.
	IQDisc bool
	IQHz   int
	// IQTaps, IQKaiser and IQCutHz set the channel filter of iqDisc: its
	// length, the beta of its Kaiser window and its cutoff. 0 means
	// iqTaps, a Hamming window and discCutHz. On the coherent tiers they
	// set the channel filter of the channelizer instead, a windowed sinc
	// in place of chanFilterTaps once any of them is set: 0 means
	// chanTaps, a Hamming window and the middle of chanPassHz and
	// chanStopHz.
	IQTaps   int
	IQKaiser float64
	IQCutHz  float64
	// IQHalfband, when above 0, is the length of a half-band filter that
	// decimates the baseband by 2 before the channel filter of iqDisc, so
	// that the channel filter runs at half the rate with twice the span
	// for its taps. Its length plus 1 must be a multiple of 4.
	IQHalfband int
	// IQTuneMaxHz, when above 0, keeps the oscillator of iqRx within that
	// many Hz of 0, so that it does not follow a carrier of another
	// channel. On the coherent tiers the channel moves only to the
	// carrier offsets of the spectrum estimate within it.
	IQTuneMaxHz float32
	// IQTuneSyncOnly moves the oscillator of iqRx while the core is
	// locked only by the carrier offset of the SYNC fits, not by the mean
	// frequency of the channel filter samples, which the gaps between the
	// bursts, where another channel can leak in, also move.
	IQTuneSyncOnly bool
	// IQAFCAvg, when above 1, is the length of a moving average of the
	// channel filter samples that the carrier estimate of iqRx reads, so
	// that the edges of the channel, where another channel leaks in, move
	// it less.
	IQAFCAvg int
	// IQFront, above 0, runs iqDisc as the lean front of iq0Front: a
	// boxcar over each decimation in place of the channel filter (1),
	// centered on the carrier by its weights (2), or a polyphase lowpass
	// centered on the carrier (3), over IQPoly decimations, 2 or 3, to
	// IQCutHz (0 means discCutHz). IQHz may then also be 9600. Without
	// SoftExt, New builds it by newIQ0; it does not take CohExt.
	IQFront int
	IQPoly  int
	// IQBuf is the number of discriminator samples that iqRx hands the
	// core at once; 0 means 1024.
	IQBuf int
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
	// CohSkipSNR, when above 0, is the SNR of the levels of the coherent
	// detection, the power of the signal over that of the noise, from
	// which it leaves a burst to the ratios of the core.
	CohSkipSNR float64
	// CohLocked leaves the SYNC search to the core: the coherent stage
	// tests no SYNC that the discriminator does not show, which costs the
	// most on noise.
	CohLocked bool
	// CohBudget, when above 0, is the work that the coherent stage may
	// spend for each slot of a stream that is not clear, in trellis steps,
	// with each decode of a burst from its ratios counted by the steps that
	// cost as much; the bursts that the credit does not hold keep the
	// ratios of the core.
	CohBudget float64
	// CohNoNext leaves out the run of the coherent stage from the SYNC of a
	// BS burst into the second half of the slot before, CohNoCross its run
	// from the SYNC of the slot before into a burst, and CohCrossVoice
	// that run for all but the voice bursts B to F, which have no SYNC of
	// their own.
	CohNoNext, CohNoCross, CohCrossVoice bool
	// CohDataPasses is the most times that the coherent stage detects a
	// BPTC(196,96) data burst that fails its check again; 0 means
	// cohDataPasses, below 0 none.
	CohDataPasses int
	// CohVoiceOnly leaves every burst but a voice burst to the ratios of
	// the core: the coherent stage only fits the carrier and timing on
	// the others.
	CohVoiceOnly bool
	// TwoPass detects each stream of a coherent tier twice: a first pass
	// measures the anchors and the carrier of up to TwoPassSlots slots (0
	// means twoPassSlots), and a second pass detects them again with the
	// timing fit to all of its anchors, the carrier of a line fit to the
	// carrier estimates of the first pass over TwoPassSmooth slots on each
	// side (0 means all of them), and the levels of TwoPassLevels: 0 the
	// mean of the anchors over the same slots, 1 those of the first pass,
	// 2 those of the anchors on each side interpolated. TwoPassKeepTiming
	// and TwoPassKeepCarrier keep the timing at the end of the first pass
	// and the carrier of the first pass in each slot instead.
	TwoPass            bool
	TwoPassSlots       int
	TwoPassSmooth      int
	TwoPassLevels      int
	TwoPassKeepTiming  bool
	TwoPassKeepCarrier bool
	// CohSignMiss is the most SYNC symbols whose sign may miss for a
	// candidate of the search that the coherent stage verifies; 0 means
	// leanExtSignMiss.
	CohSignMiss uint8
	// CohRing is the log2 of the length of the ring of samples of the
	// coherent stage, which must hold the oldest burst that softExt
	// detects; 0 means cohHist.
	CohRing int
	// CohWin, when above 0, runs the trellis of the coherent stage in
	// blocks of CohWin symbols with cohLook symbols of lookahead, which
	// shrinks its buffers to CohWin+cohLook symbols, instead of over the
	// whole run.
	CohWin int
	// MidTiming takes the signs halfway between the symbols of a hunting
	// lock from the samples of the timing loop, interpolated linearly, in
	// place of the kept samples by Catmull-Rom, which costs more. It
	// applies at up to leanCubicSPS samples per symbol.
	MidTiming bool
	// Front96 runs the audio front end at 9.6 kHz in a loop of its own,
	// with the filter state in locals. The output is the same.
	Front96 bool
	// BSSuspectRun is the number of slots in a row that must end suspect
	// before a lock of a BS stream hunts for that reason. A BS keeps its
	// timing through a missed SYNC or EMB, and its key-up after a gap
	// shows as slots without a SYNC. 0 hunts at the first suspect slot.
	BSSuspectRun uint8
	// LinearMid takes the midpoints between the symbols, which the
	// Gardner loop of the timing uses before a lock, by linear in place
	// of Catmull-Rom interpolation, with Front96.
	LinearMid bool
	// RawSearch takes the samples at 2 per symbol as the symbols and the
	// midpoints before a lock, with Front96, in place of the strobes of
	// the Gardner loop. A lock samples its SYNC again from the kept
	// samples, and the loop starts there. 2 also handles the symbols of a
	// lock in a call that leaves the locals of run96 alone, unless a SYNC
	// of the hunt matches or the slot ends.
	RawSearch uint8
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
// of C0 and C1 of each voice frame by soft ML, the detection of each burst
// that they decode with the correlation of the noise of its symbols, and
// the coherent detection again of each burst that is not clear.
func tier1Options(o options) options {
	o = softOptions(o)
	o.ExtAMBE, o.ExtNPML = true, true
	o.CohExt, o.PSPGain, o.LLRScale = true, 0.2, 0.5
	return o
}

// discTierOptions returns the settings of tier t on discriminator audio:
// Tier1 adds the stages of tier1Options, with the coherent stage at 2
// samples per symbol on the phase that discPhase builds from the input as
// it reads it, on the bursts of a lock only; Tier2 the soft stages, the
// decode of C0 and C1 by soft ML and the coherent stage on the ring of
// phasors that discPhase rebuilds, which its core runs on.
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

// tierOptions returns the settings of tier t on I/Q at BasebandRate; New
// sets the lookback of Tier1 and Tier2 from the Config.
func tierOptions(t Tier) options {
	o := discOptions()
	switch t {
	case TierDefault, Tier0:
		o.IQDisc, o.IQHz = true, 12000
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

// signMiss returns the sign misses of the SYNC candidates of the search
// that the coherent stage verifies.
func (o *options) signMiss() uint8 {
	if o.CohSignMiss > 0 {
		return o.CohSignMiss
	}
	return leanExtSignMiss
}

// cohRing returns the length of the ring of the coherent stage.
func (o *options) cohRing() int64 {
	if o.CohRing > 0 {
		return 1 << o.CohRing
	}
	return cohHist
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
