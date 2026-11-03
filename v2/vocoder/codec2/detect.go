package codec2

import "github.com/USA-RedDragon/dmrgo/v2/internal/c2mode"

const (
	detectDecide = c2mode.Decide
	detectLimit  = c2mode.Limit
	detectQuiet  = c2mode.Quiet
)

type detector struct{ d c2mode.Detector }

func (d *detector) reset() { d.d.Reset() }

func (d *detector) add(slot *[9]byte) Mode {
	if d.d.Add(slot) {
		return Mode3200
	}
	return Mode2400
}

func (d *detector) mode() Mode { return Mode(d.d.Mode()) }
