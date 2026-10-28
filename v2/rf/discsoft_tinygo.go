//go:build tinygo

package rf

const discDefaultTier = Tier0

func (r *discRx) addSoft(*options) error {
	return errAudioTier
}

func (r *discRx) write(x []int16) { r.run(x) }

func (r *discRx) inputHz() int { return r.hz() }

func (r *discRx) unionFlush() bool { return false }

func (r *discRx) unionBacklog() int { return 0 }

func (r *discRx) unionLag() float64 { return 0 }

type discUnion struct{}

func (r *leanCore) hasExt() bool { return false }

func (r *leanCore) isReplaying() bool { return false }

func (r *leanCore) isReplayDry() bool { return false }
