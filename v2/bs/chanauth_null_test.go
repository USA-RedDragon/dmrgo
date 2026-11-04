package bs_test

import (
	"testing"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/bs"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

func addressedCSBK(t *testing.T, target, source uint32) layer2.RawBurst {
	t.Helper()
	c := pdu.CSBK{LastBlock: true}
	if err := c.SetPreamble(pdu.PreamblePDU{Group: true, CSBKBlocksToFollow: 3, TargetAddress: target, SourceAddress: source}); err != nil {
		t.Fatal(err)
	}
	b, err := layer2.BuildBurst(&c, 1, layer2.SyncSourceMS)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestChanAuthNullAddressNoGrant(t *testing.T) {
	t.Parallel()

	rf := bs.Source{RF: true}
	for _, p := range []struct {
		name           string
		target, source uint32
		want           bs.SlotState
	}{
		{"both", 0, 0, bs.SlotIdle},
		{"target", 0, 1005, bs.SlotIdle},
		{"source", 9, 0, bs.SlotIdle},
		{"valid", 9, 1005, bs.SlotChanAuth},
	} {
		for _, csbk := range []bool{false, true} {
			d := keyed(t, bs.Config{ChannelAuth: bs.ChannelAuthSingleSite, CallHang: -1})
			d.until(time.Second)
			raw := header(t, p.target, p.source)
			if csbk {
				raw = addressedCSBK(t, p.target, p.source)
			}
			out := d.in(time.Second, enums.Slot1, rf, raw)
			st := d.r.SlotState(enums.Slot1)
			if p.want == bs.SlotIdle && st == bs.SlotChanAuth || p.want == bs.SlotChanAuth && st != bs.SlotChanAuth {
				t.Errorf("%s csbk=%v: state %v", p.name, csbk, st)
			}
			if p.want == bs.SlotIdle && len(filter(out, bs.OutputBurst)) != 0 {
				t.Errorf("%s csbk=%v: echoed %+v", p.name, csbk, filter(out, bs.OutputBurst))
			}
		}
	}
}
