package bs_test

import (
	"testing"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/bs"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/packet"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/voice"
)

// ETSI TS 102 361-2 clause 5.1.1.6.0
func TestChanAuthDataResponseRepeated(t *testing.T) {
	t.Parallel()

	rf := bs.Source{RF: true}
	d := keyed(t, bs.Config{ChannelAuth: bs.ChannelAuthSingleSite, CallHang: -1, DataHang: 500 * time.Millisecond})
	d.until(time.Second)
	d.in(time.Second, enums.Slot1, rf, preCSBK(t, 3120000))
	b := packet.NewBuilder(16)
	var h pdu.DataHeader
	if err := h.SetConfirmed(pdu.ConfirmedDataHeader{
		ResponseRequested: true, SAP: 4, LLIDDestination: 3191868, LLIDSource: 3120000, FullMessageFlag: true, SendSequenceNumber: 5}); err != nil {
		t.Fatal(err)
	}
	p, err := b.Segment(h, make([]byte, 30), enums.DataTypeRate12)
	if err != nil {
		t.Fatal(err)
	}
	bursts := make([]layer2.RawBurst, 0, 1+len(p.Blocks))
	bursts = append(bursts, must(layer2.BuildBurst(&p.Header, 1, layer2.SyncSourceMS)))
	for _, blk := range p.Blocks {
		bursts = append(bursts, must(layer2.BuildBurst(blk, 1, layer2.SyncSourceMS)))
	}
	in := timed(bursts, enums.Slot1, rf, 1060*time.Millisecond)
	d.feed(in)
	end := in[len(in)-1].at
	d.until(end + 60*time.Millisecond)
	var rh pdu.DataHeader
	if err := rh.SetResponse(pdu.ResponsePacketHeader{SAP: 4, LLIDDestination: 3120000, LLIDSource: 3191868, ResponseType: 1}); err != nil {
		t.Fatal(err)
	}
	mark := len(d.all)
	d.in(end+120*time.Millisecond, enums.Slot1, rf, must(layer2.BuildBurst(&rh, 1, layer2.SyncSourceMS)))
	d.until(end + 400*time.Millisecond)
	for _, o := range d.all[mark:] {
		if o.Kind == bs.OutputBurst && o.Slot == enums.Slot1 && o.To&bs.ToRF != 0 && o.Burst.SlotType().DataType == enums.DataTypeDataHeader {
			return
		}
	}
	t.Error("the response packet of the target MS in the data hang time was not repeated")
}

// ETSI TS 102 361-2 clause 5.1.1.6.1.1
func TestChanAuthLostThenOtherCall(t *testing.T) {
	t.Parallel()

	rf := bs.Source{RF: true}
	d := keyed(t, bs.Config{ChannelAuth: bs.ChannelAuthSingleSite, CallHang: -1})
	d.until(time.Second)
	d.in(time.Second, enums.Slot1, rf, header(t, 9, 3120005))
	a := call(t, 1, 9, 3120005, 1, enums.Slot1, rf, 1060*time.Millisecond)
	d.feed(a[:len(a)-1])
	start := a[len(a)-2].at + 200*time.Millisecond
	mark := len(d.all)
	d.feed(call(t, 1, 9, 3120007, 2, enums.Slot1, rf, start))
	d.until(start + 3*time.Second)
	for _, o := range d.all[mark:] {
		if o.Kind == bs.OutputCallEnd && o.Call.Source == 3120007 {
			if o.End != voice.EndTerminator {
				t.Errorf("the call of 3120007 ended %v", o.End)
			}
			return
		}
	}
	t.Error("the call of 3120007 did not end")
}

// ETSI TS 102 361-2 clause 5.1.1.6.1.1
func TestChanAuthAuthorizedCallKeepsTheSlot(t *testing.T) {
	t.Parallel()

	rf := bs.Source{RF: true}
	for _, hang := range []time.Duration{0, -1} {
		d := keyed(t, bs.Config{ChannelAuth: bs.ChannelAuthSingleSite, CallHang: hang})
		d.until(time.Second)
		d.in(time.Second, enums.Slot1, rf, header(t, 9, 3120005))
		mark := len(d.all)
		d.in(1060*time.Millisecond, enums.Slot1, rf, header(t, 9, 3120005))
		if st := d.r.SlotState(enums.Slot1); st != bs.SlotVoice {
			t.Fatalf("hang %v: after the authorized MS: %v", hang, st)
		}
		d.in(1120*time.Millisecond, enums.Slot1, rf, header(t, 9, 3120007))
		d.until(1500 * time.Millisecond)
		for _, o := range d.all[mark:] {
			if o.Kind == bs.OutputCallStart && o.Call.Source != 3120005 || o.Kind == bs.OutputCallEnd {
				t.Errorf("hang %v: %v %+v after the authorization of 3120005", hang, o.Kind, o.Call)
			}
		}
	}
}
