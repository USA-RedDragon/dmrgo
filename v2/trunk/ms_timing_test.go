package trunk_test

import (
	"testing"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/constants"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/layer3"
	"github.com/USA-RedDragon/dmrgo/v2/trunk"
)

// slotGrid is the TDMA slot grid of ETSI TS 102 361-1 clause 5.1.1 and
// Figure 5.23: outbound slot k starts at t0 + k*30 ms, TS1 for an even k,
// and inbound time slot X starts 30 ms after outbound time slot X. On an
// offset timing TSCC a logical channel uses the inbound time slot of its
// number, on an aligned one the other (Table 5.1). The CACH before
// outbound slot k has the TC of that slot and the AT of the next inbound
// slot of that TC (clauses 9.3.8 and 9.3.9).
type slotGrid struct {
	t0     time.Duration
	offset bool
}

func (g slotGrid) slot(k int) enums.Slot {
	if k%2 == 0 {
		return enums.Slot1
	}
	return enums.Slot2
}

func (g slotGrid) at(k int) time.Duration { return g.t0 + time.Duration(k)*slot }

func (g slotGrid) inbound(s enums.Slot) enums.Slot {
	if g.offset {
		return s
	}
	return s.Other()
}

func (g slotGrid) cachIndex(due time.Duration) (int, bool) {
	d := due - slot - g.t0
	if d < 0 || d%slot != 0 {
		return 0, false
	}
	return int(d / slot), true
}

func cachOf(t testing.TB, tc enums.Slot, at bool) [3]byte {
	t.Helper()
	c := layer2.CACH{TACT: pdu.TACT{AccessType: at, TDMAChannel: tc == enums.Slot2}}
	logical, err := c.Encode()
	if err != nil {
		t.Fatal(err)
	}
	return layer2.CACHInterleave(logical)
}

func timedAloha(t testing.TB, offset bool) layer2.RawBurst {
	t.Helper()
	return bsBurst(t, withPDU(t, &pdu.CSBK{LastBlock: true}, (*pdu.CSBK).SetAloha, pdu.AlohaPDU{
		Version: 1, Offset: offset, NRandWait: 2, Reg: true, Backoff: 3, SysIdentCode: sysIdent(t)}), testCC)
}

func TestMS_RandomAccessSlotGrid(t *testing.T) {
	t.Parallel()
	for _, offset := range []bool{false, true} {
		for _, own := range []enums.Slot{enums.Slot1, enums.Slot2} {
			g := slotGrid{t0: time.Second, offset: offset}
			x := g.inbound(own)
			busyAt := func(k int) bool { return (uint32(k)*2654435761)>>7&1 == 1 }
			sends := 0
			for seed := range uint64(16) {
				ms, err := trunk.NewMS(&trunk.MSConfig{Address: msAddr, Slot: own, Seed: seed, NoRegistration: true, RandomTimeout: -1})
				if err != nil {
					t.Fatal(err)
				}
				al := timedAloha(t, offset)
				out := make([]trunk.Output, 0, trunk.MaxOutputs)
				for k := 0; k < 600; k++ {
					if k == 4 {
						if err := ms.Call(g.at(k), false, 3120002, layer3.VoiceServiceOptions{}); err != nil {
							t.Fatal(err)
						}
					}
					cach := cachOf(t, g.slot(k), busyAt(k))
					out = ms.Burst(g.at(k), &al, layer2.Hint{Slot: g.slot(k)}, &cach, out[:0])
					for i := range out {
						if out[i].Kind != trunk.OutputBurst {
							continue
						}
						sends++
						j, ok := g.cachIndex(out[i].Due)
						switch {
						case out[i].Slot != own:
							t.Errorf("offset %v %v: C_RAND on %v", offset, own, out[i].Slot)
						case !ok || g.slot(j) != x:
							t.Errorf("offset %v %v: C_RAND due %v is not in inbound %v", offset, own, out[i].Due, x)
						case j > k:
							t.Errorf("offset %v %v: C_RAND due %v at %v uses the AT of a CACH still to come", offset, own, out[i].Due, g.at(k))
						case busyAt(j):
							t.Errorf("offset %v %v: C_RAND due %v in a slot that the CACH at %v withdrew", offset, own, out[i].Due, g.at(j))
						}
					}
					if ms.State() != trunk.StateControl {
						break
					}
				}
			}
			if sends == 0 {
				t.Errorf("offset %v %v: no C_RAND", offset, own)
			}
		}
	}
}

