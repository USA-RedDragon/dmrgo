package rf

import (
	"encoding/json"
	"os"
	"runtime"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/internal/rftx"
)

func TestAllocsReceiver(t *testing.T) {
	cfg := testConfig()
	cfg.BSSourced, cfg.Superframes = true, 2
	built, err := rftx.Sequence("voice", cfg)
	if err != nil {
		t.Fatal(err)
	}
	fm, env := rftx.ModulateBS(built.Slots)
	iq := rftx.Channel(rftx.ToIQ(fm, env, 2e6, 0, 0.9), 2e6, 2.048e6, 1500, 25, 0)
	const chunk = 1 << 14
	x := cf32(basebandStream(built.Slots, 1500))
	lcs := 0
	slc := func(l []ShortLC) { lcs += len(l) }
	for _, tier := range []Tier{Tier0, Tier1, Tier2, Tier3} {
		for _, c := range []struct {
			name string
			cfg  RxConfig
			in   []byte
		}{
			{"CU8 at 2.048 MHz", RxConfig{Rate: 2048000, Tier: tier, Format: IQFormatCU8, OnShortLC: slc}, iq},
			{"CF32 at 48 kHz", RxConfig{Rate: basebandRate, Tier: tier, Format: IQFormatCF32, OnShortLC: slc}, x},
		} {
			lcs = 0
			r := mustNew(c.cfg)
			var dst []Burst
			pos, bursts := 0, 0
			n := testing.AllocsPerRun(len(c.in)/chunk-2, func() {
				dst = r.AppendBursts(dst[:0], c.in[pos:pos+chunk])
				bursts += len(dst)
				pos += chunk
			})
			bursts += len(r.AppendFlush(dst[:0]))
			if n != 0 || bursts == 0 || lcs == 0 {
				t.Errorf("tier %d, %s: %v allocations per AppendBursts, %d bursts, %d Short LCs", tier, c.name, n, bursts, lcs)
			}
			bursts, lcs = 0, 0
			c.cfg.OnBursts = func(bs []Burst) { bursts += len(bs) }
			r = mustNew(c.cfg)
			pos = 0
			n = testing.AllocsPerRun(len(c.in)/chunk-2, func() {
				_, _ = r.Write(c.in[pos : pos+chunk])
				pos += chunk
			})
			_ = r.Flush()
			if n != 0 || bursts == 0 || lcs == 0 {
				t.Errorf("tier %d, %s: %v allocations per Write, %d bursts, %d Short LCs", tier, c.name, n, bursts, lcs)
			}
		}
	}
}

func TestAllocsReceiverRAMOtherGoroutines(t *testing.T) {
	e := benchEntry{benchIQ, Tier0}
	quiet := receiverRAM(e)
	stop, done := make(chan struct{}), make(chan [][]byte)
	go func() {
		var kept [][]byte
		for {
			select {
			case <-stop:
				done <- kept
				return
			default:
				kept = append(kept, make([]byte, 64))
				runtime.Gosched()
			}
		}
	}()
	got := make([]int, 0, 4)
	for range cap(got) {
		got = append(got, receiverRAM(e))
	}
	close(stop)
	<-done
	for _, n := range got {
		if n != quiet {
			t.Fatalf("receiverRAM under other allocations %v, quiet %d", got, quiet)
		}
	}
}

func TestAllocsReceiverRAM(t *testing.T) {
	b, err := os.ReadFile(rxbenchPath)
	if err != nil {
		t.Fatal(err)
	}
	var base benchData
	if err := json.Unmarshal(b, &base); err != nil {
		t.Fatal(err)
	}
	for _, want := range base.Tiers {
		e := benchEntry{want.Mode, Tier0 + Tier(want.Tier)}
		if got := receiverRAM(e); got > want.RAMBytes*11/10 {
			t.Errorf("%s: New keeps %d bytes, baseline %d", e.name(), got, want.RAMBytes)
		}
	}
}
