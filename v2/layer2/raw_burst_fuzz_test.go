package layer2_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/fec"
	"github.com/USA-RedDragon/dmrgo/v2/internal/bits"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/vocoder"
)

var fuzzSyncPatterns = [...]enums.SyncPattern{
	enums.SyncBSSourcedVoice, enums.SyncBSSourcedData, enums.SyncMSSourcedVoice, enums.SyncMSSourcedData,
	enums.SyncMSSourcedRC, enums.SyncTDMA1Voice, enums.SyncTDMA1Data, enums.SyncTDMA2Voice, enums.SyncTDMA2Data,
	enums.SyncReserved,
}

type fuzzVals struct {
	b [96]byte
	i int
}

func newFuzzVals(v []byte) *fuzzVals {
	f := &fuzzVals{}
	copy(f.b[:], v)
	return f
}

func (f *fuzzVals) next(n int) []byte {
	if f.i+n > len(f.b) {
		f.i = 0
	}
	f.i += n
	return f.b[f.i-n : f.i]
}

func (f *fuzzVals) u8() byte {
	return f.next(1)[0]
}

func burstMask(ranges ...[2]int) layer2.RawBurst {
	var m layer2.RawBurst
	for _, r := range ranges {
		for i := r[0]; i < r[1]; i++ {
			bits.SetBit(m[:], i, 1)
		}
	}
	return m
}

func changedOutside(a, b, mask layer2.RawBurst) int {
	for i := range 264 {
		if bits.Bit(mask[:], i) == 0 && bits.Bit(a[:], i) != bits.Bit(b[:], i) {
			return i
		}
	}
	return -1
}

func clean(res fec.Result) bool {
	return res.ErrorsCorrected == 0 && !res.Uncorrectable
}

func ambeFrames(v []byte) [layer2.VoiceSlotsPerBurst]vocoder.AMBEFrame {
	var frames [layer2.VoiceSlotsPerBurst]vocoder.AMBEFrame
	for i := range frames {
		copy(frames[i][:], v[7*i:])
		frames[i][6] &= 0x80
	}
	return frames
}

type rawFieldSeed struct {
	raw, v []byte
}

func rawFieldSeeds(tb testing.TB) []rawFieldSeed {
	tb.Helper()
	vals := [][]byte{
		{},
		{0x5a, 0xff, 0x13, 0x80, 0x01, 0x7f, 0x22, 0xc3, 0x9e, 0x4d},
		{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff},
	}
	frames := captureFrames(tb)
	seeds := make([]rawFieldSeed, len(frames))
	for i, r := range frames {
		seeds[i] = rawFieldSeed{r[:], vals[i%len(vals)]}
	}
	return seeds
}

type rawFields struct {
	sync, slotType, info, rate1, emb, embedded, rcEMB, rcData, voice layer2.RawBurst
	orig                                                             layer2.RawBurst
}

// ETSI TS 102 361-1 Annex E
func newRawFields() *rawFields {
	return &rawFields{
		sync:     burstMask([2]int{layer2.SyncStart, layer2.SyncEnd}),
		slotType: burstMask([2]int{layer2.SlotTypeFirstStart, layer2.SlotTypeFirstEnd}, [2]int{layer2.SlotTypeSecondStart, layer2.SlotTypeSecondEnd}),
		info:     burstMask([2]int{layer2.InfoFirstStart, layer2.InfoFirstEnd}, [2]int{layer2.InfoSecondStart, layer2.InfoSecondEnd}),
		rate1:    burstMask([2]int{0, 96}, [2]int{layer2.InfoSecondStart + 2, layer2.InfoSecondEnd}),
		emb:      burstMask([2]int{layer2.EMBFirstStart, layer2.EMBFirstEnd}, [2]int{layer2.EMBSecondStart, layer2.EMBSecondEnd}),
		embedded: burstMask([2]int{layer2.EmbeddedDataStart, layer2.EmbeddedDataEnd}),
		rcEMB:    burstMask([2]int{layer2.RCEMBFirstStart, layer2.RCEMBFirstEnd}, [2]int{layer2.RCEMBSecondStart, layer2.RCEMBSecondEnd}),
		rcData:   burstMask([2]int{layer2.RCDataFirstStart, layer2.RCDataFirstEnd}, [2]int{layer2.RCDataSecondStart, layer2.RCDataSecondEnd}),
		voice:    burstMask([2]int{0, layer2.SyncStart}, [2]int{layer2.SyncEnd, 264}),
	}
}

