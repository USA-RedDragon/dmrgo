package pdu_test

import (
	"encoding/hex"
	"errors"
	"math/rand"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/layer3"
)

func TestLCBytes_Capture(t *testing.T) {
	t.Parallel()

	l := pdu.LCBytes(hex9(t, "03000000270630b43c"))
	if g, _, _, ok := l.Call(); l.FLCO() != enums.FLCOUnitToUnitVoiceChannelUser || l.FID() != enums.FIDStandardized || !soOK(l, layer3.ServiceOptions{}) || g || !ok || l.ProtectFlag() {
		t.Fatalf("fields of %x", l)
	}
	tgt, src, ok := l.Addresses()
	if !ok || tgt != 9990 || src != 3191868 {
		t.Fatalf("Addresses %d -> %d %t, want 3191868 -> 9990", src, tgt, ok)
	}

	if l.SetGroup(true) != nil || l.SetAddresses(91, 3100999) != nil {
		t.Fatal("edit refused")
	}
	if got := hex.EncodeToString(l[:]); got != "00000000005b2f5147" {
		t.Fatalf("edited LC %s", got)
	}
	f := pdu.DecodeFullLinkControlBytes(l)
	if _, isGroup := f.AsGroupVoice(); !isGroup {
		t.Fatalf("decoded %s", f.String())
	}
	if tgt, src, _ := f.Addresses(); tgt != 91 || src != 3100999 {
		t.Fatalf("decoded %s", f.String())
	}
	cw := must(l.Codeword(enums.DataTypeVoiceLCHeader))
	if !pdu.FullLCParityOK(cw, enums.DataTypeVoiceLCHeader) {
		t.Fatal("codeword of the edited LC fails its parity")
	}

	orig := pdu.LCBytes(hex9(t, "03000000270630b43c"))
	if back := pdu.LCBytes([9]byte(cw[:9])); back.SetGroup(false) != nil || back.SetAddresses(9990, 3191868) != nil || back != orig {
		t.Fatalf("edit back gives %x", back)
	}
	vlc, tlc := must(orig.Codeword(enums.DataTypeVoiceLCHeader)), must(orig.Codeword(enums.DataTypeTerminatorWithLC))
	if hex.EncodeToString(vlc[9:]) != "5dcfc1" || hex.EncodeToString(tlc[9:]) != "52c0ce" {
		t.Fatalf("parity %x %x", vlc[9:], tlc[9:])
	}
}

func TestLCBytes_FlagBits(t *testing.T) {
	t.Parallel()

	rng := rand.New(rand.NewSource(13))
	for range 1000 {
		var raw [9]byte
		rng.Read(raw[:])
		orig := pdu.LCBytes(raw)
		for _, v := range []bool{false, true} {
			l := orig
			l.SetProtectFlag(v)
			if l.ProtectFlag() != v || l.Reserved() != orig.Reserved() || !within(diffMask(l, orig), [9]byte{0x80}) {
				t.Fatalf("%x: SetProtectFlag(%t) gives %x", orig, v, l)
			}
			l = orig
			l.SetReserved(v)
			if l.Reserved() != v || l.ProtectFlag() != orig.ProtectFlag() || !within(diffMask(l, orig), [9]byte{0x40}) {
				t.Fatalf("%x: SetReserved(%t) gives %x", orig, v, l)
			}
		}
	}
}

func TestLCBytes_ClearReservedOnFLCOChange(t *testing.T) {
	t.Parallel()

	l := pdu.LCBytes{0xC0, 0, 0x20, 0, 0, 9, 0, 0, 1}
	if err := l.SetFLCO(enums.FLCOUnitToUnitVoiceChannelUser); err != nil || l[0] != 0xC3 || !l.Reserved() || !l.ProtectFlag() {
		t.Fatalf("SetFLCO gives octet 0 %#02x", l[0])
	}
	l.SetReserved(false)
	if l[0] != 0x83 || l.Reserved() {
		t.Fatalf("SetReserved(false) gives octet 0 %#02x", l[0])
	}
	l.SetProtectFlag(false)
	if l[0] != 0x03 || l.ProtectFlag() || l.FLCO() != enums.FLCOUnitToUnitVoiceChannelUser {
		t.Fatalf("SetProtectFlag(false) gives octet 0 %#02x", l[0])
	}
}

func diffMask(a, b [9]byte) [9]byte {
	var d [9]byte
	for i := range a {
		d[i] = a[i] ^ b[i]
	}
	return d
}

func within(d, allowed [9]byte) bool {
	for i := range d {
		if d[i]&^allowed[i] != 0 {
			return false
		}
	}
	return true
}

func TestLCBytes_SetFLCORange(t *testing.T) {
	t.Parallel()

	orig := pdu.LCBytes{0xC3, 1, 2, 3, 4, 5, 6, 7, 8}
	for _, flco := range []enums.FLCO{64, 0x80, 0xFF} {
		l := orig
		if err := l.SetFLCO(flco); !isRangeError(err, "LCBytes", "FLCO", 6) || l != orig {
			t.Errorf("SetFLCO(%d) gives %x, %v", flco, l, err)
		}
	}
}

