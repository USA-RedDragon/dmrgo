package zeroalloc_test

import (
	"encoding/hex"
	"os"
	"slices"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/fec"
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

// capture returns the bursts of the capture file name in layer2/testdata.
func capture(t testing.TB, name string) []layer2.RawBurst {
	t.Helper()
	data, err := os.ReadFile("../../layer2/testdata/" + name + ".bin")
	if err != nil {
		t.Fatal(err)
	}
	n := len(layer2.RawBurst{})
	if len(data)%n != 0 {
		t.Fatalf("%s: size %d is not a multiple of %d", name, len(data), n)
	}
	out := make([]layer2.RawBurst, len(data)/n)
	for i := range out {
		out[i] = layer2.RawBurst(data[i*n : (i+1)*n])
	}
	return out
}

// decodeAll decodes each burst into its own Burst and fails on any error.
func decodeAll(t testing.TB, raw []layer2.RawBurst) []layer2.Burst {
	t.Helper()
	out := make([]layer2.Burst, len(raw))
	for i, r := range raw {
		if err := out[i].DecodeFromBytes(r); err != nil {
			t.Fatalf("burst %d: %v", i, err)
		}
	}
	return out
}

// firstBurst returns the first burst of raw whose decode matches keep.
func firstBurst(t testing.TB, raw []layer2.RawBurst, keep func(*layer2.Burst) bool) layer2.RawBurst {
	t.Helper()
	for _, r := range raw {
		var b layer2.Burst
		if err := b.DecodeFromBytes(r); err == nil && keep(&b) {
			return r
		}
	}
	t.Fatal("no matching burst")
	return layer2.RawBurst{}
}

func isDataType(dt enums.DataType) func(*layer2.Burst) bool {
	return func(b *layer2.Burst) bool { return b.Kind == layer2.KindData && b.SlotType.DataType == dt }
}

func isVoiceLCSS(lcss enums.LCSS) func(*layer2.Burst) bool {
	return func(b *layer2.Burst) bool {
		return b.Kind == layer2.KindVoice && b.HasEmbeddedSignalling() && b.EmbeddedSignalling.LCSS == lcss
	}
}

// decodeCase is a burst, the allocations of a decode of it into a reused
// Burst, and the allocations of an Encode of that Burst.
type decodeCase struct {
	name   string
	burst  layer2.RawBurst
	allocs float64
	encode float64
}

// decodeCases returns one burst of each data type, ASELSAN data bursts with
// an EMB in place of the SYNC, and voice bursts A and B.
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
		name           string
		d              layer2.Data
		allocs, encode float64
	}{
		{"CSBK BS_Dwn_Act", &csbk, 0, 0},
		{"CSBK preamble", &preamble, 0, 0},
		{"data header", &dataHeader, 0, 0},
		{"rate 1/2", &pdu.Rate12Data{Data: [12]byte{1, 2, 3}}, 0, 0},
		{"rate 3/4", &pdu.Rate34Data{Data: [18]byte{4, 5, 6}}, 0, 0},
		{"rate 1", &pdu.Rate1Data{Data: [24]byte{7, 8, 9}}, 0, 0},
		{"voice LC header", &header, 0, 0},
		{"terminator with LC", &term, 0, 0},
		{"PI header", &pdu.PIHeader{AlgorithmID: 1, MFID: 0x10, KeyID: 7, MessageIndicator: 0xDEADBEEF, Destination: 91}, 0, 0},
		{"idle", &pdu.PRFill{}, 0, 0},
		{"MBC header", &mbc, 0, 0},
		{"MBC continuation", &pdu.MBCContinuation{LastBlock: true}, 0, 0},
		{"USBD", &pdu.UnifiedSingleBlockData{ServiceType: 1}, 0, 0},
	}
	out := make([]decodeCase, 0, len(data)+4)
	for _, c := range data {
		out = append(out, decodeCase{c.name, must(layer2.BuildBurst(c.d, 1, layer2.SyncSourceBS)), c.allocs, c.encode})
	}
	var frames [layer2.VoiceSlotsPerBurst]vocoder.AMBEFrame
	emb := pdu.EmbeddedSignalling{ColorCode: 1}
	return append(out,
		decodeCase{"rate 1/2 with EMB", hexBurst(t, aselsanRate12EMB), 0, 0},
		decodeCase{"idle with EMB", hexBurst(t, aselsanIdleEMB), 0, 0},
		decodeCase{"voice A", must(layer2.BuildVoiceBurst(frames, enums.VoiceBurstA, emb, [4]byte{}, layer2.SyncSourceBS)), 0, 0},
		decodeCase{"voice B", must(layer2.BuildVoiceBurst(frames, enums.VoiceBurstB, emb, [4]byte{1, 2, 3, 4}, layer2.SyncSourceBS)), 0, 0},
	)
}