func (fs *rawFields) within(t *testing.T, name string, r, mask layer2.RawBurst) {
	t.Helper()
	if i := changedOutside(fs.orig, r, mask); i >= 0 {
		t.Fatalf("%s changed burst bit %d outside its field\nbefore %x\nafter  %x", name, i, fs.orig, r)
	}
}

func (fs *rawFields) checkSync(t *testing.T, vals *fuzzVals) {
	t.Helper()
	r := fs.orig
	p := fuzzSyncPatterns[int(vals.u8())%len(fuzzSyncPatterns)]
	if err := r.SetSync(p); err != nil {
		t.Fatalf("SetSync(%v): %v", p, err)
	}
	if got, d := r.Sync(); got != p || d != 0 {
		t.Fatalf("Sync() = %v, %d after SetSync(%v)", got, d, p)
	}
	fs.within(t, "SetSync", r, fs.sync)
	r = fs.orig
	bad := enums.SyncPattern(uint64(vals.u8())<<40 | uint64(vals.u8())<<8 | uint64(vals.u8()))
	if r.SetSync(bad) == nil {
		if got, d := r.Sync(); got != bad || d != 0 {
			t.Fatalf("SetSync(%x) took a pattern that Sync does not return: %v, %d", uint64(bad), got, d)
		}
	} else if r != fs.orig {
		t.Fatalf("failed SetSync(%x) changed the burst", uint64(bad))
	}
}

func (fs *rawFields) checkSlotType(t *testing.T, vals *fuzzVals) {
	t.Helper()
	r := fs.orig
	cc, dt := vals.u8(), vals.u8()
	st := pdu.SlotType{ColorCode: enums.ColorCode(cc & 0x1F), DataType: enums.DataType(dt & 0x1F)}
	err := r.SetSlotType(st)
	switch {
	case err == nil:
		got := r.SlotType()
		if got.ColorCode != st.ColorCode || got.DataType != st.DataType || !clean(got.FEC) {
			t.Fatalf("SlotType() = %+v after SetSlotType(%+v)", got, st)
		}
		fs.within(t, "SetSlotType", r, fs.slotType)
	case r != fs.orig:
		t.Fatalf("failed SetSlotType(%+v) changed the burst", st)
	case cc&0x10 == 0 && dt&0x10 == 0:
		t.Fatalf("SetSlotType(%+v): %v", st, err)
	}
}

func (fs *rawFields) checkPayloads(t *testing.T, vals *fuzzVals) {
	t.Helper()
	r := fs.orig
	payload := [12]byte(vals.next(12))
	r.SetBPTCPayload(payload)
	if got, res := r.BPTCPayload(); got != payload || !clean(res) {
		t.Fatalf("BPTCPayload() = %x, %+v after SetBPTCPayload(%x)", got, res, payload)
	}
	if res, _ := r.BPTCReserved(); res != 0 {
		t.Fatalf("BPTCReserved() = %d after SetBPTCPayload", res)
	}
	fs.within(t, "SetBPTCPayload", r, fs.info)

	r = fs.orig
	reserved := vals.u8() & 0x0F
	before, beforeRes := fs.orig.BPTCPayload()
	if err := r.SetBPTCReserved(reserved); err == nil {
		after, afterRes := r.BPTCPayload()
		if after != before || afterRes != beforeRes {
			t.Fatalf("SetBPTCReserved(%d) changed BPTCPayload from %x, %+v to %x, %+v", reserved, before, beforeRes, after, afterRes)
		}
		if got, _ := r.BPTCReserved(); got != reserved {
			t.Fatalf("BPTCReserved() = %d after SetBPTCReserved(%d)", got, reserved)
		}
		fs.within(t, "SetBPTCReserved", r, fs.info)
	} else if r != fs.orig || reserved < 8 {
		t.Fatalf("SetBPTCReserved(%d): %v", reserved, err)
	}

	r = fs.orig
	p34 := [18]byte(vals.next(18))
	r.SetRate34Payload(p34)
	if got, res := r.Rate34Payload(); got != p34 || !clean(res) {
		t.Fatalf("Rate34Payload() = %x, %+v after SetRate34Payload(%x)", got, res, p34)
	}
	fs.within(t, "SetRate34Payload", r, fs.info)

	r = fs.orig
	p1 := [24]byte(vals.next(24))
	r.SetRate1Payload(p1)
	if got, _ := r.Rate1Payload(); got != p1 {
		t.Fatalf("Rate1Payload() = %x after SetRate1Payload(%x)", got, p1)
	}
	fs.within(t, "SetRate1Payload", r, fs.rate1)
}

