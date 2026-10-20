//go:build tinygo

package rf

const discDefaultTier = Tier0

func (r *DiscReceiver) addSoft(Tier) error {
	return errDiscTier
}

func (r *DiscReceiver) write(x []int16) { r.run(x) }

func (r *DiscReceiver) inputHz() int { return r.hz() }
