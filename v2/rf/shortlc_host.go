//go:build !tinygo

package rf

import (
	"math"

	"github.com/USA-RedDragon/dmrgo/v2/internal/unpacked"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

const (
	slcNotes         = 512
	slcMatch         = 1e-3
	slcMaxFlips      = 8
	slcCostTenths    = 50
	slcCostTenthsIQ3 = 28
)

type slcNote struct {
	seconds float64
	llr     [layer2.CACHBits]int16
	cach    [3]byte
	set     bool
}

type shortLCOut struct {
	on     func([]ShortLC)
	asm    layer2.ShortLCAssembler
	stream int
	soft   *softDecoder
	notes  []slcNote
	head   int
	llr    [4][layer2.CACHBits]int16
	have   [4]bool
	bits   [4][layer2.CACHBits]unpacked.Bit
	out    [1]ShortLC
	cost   int64
}

type slcHook struct {
	slc *shortLCOut
}

func (s *burstSink) attachShortLC(on func([]ShortLC), t Tier, iq bool, _ error) error {
	if on == nil {
		return nil
	}
	o := &shortLCOut{on: on, cost: slcCostTenths}
	if iq && t == Tier3 && !coreTier3 {
		o.cost = slcCostTenthsIQ3
	}
	if t != Tier0 {
		o.soft = newSoftDecoder()
		o.soft.OSDOrder, o.soft.OSDWindow, o.soft.ShortLCList = 2, 0, 2
		o.notes = make([]slcNote, slcNotes)
	}
	s.slc = o
	return nil
}

func (s *burstSink) noted() *slcNote {
	if s == nil || s.slc == nil || s.slc.notes == nil {
		return nil
	}
	o := s.slc
	n := &o.notes[o.head]
	if o.head++; o.head == len(o.notes) {
		o.head = 0
	}
	n.set = true
	return n
}

func (s *burstSink) noteLean(r *leanCore, b *Burst) {
	n := s.noted()
	if n == nil {
		return
	}
	n.seconds, n.cach = b.Seconds, b.CACH
	for k := range cachSymbols {
		n.llr[2*k], n.llr[2*k+1] = r.symLLR(r.base + int64(k))
	}
}

func (s *burstSink) noteLLR(seconds float64, cach [3]byte, llr *[layer2.CACHBits]int16) {
	if n := s.noted(); n != nil {
		n.seconds, n.cach, n.llr = seconds, cach, *llr
	}
}

func (s *burstSink) seeShortLC(b *Burst) {
	if s.slc != nil {
		s.slc.put(b)
	}
}

func (o *shortLCOut) put(b *Burst) {
	if !b.HasCACH {
		return
	}
	if b.Stream != o.stream {
		o.asm.Reset()
		o.stream = b.Stream
	}
	_, cach := b.Decoded()
	lc, done, err := o.asm.Add(layer2.DecodeCACHField(cach))
	n := o.asm.Count() - 1
	if done {
		n = 3
	}
	if err != nil || n < 0 {
		return
	}
	if o.notes != nil {
		o.have[n] = o.find(b, &o.llr[n])
	}
	if !done {
		return
	}
	soft := false
	if lc.FEC.Uncorrectable {
		var ok bool
		if lc, ok = o.decodeSoft(); !ok {
			return
		}
		soft = true
	}
	o.out[0] = ShortLC{LC: lc, Stream: b.Stream, Slot: b.Slot, Seconds: b.Seconds, Soft: soft}
	o.on(o.out[:])
}

func (o *shortLCOut) find(b *Burst, dst *[layer2.CACHBits]int16) bool {
	for i := range o.notes {
		k := o.head - 1 - i
		if k < 0 {
			k += len(o.notes)
		}
		n := &o.notes[k]
		if !n.set {
			return false
		}
		if n.cach == b.CACH && math.Abs(n.seconds-b.Seconds) < slcMatch {
			*dst = n.llr
			return true
		}
	}
	return false
}

func (o *shortLCOut) decodeSoft() (pdu.ShortLC, bool) {
	if o.soft == nil || o.have != [4]bool{true, true, true, true} {
		return pdu.ShortLC{}, false
	}
	info, res := o.soft.decodeShortLC(&o.llr, &o.bits)
	if res.Uncorrectable || res.ErrorsCorrected > slcMaxFlips {
		return pdu.ShortLC{}, false
	}
	c := &softOSDCodes[softShortLCCode]
	var sum, cost int64
	for j := range c.n {
		p := c.pos[j]
		l := int64(o.llr[p/layer2.CACHBits][p%layer2.CACHBits])
		bit := o.bits[p/layer2.CACHBits][p%layer2.CACHBits] == 1
		if l < 0 {
			l = -l
			bit = !bit
		}
		sum += l
		if bit {
			cost += l
		}
	}
	if 10*cost*int64(c.n) > o.cost*sum {
		return pdu.ShortLC{}, false
	}
	v := unpacked.Uint(info[:]) << 4
	lc := pdu.DecodeShortLC([5]byte{byte(v >> 32 & 0xff), byte(v >> 24 & 0xff), byte(v >> 16 & 0xff), byte(v >> 8 & 0xff), byte(v & 0xff)})
	if lc.FEC.Uncorrectable {
		return pdu.ShortLC{}, false
	}
	lc.FEC = res.Combine(lc.FEC)
	return lc, true
}
