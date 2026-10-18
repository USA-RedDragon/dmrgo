//go:build tinygo

package rf

// discDefaultTier is the tier of a DiscConfig with TierDefault for NewDisc:
// Tier0, so that a microcontroller links no soft stage.
const discDefaultTier = Tier0

// addSoft reports that a TinyGo build has no soft stages.
func (r *DiscReceiver) addSoft() error {
	return errDiscTier
}
