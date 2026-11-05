package voice_test

import (
	"math/rand/v2"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/internal/testseed"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/voice"
)

func TestRegression_NoiseBurstInCallPins(t *testing.T) {
	t.Parallel()

	for _, run := range []uint64{2, 7, 12, 13} {
		noiseBurstInCall(t, testseed.Of(testseed.Mix(run, testseed.NameHash("TestRegression_NoiseBurstInCallIsCounted"))).Uint64(), nil)
	}
}

func TestRegression_WeakEMBDoesNotMoveTheCall(t *testing.T) {
	t.Parallel()

	for _, e := range []struct {
		cc   enums.ColorCode
		errs []int
	}{{13, nil}, {1, []int{108}}, {1, []int{109, 150}}} {
		for _, lcss := range []enums.LCSS{enums.LCSSFirstFragment, enums.LCSSLastFragment, enums.LCSSSingleFragment} {
			for seed := range uint64(16) {
				noiseBurstInCall(t, seed, func(raw *layer2.RawBurst) {
					if err := raw.SetEMB(pdu.EmbeddedSignalling{ColorCode: e.cc, LCSS: lcss}); err != nil {
						t.Fatal(err)
					}
					for _, bit := range e.errs {
						raw[bit/8] ^= 0x80 >> (bit % 8)
					}
				})
			}
		}
	}
}

func noiseBurstInCall(t *testing.T, seed uint64, emb func(*layer2.RawBurst)) {
	t.Helper()
	r := rand.New(rand.NewPCG(seed, 6))
	call := transmission(t, groupLC(t, 91, 3002068), 2, nil)
	in := timed(call, 0, layer2.Hint{})
	in[4].raw = noiseBurst(r)
	if emb != nil {
		emb(&in[4].raw)
	}
	events := feed(t, newReceiver(t, nil), in, -1)
	v := find(events, voice.EventVoice)
	end := find(events, voice.EventCallEnd)
	want := "ABCDEFABCDEF"
	if len(v) == 11 {
		want = "ABCEFABCDEF"
	}
	if len(end) != 1 || int(end[0].Stats.Bursts) != len(v) || end[0].Stats.Bursts+end[0].Stats.Lost != 12 || positions(events) != want {
		t.Errorf("seed %d %v: %s, positions %s, end %+v", seed, in[4].raw.EMB(), kinds(events), positions(events), end)
	}
}
