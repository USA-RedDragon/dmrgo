package pdu_test

import (
	"math/rand"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

func TestDecodeFullLinkControlBytes_Capture(t *testing.T) {
	t.Parallel()

	lc := hex9(t, "03000000270630b43c")
	f, err := pdu.DecodeFullLinkControlBytes(lc)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := f.AsUnitToUnit(); f.FLCO != enums.FLCOUnitToUnitVoiceChannelUser || f.FID != enums.FIDStandardized || !ok {
		t.Fatalf("decoded %s", f.String())
	}
	if tgt, src, ok := f.Addresses(); !ok || tgt != 9990 || src != 3191868 {
		t.Fatalf("addresses %d -> %d %t, want 3191868 -> 9990", src, tgt, ok)
	}
	if got := must(f.Bytes()); got != lc {
		t.Fatalf("Bytes %x, want %x", got, lc)
	}
	f.BurstDataType = enums.DataTypeTerminatorWithLC
	enc := must(f.Encode())
	if want := hex12(t, "03000000270630b43c52c0ce"); enc != want {
		t.Fatal("Encode after DecodeFullLinkControlBytes does not give the terminator codeword")
	}
}

func TestDecodeFullLinkControlBytes_RoundTripsRandom(t *testing.T) {
	t.Parallel()

	rng := rand.New(rand.NewSource(7))
	fails := 0
	for range 20000 {
		var lc [9]byte
		rng.Read(lc[:])
		if rng.Intn(2) == 0 {
			lc[1] = 0
		}
		f, err := pdu.DecodeFullLinkControlBytes(lc)
		if err != nil {
			fails++
			continue
		}
		if got := must(f.Bytes()); got != lc {
			t.Fatalf("lc %x: Bytes %x", lc, got)
		}
	}
	if fails != 0 {
		t.Errorf("%d LCs do not round trip", fails)
	}
}

func TestFullLinkControlBytes_IgnoresBurstDataType(t *testing.T) {
	t.Parallel()

	var f pdu.FullLinkControl
	noErr(f.SetGroupVoice(pdu.FLCGroupVoice{GroupAddress: 91, SourceAddress: 3100999}))
	got, err := f.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	if want := hex9(t, "00000000005b2f5147"); got != want {
		t.Fatalf("Bytes %x, want %x", got, want)
	}
}
