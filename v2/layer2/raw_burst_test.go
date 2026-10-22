package layer2_test

import (
	"errors"
	"math/rand"
	"path/filepath"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/fec"
	"github.com/USA-RedDragon/dmrgo/v2/internal/bits"
	"github.com/USA-RedDragon/dmrgo/v2/internal/unpacked"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/layer3"
	"github.com/USA-RedDragon/dmrgo/v2/vocoder"
)

func captureFrames(t testing.TB) []layer2.RawBurst {
	t.Helper()
	files, err := filepath.Glob("testdata/*.bin")
	if err != nil || len(files) == 0 {
		t.Fatalf("no captures: %v", err)
	}
	frames := make([]layer2.RawBurst, 0, 64)
	for _, f := range files {
		frames = append(frames, loadBursts(t, f)...)
	}
	return frames
}

func isBPTC(dt enums.DataType) bool {
	return dt != enums.DataTypeRate34 && dt != enums.DataTypeRate1 && !dt.IsReserved()
}

func sameBits(a, b layer2.RawBurst, skip ...[2]int) bool {
	x := [264]unpacked.Bit(unpacked.Unpack(a[:]))
	y := [264]unpacked.Bit(unpacked.Unpack(b[:]))
outer:
	for i := range x {
		for _, s := range skip {
			if i >= s[0] && i < s[1] {
				continue outer
			}
		}
		if x[i] != y[i] {
			return false
		}
	}
	return true
}

func TestRawBurst_GettersMatchBurst(t *testing.T) {
	t.Parallel()

	counts := map[string]int{}
	for _, frame := range captureFrames(t) {
		r := frame
		b, _ := layer2.NewBurstFromBytes(frame)

		p, d := r.Sync()
		if d != b.SyncDistance || (b.SyncPattern != enums.SyncNone && p != b.SyncPattern) {
			t.Errorf("%x: Sync %x/%d, Burst %x/%d", frame, int64(p), d, int64(b.SyncPattern), b.SyncDistance)
		}
		if b.Kind == layer2.KindData {
			counts["slot type"]++
			if st := r.SlotType(); st != b.SlotType || st.FEC != b.FEC.SlotType {
				t.Errorf("%x: SlotType %+v, Burst %+v", frame, st, b.SlotType)
			}
		}
		if b.Kind == layer2.KindData {
			if isBPTC(b.SlotType.DataType) {
				counts["BPTC"]++
				if _, res := r.BPTCPayload(); res != b.FEC.Payload {
					t.Errorf("%x: BPTCPayload FEC %+v, Burst %+v", frame, res, b.FEC.Payload)
				}
			}
			switch d := b.Data.(type) {
			case *pdu.Rate34Data:
				counts["rate 3/4"]++
				if got, res := r.Rate34Payload(); got != d.Data || res != b.FEC.Payload {
					t.Errorf("%x: Rate34Payload %x, Burst %x", frame, got, d.Data)
				}
			case *pdu.Rate1Data:
				counts["rate 1"]++
				if got, res := r.Rate1Payload(); got != d.Data || res != (fec.Result{}) {
					t.Errorf("%x: Rate1Payload %x, Burst %x", frame, got, d.Data)
				}
			}
			continue
		}
		if b.HasEmbeddedSignalling() {
			counts["EMB"]++
			emb := r.EMB()
			res := emb.FEC
			data := r.EmbeddedData()
			if b.SyncPattern == enums.SyncMSSourcedRC {
				emb = r.StandaloneRCEMB()
				res = emb.FEC
				data = r.StandaloneRCData()
			}
			if want := b.EmbeddedSignallingData; emb != b.EmbeddedSignalling || res != b.FEC.EMB || data != want {
				t.Errorf("%x: EMB %+v %x, Burst %+v %x", frame, emb, data, b.EmbeddedSignalling, want)
			}
		}
		if b.SyncPattern != enums.SyncMSSourcedRC {
			counts["voice"]++
			if got := r.VoiceSlots(); got != b.VoiceSlots {
				t.Errorf("%x: VoiceSlots %x", frame, got)
			}
		}
	}
	for _, k := range []string{"slot type", "BPTC", "EMB", "voice"} {
		if counts[k] == 0 {
			t.Errorf("no %s bursts in the captures", k)
		}
	}
	t.Logf("checked %v", counts)
}

