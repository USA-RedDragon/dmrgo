package dvsi_test

import (
	"bytes"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/internal/dvsisim"
	"github.com/USA-RedDragon/dmrgo/v2/vocoder/dvsi"
)

func parseSeeds(tb testing.TB) [][]byte {
	tb.Helper()
	seeds := make([][]byte, 0, 11)
	seeds = append(seeds, [][]byte{
		{},
		{0x61},
		unhex(tb, "610003 00 39 2f 15"),
		unhex(tb, "61000f 01 42 01 38 00112233445566 03 a1 02 0000"),
		unhex(tb, "61000e 01 01 50 00112233445566778899 2f 60"),
		unhex(tb, "610013 01 40 01 48 000000000000000000 03a0 020000 1748"),
		unhex(tb, "610002 00 3f 00"),
	}...)
	sp, _ := dvsi.AppendSpeech(nil, 1, ramp(), true)
	ch, _ := dvsi.AppendChannel(nil, 2, make([]byte, 9), 72, false)
	rate, _ := dvsi.AppendRateDMR(nil, 1, true)
	return append(seeds, sp, ch, rate, dvsi.AppendQuery(nil, true))
}

func checkParse(t testing.TB, b []byte) {
	for _, parity := range []bool{false, true} {
		p, n, err := dvsi.Parse(b, parity)
		if err != nil {
			if n != 0 {
				t.Fatalf("Parse(%x) = %d bytes with %v", b, n, err)
			}
			continue
		}
		if n < dvsi.HeaderLen || n > len(b) || n > dvsi.MaxPacketLen {
			t.Fatalf("Parse(%x) = %d bytes", b, n)
		}
		if p.Type > dvsi.TypeSpeech || len(p.Fields) > n-dvsi.HeaderLen {
			t.Fatalf("Parse(%x) = %+v", b, p)
		}
		for f := range 256 {
			if d, ok := p.Response(dvsi.Field(f)); ok && len(d) > len(p.Fields) {
				t.Fatalf("Response(%#x) of %x = %x", f, b, d)
			}
		}
		if pcm, ch, err := p.AppendSamples(nil); err == nil {
			if len(pcm) < dvsi.MinSamples || len(pcm) > dvsi.MaxSamples || ch < 0 || ch > dvsi.MaxChannel {
				t.Fatalf("AppendSamples of %x = %d samples, channel %d", b, len(pcm), ch)
			}
			again, err := dvsi.AppendSpeech(nil, ch, pcm, true)
			if err != nil || len(again) > dvsi.MaxPacketLen {
				t.Fatalf("AppendSpeech = %d bytes, %v", len(again), err)
			}
			q, _, _ := dvsi.Parse(again, true)
			if back, _, _ := q.AppendSamples(nil); !slices.Equal(back, pcm) {
				t.Fatalf("speech of %x does not survive AppendSpeech", b)
			}
		}
		if data, bits, ch, err := p.AppendChannelData(nil); err == nil {
			if bits < dvsi.MinBits || bits > dvsi.MaxBits || len(data) != (bits+7)/8 || ch < 0 || ch > dvsi.MaxChannel {
				t.Fatalf("AppendChannelData of %x = %x, %d bits, channel %d", b, data, bits, ch)
			}
			again, err := dvsi.AppendChannel(nil, ch, data, bits, true)
			if err != nil || len(again) > dvsi.MaxPacketLen {
				t.Fatalf("AppendChannel = %d bytes, %v", len(again), err)
			}
			q, _, err := dvsi.Parse(again, true)
			if err != nil {
				t.Fatal(err)
			}
			if back, _, _, _ := q.AppendChannelData(nil); !bytes.Equal(back, data) {
				t.Fatalf("channel data of %x does not survive AppendChannel", b)
			}
		}
	}
}

func FuzzParse(f *testing.F) {
	for _, s := range parseSeeds(f) {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, b []byte) {
		checkParse(t, b)
	})
}

func resyncSeeds() [][]byte {
	return [][]byte{
		{},
		{0x61, 0x00},
		{0x61, 0x00, 0x05, 0x02},
		{0x61, 0x00, 0x03, 0x02, 0x00, 0x2F, 0x00, 0x00, 0x61},
		bytes.Repeat([]byte{0x61}, 40),
	}
}

func checkResync(t testing.TB, garbage []byte, chunk uint8, datagram bool) {
	garbage = garbage[:min(len(garbage), dvsi.MaxPacketLen)]
	pairs := dvsisim.Pairs(2)
	d := dvsisim.New(pairs, dvsisim.Options{Chunk: int(chunk % 9), Datagram: datagram})
	p := &idleClockPort{Device: d}
	cfg := dvsi.Config{Timeout: 50 * time.Millisecond}
	dvsi.SetClock(&cfg, p.now)
	c, err := dvsi.New(p, &cfg)
	if err != nil {
		t.Fatal(err)
	}
	d.Garbage = garbage
	pcm, _, err := c.Decode(nil, pairs[0].Slot)
	if err != nil && !errors.Is(err, dvsi.ErrTimeout) {
		t.Fatalf("Decode after garbage %x = %v", garbage, err)
	}
	if err == nil && len(pcm) != dvsi.FrameSamples {
		t.Fatalf("Decode after garbage %x = %d samples", garbage, len(pcm))
	}
	d.Garbage = garbage
	if pcm, _, err := c.DecodeLost(nil); err != nil && !errors.Is(err, dvsi.ErrTimeout) || err == nil && len(pcm) != dvsi.FrameSamples {
		t.Fatalf("DecodeLost after garbage %x = %d samples, %v", garbage, len(pcm), err)
	}
	d.Garbage = garbage
	if _, err := c.Encode(pairs[1].PCM[:]); err != nil && !errors.Is(err, dvsi.ErrTimeout) {
		t.Fatalf("Encode after garbage %x = %v", garbage, err)
	}
	if len(d.Errs) > 0 {
		t.Fatalf("garbage %x: device errors %q", garbage, d.Errs)
	}
}

func FuzzResync(f *testing.F) {
	for _, s := range resyncSeeds() {
		f.Add(s, uint8(0), false)
		f.Add(s, uint8(1), false)
		f.Add(s, uint8(0), true)
	}
	f.Fuzz(func(t *testing.T, garbage []byte, chunk uint8, datagram bool) {
		checkResync(t, garbage, chunk, datagram)
	})
}

func TestResyncSeeds(t *testing.T) {
	t.Parallel()

	for _, s := range resyncSeeds() {
		checkResync(t, s, 0, false)
		checkResync(t, s, 1, false)
		checkResync(t, s, 0, true)
	}
}
