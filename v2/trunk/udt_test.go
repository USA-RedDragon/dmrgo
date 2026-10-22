package trunk_test

import (
	"testing"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/trunk"
)

func udtBursts(t testing.TB, dst uint32, text string, damage bool) []layer2.RawBurst {
	t.Helper()
	blocks := make([][12]byte, 2)
	copy(blocks[0][:], text)
	if len(text) > 12 {
		copy(blocks[1][:10], text[12:])
	}
	pdu.SetMBCLastBlockCRC(blocks)
	if damage {
		blocks[0][0] ^= 1
	}
	h := withPDU(t, &pdu.DataHeader{}, (*pdu.DataHeader).SetUDT, pdu.UDTHeader{
		LLIDDestination: dst, LLIDSource: 3120002, UAB: 1, Opcode: pdu.UDTOpcodeOutboundHeader})
	out := make([]layer2.RawBurst, 0, 1+len(blocks))
	out = append(out, bsBurst(t, h, testCC))
	for _, b := range blocks {
		raw, err := layer2.BuildDataBurst(b, enums.DataTypeRate12, testCC, layer2.SyncSourceBS)
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, raw)
	}
	return out
}

func TestFollower_UDT(t *testing.T) {
	t.Parallel()
	f, err := trunk.NewFollower(nil)
	if err != nil {
		t.Fatal(err)
	}
	out := make([]trunk.Output, 0, trunk.MaxOutputs)
	a := aloha(t)
	out = f.Burst(0, &a, layer2.Hint{Slot: enums.Slot1}, nil, out[:0])
	wantKinds(t, "C_ALOHA", out, trunk.OutputSite)
	now := time.Duration(0)
	for _, damage := range []bool{false, true} {
		bursts := udtBursts(t, msAddr, "hello trunked world", damage)
		for i := range bursts {
			now += 2 * slot
			out = f.Burst(now, &bursts[i], layer2.Hint{Slot: enums.Slot1}, nil, out[:0])
			if i < len(bursts)-1 {
				wantKinds(t, "UDT block", out)
			}
		}
		wantKinds(t, "UDT", out, trunk.OutputUDT)
		u := f.UDT()
		if u.CRCOK == damage || u.Header.AppendedBlocks != 1 || u.Header.LLIDDestination != msAddr || u.Header.LLIDSource != 3120002 {
			t.Errorf("damage %v: UDT %+v", damage, u)
		}
		if !damage && string(u.Data[:19]) != "hello trunked world" {
			t.Errorf("UDT data %q", u.Data[:22])
		}
	}
}

func TestFollower_ClearToAnotherTSCC(t *testing.T) {
	t.Parallel()
	f, err := trunk.NewFollower(&trunk.FollowerConfig{Hunt: [trunk.MaxChannels]uint16{1}, HuntLen: 1})
	if err != nil {
		t.Fatal(err)
	}
	out := make([]trunk.Output, 0, trunk.MaxOutputs)
	out = f.Tick(0, out[:0])
	a := aloha(t)
	out = f.Burst(slot, &a, layer2.Hint{}, nil, out[:0])
	g := tvGrant(t, 9, 5, false)
	out = f.Burst(2*slot, &g, layer2.Hint{}, nil, out[:0])
	wantKinds(t, "TV_GRANT", out, trunk.OutputGrant, trunk.OutputTune)
	other := bsBurst(t, withPDU(t, &pdu.CSBK{LastBlock: true}, (*pdu.CSBK).SetClear, pdu.ClearPDU{
		PhysicalChannel: 33, Group: true, TargetAddress: 10, SourceAddress: 1}), testCC)
	out = f.Burst(3*slot, &other, layer2.Hint{}, nil, out[:0])
	wantKinds(t, "P_CLEAR for another call", out)
	cl := bsBurst(t, withPDU(t, &pdu.CSBK{LastBlock: true}, (*pdu.CSBK).SetClear, pdu.ClearPDU{
		PhysicalChannel: 33, Group: true, TargetAddress: 9, SourceAddress: 1}), testCC)
	out = f.Burst(4*slot, &cl, layer2.Hint{}, nil, out[:0])
	wantKinds(t, "P_CLEAR to channel 33", out, trunk.OutputCallEnd, trunk.OutputTune)
	if out[1].Tune.Channel != 33 || f.State() != trunk.StateHunting {
		t.Errorf("Tune %+v, state %v", out[1].Tune, f.State())
	}
	out = f.Burst(5*slot, &a, layer2.Hint{}, nil, out[:0])
	wantKinds(t, "C_ALOHA on channel 33", out, trunk.OutputSite)
	if s := f.Site(); s.Channel != 33 {
		t.Errorf("Site.Channel %d", s.Channel)
	}
}