func (fs *rawFields) checkEMBs(t *testing.T, vals *fuzzVals) {
	t.Helper()
	e := vals.u8()
	emb := pdu.EmbeddedSignalling{ColorCode: enums.ColorCode(e & 0x1F), PreemptionAndPowerControlIndicator: e&0x20 != 0, LCSS: enums.LCSS(e >> 6)}
	for _, rc := range []bool{false, true} {
		r := fs.orig
		err, mask := r.SetEMB(emb), fs.emb
		if rc {
			r = fs.orig
			err, mask = r.SetStandaloneRCEMB(emb), fs.rcEMB
		}
		if err != nil {
			if r != fs.orig || e&0x10 == 0 {
				t.Fatalf("set EMB %+v (rc %v): %v", emb, rc, err)
			}
			continue
		}
		got := r.EMB()
		if rc {
			got = r.StandaloneRCEMB()
		}
		if got.ColorCode != emb.ColorCode || got.PreemptionAndPowerControlIndicator != emb.PreemptionAndPowerControlIndicator ||
			got.LCSS != emb.LCSS || !clean(got.FEC) {
			t.Fatalf("EMB = %+v after setting %+v (rc %v)", got, emb, rc)
		}
		fs.within(t, "set EMB", r, mask)
	}
}

func (fs *rawFields) checkWords(t *testing.T, vals *fuzzVals) {
	t.Helper()
	word := [4]byte(vals.next(4))
	r := fs.orig
	r.SetEmbeddedData(word)
	if got := r.EmbeddedData(); got != word {
		t.Fatalf("EmbeddedData() = %x after SetEmbeddedData(%x)", got, word)
	}
	fs.within(t, "SetEmbeddedData", r, fs.embedded)
	r = fs.orig
	r.SetStandaloneRCData(word)
	if got := r.StandaloneRCData(); got != word {
		t.Fatalf("StandaloneRCData() = %x after SetStandaloneRCData(%x)", got, word)
	}
	fs.within(t, "SetStandaloneRCData", r, fs.rcData)
}

func (fs *rawFields) checkVoice(t *testing.T, vals *fuzzVals) {
	t.Helper()
	r := fs.orig
	slots := [layer2.VoiceSlotsPerBurst][9]byte{[9]byte(vals.next(9)), [9]byte(vals.next(9)), [9]byte(vals.next(9))}
	r.SetVoiceSlots(slots)
	if got := r.VoiceSlots(); got != slots {
		t.Fatalf("VoiceSlots() = %x after SetVoiceSlots(%x)", got, slots)
	}
	fs.within(t, "SetVoiceSlots", r, fs.voice)

	r = fs.orig
	frames := ambeFrames(vals.next(21))
	if err := r.SetAMBEFrames(frames); err != nil {
		t.Fatalf("SetAMBEFrames(%x): %v", frames, err)
	}
	if got, res := r.AMBEFrames(); got != frames || !clean(res) {
		t.Fatalf("AMBEFrames() = %x, %+v after SetAMBEFrames(%x)", got, res, frames)
	}
	fs.within(t, "SetAMBEFrames", r, fs.voice)
	r = fs.orig
	frames[vals.u8()%3][6] |= 0x40
	if r.SetAMBEFrames(frames) == nil || r != fs.orig {
		t.Fatalf("SetAMBEFrames with a padding bit set did not fail cleanly")
	}
}

func (fs *rawFields) check(t *testing.T, raw, v []byte) {
	if len(raw) != 33 {
		return
	}
	fs.orig = layer2.RawBurst(raw)
	vals := newFuzzVals(v)
	fs.checkSync(t, vals)
	fs.checkSlotType(t, vals)
	fs.checkPayloads(t, vals)
	fs.checkEMBs(t, vals)
	fs.checkWords(t, vals)
	fs.checkVoice(t, vals)

	r := fs.orig
	r.SetVoiceSlots(fs.orig.VoiceSlots())
	r.SetEmbeddedData(fs.orig.EmbeddedData())
	r.SetStandaloneRCData(fs.orig.StandaloneRCData())
	p1, _ := fs.orig.Rate1Payload()
	r.SetRate1Payload(p1)
	if r != fs.orig {
		t.Fatalf("setting fields to their getters changed the burst\nbefore %x\nafter  %x", fs.orig, r)
	}
}

func FuzzRawBurstFields(f *testing.F) {
	for _, s := range rawFieldSeeds(f) {
		f.Add(s.raw, s.v)
	}
	f.Fuzz(newRawFields().check)
}
