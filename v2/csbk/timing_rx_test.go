package csbk_test

import (
	"testing"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/constants"
	"github.com/USA-RedDragon/dmrgo/v2/csbk"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

func preambleOn(t *testing.T, src layer2.SyncSource) *layer2.RawBurst {
	t.Helper()
	c := pdu.CSBK{LastBlock: true}
	if err := c.SetPreamble(pdu.PreamblePDU{Group: true, CSBKBlocksToFollow: 3, TargetAddress: 9, SourceAddress: 3120005}); err != nil {
		t.Fatal(err)
	}
	b, err := layer2.BuildBurst(&c, 1, src)
	if err != nil {
		t.Fatal(err)
	}
	return &b
}

func onTS1(at time.Duration) time.Duration {
	if r := (at - minute - 30*ms) % csbk.BurstInterval; r != 0 {
		at += csbk.BurstInterval - r
	}
	return at
}

func overlaps(t *testing.T, d *tdriver, rx time.Duration, out []csbk.Output) {
	t.Helper()
	for _, o := range out {
		if o.Kind == csbk.OutputBurst && o.Due < rx+csbk.BurstInterval/2 && o.Due+csbk.BurstInterval/2 > rx {
			t.Fatalf("CT_CSBK due %v over the burst received at %v", o.Due, rx)
		}
	}
	if got := d.txs(out); len(got) == 0 {
		t.Fatalf("no CT_CSBK after the burst received at %v", rx)
	}
}

// ETSI TS 102 361-2 clauses 6.2.2.3.4 and 6.2.2.3.5, ETSI TS 102 361-1
// clause 5.2.2.1
func TestTimingHoldoffEndsOnReceivedBurst(t *testing.T) {
	t.Parallel()
	n := 0
	for seed := range uint64(16) {
		d := newTiming(t, 100, 2, seed)
		d.rx(minute, ctf{gen: 1, sa: 20, ldi: 3, lid: 7, cto: 2, sid: 55})
		d.state(csbk.TimingLeaderAndTimingKnown)
		d.rx(minute+30*ms, ctf{sdi: 1, sid: 300})
		at, ok := d.m.Deadline()
		if !ok || at <= minute+30*ms {
			continue
		}
		n++
		at = onTS1(at)
		d.until(at - 1)
		out := d.m.Burst(at, ctBurstOn(t, ctf{sdi: 1, sid: 301}, layer2.SyncSourceDirectTS1), layer2.Hint{}, d.out[:0])
		d.keep(out)
		overlaps(t, d, at, append(append([]csbk.Output(nil), out...), d.until(at+rhotMax+time.Second)...))
	}
	if n == 0 {
		t.Fatal("no seed gave a CT_RHOT")
	}
}

// ETSI TS 102 361-2 clauses 6.2.2.3.1 and 6.2.3.5, ETSI TS 102 361-1
// clause 5.2.2.1
func TestTimingBeaconOnReceivedBurst(t *testing.T) {
	t.Parallel()
	for _, hint := range []enums.Slot{enums.SlotUnknown, enums.Slot1} {
		d := appointed(t)
		d.until(minute + constants.BeaconInterval)
		at, ok := d.m.Deadline()
		if !ok || at < minute+constants.BeaconInterval {
			t.Fatalf("beacon holdoff %v %v", at, ok)
		}
		at = onTS1(at)
		d.until(at - 1)
		out := d.m.Burst(at, preambleOn(t, layer2.SyncSourceMS), layer2.Hint{Slot: hint}, d.out[:0])
		d.keep(out)
		overlaps(t, d, at, append(append([]csbk.Output(nil), out...), d.until(at+rhotMax+time.Second)...))
	}
}
