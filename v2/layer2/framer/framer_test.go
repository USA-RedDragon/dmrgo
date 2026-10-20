package framer_test

import (
	"math"
	"math/rand"
	"os"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/internal/bits"
	"github.com/USA-RedDragon/dmrgo/v2/internal/unpacked"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/framer"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

func loadVoice(t testing.TB) [][33]byte {
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

func noise(r *rand.Rand, n int) []unpacked.Bit {
	out := make([]unpacked.Bit, n)
	for i := range out {
		out[i] = unpacked.Bit(r.Intn(2))
	}
	return out
}

func burstBits(b [33]byte) []unpacked.Bit {
	out := make([]unpacked.Bit, layer2.BurstBits)
	unpacked.UnpackAt(out, b[:], 0)
	return out
}

func pack(stream []unpacked.Bit) []byte {
	out := make([]byte, (len(stream)+7)/8)
	unpacked.PackAt(out, 0, stream)
	return out
}

func flip(b [33]byte, positions ...int) [33]byte {
	for _, p := range positions {
		b[p/8] ^= 0x80 >> (p % 8)
	}
	return b
}

func writeChunks(r *rand.Rand, f *framer.Framer, stream []unpacked.Bit) []framer.Burst {
	var out []framer.Burst
	for p := pack(stream); len(p) > 0; {
		n := min(1+r.Intn(50), len(p))
		out = append(out, f.Write(p[:n], 8*n)...)
		p = p[n:]
	}
	return out
}

func TestSearch(t *testing.T) {
	t.Parallel()
	r := rand.New(rand.NewSource(1))
	burst := flip(loadVoice(t)[1], 110, 130, 150)
	stream := pack(append(noise(r, 1004), burstBits(burst)...))
	m, ok := framer.Search(stream, 8*len(stream), 0, -1)
	if !ok || m.Offset != 1004+framer.SyncOffset || m.Pattern != enums.SyncBSSourcedVoice || m.Distance != 3 {
		t.Fatalf("got %+v %v", m, ok)
	}
	if _, ok := framer.Search(stream, 8*len(stream), 0, 2); ok {
		t.Error("threshold 2 matched a SYNC with 3 errors")
	}
	if _, ok := framer.Search(stream, 8*len(stream), m.Offset+1, -1); ok {
		t.Error("found a second SYNC")
	}
	end := m.Offset + framer.SyncBits
	if end%8 != 0 {
		t.Fatalf("the SYNC ends at bit %d, inside a byte", end)
	}
	if _, ok := framer.Search(stream[:end/8], end, 0, -1); !ok {
		t.Error("missed a SYNC that ends the stream")
	}
	if _, ok := framer.Search(stream[:end/8], end+8, 0, -1); !ok {
		t.Error("missed a SYNC with nbits over the stream")
	}
	for cut := end - 7; cut < end; cut++ {
		if _, ok := framer.Search(stream, cut, 0, -1); ok {
			t.Errorf("matched a SYNC cut at bit %d", cut)
		}
	}
	if _, ok := framer.Search(stream, 8*len(stream), -5, -1); !ok {
		t.Error("negative start")
	}
	if _, ok := framer.Search(stream, 8*len(stream), math.MaxInt, -1); ok {
		t.Error("found a SYNC after the end of the stream")
	}
	if _, ok := framer.Search(stream, -1, 0, -1); ok {
		t.Error("found a SYNC in a negative bit count")
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
		stream = append(stream, noise(r, framer.SlotPeriod-layer2.BurstBits)...)
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
		if (got[i].Sync == enums.SyncNone) != dec.HasEmbeddedSignalling() {
			t.Errorf("burst %d sync %x", i, got[i].Sync)
		}
	}
	if got[7].Distance != 4 || got[7].Sync != enums.SyncBSSourcedVoice {
		t.Errorf("burst 7: %+v", got[7])
	}
	more := f.Write(pack(noise(r, 20*framer.SlotPeriod)), 20*framer.SlotPeriod)
	if f.Locked() || len(more) == 0 || len(more) > framer.DefaultMaxMisses+1 {
		t.Errorf("after noise: locked %v, %d bursts", f.Locked(), len(more))
	}
	again := f.Write(pack(append(burstBits(bursts[1]), noise(r, 10)...)), layer2.BurstBits+10)
	if len(again) != 1 || again[0].Data != bursts[1] {
		t.Errorf("relock: %d bursts", len(again))
	}
}

func TestFramer_OutboundCACH(t *testing.T) {
	t.Parallel()
	r := rand.New(rand.NewSource(3))
	voice := loadVoice(t)
	idle := [33]byte(must(layer2.BuildDataBurst([12]byte{}, enums.DataTypeIdle, 1, layer2.SyncSourceBS)))
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
			bits.SetBit(c.Payload[:], i%17, 1)
			tx := layer2.CACHInterleave(must(c.Encode()))
			var txBits [layer2.CACHBits]unpacked.Bit
			unpacked.UnpackAt(txBits[:], tx[:], 0)
			stream = append(stream, txBits[:]...)
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
		if bits.Bit(g.CACH.Payload[:], (i/2)%17) != 1 {
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
	stream := make([]unpacked.Bit, 0, 500+3*framer.SlotPeriod)
	ts2 := [33]byte(must(layer2.BuildDataBurst([12]byte{}, enums.DataTypeIdle, 1, layer2.SyncSourceBS)))
	bits.SetField64(ts2[:], framer.SyncOffset, framer.SyncBits, uint64(enums.SyncTDMA2Data))
	stream = append(stream, noise(r, 500)...)
	for range 3 {
		stream = append(stream, burstBits(ts2)...)
		stream = append(stream, noise(r, framer.SlotPeriod-layer2.BurstBits)...)
	}
	got := framer.New(framer.Config{Threshold: -1, Period: framer.SlotPeriod}).Write(pack(stream), len(stream))
	if len(got) != 3 {
		t.Fatalf("%d bursts", len(got))
	}
	for i, g := range got {
		if g.Channel != 2 || g.Sync != enums.SyncTDMA2Data {
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
