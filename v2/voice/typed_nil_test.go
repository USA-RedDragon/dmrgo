package voice_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/voice"
)

func TestTypedNilData(t *testing.T) {
	t.Parallel()

	good, nils := typedNilBursts(t)
	for _, r := range good {
		for _, n := range nils {
			rx, err := voice.NewReceiver(nil)
			if err != nil {
				t.Fatal(err)
			}
			tx, err := voice.NewTransmitter(&voice.TransmitterConfig{Slot: enums.Slot1})
			if err != nil {
				t.Fatal(err)
			}
			in, err := voice.NewInterrupter(&voice.InterrupterConfig{Slot: enums.Slot1})
			if err != nil {
				t.Fatal(err)
			}
			for _, b := range []*layer2.Burst{r, n} {
				rx.Decoded(0, b, layer2.Hint{}, nil)
				tx.Decoded(0, b, layer2.Hint{}, nil)
				in.Decoded(0, b, layer2.Hint{}, nil)
			}
		}
	}
}

func typedNilBursts(t *testing.T) (good, nils []*layer2.Burst) {
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