// layer2Entries returns the entries of package layer2, on captured bursts
// where the captures have one. An allocation that is not 0 comes from a PDU
// decode that allocates a dispatch payload, from the Encode of a Rate 1/2,
// Rate 3/4 or Rate 1 block, from the RS(12,9) encode of a Full LC, from a
// constructor, from a form that returns a new slice by contract, or from
// errors.Join of two decode errors.
func layer2Entries(t testing.TB) []entry {
	t.Helper()
	return slices.Concat(
		burstDecodeEntries(t),
		burstEncodeEntries(t),
		rawBurstEntries(t),
		signallingEntries(t),
		voiceEntries(t),
	)
}

func burstDecodeEntries(t testing.TB) []entry {
	t.Helper()
	var out []entry
	for _, c := range decodeCases(t) {
		var b layer2.Burst
		if err := b.DecodeFromBytes(c.burst); err != nil || b.FEC.Aggregate().HasErrors() {
			t.Fatalf("%s: %v %s", c.name, err, b.String())
		}
		out = append(out, entry{"layer2.Burst.DecodeFromBytes", c.name, c.allocs, func() { _ = b.DecodeFromBytes(c.burst) }})
	}
	// A decode of a capture allocates the dispatch payloads of its CSBKs,
	// data header and Full LCs.
	for _, c := range []struct {
		name   string
		allocs float64
	}{{"d-sms", 0}, {"h-sms", 0}, {"m-sms", 0}, {"parrot_kerchunk", 0}, {"voice", 0}} {
		raw := capture(t, c.name)
		var b layer2.Burst
		decodeAll(t, raw)
		out = append(out, entry{"layer2.Burst.DecodeFromBytes", c.name + " capture", c.allocs, func() {
			for _, r := range raw {
				_ = b.DecodeFromBytes(r)
			}
		}})
	}

	sms := capture(t, "m-sms")
	parrot := capture(t, "parrot_kerchunk")
	rate12 := firstBurst(t, sms, isDataType(enums.DataTypeRate12))
	voiceB := firstBurst(t, parrot, isVoiceLCSS(enums.LCSSFirstFragment))
	reserved := rate12
	reserved.SetSlotType(pdu.SlotType{ColorCode: 1, DataType: enums.DataTypeReserved + 1})
	cc1, cc2 := enums.ColorCode(1), enums.ColorCode(2)
	rate12Type := enums.DataTypeRate12
	options := []struct {
		name   string
		burst  layer2.RawBurst
		opts   *layer2.DecodeOptions
		allocs float64
	}{
		{"nil options", rate12, nil, 0},
		{"color code match", rate12, &layer2.DecodeOptions{ColorCode: &cc1}, 0},
		{"color code mismatch", rate12, &layer2.DecodeOptions{ColorCode: &cc2}, 0},
		{"exact SYNC", rate12, &layer2.DecodeOptions{SyncThreshold: -1}, 0},
		{"kind data with data type", rate12, &layer2.DecodeOptions{Kind: layer2.KindData, DataType: &rate12Type}, 0},
		{"kind voice B", voiceB, &layer2.DecodeOptions{Kind: layer2.KindVoice, VoiceBurst: enums.VoiceBurstB}, 0},
		{"kind voice unknown", voiceB, &layer2.DecodeOptions{Kind: layer2.KindVoice}, 0},
		{"kind reverse channel", voiceB, &layer2.DecodeOptions{Kind: layer2.KindReverseChannel}, 0},
		{"reserved data type", reserved, nil, 0},
		{"reserved data type and color code mismatch", reserved, &layer2.DecodeOptions{ColorCode: &cc2}, 2},
	}
	for _, o := range options {
		var b layer2.Burst
		_ = b.DecodeFromBytesWith(o.burst, o.opts)
		out = append(out, entry{"layer2.Burst.DecodeFromBytesWith", o.name, o.allocs, func() { _ = b.DecodeFromBytesWith(o.burst, o.opts) }})
	}

	var nb *layer2.Burst
	out = append(out, entry{"layer2.NewBurstFromBytes", "rate 1/2", 2, func() { nb, _ = layer2.NewBurstFromBytes(rate12) }})
	var has bool
	var vb layer2.Burst
	_ = vb.DecodeFromBytes(voiceB)
	return append(out,
		entry{"layer2.Burst.HasEmbeddedSignalling", "voice B", 0, func() { has = vb.HasEmbeddedSignalling() }},
		entry{"layer2.Burst.HasReverseChannel", "voice B", 0, func() { has = vb.HasReverseChannel() || has || nb == nil }},
	)
}

