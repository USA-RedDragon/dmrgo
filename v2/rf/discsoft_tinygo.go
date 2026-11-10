//go:build tinygo

package rf

import "github.com/USA-RedDragon/dmrgo/v2/internal/fsktx"

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

var txHostRx bool //nolint:gochecknoglobals // set where a transmitter builds its host modulator, so builds without one drop it

func setTxHostOn() {
	txHostRx = true
	fsktx.EnableHost()
}

func txHostOn() bool { return txHostRx }

var powerRx bool //nolint:gochecknoglobals // set where an I/Q receiver attaches its slot power measure, so builds without one drop it

func setPowerOn() { powerRx = true }

func powerOn() bool { return powerRx }

func (r *leanCore) hasExt() bool { return extRx && r.ext != nil }

func (r *leanCore) isReplaying() bool { return r.hasExt() && r.replay != replayNone }

func (r *leanCore) isReplayDry() bool { return r.hasExt() && r.replay == replayDry }

func (r *leanCore) earlyOK(b *Burst) bool {
	if r.hasExt() {
		return checkedData(b)
	}
	return r.fecOK && b.HasFixed
}

func (r *leanCore) vouched(b *Burst) bool { return b.Voice != 0 || r.earlyOK(b) }
