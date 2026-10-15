package layer2_test

import (
	"bytes"
	"encoding/hex"
	"math/rand"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/fec"
	"github.com/USA-RedDragon/dmrgo/v2/fec/bptc"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/elements"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

// capturedEmbeddedLC is the embedded LC of every superframe in
// testdata/voice.bin and testdata/parrot_kerchunk.bin: UU_V_Ch_Usr from
// 3191868 to 9990.
var capturedEmbeddedLC = []byte{0x03, 0x00, 0x00, 0x00, 0x27, 0x06, 0x30, 0xB4, 0x3C} //nolint:gochecknoglobals

func capturedFragments() [4][32]bit.Bit {
	var fr [4][32]bit.Bit
	for i, word := range [4][4]byte{
		{0x00, 0x00, 0x11, 0x09},
		{0x0F, 0x12, 0x96, 0x96},
		{0x09, 0x0C, 0x35, 0x95},
		{0x84, 0xAA, 0x2B, 0xA5},
	} {
		copy(fr[i][:], bit.UnpackBits(word[:]))
	}
	return fr
}

func capturedLC() pdu.FullLinkControl {
	return pdu.FullLinkControl{
		DataType: elements.DataTypeVoiceLCHeader,
		FLCO:     enums.FLCOUnitToUnitVoiceChannelUser,
		UnitToUnit: &pdu.FLCUnitToUnit{
			TargetAddress: 9990,
			SourceAddress: 3191868,
		},
	}
}

func TestEmbeddedLC_DecodeCapture(t *testing.T) {
	lc, res := layer2.DecodeEmbeddedLC(capturedFragments())
	if res.Uncorrectable || res.ErrorsCorrected != 0 || res.BitsChecked != 128 {
		t.Fatalf("FEC = %+v, want a clean 128-bit check", res)
	}
	if got := bit.PackBits(lc[:]); !bytes.Equal(got, capturedEmbeddedLC) {
		t.Errorf("LC % X, want % X", got, capturedEmbeddedLC)
	}

	flc, res := layer2.DecodeEmbeddedFullLinkControl(capturedFragments())
	if res.Uncorrectable || flc.FEC != res {
		t.Fatalf("FEC = %+v, LC FEC = %+v", res, flc.FEC)
	}
	target, source, ok := flc.Addresses()
	if !ok || flc.FLCO != enums.FLCOUnitToUnitVoiceChannelUser || target != 9990 || source != 3191868 {
		t.Errorf("decoded %s", flc.ToString())
	}
	if flc.DataType != elements.DataTypeVoiceLCHeader {
		t.Errorf("DataType = %v", flc.DataType)
	}
}

func TestEmbeddedLC_EncodeCapture(t *testing.T) {
	lc := capturedLC()
	got, err := layer2.EncodeEmbeddedFullLinkControl(&lc)
	if err != nil {
		t.Fatal(err)
	}
	if got != capturedFragments() {
		t.Errorf("fragments differ from the capture")
	}
	var bits [72]bit.Bit
	copy(bits[:], bit.UnpackBits(capturedEmbeddedLC))
	if layer2.EncodeEmbeddedLC(bits) != capturedFragments() {
		t.Errorf("EncodeEmbeddedLC differs from the capture")
	}
}

func TestEmbeddedLC_RoundTrip(t *testing.T) {
	for seed := 0; seed < 64; seed++ {
		var lc [72]bit.Bit
		for i := range lc {
			lc[i] = bit.Bit(((i*31 + seed*17) >> 3 & 1) ^ (seed >> (i % 6) & 1))
		}
		got, res := layer2.DecodeEmbeddedLC(layer2.EncodeEmbeddedLC(lc))
		if res.HasErrors() || got != lc {
			t.Fatalf("seed %d: got %v (%+v), want %v", seed, got, res, lc)
		}
	}
}

// The Full LC decode of an embedded LC reports the embedded LC FEC result,
// with nothing added by the Full LC decode, and keeps every LC bit.
func TestEmbeddedFullLinkControl_ReportsEmbeddedFEC(t *testing.T) {
	rng := rand.New(rand.NewSource(5))
	for range 2000 {
		var fr [4][32]bit.Bit
		for i := range fr {
			for j := range fr[i] {
				fr[i][j] = bit.Bit(rng.Intn(2))
			}
		}
		if rng.Intn(2) == 0 {
			var lc [72]bit.Bit
			for i := range lc {
				lc[i] = bit.Bit(rng.Intn(2))
			}
			fr = layer2.EncodeEmbeddedLC(lc)
			fr[rng.Intn(4)][rng.Intn(32)] ^= 1
		}
		for _, d := range []struct {
			lc  func([4][32]bit.Bit) ([72]bit.Bit, fec.Result)
			flc func([4][32]bit.Bit) (pdu.FullLinkControl, fec.Result)
		}{
			{layer2.DecodeEmbeddedLC, layer2.DecodeEmbeddedFullLinkControl},
			{layer2.DecodeEmbeddedLCStrict, layer2.DecodeEmbeddedFullLinkControlStrict},
		} {
			lc, want := d.lc(fr)
			flc, got := d.flc(fr)
			if got != want || flc.FEC != want {
				t.Fatalf("FEC = %+v (LC %+v), want %+v", got, flc.FEC, want)
			}
			if want.Uncorrectable {
				continue
			}
			if enc := must(layer2.EncodeEmbeddedFullLinkControl(&flc)); enc != layer2.EncodeEmbeddedLC(lc) {
				t.Fatalf("Full LC %v does not encode back to LC %v", flc.ToString(), lc)
			}
		}
	}
}

// A BPTC codeword whose checksum bits do not match its LC decodes cleanly
// in the BPTC but must be reported as uncorrectable.
func TestEmbeddedLC_ChecksumMismatch(t *testing.T) {
	fr := capturedFragments()
	info, _ := bptc.DecodeEmbeddedLC(fr)
	info[2*11+10] ^= 1
	_, res := layer2.DecodeEmbeddedLC(bptc.EncodeEmbeddedLC(info))
	if !res.Uncorrectable || res.ErrorsCorrected != 0 {
		t.Errorf("FEC = %+v, want uncorrectable from the checksum alone", res)
	}
}

// TestEmbeddedLCStrict_MMDVMHostVectors checks DecodeEmbeddedLCStrict
// against CDMREmbeddedData::decodeEmbeddedData of MMDVMHost 0acb758. Each
// vector is an LC, its 128 transmitted bits with 1 to 6 bit errors, and
// whether MMDVMHost accepts it. MMDVMHost returns the original LC for every
// vector it accepts.
func TestEmbeddedLCStrict_MMDVMHostVectors(t *testing.T) {
	for _, v := range []struct {
		lc, raw string
		accept  bool
	}{
		{"f83e5f0c2adb924331", "e2ebddc09a0f7821505ad46a1e27e396", true},
		{"925d1d73a9ee8c2fd9", "ce665aaa78228e1174f6600fe22d65ca", true},
		{"9b90d5c5ac9d653ef6", "d22d3aaf935ac5afde246a4114bd57e7", false},
		{"e3663d5c125b76552e", "81afcc700a2db2ee4bddc61b3c6f7282", false},
		{"d24b7d1ba315ee12ba", "96ed17c9420bfa5c6ae153b80079db88", false},
		{"f33409806d6be65bff", "cab7ff800b1e8b9f0f539f8e062b1263", true},
		{"aa0dee370cd56406ee", "b34dc30af178c36a3a746c0ffa301bf9", true},
		{"f2781994b12bb527e1", "dbe7bb920a288d3148cfac41ca5c9cee", false},
		{"8189980d3bb8442c89", "984d1b11504136b0d46631e293f53066", false},
		{"9f6c22c5c5f5824e09", "bb5945a9819c8bf014b7d5562c03e8ff", true},
		{"566c7e0bcaa790d92d", "39d241881e8cba4774ebeb9c701de27b", false},
		{"dcb65064daf19ba814", "5d9944f5b48b5c22d418a9880c1d2323", true},
		{"4dafaf174cb0206b4b", "2bfe4142d0f506e1bf63e27af34744a6", false},
		{"7c9cfc9c4ee3f319fc", "48d3e3879afa7b6fdd5972b297c481d8", false},
		{"ddd0e8dc6af86ef892", "cc9c3fb091e774ed9dd1065aac576528", true},
		{"ef60a16604a0460887", "88a587a1a4d381eb36829c878622fc77", true},
		{"7819da4f5c43d39509", "61d284b4df196a2d607e12964be5bedf", false},
		{"7a7e39c1cb0ebf2be4", "56f6eece9a023c534de8bdbc559ca63c", false},
		{"0b5791f52eff3d7fb4", "4e267b6cfa7e95e64cbd16f3979cc62b", true},
		{"b4122f968cfb45c2ae", "faadbacd1ebd036a35505c5c3caa56d1", true},
		{"d0ccb74820a3c2ad83", "a9e276a00d48ad48c7875f6a650a03bd", false},
		{"ec92448480013aa3a7", "d782a241878844229b2a5c93079c4c40", false},
		{"f4a071bfdfdaf92322", "96bba9953ab56d78fa399722236366a5", true},
		{"0d3456555ae86ce17b", "6c15c83396b676a95233ed15c53cb750", true},
	} {
		raw := must(hex.DecodeString(v.raw))
		var fr [4][32]bit.Bit
		for i := range fr {
			copy(fr[i][:], bit.UnpackBits(raw[i*4:i*4+4]))
		}
		lc, res := layer2.DecodeEmbeddedLCStrict(fr)
		if res.Uncorrectable == v.accept {
			t.Errorf("%s: accepted %v, MMDVMHost %v", v.raw, !res.Uncorrectable, v.accept)
			continue
		}
		if got := hex.EncodeToString(bit.PackBits(lc[:])); v.accept && got != v.lc {
			t.Errorf("%s: LC %s, want %s", v.raw, got, v.lc)
		}
		flc, flcRes := layer2.DecodeEmbeddedFullLinkControlStrict(fr)
		if flcRes != res || flc.FEC != res {
			t.Errorf("%s: Full LC FEC %+v, want %+v", v.raw, flcRes, res)
		}
	}
}

// TestEmbeddedLC_FiveErrors checks that the checksum picks between equally
// near codewords: with 5 random bit errors DecodeEmbeddedLC recovers nearly
// every LC and returns almost no wrong LC.
func TestEmbeddedLC_FiveErrors(t *testing.T) {
	r := rand.New(rand.NewSource(5))
	const n = 2000
	recovered, wrong := 0, 0
	for i := 0; i < n; i++ {
		var lc [72]bit.Bit
		for j := range lc {
			lc[j] = bit.Bit(r.Intn(2))
		}
		fr := layer2.EncodeEmbeddedLC(lc)
		for _, p := range r.Perm(128)[:5] {
			fr[p/32][p%32] ^= 1
		}
		got, res := layer2.DecodeEmbeddedLC(fr)
		switch {
		case res.Uncorrectable:
		case got == lc:
			recovered++
		default:
			wrong++
		}
	}
	if recovered < n*98/100 || wrong > 1 {
		t.Errorf("%d of %d recovered, %d wrong", recovered, n, wrong)
	}
}

func TestEmbeddedLC_EncodeRejectsBadLC(t *testing.T) {
	lc := pdu.FullLinkControl{FLCO: enums.FLCO(64)}
	if _, err := layer2.EncodeEmbeddedFullLinkControl(&lc); err == nil {
		t.Fatal("FLCO 64 should be rejected")
	}
}
