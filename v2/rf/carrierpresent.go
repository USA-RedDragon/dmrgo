package rf

const (
	leanCarrierSplit = 1.75
	leanCarrierGain  = 64
	leanCarrierMean  = 8
)

// leanCarrier is the carrier measure of noteCarrier.
type leanCarrier struct {
	dark  [2]uint8
	noted [2]int32
	pLo   float32
	pHi   float32
	mLo   float32
	mHi   float32
}

// noteCarrier measures whether slot j at base holds a carrier, counts in dark
// the slots of each parity since the last one that did, and keeps in noted
// the last slot of each parity it measured. The burst powers of the slots
// fall in two clusters, the noise floor of the dark slots and the carrier.
// pLo follows the least power, rising by 1/leanCarrierGain a slot, and pHi
// the most, falling as fast; while they lie less than leanCarrierSplit apart
// the slots span one cluster and each holds a carrier. Otherwise mLo and mHi
// follow the mean power of the dark and the lit slots, and a slot holds a
// carrier when its power lies above their geometric mean. The measure carries
// over from one stream to the next, as the channel keeps its noise floor,
// while softExt.locked starts dark and noted again. It runs with the soft
// stages only, and not in the dry pass of the replay. At 1 dB C/N in
// 12.5 kHz the burst power at the channel filter stands near 3 times the
// floor and varies by a few percent over a burst, so the measure holds where
// a SYNC no longer does.
func (r *leanCore) noteCarrier(j int32, base int64) {
	x, ok := r.ext.(*softExt)
	if !r.hasExt() || !ok || r.isReplayDry() {
		return
	}
	on, _, ok := r.slotPower(base, false)
	if !ok {
		return
	}
	m := &x.car
	if m.pHi == 0 {
		m.pLo, m.pHi = on, on
	}
	m.pLo = min32(m.pLo*(1+1.0/leanCarrierGain), on)
	m.pHi = max32(m.pHi*(1-1.0/leanCarrierGain), on)
	one := m.pHi < leanCarrierSplit*m.pLo
	switch {
	case one:
		m.mLo = 0
	case m.mLo == 0:
		m.mLo, m.mHi = m.pLo, m.pHi
	}
	lit := one || on*on > m.mLo*m.mHi
	switch {
	case one:
	case lit:
		m.mHi += (on - m.mHi) / leanCarrierMean
	default:
		m.mLo += (on - m.mLo) / leanCarrierMean
	}
	m.noted[j&1] = j
	d := &m.dark[j&1]
	switch {
	case lit:
		*d = 0
	case *d < 255:
		*d++
	}
}