func TestRawBurst_GettersOnValue(t *testing.T) {
	t.Parallel()

	for _, frame := range captureFrames(t) {
		r := frame
		p1, d1 := frame.Sync()
		p2, d2 := r.Sync()
		st1 := frame.SlotType()
		st2 := r.SlotType()
		bp1, _ := frame.BPTCPayload()
		bp2, _ := r.BPTCPayload()
		e1 := frame.EMB()
		e2 := r.EMB()
		r1a, _ := frame.Rate1Payload()
		r1b, _ := r.Rate1Payload()
		if p1 != p2 || d1 != d2 || st1 != st2 || bp1 != bp2 || e1 != e2 ||
			frame.EmbeddedData() != r.EmbeddedData() ||
			frame.VoiceSlots() != r.VoiceSlots() ||
			r1a != r1b ||
			frame.StandaloneRCData() != r.StandaloneRCData() {
			t.Errorf("%x: getters on a value differ from getters on a variable", frame)
		}
		if r != frame {
			t.Errorf("%x: getters changed the burst", frame)
		}
	}
}

func TestRawBurst_SettersRoundTripCaptures(t *testing.T) {
	t.Parallel()

	for _, frame := range captureFrames(t) {
		r := frame
		w := r
		if p, d := r.Sync(); d == 0 {
			if err := w.SetSync(p); err != nil {
				t.Fatal(err)
			}
		}
		st := r.SlotType()
		stRes := st.FEC
		b, _ := layer2.NewBurstFromBytes(frame)
		if b.Kind == layer2.KindData {
			if !stRes.HasErrors() {
				noErr(w.SetSlotType(st))
			}
			switch dt := b.SlotType.DataType; {
			case dt == enums.DataTypeRate34:
				if p, res := r.Rate34Payload(); !res.HasErrors() {
					w.SetRate34Payload(p)
				}
			case dt == enums.DataTypeRate1:
				p, _ := r.Rate1Payload()
				w.SetRate1Payload(p)
			case isBPTC(dt):
				if p, res := r.BPTCPayload(); !res.HasErrors() {
					w.SetBPTCPayload(p)
				}
			}
		} else {
			w.SetVoiceSlots(r.VoiceSlots())
			if b.HasEmbeddedSignalling() && b.SyncPattern != enums.SyncMSSourcedRC {
				if e := r.EMB(); !e.FEC.HasErrors() {
					noErr(w.SetEMB(e))
				}
				w.SetEmbeddedData(r.EmbeddedData())
			}
		}
		if w != r {
			t.Errorf("rewrite changed the burst\ngot  %x\nwant %x", w, r)
		}
	}
}

func TestRawBurst_SettersTouchOnlyTheirField(t *testing.T) {
	t.Parallel()

	rng := rand.New(rand.NewSource(3))
	info := [][2]int{{layer2.InfoFirstStart, layer2.InfoFirstEnd}, {layer2.InfoSecondStart, layer2.InfoSecondEnd}}
	for _, frame := range captureFrames(t) {
		r := frame
		b, _ := layer2.NewBurstFromBytes(frame)
		if (b.Kind == layer2.KindData) && isBPTC(b.SlotType.DataType) {
			var p [12]byte
			rng.Read(p[:])
			w := r
			w.SetBPTCPayload(p)
			if got, res := w.BPTCPayload(); got != p || res.HasErrors() {
				t.Fatalf("BPTCPayload after set: %x %+v", got, res)
			}
			if !sameBits(w, r, info...) {
				t.Fatal("SetBPTCPayload changed bits outside the information bits")
			}
			if nb, err := layer2.NewBurstFromBytes(w); err == nil {
				if got := must(nb.Encode()); got != [33]byte(w) {
					t.Fatalf("Burst re-encodes %x, want %x", got, w)
				}
			}
			continue
		}
		if (b.Kind == layer2.KindData) || b.SyncPattern == enums.SyncMSSourcedRC {
			continue
		}
		var slots [3][9]byte
		for i := range slots {
			rng.Read(slots[i][:])
		}
		w := r
		w.SetVoiceSlots(slots)
		if w.VoiceSlots() != slots || !sameBits(w, r, [2]int{0, 108}, [2]int{156, 264}) {
			t.Fatal("SetVoiceSlots")
		}
		if !b.HasEmbeddedSignalling() {
			continue
		}
		var d [4]byte
		rng.Read(d[:])
		w = r
		w.SetEmbeddedData(d)
		if w.EmbeddedData() != d || !sameBits(w, r, [2]int{116, 148}) {
			t.Fatal("SetEmbeddedData")
		}
		w = r
		e := pdu.EmbeddedSignalling{ColorCode: enums.ColorCode(rng.Intn(16)), LCSS: enums.LCSS(rng.Intn(4))}
		noErr(w.SetEMB(e))
		if got := w.EMB(); got.ColorCode != e.ColorCode || got.LCSS != e.LCSS || got.FEC.HasErrors() || !sameBits(w, r, [2]int{108, 116}, [2]int{148, 156}) {
			t.Fatalf("SetEMB: %+v", got)
		}
	}
}