func burstEncodeEntries(t testing.TB) []entry {
	t.Helper()
	var out []entry
	var raw layer2.RawBurst
	var err error
	for _, c := range decodeCases(t) {
		var b layer2.Burst
		_ = b.DecodeFromBytes(c.burst)
		if got := must(b.Encode()); got != c.burst {
			t.Fatalf("%s: encode %x, want %x", c.name, got, c.burst)
		}
		out = append(out, entry{"layer2.Burst.Encode", "decoded " + c.name, c.encode, func() { raw, err = b.Encode() }})
	}
	// An Encode of a capture allocates in the Encode of its Rate 1/2 blocks
	// and Full LCs.
	for _, c := range []struct {
		name   string
		allocs float64
	}{{"d-sms", 0}, {"h-sms", 0}, {"m-sms", 0}, {"parrot_kerchunk", 0}, {"voice", 0}} {
		bursts := decodeAll(t, capture(t, c.name))
		out = append(out, entry{"layer2.Burst.Encode", c.name + " capture", c.allocs, func() {
			for i := range bursts {
				raw, err = bursts[i].Encode()
			}
		}})
	}

	// An edited Data makes Encode decode the payload again to compare, and
	// that decode allocates the dispatch payload of a CSBK preamble.
	sms := decodeAll(t, capture(t, "m-sms"))
	for _, c := range []struct {
		name   string
		b      layer2.Burst
		edit   func(*layer2.Burst)
		allocs float64
	}{
		{"edited CSBK preamble capture", sms[0], func(b *layer2.Burst) {
			if c, ok := b.Data.(*pdu.CSBK); ok && c.PreamblePDU != nil {
				c.PreamblePDU.CSBKBlocksToFollow++
			}
		}, 1},
		{"edited data header capture", sms[5], func(b *layer2.Burst) {
			if h, ok := b.Data.(*pdu.DataHeader); ok && h.UnconfirmedDataHeader != nil {
				h.UnconfirmedDataHeader.BlocksToFollow++
			}
		}, 1},
	} {
		b := c.b
		before := must(b.Encode())
		c.edit(&b)
		if must(b.Encode()) == before {
			t.Fatalf("%s: the edit does not change the burst", c.name)
		}
		out = append(out, entry{"layer2.Burst.Encode", c.name, c.allocs, func() { raw, err = b.Encode() }})
	}

	header := must(pdu.NewVoiceLC(true, 91, 3191868, layer3.ServiceOptions{}))
	csbk := must(pdu.NewBSOutboundActivation(310123, 3191868))
	set := []struct {
		name   string
		b      layer2.Burst
		allocs float64
	}{
		{"set rate 1/2", layer2.Burst{
			Kind: layer2.KindData, SyncPattern: enums.SyncBSSourcedData,
			SlotType: pdu.SlotType{ColorCode: 1, DataType: enums.DataTypeRate12},
			Data:     &pdu.Rate12Data{Data: [12]byte{1, 2, 3}},
		}, 0},
		{"set CSBK BS_Dwn_Act", layer2.Burst{
			Kind: layer2.KindData, SyncPattern: enums.SyncBSSourcedData,
			SlotType: pdu.SlotType{ColorCode: 1, DataType: enums.DataTypeCSBK}, Data: &csbk,
		}, 0},
		{"set voice LC header", layer2.Burst{
			Kind: layer2.KindData, SyncPattern: enums.SyncBSSourcedData,
			SlotType: pdu.SlotType{ColorCode: 1, DataType: enums.DataTypeVoiceLCHeader}, Data: &header,
		}, 0},
		{"set idle", layer2.Burst{
			Kind: layer2.KindData, SyncPattern: enums.SyncBSSourcedData,
			SlotType: pdu.SlotType{ColorCode: 1, DataType: enums.DataTypeIdle}, Data: &pdu.PRFill{},
		}, 0},
		{"set voice B", layer2.Burst{
			Kind: layer2.KindVoice, EmbeddedSignalling: pdu.EmbeddedSignalling{ColorCode: 1, LCSS: enums.LCSSFirstFragment},
		}, 0},
		{"set voice F with RC", layer2.Burst{
			Kind: layer2.KindVoice, EmbeddedSignalling: pdu.EmbeddedSignalling{
				ColorCode: 1, LCSS: enums.LCSSSingleFragment, PreemptionAndPowerControlIndicator: true,
			},
			ReverseChannel: pdu.ReverseChannel{RCCommand: enums.RCCommand(3)},
		}, 0},
		{"set standalone RC", layer2.Burst{
			Kind: layer2.KindReverseChannel, EmbeddedSignalling: pdu.EmbeddedSignalling{ColorCode: 1},
			ReverseChannel: pdu.ReverseChannel{RCCommand: enums.RCCommand(3)},
		}, 0},
	}
	for _, s := range set {
		b := s.b
		must(b.Encode())
		out = append(out, entry{"layer2.Burst.Encode", s.name, s.allocs, func() { raw, err = b.Encode() }})
	}

	voice := decodeAll(t, capture(t, "voice"))[1]
	frames, _ := voice.AMBEFrames()
	var res fec.Result
	_, _, _ = raw, err, res
	return append(out,
		entry{"layer2.Burst.AMBEFrames", "voice capture", 0, func() { frames, res = voice.AMBEFrames() }},
		entry{"layer2.Burst.SetAMBEFrames", "voice capture", 0, func() { voice.SetAMBEFrames(frames) }},
	)
}

