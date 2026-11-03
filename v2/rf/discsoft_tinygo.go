//go:build tinygo

package rf

const discDefaultTier = Tier0

func (r *discRx) addSoft(*options) error {
	return errAudioTier
}

func (r *discRx) attachFix(bool) {}

func (r *discRx) fixLag() float64 { return 0 }

func (r *discRx) write(x []int16) { r.run(x) }

func (r *discRx) inputHz() int { return r.hz() }

func (r *leanCore) hasExt() bool { return false }

func (r *leanCore) isReplaying() bool { return false }

func (r *leanCore) isReplayDry() bool { return false }

type acStage struct{}

func (r *leanCore) cohVerify() bool { return false }