func TestRawBurst_SetSync(t *testing.T) {
	t.Parallel()

	var r layer2.RawBurst
	for i := range r {
		r[i] = 0xA5
	}
	before := r
	if err := r.SetSync(enums.SyncNone); err == nil || r != before {
		t.Fatal("SetSync accepted a non-pattern")
	}
	if err := r.SetSync(enums.SyncMSSourcedVoice); err != nil {
		t.Fatal(err)
	}
	if p, d := r.Sync(); p != enums.SyncMSSourcedVoice || d != 0 || !sameBits(r, before, [2]int{108, 156}) {
		t.Fatalf("Sync %x/%d", int64(p), d)
	}
}

func TestRawBurst_StandaloneRC(t *testing.T) {
	t.Parallel()

	frame := buildStandaloneRCBurst(7, enums.RCCeaseTransmissionRequest)
	r := layer2.RawBurst(frame)
	b := must(layer2.NewBurstFromBytes(frame))
	emb := r.StandaloneRCEMB()
	res := emb.FEC
	if emb != b.EmbeddedSignalling || res.HasErrors() || emb.ColorCode != 7 {
		t.Fatalf("StandaloneRCEMB %+v %+v", emb, res)
	}
	data := r.StandaloneRCData()
	if data != b.EmbeddedSignallingData {
		t.Fatalf("StandaloneRCData %x", data)
	}
	if rc := layer2.DecodeEmbeddedRC(data); rc.RCCommand != enums.RCCeaseTransmissionRequest {
		t.Fatalf("RC %+v", rc)
	}

	var w layer2.RawBurst
	if err := w.SetSync(enums.SyncMSSourcedRC); err != nil {
		t.Fatal(err)
	}
	noErr(w.SetStandaloneRCEMB(emb))
	w.SetStandaloneRCData(data)
	if w != r {
		t.Fatalf("rebuilt RC burst\ngot  %x\nwant %x", w, r)
	}
}

func TestBuildDataBurst_MatchesExistingBuilders(t *testing.T) {
	t.Parallel()

	cw := must(pdu.LCBytes(capturedEmbeddedLC).Codeword(enums.DataTypeVoiceLCHeader))
	for cc := enums.ColorCode(0); cc < 16; cc++ {
		got := must(layer2.BuildDataBurst(cw, enums.DataTypeVoiceLCHeader, cc, layer2.SyncSourceBS))
		want := must(lcBurst(cw, enums.DataTypeVoiceLCHeader, cc))
		if got != layer2.RawBurst(want) {
			t.Fatalf("cc %d: BuildDataBurst differs from a hand-built burst", cc)
		}
	}
	for _, src := range []layer2.SyncSource{layer2.SyncSourceBS, layer2.SyncSourceMS, layer2.SyncSourceDirectTS1, layer2.SyncSourceDirectTS2} {
		tx := must(layer2.NewVoiceTransmitter(&layer2.VoiceTransmitterOptions{LC: capturedLC(), ColorCode: 3, Source: src, Silence: silenceSlot()}))
		got := must(layer2.BuildDataBurst(cw, enums.DataTypeVoiceLCHeader, 3, src))
		if got != tx.Header() {
			t.Errorf("source %d: BuildDataBurst differs from the VoiceTransmitter header", src)
		}
	}
}