func rawBurstEntries(t testing.TB) []entry {
	t.Helper()
	sms := capture(t, "m-sms")
	parrot := capture(t, "parrot_kerchunk")
	voiceA := firstBurst(t, parrot, func(b *layer2.Burst) bool { return b.Kind == layer2.KindVoice && !b.HasEmbeddedSignalling() })
	voiceB := firstBurst(t, parrot, isVoiceLCSS(enums.LCSSFirstFragment))
	rate12 := firstBurst(t, sms, isDataType(enums.DataTypeRate12))
	header := firstBurst(t, sms, isDataType(enums.DataTypeDataHeader))
	csbkBurst := firstBurst(t, sms, isDataType(enums.DataTypeCSBK))
	lcHeader := firstBurst(t, parrot, isDataType(enums.DataTypeVoiceLCHeader))
	rate34 := must(layer2.BuildRate34DataBurst([18]byte{1, 2, 3}, 1, layer2.SyncSourceBS))
	rate1 := must(layer2.BuildRate1DataBurst([24]byte{4, 5, 6}, 1, layer2.SyncSourceBS))
	rc := must((&layer2.Burst{
		Kind: layer2.KindReverseChannel, EmbeddedSignalling: pdu.EmbeddedSignalling{ColorCode: 1},
		ReverseChannel: pdu.ReverseChannel{RCCommand: enums.RCCommand(3)},
	}).Encode())
	frames, _ := voiceA.AMBEFrames()
	slots := voiceA.VoiceSlots()
	emb := pdu.EmbeddedSignalling{ColorCode: 1, LCSS: enums.LCSSFirstFragment}
	csbk := must(pdu.NewBSOutboundActivation(310123, 3191868))
	preamble := pdu.CSBK{Opcode: pdu.CSBKPreamble, PreamblePDU: &pdu.PreamblePDU{Group: true, CSBKBlocksToFollow: 2}}
	lc := must(pdu.NewVoiceLC(true, 91, 3191868, layer3.ServiceOptions{}))
	payload, _ := header.BPTCPayload()
	csbkPayload, _ := csbkBurst.BPTCPayload()
	lcPayload, _ := lcHeader.BPTCPayload()
	rate12Data := &pdu.Rate12Data{Data: payload}
	rate34Data := &pdu.Rate34Data{Data: [18]byte{1, 2, 3}}
	rate1Data := &pdu.Rate1Data{Data: [24]byte{4, 5, 6}}
	idle := &pdu.PRFill{}
	contPayload := payload

	var (
		r      layer2.RawBurst
		p12    [12]byte
		p18    [18]byte
		p24    [24]byte
		res    fec.Result
		st     pdu.SlotType
		e      pdu.EmbeddedSignalling
		w      [4]byte
		sync   enums.SyncPattern
		dist   int
		vs     [layer2.VoiceSlotsPerBurst][9]byte
		fr     [layer2.VoiceSlotsPerBurst]vocoder.AMBEFrame
		rsv    [3]bit.Bit
		accept bool
		err    error
	)
	p12, _ = header.BPTCPayload()
	p18, _ = rate34.Rate34Payload()
	p24, _ = rate1.Rate1Payload()
	st = rate12.SlotType()
	e = voiceB.EMB()
	w = voiceB.EmbeddedData()
	rsv, _ = header.BPTCReserved()
	_, _, _, _, _, _, _, _, _, _ = sync, dist, vs, fr, accept, res, rsv, err, e, st
	return []entry{
		{"layer2.RawBurst.Sync", "rate 1/2 capture", 0, func() { sync, dist = rate12.Sync() }},
		{"layer2.RawBurst.SetSync", "BS data", 0, func() { err = r.SetSync(enums.SyncBSSourcedData) }},
		{"layer2.RawBurst.SlotType", "rate 1/2 capture", 0, func() { st = rate12.SlotType() }},
		{"layer2.RawBurst.SetSlotType", "rate 1/2", 0, func() { r.SetSlotType(st) }},
		{"layer2.RawBurst.BPTCPayload", "data header capture", 0, func() { p12, res = header.BPTCPayload() }},
		{"layer2.RawBurst.SetBPTCPayload", "data header", 0, func() { r.SetBPTCPayload(p12) }},
		{"layer2.RawBurst.BPTCReserved", "data header capture", 0, func() { rsv, res = header.BPTCReserved() }},
		{"layer2.RawBurst.SetBPTCReserved", "R(2)", 0, func() { r.SetBPTCReserved([3]bit.Bit{0, 0, 1}) }},
		{"layer2.RawBurst.Rate34Payload", "built", 0, func() { p18, res = rate34.Rate34Payload() }},
		{"layer2.RawBurst.SetRate34Payload", "built", 0, func() { r.SetRate34Payload(p18) }},
		{"layer2.RawBurst.Rate1Payload", "built", 0, func() { p24, res = rate1.Rate1Payload() }},
		{"layer2.RawBurst.SetRate1Payload", "built", 0, func() { r.SetRate1Payload(p24) }},
		{"layer2.RawBurst.EMB", "voice B capture", 0, func() { e = voiceB.EMB() }},
		{"layer2.RawBurst.SetEMB", "voice B", 0, func() { r.SetEMB(e) }},
		{"layer2.RawBurst.EmbeddedData", "voice B capture", 0, func() { w = voiceB.EmbeddedData() }},
		{"layer2.RawBurst.SetEmbeddedData", "voice B", 0, func() { r.SetEmbeddedData(w) }},
		{"layer2.RawBurst.StandaloneRCEMB", "built RC", 0, func() { e = rc.StandaloneRCEMB() }},
		{"layer2.RawBurst.SetStandaloneRCEMB", "built RC", 0, func() { r.SetStandaloneRCEMB(e) }},
		{"layer2.RawBurst.StandaloneRCData", "built RC", 0, func() { w = rc.StandaloneRCData() }},
		{"layer2.RawBurst.SetStandaloneRCData", "built RC", 0, func() { r.SetStandaloneRCData(w) }},
		{"layer2.RawBurst.VoiceSlots", "voice A capture", 0, func() { vs = voiceA.VoiceSlots() }},
		{"layer2.RawBurst.SetVoiceSlots", "voice A capture", 0, func() { r.SetVoiceSlots(slots) }},
		{"layer2.RawBurst.AMBEFrames", "voice A capture", 0, func() { fr, res = voiceA.AMBEFrames() }},
		{"layer2.RawBurst.SetAMBEFrames", "voice A capture", 0, func() { r.SetAMBEFrames(frames) }},
		{"layer2.RawBurst.MMDVMHostPayload", "data header capture", 0, func() { p12, accept = header.MMDVMHostPayload(enums.DataTypeDataHeader) }},
		{"layer2.MMDVMHostAccepts", "data header capture", 0, func() { accept = layer2.MMDVMHostAccepts(enums.DataTypeDataHeader, payload) }},
		{"layer2.MMDVMHostAccepts", "CSBK capture", 0, func() { accept = layer2.MMDVMHostAccepts(enums.DataTypeCSBK, csbkPayload) }},
		{"layer2.MMDVMHostAccepts", "voice LC header capture", 0, func() { accept = layer2.MMDVMHostAccepts(enums.DataTypeVoiceLCHeader, lcPayload) }},
		{"layer2.MMDVMHostAccepts", "MBC continuation", 0, func() { accept = layer2.MMDVMHostAccepts(enums.DataTypeMBCContinuation, payload) }},
		{"layer2.MMDVMHostSeal", "data header", 0, func() { err = layer2.MMDVMHostSeal(enums.DataTypeDataHeader, &p12) }},
		{"layer2.MMDVMHostSeal", "voice LC header", 0, func() { err = layer2.MMDVMHostSeal(enums.DataTypeVoiceLCHeader, &lcPayload) }},
		{"layer2.MMDVMHostSeal", "MBC continuation", 0, func() { err = layer2.MMDVMHostSeal(enums.DataTypeMBCContinuation, &contPayload) }},
		{"layer2.MMDVMHostSeal", "rate 1/2", 0, func() { err = layer2.MMDVMHostSeal(enums.DataTypeRate12, &p12) }},
		{"layer2.BuildVoiceBurst", "A", 0, func() { r, err = layer2.BuildVoiceBurst(frames, enums.VoiceBurstA, emb, w, layer2.SyncSourceBS) }},
		{"layer2.BuildVoiceBurst", "B", 0, func() { r, err = layer2.BuildVoiceBurst(frames, enums.VoiceBurstB, emb, w, layer2.SyncSourceMS) }},
		{"layer2.BuildBurst", "CSBK BS_Dwn_Act", 0, func() { r, err = layer2.BuildBurst(&csbk, 1, layer2.SyncSourceBS) }},
		{"layer2.BuildBurst", "CSBK preamble", 0, func() { r, err = layer2.BuildBurst(&preamble, 1, layer2.SyncSourceBS) }},
		{"layer2.BuildBurst", "voice LC header", 0, func() { r, err = layer2.BuildBurst(&lc, 1, layer2.SyncSourceBS) }},
		{"layer2.BuildBurst", "rate 1/2", 0, func() { r, err = layer2.BuildBurst(rate12Data, 1, layer2.SyncSourceBS) }},
		{"layer2.BuildBurst", "rate 3/4", 0, func() { r, err = layer2.BuildBurst(rate34Data, 1, layer2.SyncSourceBS) }},
		{"layer2.BuildBurst", "rate 1", 0, func() { r, err = layer2.BuildBurst(rate1Data, 1, layer2.SyncSourceBS) }},
		{"layer2.BuildBurst", "idle", 0, func() { r, err = layer2.BuildBurst(idle, 1, layer2.SyncSourceBS) }},
		{"layer2.BuildDataBurst", "data header", 0, func() {
			r, err = layer2.BuildDataBurst(payload, enums.DataTypeDataHeader, 1, layer2.SyncSourceBS)
		}},
		{"layer2.BuildRate34DataBurst", "built", 0, func() { r, err = layer2.BuildRate34DataBurst(p18, 1, layer2.SyncSourceBS) }},
		{"layer2.BuildRate1DataBurst", "built", 0, func() { r, err = layer2.BuildRate1DataBurst(p24, 1, layer2.SyncSourceBS) }},
	}
}

