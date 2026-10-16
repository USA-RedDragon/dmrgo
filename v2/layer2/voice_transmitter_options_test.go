package layer2_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/layer3"
)

func talkerAliasLC() pdu.FullLinkControl {
	var data [48]bit.Bit
	copy(data[:], bit.Unpack([]byte("KI5VMF")))
	return pdu.FullLinkControl{
		FLCO: enums.FLCOTalkerAliasHeader,
		FID:  enums.FIDStandardized,
		TalkerAliasHeader: &pdu.FLCTalkerAliasHeader{
			TalkerAliasDataFormat: layer3.TalkerAliasISOEightBit,
			TalkerAliasDataLength: 6,
			TalkerAliasData:       bit.Uint(data[:]),
		},
	}
}

func superframeLC(t *testing.T, bursts []layer2.RawBurst, sf int) [4][32]bit.Bit {
	t.Helper()
	var frags [4][32]bit.Bit
	for i := range frags {
		b := must(layer2.NewBurstFromBytes(bursts[sf*6+1+i]))
		frags[i] = b.EmbeddedSignallingData
	}
	return frags
}

func TestVoiceTransmitter_EmbeddedLCRotation(t *testing.T) {
	t.Parallel()

	call := groupLC(enums.FIDStandardized)
	alias := talkerAliasLC()
	var calls []int
	tx := must(layer2.NewVoiceTransmitter(&layer2.VoiceTransmitterOptions{
		LC: call, ColorCode: 2, Source: layer2.SyncSourceMS, Silence: silenceSlot(),
		EmbeddedLC: func(sf int) *pdu.FullLinkControl {
			calls = append(calls, sf)
			if sf%2 == 1 {
				return &alias
			}
			return nil
		},
	}))
	var bursts []layer2.RawBurst
	for n := 0; n < 4*18; n++ {
		bursts = append(bursts, must(tx.AddSlots(testSlot(n)))...)
	}
	if len(calls) != 4 || calls[0] != 0 || calls[3] != 3 {
		t.Fatalf("hook calls %v", calls)
	}
	callFrags := must(encodeFLCBits(&call))
	aliasFrags := must(encodeFLCBits(&alias))
	for sf := 0; sf < 4; sf++ {
		want := callFrags
		if sf%2 == 1 {
			want = aliasFrags
		}
		got := superframeLC(t, bursts, sf)
		if got != want {
			t.Errorf("superframe %d: wrong embedded LC", sf)
		}
		lc, res := decodeFLCBits(got)
		if res.Uncorrectable || (sf%2 == 1) != (lc.FLCO == enums.FLCOTalkerAliasHeader) {
			t.Errorf("superframe %d: decoded %s", sf, lc.String())
		}
	}

	if err := tx.SetEmbeddedLC(alias); err != nil {
		t.Fatal(err)
	}
	for n := 0; n < 2*18; n++ {
		bursts = append(bursts, must(tx.AddSlots(testSlot(n)))...)
	}
	if superframeLC(t, bursts, 4) != aliasFrags || superframeLC(t, bursts, 5) != aliasFrags {
		t.Error("SetEmbeddedLC superframe or the hook superframe after it is wrong")
	}
	if len(calls) != 5 || calls[4] != 5 {
		t.Errorf("hook calls %v", calls)
	}
	bad := call
	bad.FLCO = enums.FLCO(64)
	if err := tx.SetEmbeddedLC(bad); err == nil {
		t.Error("SetEmbeddedLC accepted FLCO 64")
	}
}

func TestVoiceTransmitter_SetEmbeddedFragments(t *testing.T) {
	t.Parallel()

	call := groupLC(enums.FIDStandardized)
	tx := must(layer2.NewVoiceTransmitter(&layer2.VoiceTransmitterOptions{LC: call, ColorCode: 2, Source: layer2.SyncSourceMS, Silence: silenceSlot()}))
	f := layer2.EncodeEmbeddedLC([9]byte{1, 2, 3, 4, 5, 6, 7, 8, 9})
	if err := tx.SetEmbeddedFragments(f); err != nil {
		t.Fatal(err)
	}
	var bursts []layer2.RawBurst
	for n := 0; n < 2*18; n++ {
		bursts = append(bursts, must(tx.AddSlots(testSlot(n)))...)
	}
	if superframeLC(t, bursts, 0) != fragBits(f) {
		t.Error("the fragments were not passed through")
	}
	if superframeLC(t, bursts, 1) != must(encodeFLCBits(&call)) {
		t.Error("the next superframe does not carry the call LC")
	}
	for i, raw := range bursts[1:6] {
		pos := enums.VoiceBurstB + enums.VoiceBurstType(i)
		want, ok := pos.LCSS()
		if b := must(layer2.NewBurstFromBytes(raw)); !ok || b.EmbeddedSignalling.LCSS != want {
			t.Errorf("burst %s: LCSS %s, VoiceBurstType.LCSS %s", pos, b.EmbeddedSignalling.LCSS, want)
		}
	}
	for _, pos := range []enums.VoiceBurstType{enums.VoiceBurstUnknown, enums.VoiceBurstA} {
		if _, ok := pos.LCSS(); ok {
			t.Errorf("(%s).LCSS() is ok", pos)
		}
	}
	tail := must(tx.Terminate())
	if len(tail) == 0 || tx.SetEmbeddedFragments(f) == nil {
		t.Error("SetEmbeddedFragments after Terminate succeeded")
	}
}

