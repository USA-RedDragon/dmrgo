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

func ctBurstOn(t *testing.T, f ctf, src layer2.SyncSource) *layer2.RawBurst {
	t.Helper()
	p := pdu.ChannelTimingPDU{Generation: f.gen, SyncAge: f.sa, LeaderDynamicIdentifier: f.ldi, LeaderIdentifier: f.lid, NewLeader: f.nl,
		ChannelTimingOp1: f.cto&2 != 0, ChannelTimingOp0: f.cto&1 != 0, SourceDynamicIdentifier: f.sdi, SourceIdentifier: f.sid}
	c := pdu.CSBK{LastBlock: true}
	if err := c.SetChannelTiming(p); err != nil {
		t.Fatal(err)
	}
	b, err := layer2.BuildBurst(&c, csbk.AllSiteColorCode, src)
	if err != nil {
		t.Fatal(err)
	}
	return &b
}

// The CT_CSBKs of a radio go in its provisioned slot, on the slot timing
// that it took, and after the CT_CSBK that it answers: the next slot of
// the channel for a CT_CSBK heard in the other slot, and a TDMA frame
// later for one heard in the provisioned slot (ETSI TS 102 361-2 clauses
// 6.2.2.1, 6.2.2.2 and 6.2.3.7).
func TestTimingSlotGrid(t *testing.T) {
	t.Parallel()
	appoint := ctf{gen: 1, sa: 40, ldi: 2, lid: 100, nl: true, cto: 2, sid: 300}
	for _, tt := range []struct {
		name  string
		src   layer2.SyncSource
		hint  enums.Slot
		first time.Duration
	}{
		{"other slot", layer2.SyncSourceDirectTS2, enums.SlotUnknown, minute + 30*ms},
		{"provisioned slot", layer2.SyncSourceDirectTS1, enums.SlotUnknown, minute + 60*ms},
		{"other slot by hint", layer2.SyncSourceMS, enums.Slot2, minute + 30*ms},
		{"provisioned slot by hint", layer2.SyncSourceMS, enums.Slot1, minute + 60*ms},
	} {
		d := newTiming(t, 100, 2, 7)
		d.until(minute)
		d.keep(d.m.Burst(minute, ctBurstOn(t, appoint, tt.src), layer2.Hint{Slot: tt.hint}, d.out[:0]))
		d.state(csbk.TimingLeader)
		got := d.txs(d.until(minute + time.Second))
		if len(got) != 1 || got[0].first != tt.first || got[0].n != 10 {
			t.Errorf("%s: beacon %+v, want it at %v", tt.name, got, tt.first)
		}
		got = d.txs(d.until(minute + constants.BeaconInterval + rhotMax + time.Second))
		if len(got) != 1 || (got[0].first-tt.first)%csbk.BurstInterval != 0 {
			t.Errorf("%s: next beacon %+v off the slot grid of %v", tt.name, got, tt.first)
		}
	}

	for seed := range uint64(32) {
		d := newTiming(t, 100, 2, seed)
		d.until(minute)
		d.keep(d.m.Burst(minute, ctBurstOn(t, ctf{sdi: 1, sid: 55}, layer2.SyncSourceDirectTS1), layer2.Hint{}, d.out[:0]))
		got := d.txs(d.until(minute + rhotMax + time.Second))
		if len(got) != 1 || got[0].first < minute+csbk.BurstInterval || (got[0].first-minute)%csbk.BurstInterval != 0 {
			t.Errorf("seed %d: request %+v, want it on the slot grid of %v after the CT_CSBK", seed, got, minute)
		}
	}
}
