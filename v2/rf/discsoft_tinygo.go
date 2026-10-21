//go:build tinygo

package rf

const discDefaultTier = Tier0

func (r *DiscReceiver) addSoft(*options) error {
	return errDiscTier
}

func (r *DiscReceiver) write(x []int16) { r.run(x) }

func (r *DiscReceiver) inputHz() int { return r.hz() }

func (r *DiscReceiver) unionFlush() bool { return false }

type discUnion struct{}
