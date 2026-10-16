package layer2_test

import (
	"math/rand/v2"
	"reflect"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/vocoder"
)

// ASELSAN Tier III bursts from a recording of a BS (CC 2) that sends every
// sixth burst of a channel with an EMB (PI 0, LCSS single fragment) and a
// Null embedded field in place of the data SYNC. They decode with 0 FEC
// corrections. aselsanCSBKSync is the same C_BCAST CSBK with the SYNC.
const (
	aselsanCSBKSync  = "7fcade72ca00d2684509878048ddff57d75df5dc60a27631607a3c638fd79f3767"
	aselsanCSBKEMB   = "7fcade72ca00d2684509878048d2100000000b7c60a27631607a3c638fd79f3767"
	aselsanIdleEMB   = "0000000000000000000000000a42100000000b7f80000000000000000000000000"
	aselsanHeaderEMB = "3b5734147bed3a5992b36d2fc992100000000b75900cdab8110269e7150a109453"
	aselsanRate12EMB = "0812857c7f7eb03c72b08a6109f2100000000b763ffc85d09db075219c0379a612"
	aselsanColorCode = 2
)

func TestDataBurstWithEMB_ASELSAN(t *testing.T) {
	t.Parallel()
	wantEMB := pdu.EmbeddedSignalling{ColorCode: aselsanColorCode, LCSS: enums.LCSSSingleFragment}
	for _, v := range []struct {
		raw string
		dt  enums.DataType
	}{
		{aselsanCSBKEMB, enums.DataTypeCSBK},
		{aselsanIdleEMB, enums.DataTypeIdle},
		{aselsanHeaderEMB, enums.DataTypeDataHeader},
		{aselsanRate12EMB, enums.DataTypeRate12},
	} {
		frame := hexBurst(t, v.raw)
		for _, opts := range []layer2.DecodeOptions{{}, {Kind: layer2.KindData}} {
			var b layer2.Burst
			if err := b.DecodeFromBytesWith(frame, &opts); err != nil {
				t.Fatal(err)
			}
			if b.Kind != layer2.KindData || b.SyncPattern != enums.SyncNone || !b.HasEmbeddedSignalling() || b.HasReverseChannel() {
				t.Fatalf("%s %+v: got %s", v.dt, opts, b.String())
			}
			if b.SlotType.DataType != v.dt || b.SlotType.ColorCode != aselsanColorCode {
				t.Errorf("%s: slot type %s", v.dt, b.SlotType.String())
			}
			emb := b.EmbeddedSignalling
			emb.FEC = pdu.EmbeddedSignalling{}.FEC
			if emb != wantEMB || b.EmbeddedSignallingData != [4]byte{} {
				t.Errorf("%s: EMB %s, embedded data %x", v.dt, b.EmbeddedSignalling.String(), b.EmbeddedSignallingData)
			}
			if agg := b.FEC.Aggregate(); agg.ErrorsCorrected != 0 || agg.Uncorrectable {
				t.Errorf("%s: FEC %+v", v.dt, agg)
			}
			if got := must(b.Encode()); got != frame {
				t.Errorf("%s: Encode\ngot  %x\nwant %x", v.dt, got, frame)
			}
		}

		r := layer2.RawBurst(frame)
		p, _ := r.BPTCPayload()
		built := must(layer2.BuildDataBurst(p, v.dt, aselsanColorCode, layer2.SyncSourceBS))
		noErr(built.SetEMB(r.EMB()))
		built.SetEmbeddedData(r.EmbeddedData())
		if built != r {
			t.Errorf("%s: BuildDataBurst, SetEMB and SetEmbeddedData\ngot  %x\nwant %x", v.dt, built, frame)
		}
	}
}

func TestDataBurstWithEMB_SameDataAsWithSync(t *testing.T) {
	t.Parallel()
	withSync := must(layer2.NewBurstFromBytes(hexBurst(t, aselsanCSBKSync)))
	withEMB := must(layer2.NewBurstFromBytes(hexBurst(t, aselsanCSBKEMB)))
	if withSync.SyncPattern != enums.SyncBSSourcedData || withSync.HasEmbeddedSignalling() {
		t.Fatalf("burst with SYNC: %s", withSync.String())
	}
	csbk, ok := withEMB.Data.(*pdu.CSBK)
	if !ok || csbk.Opcode != pdu.CSBKBroadcast || csbk.FEC.Uncorrectable {
		t.Fatalf("burst with EMB carries %s", withEMB.String())
	}
	if !reflect.DeepEqual(withSync.Data, withEMB.Data) {
		t.Errorf("Data differs:\nSYNC %s\nEMB  %s", withSync.Data, withEMB.Data)
	}

	b := must(layer2.NewBurstFromBytes(hexBurst(t, aselsanCSBKSync)))
	b.SyncPattern = enums.SyncNone
	b.EmbeddedSignalling = pdu.EmbeddedSignalling{ColorCode: aselsanColorCode, LCSS: enums.LCSSSingleFragment}
	if got, err := b.Encode(); err == nil {
		t.Errorf("a data burst decoded with a SYNC encodes with SyncNone as %x", got)
	}

	rewritten := must(layer2.NewBurstFromBytes(hexBurst(t, aselsanCSBKEMB)))
	rewritten.SlotType.ColorCode = aselsanColorCode + 1
	if got, err := rewritten.Encode(); err == nil {
		t.Errorf("a slot type color code that differs from the EMB encodes as %x", got)
	}

	r := must(layer2.BuildBurst(withSync.Data, aselsanColorCode, layer2.SyncSourceBS))
	noErr(r.SetEMB(b.EmbeddedSignalling))
	r.SetEmbeddedData([4]byte{})
	if r != hexBurst(t, aselsanCSBKEMB) {
		t.Errorf("BuildBurst with SetEMB\ngot  %x\nwant %s", r, aselsanCSBKEMB)
	}
}