func TestLCBytes_EditsKeepOtherBits(t *testing.T) {
	t.Parallel()

	flcoBits := [9]byte{0x3F}
	fidBits := [9]byte{1: 0xFF}
	soBits := [9]byte{2: 0xFF}
	addrBits := [9]byte{3: 0xFF, 4: 0xFF, 5: 0xFF, 6: 0xFF, 7: 0xFF, 8: 0xFF}
	rng := rand.New(rand.NewSource(11))
	for range 5000 {
		var raw [9]byte
		rng.Read(raw[:])
		if rng.Intn(2) == 0 {
			raw[0] = raw[0]&0xC0 | []byte{0, 3}[rng.Intn(2)]
		}
		orig := pdu.LCBytes(raw)
		voice := orig.FLCO() == enums.FLCOGroupVoiceChannelUser || orig.FLCO() == enums.FLCOUnitToUnitVoiceChannelUser

		l := orig
		flco := enums.FLCO(rng.Intn(64))
		if err := l.SetFLCO(flco); err != nil || l.FLCO() != flco || !within(diffMask(l, orig), flcoBits) {
			t.Fatalf("%x: SetFLCO changed %x", orig, diffMask(l, orig))
		}
		l = orig
		fid := enums.FID(rng.Intn(256))
		l.SetFID(fid)
		if l.FID() != fid || !within(diffMask(l, orig), fidBits) {
			t.Fatalf("%x: SetFID", orig)
		}
		l = orig
		so := layer3.DecodeServiceOptions([1]byte{byte(rng.Intn(256))})
		err := l.SetServiceOptions(so)
		got, ok := l.ServiceOptions()
		if ok != (err == nil) || ok && got != so || !within(diffMask(l, orig), soBits) || !ok && l != orig {
			t.Fatalf("%x: SetServiceOptions %v", orig, err)
		}
		if !ok && !errors.Is(err, pdu.ErrNoServiceOptions) {
			t.Fatalf("%x: SetServiceOptions %v, want ErrNoServiceOptions", orig, err)
		}

		l = orig
		tgt, src := uint32(rng.Intn(1<<24)), uint32(rng.Intn(1<<24))
		if (l.SetAddresses(tgt, src) == nil) != voice {
			t.Fatalf("%x: SetAddresses ok %t for FLCO %d", orig, !voice, orig.FLCO())
		}
		if !voice {
			if l != orig {
				t.Fatalf("%x: refused SetAddresses changed the LC", orig)
			}
			if _, _, ok := l.Addresses(); ok {
				t.Fatalf("%x: Addresses ok for FLCO %d", orig, orig.FLCO())
			}
			if err := l.SetGroup(true); !errors.Is(err, pdu.ErrNoAddresses) || l != orig {
				t.Fatalf("%x: SetGroup accepted FLCO %d", orig, orig.FLCO())
			}
			continue
		}
		if gt, gs, ok := l.Addresses(); !ok || gt != tgt || gs != src || !within(diffMask(l, orig), addrBits) {
			t.Fatalf("%x: SetAddresses %d %d gives %x", orig, tgt, src, l)
		}
		if l.SetAddresses(1<<24, 0) == nil || l.SetAddresses(0, 1<<24) == nil {
			t.Fatalf("%x: SetAddresses accepts 25 bits", orig)
		}

		want := pdu.DecodeFullLinkControlBytes(orig)
		noErr(want.SetAddresses(tgt, src))
		if must(want.Bytes()) != [9]byte(l) {
			t.Fatalf("%x: LCBytes.SetAddresses %x, FullLinkControl.SetAddresses %x", orig, l, must(want.Bytes()))
		}

		l = orig
		g := rng.Intn(2) == 0
		set := l.SetGroup(g)
		if got, _, _, ok := l.Call(); set != nil || !ok || got != g || !within(diffMask(l, orig), flcoBits) {
			t.Fatalf("%x: SetGroup(%t) gives %x", orig, g, l)
		}
	}
}