func TestBuildDataBurst_ReproducesCaptures(t *testing.T) {
	t.Parallel()

	n := 0
	for _, frame := range captureFrames(t) {
		r := frame
		st := r.SlotType()
		stRes := st.FEC
		sync, d := r.Sync()
		if stRes.HasErrors() || d != 0 || !isBPTC(st.DataType) {
			continue
		}
		src, ok := dataSource(sync)
		if !ok {
			continue
		}
		p, res := r.BPTCPayload()
		if res.HasErrors() {
			continue
		}
		got := must(layer2.BuildDataBurst(p, st.DataType, st.ColorCode, src))
		if got != r {
			t.Errorf("rebuilt %x\nwant    %x", got, r)
		}
		n++
	}
	if n == 0 {
		t.Fatal("no clean BPTC data bursts in the captures")
	}
}

func TestBuildDataBurst_Rejects(t *testing.T) {
	t.Parallel()

	if _, err := layer2.BuildDataBurst([12]byte{}, enums.DataTypeRate34, 1, layer2.SyncSourceBS); err == nil {
		t.Error("Rate 3/4 accepted")
	}
	if _, err := layer2.BuildDataBurst([12]byte{}, enums.DataTypeRate1, 1, layer2.SyncSourceBS); err == nil {
		t.Error("Rate 1 accepted")
	}
	var rangeErr *dmrgo.FieldRangeError
	if _, err := layer2.BuildDataBurst([12]byte{}, enums.DataTypeCSBK, 0x17, layer2.SyncSourceBS); !errors.As(err, &rangeErr) || rangeErr.Field != "ColorCode" {
		t.Errorf("color code 0x17: error %v, want the FieldRangeError of ColorCode", err)
	}
	if _, err := layer2.BuildDataBurst([12]byte{}, enums.DataTypeCSBK, 1, layer2.SyncSource(9)); err == nil {
		t.Error("unknown source accepted")
	}
}

func TestBuildRateDataBursts_DecodeAsBurst(t *testing.T) {
	t.Parallel()

	rng := rand.New(rand.NewSource(4))
	for n := 0; n < 50; n++ {
		var p34 [18]byte
		rng.Read(p34[:])
		r := must(layer2.BuildRate34DataBurst(p34, 5, layer2.SyncSourceMS))
		b := must(layer2.NewBurstFromBytes(r))
		d, ok := b.Data.(*pdu.Rate34Data)
		if !ok || d.Data != p34 || b.FEC.Payload.HasErrors() || b.SlotType.ColorCode != 5 || b.SyncPattern != enums.SyncMSSourcedData {
			t.Fatalf("Rate 3/4 burst decodes as %+v", b.Data)
		}
		if got, _ := r.Rate34Payload(); got != p34 {
			t.Fatal("Rate34Payload")
		}
		if must(b.Encode()) != r {
			t.Fatal("Rate 3/4 burst does not re-encode")
		}

		var p1 [24]byte
		rng.Read(p1[:])
		r = must(layer2.BuildRate1DataBurst(p1, 9, layer2.SyncSourceBS))
		b = must(layer2.NewBurstFromBytes(r))
		d1, ok := b.Data.(*pdu.Rate1Data)
		if !ok || d1.Data != p1 || b.SlotType.ColorCode != 9 {
			t.Fatalf("Rate 1 burst decodes as %+v", b.Data)
		}
		if got, _ := r.Rate1Payload(); got != p1 || must(b.Encode()) != r {
			t.Fatal("Rate 1 burst round trip")
		}
		w := r
		w.SetRate1Payload(p1)
		if w != r {
			t.Fatal("SetRate1Payload changed the reserved bits")
		}
	}
}

func TestEmbeddedLCBytes_Capture(t *testing.T) {
	t.Parallel()

	lc := [9]byte(capturedEmbeddedLC)
	want := fragmentWords(capturedFragments())
	if got := layer2.EncodeEmbeddedLC(lc); got != want {
		t.Fatalf("fragments %x, want %x", got, want)
	}
	if got, res := layer2.DecodeEmbeddedLC(want, false); got != lc || res.HasErrors() {
		t.Errorf("decode: %x %+v", got, res)
	}
	if got, res := layer2.DecodeEmbeddedLC(want, true); got != lc || res.HasErrors() {
		t.Errorf("strict decode: %x %+v", got, res)
	}
	bad := want
	bad[2][1] ^= 0x01
	if got, res := layer2.DecodeEmbeddedLC(bad, false); got != lc || !res.HasErrors() {
		t.Errorf("one bit error: %x %+v", got, res)
	}
}