func TestDataBurstWithEMB_KnownVoice(t *testing.T) {
	t.Parallel()
	var b layer2.Burst
	if err := b.DecodeFromBytesWith(hexBurst(t, aselsanCSBKEMB), &layer2.DecodeOptions{Kind: layer2.KindVoice}); err != nil {
		t.Fatal(err)
	}
	if b.Kind != layer2.KindVoice || !b.HasEmbeddedSignalling() {
		t.Fatalf("got %s, want voice", b.String())
	}
}

func TestDataBurstWithEMB_DamagedSyncStaysSync(t *testing.T) {
	t.Parallel()
	p, _ := layer2.RawBurst(hexBurst(t, aselsanCSBKSync)).BPTCPayload()
	rng := rand.New(rand.NewPCG(3, 4))
	for _, src := range []layer2.SyncSource{layer2.SyncSourceBS, layer2.SyncSourceMS, layer2.SyncSourceDirectTS1, layer2.SyncSourceDirectTS2} {
		for cc := range enums.ColorCode(16) {
			frame := must(layer2.BuildDataBurst(p, enums.DataTypeCSBK, cc, src))
			for errs := layer2.DefaultSyncThreshold + 1; errs <= 12; errs++ {
				for range 20 {
					f := [33]byte(frame)
					for _, i := range rng.Perm(48)[:errs] {
						pos := layer2.SyncStart + i
						f[pos/8] ^= 0x80 >> (pos % 8)
					}
					var b layer2.Burst
					if err := b.DecodeFromBytesWith(f, &layer2.DecodeOptions{Kind: layer2.KindData}); err != nil {
						t.Fatal(err)
					}
					if b.SyncPattern == enums.SyncNone || b.HasEmbeddedSignalling() {
						t.Fatalf("source %d CC %d, %d SYNC errors: KindData gives %s", src, cc, errs, b.String())
					}
					if err := b.DecodeFromBytes(f); err != nil {
						t.Fatal(err)
					}
					if b.Kind == layer2.KindData && b.SyncPattern == enums.SyncNone {
						t.Fatalf("source %d CC %d, %d SYNC errors: decodes as %s", src, cc, errs, b.String())
					}
				}
			}
		}
	}
}

func TestDataBurstWithEMB_VoiceStaysVoice(t *testing.T) {
	t.Parallel()
	n := 0
	for _, frame := range captureFrames(t) {
		b := must(layer2.NewBurstFromBytes(frame))
		if b.SyncDistance <= layer2.DefaultSyncThreshold {
			continue
		}
		n++
		if b.Kind != layer2.KindVoice {
			t.Errorf("voice burst %x decodes as %s", frame, b.String())
		}
	}
	if n == 0 {
		t.Fatal("no voice bursts with an EMB")
	}

	silence := must(vocoder.EncodeAMBE72(vocoder.AMBESilence()))
	for _, slot := range [][9]byte{silence, {}} {
		var slots [layer2.VoiceSlotsPerBurst][9]byte
		for i := range slots {
			slots[i] = slot
		}
		for e := range 128 {
			var r layer2.RawBurst
			r.SetVoiceSlots(slots)
			noErr(r.SetEMB(pdu.EmbeddedSignalling{ColorCode: enums.ColorCode(e & 15), PreemptionAndPowerControlIndicator: e&16 != 0, LCSS: enums.LCSS(e >> 5)}))
			if b := must(layer2.NewBurstFromBytes(r)); b.Kind != layer2.KindVoice {
				t.Errorf("voice burst %x decodes as %s", r, b.String())
			}
		}
	}

	rng := rand.New(rand.NewPCG(1, 2))
	for range 200000 {
		var r layer2.RawBurst
		for i := range r {
			r[i] = byte(rng.UintN(256))
		}
		v := rng.UintN(128)
		noErr(r.SetEMB(pdu.EmbeddedSignalling{ColorCode: enums.ColorCode(v & 15), PreemptionAndPowerControlIndicator: v&16 != 0, LCSS: enums.LCSS(v >> 5)}))
		var b layer2.Burst
		_ = b.DecodeFromBytes(r)
		if b.Kind == layer2.KindData && b.SyncPattern == enums.SyncNone {
			t.Errorf("random burst %x decodes as %s", r, b.String())
		}
	}
}
