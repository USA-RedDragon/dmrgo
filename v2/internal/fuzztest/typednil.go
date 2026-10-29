package fuzztest

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

// TypedNilBursts returns, for each data PDU type and both sync sources, a
// decoded burst in good and the same burst with its Data set to a typed nil
// pointer of that type in nils, for tests that feed a typed nil to a
// receiver. It stops t on an error.
func TypedNilBursts(t testing.TB) (good, nils []*layer2.Burst) {
	t.Helper()
	pdus := []struct{ d, n layer2.Data }{
		{&pdu.CSBK{}, (*pdu.CSBK)(nil)},
		{&pdu.DataHeader{}, (*pdu.DataHeader)(nil)},
		{&pdu.Rate12Data{}, (*pdu.Rate12Data)(nil)},
		{&pdu.Rate34Data{}, (*pdu.Rate34Data)(nil)},
		{&pdu.Rate1Data{}, (*pdu.Rate1Data)(nil)},
		{&pdu.FullLinkControl{BurstDataType: enums.DataTypeVoiceLCHeader}, (*pdu.FullLinkControl)(nil)},
		{&pdu.FullLinkControl{BurstDataType: enums.DataTypeTerminatorWithLC}, (*pdu.FullLinkControl)(nil)},
		{&pdu.MBCHeader{}, (*pdu.MBCHeader)(nil)},
		{&pdu.MBCContinuation{}, (*pdu.MBCContinuation)(nil)},
		{&pdu.UnifiedSingleBlockData{}, (*pdu.UnifiedSingleBlockData)(nil)},
		{&pdu.PIHeader{}, (*pdu.PIHeader)(nil)},
	}
	for _, src := range []layer2.SyncSource{layer2.SyncSourceBS, layer2.SyncSourceMS} {
		for _, p := range pdus {
			raw, err := layer2.BuildBurst(p.d, 1, src)
			if err != nil {
				t.Fatal(err)
			}
			b, n := new(layer2.Burst), new(layer2.Burst)
			if err := b.DecodeFromBytes(raw); err != nil {
				t.Fatal(err)
			}
			if err := n.DecodeFromBytes(raw); err != nil {
				t.Fatal(err)
			}
			n.Data = p.n
			good, nils = append(good, b), append(nils, n)
		}
	}
	return good, nils
}
