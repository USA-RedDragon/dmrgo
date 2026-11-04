//go:build tinygo

package rf

const discDefaultTier = Tier0

func (r *discRx) addSoft(opt *options) error {
	r.addExt(opt)
	return nil
}

func (r *discRx) writeIQ(x []int16) {
	if r.hasExt() {
		r.write(x)
		return
	}
	r.runN(x)
}

var extRx bool //nolint:gochecknoglobals // set where a receiver attaches its soft stages, so builds without one drop them

func setExtOn() { extRx = true }

func (r *leanCore) hasExt() bool { return extRx && r.ext != nil }

func (r *leanCore) isReplaying() bool { return r.hasExt() && r.replaying }

func (r *leanCore) isReplayDry() bool { return r.hasExt() && r.replayDry }

func (r *leanCore) earlyOK(b *Burst) bool {
	if r.hasExt() {
		return checkedData(b)
	}
	return r.fecOK && b.HasFixed
}

func (r *leanCore) vouched(b *Burst) bool { return b.Voice != 0 || r.earlyOK(b) }
