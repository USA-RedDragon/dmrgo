package zeroalloc_test

import (
	"encoding/hex"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/layer3"
	"github.com/USA-RedDragon/dmrgo/v2/vocoder"
)

// ASELSAN Tier III data bursts (CC 2) with an EMB in place of the data SYNC,
// from the captures that layer2/embedded_data_burst_test.go describes.
const (
	aselsanIdleEMB   = "0000000000000000000000000a42100000000b7f80000000000000000000000000"
	aselsanRate12EMB = "0812857c7f7eb03c72b08a6109f2100000000b763ffc85d09db075219c0379a612"
)

// must returns v and panics on err, for setup calls that must succeed.
func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}

func hexBurst(t testing.TB, s string) layer2.RawBurst {
	t.Helper()
	b, err := hex.DecodeString(s)
	if err != nil || len(b) != len(layer2.RawBurst{}) {
		t.Fatalf("bad burst hex %q: %v", s, err)
	}
	return layer2.RawBurst(b)
}

// decodeCase is a burst and the allocations of a decode of it into a reused
// Burst.
type decodeCase struct {
	name   string
	burst  layer2.RawBurst
	allocs float64
}

// decodeCases returns one burst of each data type, ASELSAN data bursts with
// an EMB in place of the SYNC, and voice bursts A and B. A CSBK, Full LC,
// data header or MBC header payload struct that a dispatch field points to
// is allocated on each decode.
func decodeCases(t testing.TB) []decodeCase {
	t.Helper()
	header := must(pdu.NewVoiceLC(true, 91, 3191868, layer3.ServiceOptions{}))
	term := header
	term.BurstDataType = enums.DataTypeTerminatorWithLC
	csbk := must(pdu.NewBSOutboundActivation(310123, 3191868))
	dataHeader := pdu.DataHeader{
		Format:                pdu.FormatUnconfirmed,
		UnconfirmedDataHeader: &pdu.UnconfirmedDataHeader{Group: true, SAP: 4, FullMessageFlag: true, BlocksToFollow: 3},
	}
	preamble := pdu.CSBK{Opcode: pdu.CSBKPreamble, PreamblePDU: &pdu.PreamblePDU{Group: true, CSBKBlocksToFollow: 2}}
	mbc := pdu.MBCHeader{Opcode: pdu.CSBKTalkgroupVoiceGrant, TalkgroupVoiceGrantPDU: &pdu.TalkgroupVoiceGrantPDU{PhysicalChannel: 0xFFF}}
	data := []struct {
		name   string
		d      layer2.Data
		allocs float64
	}{
		{"CSBK BS_Dwn_Act", &csbk, 0},
		{"CSBK preamble", &preamble, 1},
		{"data header", &dataHeader, 1},
		{"rate 1/2", &pdu.Rate12Data{Data: [12]byte{1, 2, 3}}, 0},
		{"rate 3/4", &pdu.Rate34Data{Data: [18]byte{4, 5, 6}}, 0},
		{"rate 1", &pdu.Rate1Data{Data: [24]byte{7, 8, 9}}, 0},
		{"voice LC header", &header, 1},
		{"terminator with LC", &term, 1},
		{"PI header", &pdu.PIHeader{AlgorithmID: 1, MFID: 0x10, KeyID: 7, MessageIndicator: 0xDEADBEEF, Destination: 91}, 0},
		{"idle", &pdu.PRFill{}, 0},
		{"MBC header", &mbc, 1},
		{"MBC continuation", &pdu.MBCContinuation{LastBlock: true}, 0},
		{"USBD", &pdu.UnifiedSingleBlockData{ServiceType: 1}, 0},
	}
	out := make([]decodeCase, 0, len(data)+4)
	for _, c := range data {
		out = append(out, decodeCase{c.name, must(layer2.BuildBurst(c.d, 1, layer2.SyncSourceBS)), c.allocs})
	}
	var frames [layer2.VoiceSlotsPerBurst]vocoder.AMBEFrame
	emb := pdu.EmbeddedSignalling{ColorCode: 1}
	return append(out,
		decodeCase{"rate 1/2 with EMB", hexBurst(t, aselsanRate12EMB), 0},
		decodeCase{"idle with EMB", hexBurst(t, aselsanIdleEMB), 0},
		decodeCase{"voice A", must(layer2.BuildVoiceBurst(frames, enums.VoiceBurstA, emb, [4]byte{}, layer2.SyncSourceBS)), 0},
		decodeCase{"voice B", must(layer2.BuildVoiceBurst(frames, enums.VoiceBurstB, emb, [4]byte{1, 2, 3, 4}, layer2.SyncSourceBS)), 0},
	)
}

// layer2Entries returns the entries of package layer2.
func layer2Entries(t testing.TB) []entry {
	t.Helper()
	cases := decodeCases(t)
	out := make([]entry, 0, len(cases))
	for _, c := range cases {
		var b layer2.Burst
		if err := b.DecodeFromBytes(c.burst); err != nil || b.FEC.Aggregate().HasErrors() {
			t.Fatalf("%s: %v %s", c.name, err, b.String())
		}
		out = append(out, entry{"layer2.Burst.DecodeFromBytes", c.name, c.allocs, func() { _ = b.DecodeFromBytes(c.burst) }})
	}
	return out
}
