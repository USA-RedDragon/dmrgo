package layer2_test

import (
	"math/rand"
	"path/filepath"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

func captureFrames(t *testing.T) [][33]byte {
	t.Helper()
	files, err := filepath.Glob("testdata/*.bin")
	if err != nil || len(files) == 0 {
		t.Fatalf("no captures: %v", err)
	}
	frames := make([][33]byte, 0, 64)
	for _, f := range files {
		frames = append(frames, loadBursts(t, f)...)
	}
	return frames
}

func packSlots(s [layer2.VoiceSlotsPerBurst][72]bit.Bit) [layer2.VoiceSlotsPerBurst][9]byte {
	var out [layer2.VoiceSlotsPerBurst][9]byte
	for i := range s {
		copy(out[i][:], bit.PackBits(s[i][:]))
	}
	return out
}

func isBPTC(dt enums.DataType) bool {
	return dt != enums.DataTypeRate34 && dt != enums.DataTypeRate1 && !dt.IsReserved()
}

// sameBits reports whether a and b agree outside the burst bit ranges in
// skip, each given as [start, end).
func sameBits(a, b layer2.RawBurst, skip ...[2]int) bool {
	x := bit.UnpackBytesToBits264(a)
	y := bit.UnpackBytesToBits264(b)
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
	counts := map[string]int{}
	for _, frame := range captureFrames(t) {
		r := layer2.RawBurst(frame)
		b, _ := layer2.NewBurstFromBytes(frame)

		p, d := r.Sync()
		if d != b.SyncDistance || (b.SyncPattern != enums.SyncNone && p != b.SyncPattern) {
			t.Errorf("%x: Sync %x/%d, Burst %x/%d", frame, int64(p), d, int64(b.SyncPattern), b.SyncDistance)
		}
		if b.Kind == layer2.KindData {
			counts["slot type"]++
			if st, res := r.SlotType(); st != b.SlotType || res != b.FEC.SlotType {
				t.Errorf("%x: SlotType %+v, Burst %+v", frame, st, b.SlotType)
			}
		}
		if b.Kind == layer2.KindData {
			if want, wantRes, ok := b.BPTCPayload(); ok {
				counts["BPTC"]++
				if got, res := r.BPTCPayload(); got != want || res != wantRes {
					t.Errorf("%x: BPTCPayload %x, Burst %x", frame, got, want)
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
				if got := r.Rate1Payload(); got != d.Data {
					t.Errorf("%x: Rate1Payload %x, Burst %x", frame, got, d.Data)
				}
			}
			continue
		}
		if b.HasEmbeddedSignalling() {
			counts["EMB"]++
			emb, res := r.EMB()
			data := r.EmbeddedData()
			if b.SyncPattern == enums.SyncMSSourcedRC {
				emb, res = r.StandaloneRCEMB()
				data = r.StandaloneRCData()
			}
			if emb != b.EmbeddedSignalling || res != b.FEC.EMB || data != b.PackEmbeddedSignallingData() {
				t.Errorf("%x: EMB %+v %x, Burst %+v %x", frame, emb, data, b.EmbeddedSignalling, b.PackEmbeddedSignallingData())
			}
		}
		if b.SyncPattern != enums.SyncMSSourcedRC {
			counts["voice"]++
			if got := r.VoiceSlots(); got != packSlots(b.VoiceSlots) {
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

func TestRawBurst_SettersRoundTripCaptures(t *testing.T) {
	for _, frame := range captureFrames(t) {
		r := layer2.RawBurst(frame)
		w := r
		if p, d := r.Sync(); d == 0 {
			if err := w.SetSync(p); err != nil {
				t.Fatal(err)
			}
		}
		st, stRes := r.SlotType()
		b, _ := layer2.NewBurstFromBytes(frame)
		if b.Kind == layer2.KindData {
			if !stRes.HasErrors() {
				if err := w.SetSlotType(st); err != nil {
					t.Fatal(err)
				}
			}
			switch dt := b.SlotType.DataType; {
			case dt == enums.DataTypeRate34:
				if p, res := r.Rate34Payload(); !res.HasErrors() {
					w.SetRate34Payload(p)
				}
			case dt == enums.DataTypeRate1:
				w.SetRate1Payload(r.Rate1Payload())
			case isBPTC(dt):
				if p, res := r.BPTCPayload(); !res.HasErrors() {
					w.SetBPTCPayload(p)
				}
			}
		} else {
			w.SetVoiceSlots(r.VoiceSlots())
			if b.HasEmbeddedSignalling() && b.SyncPattern != enums.SyncMSSourcedRC {
				if e, res := r.EMB(); !res.HasErrors() {
					if err := w.SetEMB(e); err != nil {
						t.Fatal(err)
					}
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
	rng := rand.New(rand.NewSource(3))
	info := [][2]int{{layer2.InfoFirstStart, layer2.InfoFirstEnd}, {layer2.InfoSecondStart, layer2.InfoSecondEnd}}
	for _, frame := range captureFrames(t) {
		r := layer2.RawBurst(frame)
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
				if got, _, ok := nb.BPTCPayload(); !ok || got != p {
					t.Fatalf("Burst reads %x, want %x", got, p)
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
		e := pdu.EmbeddedSignalling{ColorCode: rng.Intn(16), LCSS: enums.LCSS(rng.Intn(4))}
		if err := w.SetEMB(e); err != nil {
			t.Fatal(err)
		}
		if got, res := w.EMB(); got.ColorCode != e.ColorCode || got.LCSS != e.LCSS || res.HasErrors() || !sameBits(w, r, [2]int{108, 116}, [2]int{148, 156}) {
			t.Fatalf("SetEMB: %+v %+v", got, res)
		}
	}
}

func TestRawBurst_SetSync(t *testing.T) {
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
	frame := buildStandaloneRCBurst(7, enums.RCCeaseTransmissionRequest)
	r := layer2.RawBurst(frame)
	b := must(layer2.NewBurstFromBytes(frame))
	emb, res := r.StandaloneRCEMB()
	if emb != b.EmbeddedSignalling || res.HasErrors() || emb.ColorCode != 7 {
		t.Fatalf("StandaloneRCEMB %+v %+v", emb, res)
	}
	data := r.StandaloneRCData()
	if data != b.PackEmbeddedSignallingData() {
		t.Fatalf("StandaloneRCData %x", data)
	}
	var bits [32]bit.Bit
	copy(bits[:], bit.UnpackBits(data[:]))
	if rc, _ := layer2.DecodeEmbeddedRC(bits); rc.RCCommand != enums.RCCeaseTransmissionRequest {
		t.Fatalf("RC %+v", rc)
	}

	var w layer2.RawBurst
	if err := w.SetSync(enums.SyncMSSourcedRC); err != nil {
		t.Fatal(err)
	}
	if err := w.SetStandaloneRCEMB(emb); err != nil {
		t.Fatal(err)
	}
	w.SetStandaloneRCData(data)
	if w != r {
		t.Fatalf("rebuilt RC burst\ngot  %x\nwant %x", w, r)
	}
}

func TestBuildDataBurst_MatchesExistingBuilders(t *testing.T) {
	cw := must(pdu.FullLCCodeword([9]byte(capturedEmbeddedLC), enums.DataTypeVoiceLCHeader))
	for cc := uint8(0); cc < 16; cc++ {
		got := must(layer2.BuildDataBurst(cw, enums.DataTypeVoiceLCHeader, cc, layer2.SyncSourceBS))
		want := must(layer2.BuildLCDataBurst(cw, enums.DataTypeVoiceLCHeader, cc))
		if got != layer2.RawBurst(want) {
			t.Fatalf("cc %d: BuildDataBurst differs from BuildLCDataBurst", cc)
		}
	}
	for _, src := range []layer2.SyncSource{layer2.SyncSourceBS, layer2.SyncSourceMS, layer2.SyncSourceDirectTS1, layer2.SyncSourceDirectTS2} {
		tx := must(layer2.NewVoiceTransmitter(capturedLC(), 3, src, silenceSlot()))
		got := must(layer2.BuildDataBurst(cw, enums.DataTypeVoiceLCHeader, 3, src))
		if got != layer2.RawBurst(tx.Header()) {
			t.Errorf("source %d: BuildDataBurst differs from the VoiceTransmitter header", src)
		}
	}
}

func TestBuildDataBurst_ReproducesCaptures(t *testing.T) {
	n := 0
	for _, frame := range captureFrames(t) {
		r := layer2.RawBurst(frame)
		st, stRes := r.SlotType()
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
		got := must(layer2.BuildDataBurst(p, st.DataType, uint8(st.ColorCode), src)) //nolint:gosec // 4-bit color code
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
	if _, err := layer2.BuildDataBurst([12]byte{}, enums.DataTypeRate34, 1, layer2.SyncSourceBS); err == nil {
		t.Error("Rate 3/4 accepted")
	}
	if _, err := layer2.BuildDataBurst([12]byte{}, enums.DataTypeRate1, 1, layer2.SyncSourceBS); err == nil {
		t.Error("Rate 1 accepted")
	}
	if _, err := layer2.BuildDataBurst([12]byte{}, enums.DataTypeCSBK, 16, layer2.SyncSourceBS); err == nil {
		t.Error("color code 16 accepted")
	}
	if _, err := layer2.BuildDataBurst([12]byte{}, enums.DataTypeCSBK, 1, layer2.SyncSource(9)); err == nil {
		t.Error("unknown source accepted")
	}
}

func TestBuildRateDataBursts_DecodeAsBurst(t *testing.T) {
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
		if r.Rate1Payload() != p1 || must(b.Encode()) != r {
			t.Fatal("Rate 1 burst round trip")
		}
		w := r
		w.SetRate1Payload(p1)
		if w != r {
			t.Fatal("SetRate1Payload changed the reserved bits")
		}
	}
}

func TestEmbeddedLCFragments_Capture(t *testing.T) {
	lc := [9]byte(capturedEmbeddedLC)
	want := fragmentWords(capturedFragments())
	if got := layer2.EmbeddedLCFragments(lc); got != want {
		t.Fatalf("fragments %x, want %x", got, want)
	}
	if got, res := layer2.DecodeEmbeddedLCFragments(want); got != lc || res.HasErrors() {
		t.Errorf("decode: %x %+v", got, res)
	}
	if got, res := layer2.DecodeEmbeddedLCFragmentsStrict(want); got != lc || res.HasErrors() {
		t.Errorf("strict decode: %x %+v", got, res)
	}
	bad := want
	bad[2][1] ^= 0x01
	if got, res := layer2.DecodeEmbeddedLCFragments(bad); got != lc || !res.HasErrors() {
		t.Errorf("one bit error: %x %+v", got, res)
	}
}

func fragmentWords(fr [4][32]bit.Bit) [4][4]byte {
	var out [4][4]byte
	for i := range fr {
		copy(out[i][:], bit.PackBits(fr[i][:]))
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
