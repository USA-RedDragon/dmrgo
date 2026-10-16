package main

// Options holds the stage settings of a Receiver. TierOptions returns the
// settings of each tier; a sweep of the settings measures what each one
// costs and gains.
type Options struct {
	// Coherent selects the coherent trellis detector instead of the FM
	// discriminator.
	Coherent bool
	// Window is the number of symbols on each side of a symbol that the
	// trellis models, 1 or 2.
	Window int
	// TrellisSPS is the number of samples per symbol that the trellis
	// correlates, such as 5 or 10.
	TrellisSPS int
	// ExactPhase carries the carrier phase in the trellis state, so that
	// paths merge only with the same phase, instead of with each survivor.
	ExactPhase bool
	// PhaseStates is the number of carrier phases of the ExactPhase
	// trellis; 0 means 200, the exact count for h = 0.27. Fewer phases
	// round the phase turn of each symbol and cost proportionally less.
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
	// RefinePrior, when above 0, detects each voice burst again with the
	// bits that soft-decision decoding fixed as priors of that many nats.
	RefinePrior float32
	// WarmUp detects the first slots of a stream twice, the first time
	// only to settle the carrier, timing and modulation index estimates.
	WarmUp bool
	// TrackCarrier follows the carrier between anchors with a
	// decision-directed estimate.
	TrackCarrier bool
	// TrackH estimates the modulation index of each stream.
	TrackH bool
	// DiscCutHz is the cutoff of the discriminator prefilter.
	DiscCutHz float64
	// DiscDecimation is the factor, 1, 2 or 4, by which the discriminator
	// runs below basebandRate; 0 means 1.
	DiscDecimation int
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
}

// TierOptions returns the settings of tier t.
func TierOptions(t Tier) Options {
	o := Options{TrellisSPS: 10, KnownPrior: 4, PSPGain: 0.02, WarmUp: true, TrackCarrier: true, DiscCutHz: 6000, DiscOutlier: 0.01, Sources: 2}
	o.SoftFEC, o.GridBPTC = true, true
	switch t {
	case TierDisc:
		o.DiscDecimation = 2
	case TierCoherent:
		o.Coherent, o.Window, o.TrellisSPS, o.PSPGain, o.BPTCOSD, o.RefinePrior = true, 1, 5, 0.12, true, 20
		o.ChanDecimation, o.CompactChannel = 2, true
	case TierExact:
		o.Coherent, o.Window, o.ExactPhase, o.PhaseStates, o.BPTCOSD, o.RefinePrior = true, 1, true, 52, true, 20
	case TierMax:
		o.Coherent, o.Window, o.ExactPhase, o.BPTCOSD, o.RefinePrior = true, 2, true, true, 20
	}
	return o
}
