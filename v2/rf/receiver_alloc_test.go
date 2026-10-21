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
	for _, tier := range []Tier{Tier0, Tier1, Tier2, Tier3, Tier4} {
		r := mustNew(Config{Rate: 2.048e6, Tier: tier, MaxWrite: chunk})
		pos, bursts := 0, 0
		n := testing.AllocsPerRun(len(iq)/chunk-2, func() {
			bursts += len(must(r.Write(iq[pos : pos+chunk])))
			pos += chunk
		})
		if n != 0 || bursts == 0 {
			t.Errorf("tier %d: %v allocations per Write, %d bursts", tier, n, bursts)
		}
		x := basebandStream(built.Slots, 1500)
		r = mustNew(Config{Rate: basebandRate, Tier: tier})
		pos, bursts = 0, 0
		n = testing.AllocsPerRun(len(x)/chunk-2, func() {
			bursts += len(must(r.WriteBaseband(x[pos : pos+chunk])))
			pos += chunk
		})
		if n != 0 || bursts == 0 {
			t.Errorf("tier %d: %v allocations per WriteBaseband, %d bursts", tier, n, bursts)
		}
		bursts = 0
		r = mustNew(Config{Rate: basebandRate, Tier: tier, OnBurst: func(*Burst) { bursts++ }})
		pos = 0
		n = testing.AllocsPerRun(len(x)/chunk-2, func() {
			must(r.WriteBaseband(x[pos : pos+chunk]))
			pos += chunk
		})
		if n != 0 || bursts == 0 {
			t.Errorf("tier %d: %v allocations per WriteBaseband with OnBurst, %d bursts", tier, n, bursts)
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