func signallingEntries(t testing.TB) []entry {
	t.Helper()
	parrot := decodeAll(t, capture(t, "parrot_kerchunk"))
	var frags [4][4]byte
	var fragBits [4][32]bit.Bit
	for i := range frags {
		fragBits[i] = parrot[2+i].EmbeddedSignallingData
		frags[i] = bit.Pack32(fragBits[i])
	}
	lcss := [4]enums.LCSS{enums.LCSSFirstFragment, enums.LCSSContinuationFragment, enums.LCSSContinuationFragment, enums.LCSSLastFragment}
	lc9, _ := layer2.DecodeEmbeddedLC(frags, false)
	flc := layer2.DecodeEmbeddedFullLinkControl(frags, false)

	slc := pdu.ShortLC{SLCO: enums.SLCOActivityUpdate, ActivityUpdate: &pdu.ShortLCActivityUpdate{HashTS1: 0x5A}}
	rawSLC := pdu.ShortLC{SLCO: enums.SLCO(9), RawPayload: [3]byte{1, 2, 3}}
	tact := pdu.TACT{AccessType: true}
	fields := must(layer2.EncodeShortLCToCACH(&slc, tact))
	rawFields := must(layer2.EncodeShortLCToCACH(&rawSLC, tact))
	var cachs, rawCACHs [4]layer2.CACH
	for i := range fields {
		cachs[i] = layer2.DecodeCACHField(fields[i])
		rawCACHs[i] = layer2.DecodeCACHField(rawFields[i])
	}
	logical := layer2.CACHDeinterleave(fields[0])
	cach := cachs[0]

	rc := pdu.ReverseChannel{RCCommand: enums.RCCommand(3)}
	rcBits := must(layer2.EncodeEmbeddedRC(&rc))

	var (
		a       layer2.EmbeddedLCAssembler
		s       layer2.ShortLCAssembler
		lcOut   [9]byte
		res     fec.Result
		done    bool
		err     error
		flcOut  pdu.FullLinkControl
		fragOut [4][4]byte
		cachOut layer2.CACH
		bits24  [layer2.CACHBits]bit.Bit
		f4      [4][layer2.CACHBits]bit.Bit
		slcOut  pdu.ShortLC
		n       int
		rcOut   pdu.ReverseChannel
		rc32    [32]bit.Bit
		info    [96]bit.Bit
		ok      bool
	)
	_, _, _, _, _, _, _, _, _, _, _, _, _, _ = lcOut, res, done, err, flcOut, fragOut, cachOut, bits24, f4, slcOut, n, rcOut, rc32, ok
	addCACHs := func(cs *[4]layer2.CACH) {
		for _, c := range cs {
			slcOut, done, err = s.Add(c)
		}
	}
	return []entry{
		{"layer2.EncodeEmbeddedLC", "parrot capture", 0, func() { frags = layer2.EncodeEmbeddedLC(lc9) }},
		{"layer2.DecodeEmbeddedLC", "parrot capture", 0, func() { lcOut, res = layer2.DecodeEmbeddedLC(frags, false) }},
		{"layer2.DecodeEmbeddedLC", "parrot capture strict", 0, func() { lcOut, res = layer2.DecodeEmbeddedLC(frags, true) }},
		{"layer2.EmbeddedLCAssembler.Add", "parrot capture B to E", 0, func() {
			for i := range frags {
				lcOut, res, done, err = a.Add(frags[i], lcss[i])
			}
		}},
		{"layer2.EmbeddedLCAssembler.Reset", "strict", 0, func() { a.Strict = true; a.Reset() }},
		{"layer2.EncodeEmbeddedFullLinkControl", "parrot capture", 0, func() { fragOut, err = layer2.EncodeEmbeddedFullLinkControl(&flc) }},
		{"layer2.DecodeEmbeddedFullLinkControl", "parrot capture", 1, func() { flcOut = layer2.DecodeEmbeddedFullLinkControl(frags, false) }},
		{"layer2.DecodeEmbeddedRC", "built", 0, func() { rcOut = layer2.DecodeEmbeddedRC(rcBits) }},
		{"layer2.EncodeEmbeddedRC", "built", 0, func() { rc32, err = layer2.EncodeEmbeddedRC(&rc) }},
		{"layer2.IdleMessageInfo", "table", 0, func() { info = layer2.IdleMessageInfo() }},
		{"layer2.IsIdleMessage", "table", 0, func() { ok = layer2.IsIdleMessage(info) }},
		{"layer2.IsNullEmbeddedLC", "parrot capture", 0, func() { ok = layer2.IsNullEmbeddedLC(fragBits[0]) }},
		{"layer2.CACHDeinterleave", "activity update", 0, func() { logical = layer2.CACHDeinterleave(fields[0]) }},
		{"layer2.CACHInterleave", "activity update", 0, func() { bits24 = layer2.CACHInterleave(logical) }},
		{"layer2.DecodeCACH", "activity update", 0, func() { cachOut = layer2.DecodeCACH(logical) }},
		{"layer2.DecodeCACHField", "activity update", 0, func() { cachOut = layer2.DecodeCACHField(fields[1]) }},
		{"layer2.CACH.Encode", "activity update", 0, func() { bits24, err = cach.Encode() }},
		{"layer2.EncodeShortLCToCACH", "activity update", 0, func() { f4, err = layer2.EncodeShortLCToCACH(&slc, tact) }},
		{"layer2.ShortLCAssembler.Add", "activity update", 1, func() { addCACHs(&cachs) }},
		{"layer2.ShortLCAssembler.Add", "raw SLCO", 0, func() { addCACHs(&rawCACHs) }},
		{"layer2.ShortLCAssembler.AddFragment", "first", 0, func() { done, err = s.AddFragment(enums.LCSSFirstFragment, cach.Payload) }},
		{"layer2.ShortLCAssembler.Complete", "incomplete", 0, func() { slcOut, err = s.Complete() }},
		{"layer2.ShortLCAssembler.Count", "after first", 0, func() { n = s.Count() }},
		{"layer2.ShortLCAssembler.Reset", "after first", 0, func() { s.Reset() }},
		{"layer2.SyncSource.SyncPatterns", "MS", 0, func() { _, _, ok = layer2.SyncSourceMS.SyncPatterns() }},
		{"layer2.SyncSourceOf", "TDMA2 data", 0, func() { _, _, ok = layer2.SyncSourceOf(enums.SyncTDMA2Data) }},
	}
}

