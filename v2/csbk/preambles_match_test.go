package csbk_test

import (
	"slices"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/csbk"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

func TestPreamblesJoinOnlySameParties(t *testing.T) {
	t.Parallel()

	build := func(d layer2.Data) layer2.RawBurst {
		t.Helper()
		b, err := layer2.BuildBurst(d, 1, layer2.SyncSourceMS)
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	pre := func(data bool, target, source uint32) layer2.RawBurst {
		c := pdu.CSBK{LastBlock: true}
		if err := c.SetPreamble(pdu.PreamblePDU{Data: data, CSBKBlocksToFollow: 1, TargetAddress: target, SourceAddress: source}); err != nil {
			t.Fatal(err)
		}
		return build(&c)
	}
	ahoy := func(target, source uint32) layer2.RawBurst {
		c := pdu.CSBK{LastBlock: true}
		if err := c.SetAhoy(pdu.AhoyPDU{TargetAddress: target, SourceAddress: source}); err != nil {
			t.Fatal(err)
		}
		return build(&c)
	}
	header := func(target, source uint32) layer2.RawBurst {
		var h pdu.DataHeader
		if err := h.SetUnconfirmed(pdu.UnconfirmedDataHeader{LLIDDestination: target, LLIDSource: source, BlocksToFollow: 1}); err != nil {
			t.Fatal(err)
		}
		return build(&h)
	}
	mbc := func(target, source uint32) layer2.RawBurst {
		var h pdu.MBCHeader
		if err := h.SetPrivateVoiceGrant(pdu.PrivateVoiceGrantPDU{TargetAddress: target, SourceAddress: source}); err != nil {
			t.Fatal(err)
		}
		return build(&h)
	}
	for _, tt := range []struct {
		name   string
		data   bool
		follow layer2.RawBurst
		joined bool
	}{
		{"CSBK same parties", false, ahoy(2, 1), true},
		{"CSBK other target", false, ahoy(3, 1), false},
		{"CSBK other source", false, ahoy(2, 4), false},
		{"data header same parties", true, header(2, 1), true},
		{"data header other target", true, header(3, 1), false},
		{"data header other source", true, header(2, 4), false},
		{"MBC header same parties", false, mbc(2, 1), true},
		{"MBC header other target", false, mbc(3, 1), false},
	} {
		p := mustPreambles(t, nil)
		first := pre(tt.data, 2, 1)
		out := p.Burst(0, &first, layer2.Hint{}, nil)
		out = append(out, p.Burst(60*ms, &tt.follow, layer2.Hint{}, nil)...)
		want := []csbk.OutputKind{csbk.OutputAnnounced, csbk.OutputExpired}
		if tt.joined {
			want[1] = csbk.OutputJoined
		}
		if !slices.Equal(kinds(out), want) {
			t.Errorf("%s: outputs %v, want %v", tt.name, kinds(out), want)
			continue
		}
		if out[1].Call != (layer2.Call{Target: 2, Source: 1}) || p.State() != csbk.PreamblesIdle {
			t.Errorf("%s: %+v, state %v", tt.name, out[1], p.State())
		}
		if out = run(t, p, 3600*1000*ms); len(out) != 0 {
			t.Errorf("%s: after the burst %v", tt.name, kinds(out))
		}
	}
}