func TestVoiceTransmitter_EmbeddedLCHookError(t *testing.T) {
	t.Parallel()

	bad := groupLC(enums.FIDStandardized)
	bad.FLCO = enums.FLCO(64)
	fail := true
	tx := must(layer2.NewVoiceTransmitter(&layer2.VoiceTransmitterOptions{
		LC: groupLC(enums.FIDStandardized), ColorCode: 1, Source: layer2.SyncSourceBS, Silence: silenceSlot(),
		EmbeddedLC: func(int) *pdu.FullLinkControl {
			if fail {
				return &bad
			}
			return nil
		},
	}))
	if _, err := tx.AddSlots(testSlot(0), testSlot(1), testSlot(2)); err == nil {
		t.Fatal("hook LC that does not encode accepted")
	}
	fail = false
	out := must(tx.AddSlots(testSlot(0), testSlot(1), testSlot(2)))
	if len(out) != 1 {
		t.Fatalf("%d bursts after the error", len(out))
	}
	if b := must(layer2.NewBurstFromBytes(out[0])); b.VoiceBurst != enums.VoiceBurstA {
		t.Errorf("burst after the error is %s", b.VoiceBurst.String())
	}
}

func TestVoiceTransmitter_PIHeader(t *testing.T) {
	t.Parallel()

	lc := groupLC(enums.FIDStandardized)
	pi := pdu.PIHeader{AlgorithmID: 1, MFID: 0x10, KeyID: 7, MessageIndicator: 0xDEADBEEF, Destination: 91}
	sources := map[layer2.SyncSource]enums.SyncPattern{
		layer2.SyncSourceBS:        enums.SyncBSSourcedData,
		layer2.SyncSourceMS:        enums.SyncMSSourcedData,
		layer2.SyncSourceDirectTS1: enums.SyncTDMA1Data,
		layer2.SyncSourceDirectTS2: enums.SyncTDMA2Data,
	}
	for source, sync := range sources {
		tx := must(layer2.NewVoiceTransmitter(&layer2.VoiceTransmitterOptions{
			LC: lc, ColorCode: 5, Source: source, Silence: silenceSlot(), PIHeader: &pi,
		}))
		headers := tx.Headers()
		if len(headers) != 2 || headers[0] != tx.Header() {
			t.Fatalf("source %d: %d headers", source, len(headers))
		}
		if p, ok := tx.PIHeader(); !ok || p != headers[1] {
			t.Errorf("source %d: PIHeader is not the second header", source)
		}
		checkLCBurst(t, headers[0], lc, enums.DataTypeVoiceLCHeader, 5, sync)
		b := must(layer2.NewBurstFromBytes(headers[1]))
		got, ok := b.Data.(*pdu.PIHeader)
		if !ok || b.Kind != layer2.KindData || b.SyncPattern != sync || b.SlotType.DataType != enums.DataTypePIHeader || b.SlotType.ColorCode != 5 {
			t.Fatalf("source %d: PI burst %s", source, b.String())
		}
		if got.FEC.HasErrors() || got.AlgorithmID != 1 || got.MFID != 0x10 || got.KeyID != 7 || got.MessageIndicator != 0xDEADBEEF || got.Destination != 91 {
			t.Errorf("source %d: PI header %s", source, got.String())
		}
	}
	tx := must(layer2.NewVoiceTransmitter(&layer2.VoiceTransmitterOptions{LC: lc, Silence: silenceSlot()}))
	if h := tx.Headers(); len(h) != 1 || h[0] != tx.Header() {
		t.Errorf("no PI header: %d headers", len(h))
	}
	if _, ok := tx.PIHeader(); ok {
		t.Error("no PI header: PIHeader reports one")
	}
	pi.Destination = 1 << 24
	if _, err := layer2.NewVoiceTransmitter(&layer2.VoiceTransmitterOptions{LC: lc, PIHeader: &pi}); err == nil {
		t.Error("PI header destination 1<<24 accepted")
	}
}

func TestVoiceTransmitter_OptionsMatchPositional(t *testing.T) {
	t.Parallel()

	lc := groupLC(enums.FIDStandardized)
	for _, source := range []layer2.SyncSource{layer2.SyncSourceBS, layer2.SyncSourceMS, layer2.SyncSourceDirectTS1, layer2.SyncSourceDirectTS2} {
		a := must(layer2.NewVoiceTransmitter(&layer2.VoiceTransmitterOptions{LC: lc, ColorCode: 3, Source: source, Silence: silenceSlot()}))
		b := must(layer2.NewVoiceTransmitter(&layer2.VoiceTransmitterOptions{
			LC: lc, ColorCode: 3, Source: source, Silence: silenceSlot(),
			EmbeddedLC: func(int) *pdu.FullLinkControl { return nil },
		}))
		got := []layer2.RawBurst{a.Header()}
		want := []layer2.RawBurst{b.Header()}
		for n := 0; n < 40; n++ {
			got = append(got, must(a.AddSlots(testSlot(n)))...)
			want = append(want, must(b.AddSlots(testSlot(n)))...)
		}
		got = append(got, must(a.Terminate())...)
		want = append(want, must(b.Terminate())...)
		if len(got) != len(want) {
			t.Fatalf("source %d: %d bursts, want %d", source, len(got), len(want))
		}
		for i := range got {
			if got[i] != want[i] {
				t.Errorf("source %d burst %d differs", source, i)
			}
		}
	}
	if _, err := layer2.NewVoiceTransmitter(&layer2.VoiceTransmitterOptions{LC: lc, Source: layer2.SyncSource(4)}); err == nil {
		t.Error("sync source 4 accepted")
	}
	zero := must(layer2.NewVoiceTransmitter(&layer2.VoiceTransmitterOptions{}))
	if nilOpts := must(layer2.NewVoiceTransmitter(nil)); nilOpts.Header() != zero.Header() {
		t.Error("nil options differ from the zero options")
	}
}