func TestMS_AnswerSlotGrid(t *testing.T) {
	t.Parallel()
	for _, offset := range []bool{false, true} {
		for _, own := range []enums.Slot{enums.Slot1, enums.Slot2} {
			g := slotGrid{t0: time.Second, offset: offset}
			ms, err := trunk.NewMS(&trunk.MSConfig{Address: msAddr, Slot: own, NoRegistration: true})
			if err != nil {
				t.Fatal(err)
			}
			al := timedAloha(t, offset)
			ahoy := bsBurst(t, withPDU(t, &pdu.CSBK{LastBlock: true}, (*pdu.CSBK).SetAhoy, pdu.AhoyPDU{
				ServiceKind: uint8(enums.ServiceKindIndividualVoice), TargetAddress: msAddr, SourceAddress: 3120002}), testCC)
			out := make([]trunk.Output, 0, trunk.MaxOutputs)
			k := 0
			for ; k < 4 || g.slot(k) != own; k++ {
				cach := cachOf(t, g.slot(k), false)
				out = ms.Burst(g.at(k), &al, layer2.Hint{Slot: g.slot(k)}, &cach, out[:0])
			}
			cach := cachOf(t, g.slot(k), false)
			out = ms.Burst(g.at(k), &ahoy, layer2.Hint{Slot: own}, &cach, out[:0])
			wantKinds(t, "C_AHOY", out, trunk.OutputAhoy, trunk.OutputBurst)
			want := g.at(k) + 60*time.Millisecond
			if offset {
				want += slot
			}
			j, ok := g.cachIndex(out[1].Due)
			if out[1].Due != want || out[1].Slot != own || !ok || g.slot(j) != g.inbound(own) {
				t.Errorf("offset %v %v: C_ACKU to the C_AHOY at %v due %v slot %v, want %v in inbound %v",
					offset, own, g.at(k), out[1].Due, out[1].Slot, want, g.inbound(own))
			}
		}
	}
}

func TestMS_EndCallSlotGrid(t *testing.T) {
	t.Parallel()
	for _, offset := range []bool{false, true} {
		r := newRig(t, &trunk.MSConfig{Address: msAddr, NoRegistration: true})
		a := aloha(t)
		r.feed(a, &idle)
		if err := r.ms.Call(r.now, true, 9, layer3.VoiceServiceOptions{}); err != nil {
			t.Fatal(err)
		}
		r.feed(a, &idle)
		grant := bsBurst(t, withPDU(t, &pdu.CSBK{LastBlock: true}, (*pdu.CSBK).SetTalkgroupVoiceGrant, pdu.TalkgroupVoiceGrantPDU{
			PhysicalChannel: 5, LogicalChannel: true, Offset: offset, TargetAddress: 9, SourceAddress: msAddr}), testCC)
		wantKinds(t, "TV_GRANT", r.feed(grant, &idle), trunk.OutputGrant, trunk.OutputTune)
		heard := r.now + slot
		term := terminator(t, 9)
		r.ms.Burst(heard, &term, layer2.Hint{Slot: enums.Slot2}, nil, r.out[:0])
		out := r.ms.EndCall(heard+10*time.Millisecond, r.out[:0])
		first := heard + 60*time.Millisecond
		if offset {
			first = heard + slot
		}
		for i := range constants.NMaint {
			if want := first + time.Duration(i)*60*time.Millisecond; out[i].Kind != trunk.OutputBurst || out[i].Due != want {
				t.Errorf("offset %v: P_MAINT %d due %v, want %v", offset, i, out[i].Due, want)
			}
		}
	}
}

func TestTSCC_WithdrawsTheAnswerSlot(t *testing.T) {
	t.Parallel()
	ts, err := trunk.NewTSCC(tsccConfig())
	if err != nil {
		t.Fatal(err)
	}
	type sent struct {
		slot enums.Slot
		ahoy bool
		at   bool
	}
	var got []sent
	out := make([]trunk.TSCCOutput, 0, trunk.MaxTSCCOutputs)
	for k := range 40 {
		if k == 10 {
			if err := ts.Stun(time.Duration(k)*slot, 3120001, false); err != nil {
				t.Fatal(err)
			}
		}
		out = ts.Tick(time.Duration(k)*slot, out[:0])
		for i := range out {
			if out[i].Kind != trunk.TSCCBurst || out[i].Channel != tsccChannel {
				continue
			}
			b, err := layer2.NewBurstFromBytes(out[i].Burst)
			if err != nil {
				t.Fatal(err)
			}
			c, _ := b.Data.(*pdu.CSBK)
			_, ahoy := c.AsAhoy()
			got = append(got, sent{slot: out[i].Slot, ahoy: ahoy, at: layer2.DecodeCACHField(out[i].CACH).TACT.AccessType})
		}
	}
	n := 0
	for k, s := range got {
		if !s.ahoy {
			continue
		}
		n++
		if s.at || k+1 >= len(got) || !got[k+1].at || got[k+1].slot != s.slot.Other() {
			t.Errorf("C_AHOY on %v at slot %d: AT %v before it, the next CACH %+v; want the CACH after it busy", s.slot, k, s.at, got[min(k+1, len(got)-1)])
		}
	}
	busy := 0
	for _, s := range got {
		if s.at {
			busy++
		}
	}
	if n == 0 || busy != n {
		t.Errorf("%d C_AHOY, %d busy CACHs, want one busy CACH for each C_AHOY", n, busy)
	}
}
