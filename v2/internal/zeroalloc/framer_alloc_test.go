//go:build !tinygo

package zeroalloc_test

import (
	"math/rand"
	"os"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/framer"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

// voiceBursts returns the bursts of layer2/testdata/voice.bin.
func voiceBursts(t testing.TB) []layer2.RawBurst {
	t.Helper()
	data, err := os.ReadFile("../../layer2/testdata/voice.bin")
	if err != nil {
		t.Fatal(err)
	}
	out := make([]layer2.RawBurst, len(data)/len(layer2.RawBurst{}))
	for i := range out {
		copy(out[i][:], data[i*len(layer2.RawBurst{}):])
	}
	return out
}

// framerStream is a cyclic bit stream and the chunks a test writes it in.
type framerStream struct {
	name   string
	cfg    framer.Config
	stream []bit.Bit
	chunks [][]bit.Bit
}

func noiseBits(r *rand.Rand, n int) []bit.Bit {
	out := make([]bit.Bit, n)
	for i := range out {
		out[i] = bit.Bit(r.Intn(2))
	}
	return out
}

// chunked cuts stream into chunks of size(i) bits, the last one shorter.
func chunked(stream []bit.Bit, size func(i int) int) [][]bit.Bit {
	var out [][]bit.Bit
	for i := 0; len(stream) > 0; i++ {
		n := min(size(i), len(stream))
		out = append(out, stream[:n])
		stream = stream[n:]
	}
	return out
}

// framerStreams returns the voice.bin bursts one per SlotPeriod, in chunks
// of up to 3 periods, the same
// bursts on an outbound stream with a CACH before each burst and an idle
// burst on channel 2, and noise. Each stream is a whole number of periods,
// so it can repeat.
func framerStreams(t testing.TB) []framerStream {
	t.Helper()
	r := rand.New(rand.NewSource(1))
	voice := voiceBursts(t)
	slot := make([]bit.Bit, 0, len(voice)*framer.SlotPeriod)
	for _, b := range voice {
		slot = append(slot, bit.Unpack(b[:])...)
		slot = append(slot, noiseBits(r, framer.SlotPeriod-layer2.BurstBits)...)
	}
	idle := must(layer2.BuildDataBurst([12]byte{}, enums.DataTypeIdle, 1, layer2.SyncSourceBS))
	outbound := make([]bit.Bit, 0, len(voice)*framer.SlotPeriod)
	for i, v := range voice {
		for ch := 1; ch <= 2; ch++ {
			data := idle
			if ch == 1 {
				data = v
			}
			c := layer2.CACH{TACT: pdu.TACT{AccessType: true, TDMAChannel: ch == 2}}
			c.Payload[i%len(c.Payload)] = 1
			tx := layer2.CACHInterleave(must(c.Encode()))
			outbound = append(outbound, tx[:]...)
			outbound = append(outbound, bit.Unpack(data[:])...)
		}
	}
	slotCfg := framer.Config{Threshold: -1, Period: framer.SlotPeriod}
	cachCfg := framer.Config{Threshold: -1, CACH: true}
	noise := noiseBits(r, 40*framer.OutboundPeriod)
	return []framerStream{
		{"SlotPeriod chunks", slotCfg, slot, chunked(slot, func(int) int { return framer.SlotPeriod })},
		{"random chunks", slotCfg, slot, chunked(slot, func(int) int { return 1 + r.Intn(framer.SlotPeriod) })},
		{"one bit chunks", slotCfg, slot, chunked(slot, func(int) int { return 1 })},
		{"3 period chunks", slotCfg, slot, chunked(slot, func(int) int { return 3 * framer.SlotPeriod })},
		{"outbound CACH", cachCfg, outbound, chunked(outbound, func(int) int { return framer.OutboundPeriod })},
		{"outbound random chunks", cachCfg, outbound, chunked(outbound, func(int) int { return 1 + r.Intn(framer.OutboundPeriod) })},
		{"noise unlocked", framer.Config{Threshold: 0}, noise, chunked(noise, func(int) int { return 1 + r.Intn(framer.OutboundPeriod) })},
	}
}

// framerEntries returns the entries of package layer2/framer.
func framerEntries(t testing.TB) []entry {
	t.Helper()
	streams := framerStreams(t)
	slot := streams[0]
	m, ok := framer.Search(slot.stream, 0, -1)
	if !ok || m.Offset != framer.SyncOffset {
		t.Fatalf("Search: %+v %v", m, ok)
	}
	var found bool
	var locked bool
	out := make([]entry, 0, len(streams)+5)
	out = append(out, entry{"layer2/framer.Search", "voice.bin", 0, func() { m, found = framer.Search(slot.stream, 1, -1) }})
	for _, s := range streams {
		f := framer.New(s.cfg)
		dst := make([]framer.Burst, 0, 4*framer.DefaultMaxMisses)
		var i int
		write := func() {
			dst = f.AppendBursts(dst[:0], s.chunks[i%len(s.chunks)]...)
			i++
		}
		for range 2 * len(s.chunks) {
			write()
		}
		if s.cfg.Threshold == 0 {
			if f.Locked() {
				t.Fatalf("%s: locked on noise", s.name)
			}
		} else if !f.Locked() {
			t.Fatalf("%s: not locked", s.name)
		}
		out = append(out, entry{"layer2/framer.Framer.AppendBursts", s.name, 0, write})
	}
	fresh := make([]framer.Burst, 0, 4)
	var kept *framer.Framer
	out = append(out,
		entry{"layer2/framer.New", "", 2, func() { kept = framer.New(slot.cfg) }},
		entry{"layer2/framer.New", "New and 4 AppendBursts", 2, func() {
			kept = framer.New(slot.cfg)
			for _, c := range slot.chunks[:4] {
				fresh = kept.AppendBursts(fresh[:0], c...)
			}
		}},
	)
	f := framer.New(slot.cfg)
	var i int
	var bursts []framer.Burst
	for range 2 * len(slot.chunks) {
		bursts = f.Write(slot.chunks[i%len(slot.chunks)]...)
		i++
	}
	out = append(out,
		entry{"layer2/framer.Framer.Write", "SlotPeriod chunks", 1, func() {
			bursts = f.Write(slot.chunks[i%len(slot.chunks)]...)
			i++
		}},
		entry{"layer2/framer.Framer.Locked", "", 0, func() { locked = f.Locked() }},
	)
	_, _, _, _, _ = found, locked, fresh, bursts, kept
	return out
}