func voiceEntries(t testing.TB) []entry {
	t.Helper()
	parrot := capture(t, "parrot_kerchunk")
	bursts := decodeAll(t, parrot)
	voice := bursts[1:7]
	slots := voice[0].VoiceSlots
	frames, _ := voice[0].AMBEFrames()
	call := must(pdu.NewVoiceLC(true, 91, 3191868, layer3.ServiceOptions{}))
	opts := &layer2.VoiceTransmitterOptions{
		LC: call, ColorCode: 1, Silence: vocoder.AMBESilenceSlot(),
		PIHeader: &pdu.PIHeader{AlgorithmID: 1, MFID: 0x10, KeyID: 7, MessageIndicator: 0xDEADBEEF, Destination: 91},
	}
	tx := must(layer2.NewVoiceTransmitter(opts))
	var frags [4][4]byte
	for i := range frags {
		frags[i] = bit.Pack32(bursts[2+i].EmbeddedSignallingData)
	}
	mid := must(layer2.NewVoiceTransmitter(opts))
	must(mid.AddSlots(slots[:]...))
	must(mid.AddSlots(slots[0]))
	term := new(layer2.VoiceTransmitter)
	buf := make([]layer2.RawBurst, 0, layer2.VoiceBurstsPerSuperframe+1)

	var (
		rx    layer2.VoiceReceiver
		vt    *layer2.VoiceTransmitter
		out   []layer2.RawBurst
		raw   layer2.RawBurst
		err   error
		flc   pdu.FullLinkControl
		ok    bool
		rcErr error
	)
	_, _, _, _, _, _, _ = vt, out, raw, err, flc, ok, rcErr
	return []entry{
		{"layer2.NewVoiceTransmitter", "PI header", 4, func() { vt, err = layer2.NewVoiceTransmitter(opts) }},
		{"layer2.VoiceTransmitter.Header", "group voice", 0, func() { raw = tx.Header() }},
		{"layer2.VoiceTransmitter.Headers", "PI header", 1, func() { out = tx.Headers() }},
		{"layer2.VoiceTransmitter.AppendHeaders", "PI header", 0, func() { out = tx.AppendHeaders(buf[:0]) }},
		{"layer2.VoiceTransmitter.SetReverseChannel", "RC 3", 0, func() {
			rcErr = tx.SetReverseChannel(pdu.ReverseChannel{RCCommand: enums.RCCommand(3)})
		}},
		{"layer2.VoiceTransmitter.SetEmbeddedLC", "group voice", 0, func() { err = tx.SetEmbeddedLC(call) }},
		{"layer2.VoiceTransmitter.SetEmbeddedFragments", "parrot capture", 0, func() { err = tx.SetEmbeddedFragments(frags) }},
		{"layer2.VoiceTransmitter.AddSlots", "one burst", 1, func() { out, err = tx.AddSlots(slots[:]...) }},
		{"layer2.VoiceTransmitter.AppendSlots", "one burst", 0, func() { out, err = tx.AppendSlots(buf[:0], slots[:]...) }},
		{"layer2.VoiceTransmitter.AddFrames", "one burst", 1, func() { out, err = tx.AddFrames(frames[:]...) }},
		{"layer2.VoiceTransmitter.AppendFrames", "one burst", 0, func() { out, err = tx.AppendFrames(buf[:0], frames[:]...) }},
		{"layer2.VoiceTransmitter.Terminate", "after burst B", 1, func() { *term = *mid; out, err = term.Terminate() }},
		{"layer2.VoiceTransmitter.AppendTerminate", "after burst B", 0, func() { *term = *mid; out, err = term.AppendTerminate(buf[:0]) }},
		{"layer2.VoiceReceiver.Receive", "parrot capture superframe", 1, func() {
			for i := range voice {
				flc, ok, err = rx.Receive(&voice[i])
			}
		}},
		{"layer2.VoiceReceiver.Reset", "strict", 0, func() { rx.Strict = true; rx.Reset() }},
	}
}
