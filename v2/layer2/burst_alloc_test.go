//go:build !tinygo

package layer2_test

import (
	"math/rand"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
)

func TestAllocsBurstDecode(t *testing.T) {
	frames := captureFrames(t)
	frames = append(frames, fuzzCorpus(t, "FuzzBurstDecode")...)
	r := rand.New(rand.NewSource(1))
	n := len(frames)
	for i := range 4 * n {
		f := frames[i%n]
		for range 1 + r.Intn(12) {
			p := r.Intn(264)
			f[p/8] ^= 0x80 >> (p % 8)
		}
		frames = append(frames, f)
	}
	for range 1000 {
		var f layer2.RawBurst
		r.Read(f[:])
		frames = append(frames, f)
	}

	cc := enums.ColorCode(1)
	dt := enums.DataTypeCSBK
	opts := []*layer2.DecodeOptions{
		nil,
		{ColorCode: &cc},
		{Kind: layer2.KindVoice, VoiceBurst: enums.VoiceBurstC},
		{Kind: layer2.KindData, DataType: &dt},
		{Kind: layer2.KindReverseChannel},
		{SyncThreshold: -1},
	}
	var b layer2.Burst
	for _, o := range opts {
		all := testing.AllocsPerRun(2, func() {
			for _, f := range frames {
				_ = b.DecodeFromBytesWith(f, o)
			}
		})
		if all == 0 {
			continue
		}
		for _, f := range frames {
			if n := testing.AllocsPerRun(2, func() { _ = b.DecodeFromBytesWith(f, o) }); n != 0 {
				err := b.DecodeFromBytesWith(f, o)
				t.Fatalf("decode of %x with %+v allocates %.0f times (error %v)", f, o, n, err)
			}
		}
	}
}
