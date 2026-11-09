package rf

import (
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/internal/fec/golay"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
)

const (
	polDist   = 2
	polGap    = 2
	polPeriod = 12
	polGood   = 4
	polFlip   = 3
)

const (
	polOn = 1 << iota
	polInv
	polFlipped
	polWait
)

type discPol struct {
	stream      uint8
	voice, data [2]uint8
}

func (f *audioFront) observe(b *Burst) {
	if f.polBits&polOn == 0 {
		return
	}
	p := &f.pol
	s := uint8(b.Stream & 0xff)
	if b.Timed() || b.Distance > polDist || f.polBits&polWait != 0 && s == p.stream {
		return
	}
	f.polBits &^= polWait
	slot, k := uint8(b.Slot&0xff), b.Slot&1
	if s != p.stream {
		p.stream = s
		p.voice = [2]uint8{slot - polPeriod - 1, slot - polPeriod - 1}
		p.data = p.voice
	}
	score := int(f.polSum)
	switch b.Sync {
	case enums.SyncBSSourcedData, enums.SyncMSSourcedData, enums.SyncTDMA1Data, enums.SyncTDMA2Data:
		_, res := golay.Decode2087(slotTypeWord(&b.Data))
		switch {
		case b.Distance <= 1 && slot-p.data[k] == polPeriod:
			score++
		case !res.Uncorrectable && res.ErrorsCorrected <= 1:
			score -= polGood
		}
		p.data[k] = slot
	case enums.SyncBSSourcedVoice, enums.SyncMSSourcedVoice, enums.SyncTDMA1Voice, enums.SyncTDMA2Voice:
		if slot-p.voice[k] <= polGap {
			score++
		}
		p.voice[k] = slot
	case enums.SyncMSSourcedRC, enums.SyncReserved, enums.SyncNone:
	}
	score = max(score, -polFlip)
	if score >= polFlip {
		f.polBits = (f.polBits ^ polInv) | polFlipped | polWait
		score = 0
	}
	f.polSum = int8(score)
}

func (r *discRx) polarity() {
	if f := &r.f; f.polBits&polFlipped != 0 {
		f.polBits &^= polFlipped
		r.scaleAudio(-1)
		if r.locked() {
			r.unlock()
		}
	}
}

func slotTypeWord(d *layer2.RawBurst) uint32 {
	var cw uint32
	for j := range 20 {
		i := 98 + j
		if j >= 10 {
			i += 48
		}
		cw = cw<<1 | uint32(d[i/8]>>(7-i%8)&1)
	}
	return cw
}
