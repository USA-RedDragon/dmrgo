//go:build !tinygo

package rf

// discDefaultTier is the tier of a DiscConfig with TierDefault for NewDisc.
const discDefaultTier = Tier1

// addSoft adds to r the soft stages of Tier1, with 8 slots of lookback.
func (r *DiscReceiver) addSoft() error {
	opt := softOptions(discOptions())
	opt.ExtLookback = 8
	r.ext = softExtOf(&opt, 2*r.tm.step)
	return nil
}