func fragmentWords(fr [4][32]unpacked.Bit) [4][4]byte {
	var out [4][4]byte
	for i := range fr {
		copy(out[i][:], unpacked.Pack(fr[i][:]))
	}
	return out
}

func dataSource(sync enums.SyncPattern) (layer2.SyncSource, bool) {
	for _, src := range []layer2.SyncSource{layer2.SyncSourceBS, layer2.SyncSourceMS, layer2.SyncSourceDirectTS1, layer2.SyncSourceDirectTS2} {
		if _, data, _ := src.SyncPatterns(); data == sync {
			return src, true
		}
	}
	return 0, false
}

func TestBuildBurst_MatchesBurstEncode(t *testing.T) {
	t.Parallel()

	lc := must(pdu.NewVoiceLC(true, 91, 3191868, layer3.MotorolaServiceOptions()))
	term := lc
	term.BurstDataType = enums.DataTypeTerminatorWithLC
	var r34 pdu.Rate34Data
	var r1 pdu.Rate1Data
	for i := range r34.Data {
		r34.Data[i] = byte(i * 7)
	}
	for i := range r1.Data {
		r1.Data[i] = byte(i * 11)
	}
	csbk := must(pdu.NewBSOutboundActivation(310123, 3191868))
	var mbc pdu.MBCHeader
	mustSet(mbc.SetTalkgroupVoiceGrant(pdu.TalkgroupVoiceGrantPDU{PhysicalChannel: 0xFFF}))
	for _, d := range []layer2.Data{
		&lc, &term, &csbk, &r34, &r1, &pdu.Rate12Data{Data: [12]byte{1, 2, 3}}, &pdu.PRFill{}, &mbc,
	} {
		got, err := layer2.BuildBurst(d, 9, layer2.SyncSourceMS)
		if err != nil {
			t.Fatalf("%T: %v", d, err)
		}
		b := layer2.Burst{Kind: layer2.KindData, SyncPattern: enums.SyncMSSourcedData, SlotType: pdu.SlotType{ColorCode: 9, DataType: d.DataType()}, Data: d}
		if want := must(b.Encode()); got != want {
			t.Errorf("%T: BuildBurst\ngot  %x\nwant %x", d, got, want)
		}
		dec := must(layer2.NewBurstFromBytes(got))
		if dec.FEC.Aggregate().HasErrors() || dec.SlotType.DataType != d.DataType() {
			t.Errorf("%T: decodes as %s", d, dec.String())
		}
	}
	if _, err := layer2.BuildBurst(nil, 1, layer2.SyncSourceBS); err == nil {
		t.Error("nil Data accepted")
	}
	if _, err := layer2.BuildBurst((*pdu.CSBK)(nil), 1, layer2.SyncSourceBS); err == nil {
		t.Error("nil *CSBK accepted")
	}
	if _, err := layer2.BuildBurst(&csbk, 1, layer2.SyncSource(9)); err == nil {
		t.Error("unknown source accepted")
	}
	bad := lc
	bad.BurstDataType = enums.DataTypeCSBK
	if _, err := layer2.BuildBurst(&bad, 1, layer2.SyncSourceBS); err == nil {
		t.Error("Full LC with a CSBK data type accepted")
	}
}

