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
	// correlates, 5 or 10.
	TrellisSPS int
	// ExactPhase carries the carrier phase in the trellis state, so that
	// paths merge only with the same phase, instead of with each survivor.
	ExactPhase bool
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
	// Sources is the number of SYNC sources the search tries: 2 for BS
	// and MS, 4 with both direct mode slots.
	Sources int
}

// TierOptions returns the settings of tier t.
func TierOptions(t Tier) Options {
	o := Options{TrellisSPS: 10, KnownPrior: 4, PSPGain: 0.02, WarmUp: true, TrackCarrier: true, DiscCutHz: 6000, Sources: 2}
	o.SoftFEC = true
	switch t {
	case TierDisc:
	case TierCoherent:
		o.Coherent, o.Window, o.TrellisSPS, o.PSPGain, o.BPTCOSD, o.RefinePrior = true, 1, 5, 0.12, true, 20
	case TierExact:
		o.Coherent, o.Window, o.ExactPhase, o.BPTCOSD, o.RefinePrior = true, 1, true, true, 20
	case TierMax:
		o.Coherent, o.Window, o.ExactPhase, o.BPTCOSD, o.RefinePrior = true, 2, true, true, 20
	}
	return o
}
