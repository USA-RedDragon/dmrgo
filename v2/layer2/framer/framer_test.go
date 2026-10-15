package framer_test

import (
	"math/rand"
	"os"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/elements"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/framer"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

func loadVoice(t *testing.T) [][33]byte {
	t.Helper()
	data, err := os.ReadFile("../testdata/voice.bin")
	if err != nil {
		t.Fatal(err)
	}
	out := make([][33]byte, len(data)/33)
	for i := range out {
		copy(out[i][:], data[i*33:])
	}
	return out
}

func noise(r *rand.Rand, n int) []bit.Bit {
	out := make([]bit.Bit, n)
	for i := range out {
		out[i] = bit.Bit(r.Intn(2))
	}
	return out
}

func burstBits(b [33]byte) []bit.Bit {
	return bit.UnpackBits(b[:])
}

func flip(b [33]byte, positions ...int) [33]byte {
	for _, p := range positions {
		b[p/8] ^= 0x80 >> (p % 8)
	}
	return b
}

func writeChunks(r *rand.Rand, f *framer.Framer, stream []bit.Bit) []framer.Burst {
	var out []framer.Burst
	for len(stream) > 0 {
		n := min(1+r.Intn(400), len(stream))
		out = append(out, f.Write(stream[:n]...)...)
		stream = stream[n:]
	}
	return out
}

func TestSearch(t *testing.T) {
	t.Parallel()
	r := rand.New(rand.NewSource(1))
	burst := flip(loadVoice(t)[1], 110, 130, 150)
	stream := append(noise(r, 1001), burstBits(burst)...)
	m, ok := framer.Search(stream, 0, -1)
	if !ok || m.Offset != 1001+framer.SyncOffset || m.Pattern != enums.BsSourcedVoice || m.Distance != 3 {
		t.Fatalf("got %+v %v", m, ok)
	}
	if _, ok := framer.Search(stream, 0, 2); ok {
		t.Error("threshold 2 matched a SYNC with 3 errors")
	}
	if _, ok := framer.Search(stream, m.Offset+1, -1); ok {
		t.Error("found a second SYNC")
	}
	if _, ok := framer.Search(stream[:m.Offset+framer.SyncBits-1], 0, -1); ok {
		t.Error("matched a partial SYNC")
	}
	if _, ok := framer.Search(stream, -5, -1); !ok {
		t.Error("negative start")
	}
}

func TestFramer_SlotPeriod(t *testing.T) {
	t.Parallel()
	r := rand.New(rand.NewSource(2))
	bursts := loadVoice(t)
	bursts[7] = flip(bursts[7], 108, 120, 140, 155)
	bursts[20] = flip(bursts[20], 3, 200)
	stream := noise(r, 777)
	for _, b := range bursts {
		stream = append(stream, burstBits(b)...)
		stream = append(stream, noise(r, framer.SlotPeriod-framer.BurstBits)...)
	}
	f := framer.New(framer.Config{Threshold: -1, Period: framer.SlotPeriod})
	got := writeChunks(r, f, stream)
	if len(got) < len(bursts) {
		t.Fatalf("%d bursts, want %d", len(got), len(bursts))
	}
	for i, b := range bursts {
		if got[i].Data != b {
			t.Fatalf("burst %d differs", i)
		}
		if want := int64(777 + i*framer.SlotPeriod); got[i].Offset != want {
			t.Errorf("burst %d offset %d, want %d", i, got[i].Offset, want)
		}
		if got[i].Channel != 0 || got[i].HasCACH {
			t.Errorf("burst %d channel %d", i, got[i].Channel)
		}
		dec, err := layer2.NewBurstFromBytes(got[i].Data)
		if err != nil {
			t.Fatalf("burst %d: %v", i, err)
		}
		if (got[i].Sync == enums.EmbeddedSignallingPattern) != dec.HasEmbeddedSignalling {
			t.Errorf("burst %d sync %x", i, got[i].Sync)
		}
	}
	if got[7].Distance != 4 || got[7].Sync != enums.BsSourcedVoice {
		t.Errorf("burst 7: %+v", got[7])
	}
	more := f.Write(noise(r, 20*framer.SlotPeriod)...)
	if f.Locked() || len(more) == 0 || len(more) > framer.DefaultMaxMisses+1 {
		t.Errorf("after noise: locked %v, %d bursts", f.Locked(), len(more))
	}
	again := f.Write(append(burstBits(bursts[1]), noise(r, 10)...)...)
	if len(again) != 1 || again[0].Data != bursts[1] {
		t.Errorf("relock: %d bursts", len(again))
	}
}

func TestFramer_OutboundCACH(t *testing.T) {
	t.Parallel()
	r := rand.New(rand.NewSource(3))
	voice := loadVoice(t)
	idle := must(layer2.BuildLCDataBurst([12]byte{}, elements.DataTypeIdle, 1))
	type sent struct {
		data    [33]byte
		channel int
	}
	var want []sent
	stream := noise(r, 1234)
	for i, v := range voice {
		for ch := 1; ch <= 2; ch++ {
			data := idle
			if ch == 1 {
				data = v
			}
			c := layer2.CACH{TACT: pdu.TACT{AccessType: true, TDMAChannel: ch == 2}}
			c.Payload[i%17] = 1
			logical := must(layer2.EncodeCACH(&c))
			tx := layer2.CACHInterleave(logical)
			stream = append(stream, tx[:]...)
			stream = append(stream, burstBits(data)...)
			want = append(want, sent{data, ch})
		}
	}
	f := framer.New(framer.Config{Threshold: -1, CACH: true})
	got := writeChunks(r, f, stream)
	if len(got) < len(want) {
		t.Fatalf("%d bursts, want %d", len(got), len(want))
	}
	for i, w := range want {
		g := got[i]
		if g.Data != w.data || g.Channel != w.channel || !g.HasCACH || !g.CACH.TACT.AccessType {
			t.Fatalf("burst %d: channel %d, want %d, CACH %+v", i, g.Channel, w.channel, g.CACH)
		}
		if g.CACH.Payload[(i/2)%17] != 1 {
			t.Errorf("burst %d: CACH payload", i)
		}
		if wantOff := int64(1234 + layer2.CACHBits + i*framer.OutboundPeriod); g.Offset != wantOff {
			t.Errorf("burst %d offset %d, want %d", i, g.Offset, wantOff)
		}
	}
}

func TestFramer_DirectChannel(t *testing.T) {
	t.Parallel()
	r := rand.New(rand.NewSource(4))
	stream := make([]bit.Bit, 0, 500+3*framer.SlotPeriod)
	ts2 := must(layer2.BuildLCDataBurst([12]byte{}, elements.DataTypeIdle, 1))
	var bits [264]bit.Bit
	copy(bits[:], bit.UnpackBits(ts2[:]))
	for i := range framer.SyncBits {
		bits[framer.SyncOffset+i] = bit.Bit(uint64(enums.Tdma2Data) >> (framer.SyncBits - 1 - i) & 1)
	}
	ts2 = bit.PackBits264(bits)
	stream = append(stream, noise(r, 500)...)
	for range 3 {
		stream = append(stream, burstBits(ts2)...)
		stream = append(stream, noise(r, framer.SlotPeriod-framer.BurstBits)...)
	}
	got := framer.New(framer.Config{Threshold: -1, Period: framer.SlotPeriod}).Write(stream...)
	if len(got) != 3 {
		t.Fatalf("%d bursts", len(got))
	}
	for i, g := range got {
		if g.Channel != 2 || g.Sync != enums.Tdma2Data {
			t.Errorf("burst %d: %+v", i, g)
		}
	}
}

func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}
