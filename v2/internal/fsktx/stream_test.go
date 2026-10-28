//go:build !tinygo

package fsktx

import (
	"math"
	"math/bits"
	"slices"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/internal/rftx"
	"github.com/USA-RedDragon/dmrgo/v2/internal/unpacked"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
)

type testStream struct {
	syms  []int8
	lead  int
	slots []rftx.BSSlot
	kind  []string
}

const (
	kindVoice = "voice"
	kindData  = "data"
	kindIdle  = "idle"
)

const streamLead = 6

func newTestStream(t testing.TB, n, tail int) *testStream {
	t.Helper()
	return newStreamOf(t, rftx.SeqConfig{ColorCode: 7, Source: 3191868, Dest: 9, BS: 314926, Group: true, Wakeups: 1, Superframes: n, BSSourced: true, Slot: 1}, tail)
}

func newStreamOf(t testing.TB, cfg rftx.SeqConfig, tail int) *testStream {
	t.Helper()
	voice, err := rftx.Sequence("voice", cfg)
	if err != nil {
		t.Fatal(err)
	}
	slots := voice.Slots
	var data []layer2.RawBurst
	for len(data) < len(slots)/2+1 {
		c := cfg
		c.Slot, c.Group, c.Dest = 0, false, 3140598
		b, err := rftx.Sequence("data12", c)
		if err != nil {
			t.Fatal(err)
		}
		for i, s := range b.Slots {
			if x, err := layer2.NewBurstFromBytes(s.Burst); i%2 == 0 && err == nil && x.Kind == layer2.KindData && x.SlotType.DataType == enums.DataTypeRate12 {
				data = append(data, s.Burst)
			}
		}
	}
	ts := &testStream{lead: streamLead, slots: slots, kind: make([]string, len(slots))}
	ts.syms = make([]int8, streamLead, streamLead+len(slots)*144+tail)
	pos := enums.VoiceBurstUnknown
	for i := range slots {
		if i%2 == 0 {
			slots[i].Burst = data[i/2]
		}
		b, err := layer2.NewBurstFromBytes(slots[i].Burst)
		switch {
		case i%2 == 1 && err == nil && b.Kind == layer2.KindVoice && b.SyncPattern == enums.SyncBSSourcedVoice:
			pos = enums.VoiceBurstA
			ts.kind[i] = kindVoice
		case i%2 == 1 && pos != enums.VoiceBurstUnknown && pos < enums.VoiceBurstF && (err != nil || b.Kind != layer2.KindData):
			pos++
			ts.kind[i] = kindVoice
		case i%2 == 0:
			ts.kind[i] = kindData
		default:
			if i%2 == 1 {
				pos = enums.VoiceBurstUnknown
			}
			ts.kind[i] = kindIdle
		}
		ts.syms = appendBits(ts.syms, unpacked.Unpack(slots[i].CACH[:]))
		var bits [264]unpacked.Bit
		unpacked.UnpackAt(bits[:], slots[i].Burst[:], 0)
		ts.syms = appendBits(ts.syms, bits[:])
	}
	ts.syms = append(ts.syms, make([]int8, tail)...)
	return ts
}

func (s *testStream) withLead(n int) *testStream {
	c := *s
	c.lead += n
	c.syms = append(make([]int8, n, n+len(s.syms)), s.syms...)
	return &c
}

// ETSI TS 102 361-1 Table 10.3
func appendBits(dst []int8, bits []unpacked.Bit) []int8 {
	for i := 0; i+1 < len(bits); i += 2 {
		switch bits[i]<<1 | bits[i+1] {
		case 0b01:
			dst = append(dst, 3)
		case 0b00:
			dst = append(dst, 1)
		case 0b10:
			dst = append(dst, -1)
		default:
			dst = append(dst, -3)
		}
	}
	return dst
}

type score struct {
	voice, voiceOK, data, dataOK int
}

func (s *testStream) score(got []RxBurst, pad float64) score {
	byIndex := s.align(got, pad)
	var sc score
	for i, k := range s.kind {
		r, have := byIndex[i]
		data, _ := r.Decoded()
		want := s.slots[i].Burst
		switch k {
		case kindVoice:
			wf, _ := want.AMBEFrames()
			gf, _ := data.AMBEFrames()
			for j := range wf {
				sc.voice++
				if have && gf[j] == wf[j] {
					sc.voiceOK++
				}
			}
		case kindData:
			sc.data++
			a, _ := data.BPTCPayload()
			b, _ := want.BPTCPayload()
			if have && data.SlotType() == want.SlotType() && a == b {
				sc.dataOK++
			}
		}
	}
	return sc
}

func (s *testStream) align(got []RxBurst, pad float64) map[int]RxBurst {
	first := pad + float64(s.lead+12)/SymbolRate
	res := make([]float64, 0, len(got))
	for _, r := range got {
		d := r.Seconds - first
		res = append(res, d-math.Round(d/0.030)*0.030)
	}
	slices.Sort(res)
	if len(res) > 0 {
		first += res[len(res)/2]
	}
	byIndex := map[int]RxBurst{}
	for _, r := range got {
		i := int(math.Round((r.Seconds - first) / 0.030))
		if i >= 0 && i < len(s.slots) && math.Abs(r.Seconds-first-float64(i)*0.030) < 0.002 {
			byIndex[i] = r
		}
	}
	return byIndex
}

type rawScore struct {
	bursts, exact, errs, bits int
	lastBad, stray            int
}

func (s *testStream) rawScore(got []RxBurst, pad float64) rawScore {
	byIndex := s.align(got, pad)
	sc := rawScore{lastBad: -1, stray: len(got) - len(byIndex)}
	for i, slot := range s.slots {
		if slot.Burst == (layer2.RawBurst{}) {
			continue
		}
		sc.bursts++
		sc.bits += 8 * len(slot.Burst)
		r, have := byIndex[i]
		if !have {
			sc.errs += 4 * len(slot.Burst)
			sc.lastBad = i
			continue
		}
		n := 0
		for k := range slot.Burst {
			n += bits.OnesCount8(r.Data[k] ^ slot.Burst[k])
		}
		sc.errs += n
		if r.HasCACH {
			for k := range slot.CACH {
				n += bits.OnesCount8(r.CACH[k] ^ slot.CACH[k])
			}
		}
		if n == 0 {
			sc.exact++
		} else {
			sc.lastBad = i
		}
	}
	return sc
}
