//go:build tinygo

package rf

// discDefaultTier is the tier of a DiscConfig with TierDefault for NewDisc:
// Tier0, so that a microcontroller links no soft stage.
const discDefaultTier = Tier0

// addSoft reports that a TinyGo build has no soft stages.
func (r *DiscReceiver) addSoft(Tier) error {
	return errDiscTier
}

// write runs the samples x through the core.
func (r *DiscReceiver) write(x []int16) { r.run(x) }

// inputHz returns the rate of the input of r.
func (r *DiscReceiver) inputHz() int { return r.hz() }
