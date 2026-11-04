//go:build !tinygo

package rf

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/internal/rftx"
)

func TestCoreTier3RawBitsAtLock(t *testing.T) {
	t.Parallel()
	cfg := rftx.SeqConfig{ColorCode: 1, Source: 3009101, Dest: 9990, BS: 990911, Wakeups: 2, Message: "DMRGO TEST MESSAGE"}
	for _, seq := range []string{"ack", "data12", "voice"} {
		built, err := rftx.Sequence(seq, cfg)
		if err != nil {
			t.Fatal(err)
		}
		fm, env := rftx.Modulate(built.Bursts, rftx.Schedule{Slot: 0})
		for _, off := range []float64{1500, 3000, -3000, 4000} {
			iq := rftx.Channel(rftx.ToIQ(fm, env, 2e6, 0, 0.9), 2e6, 2.048e6, off, 25, 5)
			r, err := newReceiver(RxConfig{Rate: 2.048e6, Tier: Tier3, Format: IQFormatCU8}, nil)
			if err != nil {
				t.Fatal(err)
			}
			got := r.AppendFlush(r.AppendBursts(nil, iq))
			if len(got) != len(built.Bursts) {
				t.Errorf("%s %+.0f Hz: %d bursts, want %d", seq, off, len(got), len(built.Bursts))
				continue
			}
			for i := range got {
				if got[i].Data != built.Bursts[i] {
					t.Errorf("%s %+.0f Hz burst %d: raw bits differ", seq, off, i)
				}
			}
		}
	}
}

func TestCoreTier3LockRetune(t *testing.T) {
	t.Parallel()
	row := mxRowOf(t, "s0.noise.+1500.bs.pi.0ppm.high.cs16-48k.inf")
	s, err := mxBuild(row)
	if err != nil {
		t.Fatal(err)
	}
	in := mxInputOf(s, row)
	r, err := newReceiver(RxConfig{Rate: in.rate, Tier: Tier3, Format: in.iqFmt}, nil)
	if err != nil {
		t.Fatal(err)
	}
	got := r.AppendFlush(r.AppendBursts(nil, in.iq))
	n := 0
	for i := range got {
		j := mxNearest(s, got[i].Seconds+in.delay)
		if j < 0 {
			continue
		}
		n++
		if got[i].Data != s.truth[j].data {
			t.Errorf("burst %d: raw bits differ", j)
		}
	}
	if n != len(s.truth) {
		t.Errorf("%d of %d bursts", n, len(s.truth))
	}
}

func TestCoreTier3Adjacent(t *testing.T) {
	t.Parallel()
	o := tierOptions(Tier3)
	for _, seed := range []uint64{1, 36, 43} {
		if n, want, _ := adjacentReceiveSeed(t, Tier3, &o, 0, seed); n < want {
			t.Errorf("seed %d: C0 and C1 of %d of %d voice frames exact", seed, n, want)
		}
	}
}