func TestBuildVoiceBurst_MatchesVoiceTransmitter(t *testing.T) {
	t.Parallel()

	lc := must(pdu.NewVoiceLC(true, 91, 3191868, layer3.MotorolaServiceOptions()))
	frags := must(layer2.EncodeEmbeddedFullLinkControl(&lc))
	for _, src := range []layer2.SyncSource{layer2.SyncSourceBS, layer2.SyncSourceMS} {
		tx := must(layer2.NewVoiceTransmitter(&layer2.VoiceTransmitterOptions{LC: lc, ColorCode: 5, Source: src, Silence: silenceSlot()}))
		var frames [18]vocoder.AMBEFrame
		for i := range frames {
			bits.SetField64(frames[i][:], 0, 49, uint64(i)*0x1F2E3D4C5B)
		}
		sent := must(tx.AddFrames(frames[:]...))
		if len(sent) != 6 {
			t.Fatalf("%d bursts", len(sent))
		}
		for i, want := range sent {
			pos := must2(enums.VoiceBurstAt(i))
			var embedded [4]byte
			if i >= 1 && i <= 4 {
				embedded = frags[i-1]
			}
			got := must(layer2.BuildVoiceBurst([3]vocoder.AMBEFrame(frames[i*3:]), pos, pdu.EmbeddedSignalling{ColorCode: 5}, embedded, src))
			if got != want {
				t.Errorf("source %d burst %s:\ngot  %x\nwant %x", src, pos, got, want)
			}
			back, res := got.AMBEFrames()
			if back != [3]vocoder.AMBEFrame(frames[i*3:]) || res.HasErrors() {
				t.Errorf("source %d burst %s: AMBEFrames %v %+v", src, pos, back, res)
			}
		}
	}
	if _, err := layer2.BuildVoiceBurst([3]vocoder.AMBEFrame{}, enums.VoiceBurstUnknown, pdu.EmbeddedSignalling{}, [4]byte{}, layer2.SyncSourceBS); err == nil {
		t.Error("unknown position accepted")
	}
	if _, err := layer2.BuildVoiceBurst([3]vocoder.AMBEFrame{}, enums.VoiceBurstA, pdu.EmbeddedSignalling{}, [4]byte{}, layer2.SyncSource(9)); err == nil {
		t.Error("unknown source accepted")
	}
}

func must2[T any](v T, ok bool) T {
	if !ok {
		panic("not ok")
	}
	return v
}

func TestRawBurst_AMBEVoice(t *testing.T) {
	t.Parallel()

	frames := [layer2.VoiceSlotsPerBurst]vocoder.AMBEFrame{vocoder.AMBESilence(), {0x12, 0x34, 0x56, 0x78, 0x9A, 0xBC, 0x80}, vocoder.AMBESilence()}
	raw, err := layer2.BuildVoiceBurst(frames, enums.VoiceBurstA, pdu.EmbeddedSignalling{}, [4]byte{}, layer2.SyncSourceBS)
	if err != nil {
		t.Fatal(err)
	}
	p, bad, res := raw.AMBEVoice()
	want, _ := vocoder.PackAMBEVoice(frames)
	if p != want || bad != [3]bool{} || res.Uncorrectable || res.ErrorsCorrected != 0 {
		t.Fatalf("clean: %x %v %+v", p, bad, res)
	}
	clean := raw.VoiceSlots()
	found := false
	for k := range 256 {
		slots := clean
		slots[2][0] ^= byte(k)
		slots[2][1] ^= 0x0F
		if _, r := vocoder.DecodeAMBE72(slots[2]); r.Uncorrectable {
			raw.SetVoiceSlots(slots)
			found = true
			break
		}
	}
	if !found {
		t.Fatal("no uncorrectable damage found")
	}
	p, bad, res = raw.AMBEVoice()
	_, flags := vocoder.UnpackAMBEVoiceFlags(p)
	if !bad[2] || bad[0] || bad[1] || !res.Uncorrectable || !flags[2] || flags[1] {
		t.Fatalf("frame 3 damaged: %v %v %+v", bad, flags, res)
	}
}

func TestRawBurst_Info(t *testing.T) {
	t.Parallel()

	var p [12]byte
	p[0] = 0x5A
	raw, err := layer2.BuildDataBurst(p, enums.DataTypeRate12, 3, layer2.SyncSourceMS)
	if err != nil {
		t.Fatal(err)
	}
	info := raw.Info()
	if info[24]&0x0F != 0 {
		t.Fatalf("padding %x", info[24])
	}
	var back layer2.RawBurst
	if err := back.SetSlotType(raw.SlotType()); err != nil {
		t.Fatal(err)
	}
	if err := back.SetSync(enums.SyncMSSourcedData); err != nil {
		t.Fatal(err)
	}
	if err := back.SetInfo(info); err != nil || back != raw {
		t.Fatalf("SetInfo: %v %x, want %x", err, back, raw)
	}
	bad := info
	bad[24] |= 0x01
	var fre *dmrgo.FieldRangeError
	if err := back.SetInfo(bad); !errors.As(err, &fre) || back != raw {
		t.Fatalf("padding bit: %v", err)
	}
}
