package data_test

import (
	"testing"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/data"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

var nullParties = []struct {
	name           string
	target, source uint32
}{
	{"both", 0, 0},
	{"target", 0, srcID},
	{"source", dstID, 0},
}

func nullPreamble(t *testing.T, target, source uint32) layer2.RawBurst {
	t.Helper()
	c := pdu.CSBK{LastBlock: true}
	if err := c.SetPreamble(pdu.PreamblePDU{Data: true, CSBKBlocksToFollow: 2, TargetAddress: target, SourceAddress: source}); err != nil {
		t.Fatal(err)
	}
	raw, err := layer2.BuildBurst(&c, 1, layer2.SyncSourceMS)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func nullPacket(t *testing.T, target, source uint32) []layer2.RawBurst {
	t.Helper()
	h := setHeader(t, (*pdu.DataHeader).SetUnconfirmed, pdu.UnconfirmedDataHeader{
		SAP: enums.SAPIPBasedPacketData, LLIDDestination: target, LLIDSource: source, FullMessageFlag: true})
	return build(t, h, payload(10), enums.DataTypeRate12)
}

func TestReceiverNullAddressNoCall(t *testing.T) {
	t.Parallel()

	u := setHeader(t, (*pdu.DataHeader).SetUnconfirmed, pdu.UnconfirmedDataHeader{
		SAP: enums.SAPIPBasedPacketData, LLIDDestination: dstID, LLIDSource: srcID, FullMessageFlag: true})
	good := build(t, u, payload(40), enums.DataTypeRate1)
	for _, p := range nullParties {
		for _, inCall := range []bool{false, true} {
			for _, pre := range []bool{false, true} {
				var in []layer2.RawBurst
				if inCall {
					in = append(in, good...)
				}
				if pre {
					in = append(in, nullPreamble(t, p.target, p.source))
				} else {
					in = append(in, nullPacket(t, p.target, p.source)...)
				}
				r := newReceiver(t, nil)
				ev := make([]data.Event, 0, data.MaxEvents)
				var all []data.Event
				for i := range in {
					all = append(all, r.Burst(time.Duration(i)*60*time.Millisecond, &in[i], layer2.Hint{}, ev[:0])...)
				}
				for _, e := range all {
					if e.Call.Target == 0 || e.Call.Source == 0 || e.Kind == data.EventCallEnd {
						t.Errorf("%s inCall=%v preamble=%v: %s", p.name, inCall, pre, kinds(all))
						break
					}
				}
				if want := inCall; (r.State() != data.StateIdle) != want {
					t.Errorf("%s inCall=%v preamble=%v: state %v", p.name, inCall, pre, r.State())
				}
			}
		}
	}
}
