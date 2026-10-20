package codec2

import "math/bits"

const (
	detectDecide = 5
	detectLimit  = 8
	detectQuiet  = 5
)

type detector struct {
	score int8
	high  bool
}

func (d *detector) reset() { *d = detector{} }

func (d *detector) add(slot *[9]byte) Mode {
	head := bits.OnesCount32(uint32(slot[0])<<16|uint32(slot[1])<<8|uint32(slot[2])) +
		bits.OnesCount32(uint32(slot[3])<<16|uint32(slot[4])<<8|uint32(slot[5]))
	if head > detectQuiet {
		switch tail := bits.OnesCount16(uint16(slot[6])<<8 | uint16(slot[7])); {
		case tail <= 1:
			d.score = max(d.score-1, -detectLimit)
		case tail >= 3:
			d.score = min(d.score+1, detectLimit)
		}
	}
	if d.score != 0 {
		d.high = d.score > 0
	}
	if d.high {
		return Mode3200
	}
	return Mode2400
}

func (d *detector) mode() Mode {
	switch {
	case d.score >= detectDecide:
		return Mode3200
	case d.score <= -detectDecide:
		return Mode2400
	default:
		return ModeAuto
	}
}