func TestNewVoiceLC(t *testing.T) {
	t.Parallel()

	for _, group := range []bool{true, false} {
		so := layer3.ServiceOptions{IsPrivacy: true, PriorityLevel: 2}
		f := must(pdu.NewVoiceLC(group, 91, 3191868, so))
		_, isGroup := f.AsGroupVoice()
		_, isUnit := f.AsUnitToUnit()
		if f.FID != enums.FIDStandardized || f.BurstDataType != enums.DataTypeVoiceLCHeader || isGroup != group || isUnit == group {
			t.Fatalf("group %t: %s", group, f.String())
		}
		if tgt, src, ok := f.Addresses(); !ok || tgt != 91 || src != 3191868 {
			t.Errorf("group %t: Addresses %d %d %t", group, tgt, src, ok)
		}
		if got, ok := f.ServiceOptions(); !ok || got != so {
			t.Errorf("group %t: ServiceOptions %s %t", group, got.String(), ok)
		}
		l := must(pdu.NewVoiceLCBytes(group, 91, 3191868, so))
		if b := must(f.Bytes()); b != [9]byte(l) {
			t.Errorf("group %t: NewVoiceLCBytes %x, NewVoiceLC.Bytes %x", group, l, b)
		}
		if g, _, _, ok := l.Call(); !ok || g != group || !soOK(l, so) {
			t.Errorf("group %t: LCBytes %x", group, l)
		}
		if g, tgt, src, ok := f.Call(); !ok || g != group || tgt != 91 || src != 3191868 {
			t.Errorf("group %t: LCBytes %x", group, l)
		}
	}
	var rangeErr *dmrgo.FieldRangeError
	if _, err := pdu.NewVoiceLC(true, 1<<24, 1, layer3.ServiceOptions{}); !errors.As(err, &rangeErr) {
		t.Errorf("target 1<<24: %v", err)
	}
	if _, err := pdu.NewVoiceLCBytes(false, 1, 1<<24, layer3.ServiceOptions{}); !errors.As(err, &rangeErr) {
		t.Errorf("source 1<<24: %v", err)
	}
	raw := pdu.FullLinkControl{FLCO: enums.FLCOGroupVoiceChannelUser, FID: 0x10, RawPayload: [7]byte{0x20}}
	if so, ok := raw.ServiceOptions(); !ok || so != layer3.MotorolaServiceOptions() {
		t.Errorf("manufacturer FID: ServiceOptions %s %t", so.String(), ok)
	}
	raw.FLCO = enums.FLCOGPSInfo
	if _, ok := raw.ServiceOptions(); ok {
		t.Error("GPS LC has Service Options")
	}
}

func soOK(l pdu.LCBytes, want layer3.ServiceOptions) bool {
	got, ok := l.ServiceOptions()
	return ok && got == want
}

func TestLCBytes_SetServiceOptionsRange(t *testing.T) {
	t.Parallel()

	l := must(pdu.NewVoiceLCBytes(true, 91, 1, layer3.ServiceOptions{}))
	orig := l
	var rangeErr *dmrgo.FieldRangeError
	if err := l.SetServiceOptions(layer3.ServiceOptions{Reserved: 4}); !errors.As(err, &rangeErr) || l != orig {
		t.Errorf("Reserved 4: %v, %x", err, l)
	}
	var zero pdu.LCBytes
	zero[0] = byte(enums.FLCOGPSInfo)
	if err := zero.SetServiceOptions(layer3.ServiceOptions{}); !errors.Is(err, pdu.ErrNoServiceOptions) {
		t.Errorf("GPS LC: %v", err)
	}
	var nilLC *pdu.LCBytes
	if err := nilLC.SetServiceOptions(layer3.ServiceOptions{}); !errors.Is(err, pdu.ErrNoServiceOptions) {
		t.Errorf("nil LCBytes: %v", err)
	}
	if err := l.SetServiceOptions(layer3.ServiceOptions{IsEmergency: true}); err != nil || !soOK(l, layer3.ServiceOptions{IsEmergency: true}) {
		t.Errorf("emergency: %v, %x", err, l)
	}
}

func TestLCBytes_CallOtherFLCO(t *testing.T) {
	t.Parallel()

	l := pdu.LCBytes{byte(enums.FLCOGPSInfo)}
	if g, _, _, ok := l.Call(); g || ok {
		t.Errorf("GPS LC: Call %t %t", g, ok)
	}
	if err := l.SetGroup(true); !errors.Is(err, pdu.ErrNoAddresses) {
		t.Errorf("GPS LC: SetGroup %v", err)
	}
}

func TestLCBytes_Call(t *testing.T) {
	t.Parallel()

	l := pdu.LCBytes(hex9(t, "c3100000270630b43c"))
	if g, tg, src, ok := l.Call(); g || tg != 0x2706 || src != 0x30b43c || !ok {
		t.Fatalf("Call of %x: %v %d %d %v", l, g, tg, src, ok)
	}
	if err := l.SetCall(true, 91, 3120001); err != nil || l[0] != 0xC0 || l[1] != 0x10 {
		t.Fatalf("SetCall: %v %x", err, l)
	}
	if g, tg, src, ok := l.Call(); !g || tg != 91 || src != 3120001 || !ok {
		t.Fatalf("Call after SetCall: %v %d %d %v", g, tg, src, ok)
	}
	want := l
	var fre *dmrgo.FieldRangeError
	if err := l.SetCall(false, 1<<24, 1); !errors.As(err, &fre) || l != want {
		t.Fatalf("target over 24 bits: %v %x", err, l)
	}
	if err := l.SetCall(false, 1, 1<<24); !errors.As(err, &fre) || l != want {
		t.Fatalf("source over 24 bits: %v %x", err, l)
	}
	other := pdu.LCBytes{byte(enums.FLCOTerminatorDataLinkControl)}
	if g, tg, src, ok := other.Call(); g || tg != 0 || src != 0 || ok {
		t.Fatalf("Call of another FLCO: %v %d %d %v", g, tg, src, ok)
	}
}
