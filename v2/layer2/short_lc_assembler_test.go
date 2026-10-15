package layer2_test

import (
	"errors"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/fec"
	"github.com/USA-RedDragon/dmrgo/v2/internal/crc"
	"github.com/USA-RedDragon/dmrgo/v2/internal/fec/bptc"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

// buildValidShortLCFragments constructs 4x17-bit CACH payloads that encode
// a valid Short LC PDU with the given SLCO and 24-bit data payload.
func buildValidShortLCFragments(slco byte, data [24]bit.Bit) [4][17]bit.Bit {
	var bits [36]bit.Bit
	bits[0] = bit.Bit((slco >> 3) & 1)
	bits[1] = bit.Bit((slco >> 2) & 1)
	bits[2] = bit.Bit((slco >> 1) & 1)
	bits[3] = bit.Bit(slco & 1)
	copy(bits[4:28], data[:])
	crcVal := crc.CRC8(bits[:28])
	for i := range 8 {
		bits[28+i] = bit.Bit((crcVal >> (7 - i)) & 1)
	}
	return bptc.EncodeCACH(bits)
}

//nolint:gochecknoglobals
var shortLCSequence = [4]enums.LCSS{
	enums.LCSSFirstFragment,
	enums.LCSSContinuationFragment,
	enums.LCSSContinuationFragment,
	enums.LCSSLastFragment,
}

func addShortLCFragments(t *testing.T, a *layer2.ShortLCAssembler, fragments [4][17]bit.Bit) {
	t.Helper()
	for i := range fragments {
		ready, err := a.AddFragment(shortLCSequence[i], fragments[i])
		if err != nil {
			t.Fatalf("fragment %d: %v", i, err)
		}
		if ready != (i == 3) {
			t.Fatalf("fragment %d: ready=%v", i, ready)
		}
	}
}

// decodeShortLCFragments adds fragments to a new ShortLCAssembler as the
// CACHs First, Continuation, Continuation and Last with Add.
func decodeShortLCFragments(t *testing.T, fragments [4][17]bit.Bit) pdu.ShortLC {
	t.Helper()
	var a layer2.ShortLCAssembler
	for i, f := range fragments {
		slc, done, err := a.Add(layer2.CACH{TACT: pdu.TACT{LCSS: shortLCSequence[i]}, Payload: f})
		if err != nil || done != (i == 3) {
			t.Fatalf("fragment %d: done %t err %v", i, done, err)
		}
		if done {
			return slc
		}
	}
	return pdu.ShortLC{}
}

func TestShortLCAssembler_AddUncorrectableTACT(t *testing.T) {
	t.Parallel()
	var data [24]bit.Bit
	fragments := buildValidShortLCFragments(byte(enums.SLCONullMessage), data)
	var a layer2.ShortLCAssembler
	if _, _, err := a.Add(layer2.CACH{TACT: pdu.TACT{FEC: fec.Result{Uncorrectable: true}}}); err != nil {
		t.Fatalf("uncorrectable TACT without a partial Short LC: %v", err)
	}
	if _, _, err := a.Add(layer2.CACH{TACT: pdu.TACT{LCSS: enums.LCSSFirstFragment}, Payload: fragments[0]}); err != nil {
		t.Fatal(err)
	}
	if _, done, err := a.Add(layer2.CACH{TACT: pdu.TACT{FEC: fec.Result{Uncorrectable: true}}}); done || !errors.Is(err, layer2.ErrShortLCSequence) || a.Count() != 0 {
		t.Fatalf("uncorrectable TACT after a fragment: done %t err %v count %d", done, err, a.Count())
	}
	if _, _, err := a.Add(layer2.CACH{TACT: pdu.TACT{LCSS: enums.LCSSSingleFragment}}); err != nil {
		t.Fatalf("Single fragment without a partial Short LC: %v", err)
	}
}

func TestShortLCAssembler_Lifecycle(t *testing.T) {
	t.Parallel()
	var a layer2.ShortLCAssembler

	if a.Count() != 0 {
		t.Errorf("initial count should be 0, got %d", a.Count())
	}

	var payload [layer2.CACHPayloadBits]bit.Bit
	for i := 0; i < 3; i++ {
		ready, err := a.AddFragment(shortLCSequence[i], payload)
		if err != nil {
			t.Fatalf("fragment %d: %v", i, err)
		}
		if ready {
			t.Errorf("should not be ready after %d fragments", i+1)
		}
		if a.Count() != i+1 {
			t.Errorf("count should be %d, got %d", i+1, a.Count())
		}
	}

	ready, err := a.AddFragment(enums.LCSSLastFragment, payload)
	if err != nil {
		t.Fatal(err)
	}
	if !ready {
		t.Error("should be ready after 4 fragments")
	}
	if a.Count() != 4 {
		t.Errorf("count should be 4, got %d", a.Count())
	}

	a.Reset()
	if a.Count() != 0 {
		t.Errorf("count should be 0 after reset, got %d", a.Count())
	}
	if _, err := a.Complete(); !errors.Is(err, layer2.ErrShortLCIncomplete) {
		t.Errorf("Complete after Reset: got %v, want ErrShortLCIncomplete", err)
	}
}

func TestShortLCAssembler_RejectsBadSequence(t *testing.T) {
	t.Parallel()
	first := enums.LCSSFirstFragment
	cont := enums.LCSSContinuationFragment
	last := enums.LCSSLastFragment
	single := enums.LCSSSingleFragment

	tests := []struct {
		name string
		seq  []enums.LCSS
	}{
		{"ContinuationWithoutFirst", []enums.LCSS{cont}},
		{"LastWithoutFirst", []enums.LCSS{last}},
		{"LastAfterFirst", []enums.LCSS{first, last}},
		{"LastAfterOneContinuation", []enums.LCSS{first, cont, last}},
		{"ThirdContinuation", []enums.LCSS{first, cont, cont, cont}},
		{"SingleFragment", []enums.LCSS{single}},
		{"SingleFragmentMidSequence", []enums.LCSS{first, cont, single}},
		{"ContinuationAfterLast", []enums.LCSS{first, cont, cont, last, cont}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var a layer2.ShortLCAssembler
			var payload [layer2.CACHPayloadBits]bit.Bit
			var err error
			for _, lcss := range tt.seq {
				_, err = a.AddFragment(lcss, payload)
			}
			if !errors.Is(err, layer2.ErrShortLCSequence) {
				t.Fatalf("last AddFragment: got %v, want ErrShortLCSequence", err)
			}
			if a.Count() != 0 {
				t.Errorf("count after bad fragment: got %d, want 0", a.Count())
			}
			if _, err := a.Complete(); !errors.Is(err, layer2.ErrShortLCIncomplete) {
				t.Errorf("Complete: got %v, want ErrShortLCIncomplete", err)
			}
		})
	}
}

func TestShortLCAssembler_FirstRestartsSequence(t *testing.T) {
	t.Parallel()
	var garbage [layer2.CACHPayloadBits]bit.Bit
	for i := range garbage {
		garbage[i] = 1
	}
	var data [24]bit.Bit
	fragments := buildValidShortLCFragments(byte(enums.SLCONullMessage), data)

	var a layer2.ShortLCAssembler
	if _, err := a.AddFragment(enums.LCSSFirstFragment, garbage); err != nil {
		t.Fatal(err)
	}
	if _, err := a.AddFragment(enums.LCSSContinuationFragment, garbage); err != nil {
		t.Fatal(err)
	}
	addShortLCFragments(t, &a, fragments)

	slc, err := a.Complete()
	fecResult := slc.FEC
	if err != nil {
		t.Fatal(err)
	}
	if fecResult.Uncorrectable || fecResult.ErrorsCorrected != 0 {
		t.Fatalf("unexpected FEC result %+v", fecResult)
	}
	if slc.SLCO != enums.SLCONullMessage {
		t.Errorf("expected SLCONullMessage, got %d", slc.SLCO)
	}
}

func TestShortLCAssembler_CompleteIncomplete(t *testing.T) {
	t.Parallel()
	var a layer2.ShortLCAssembler
	var payload [layer2.CACHPayloadBits]bit.Bit
	for i := 0; i < 3; i++ {
		if _, err := a.AddFragment(shortLCSequence[i], payload); err != nil {
			t.Fatal(err)
		}
		if _, err := a.Complete(); !errors.Is(err, layer2.ErrShortLCIncomplete) {
			t.Errorf("Complete with %d fragments: got %v, want ErrShortLCIncomplete", i+1, err)
		}
	}
}

func TestShortLCAssembler_Complete_NullMessage(t *testing.T) {
	t.Parallel()
	var data [24]bit.Bit
	fragments := buildValidShortLCFragments(byte(enums.SLCONullMessage), data)

	var a layer2.ShortLCAssembler
	addShortLCFragments(t, &a, fragments)

	slc, err := a.Complete()
	fecResult := slc.FEC
	if err != nil {
		t.Fatal(err)
	}
	if fecResult.Uncorrectable {
		t.Fatal("unexpected uncorrectable error")
	}
	if slc.SLCO != enums.SLCONullMessage {
		t.Errorf("expected SLCONullMessage, got %d", slc.SLCO)
	}
	if slc.NullMessage == nil {
		t.Error("NullMessage should not be nil")
	}
}

func TestShortLCAssembler_Complete_ActivityUpdate(t *testing.T) {
	t.Parallel()
	var data [24]bit.Bit
	data[0], data[1], data[2], data[3] = 0, 1, 0, 1
	data[4], data[5], data[6], data[7] = 0, 0, 1, 1
	data[8], data[9], data[10], data[11] = 1, 0, 1, 0
	data[12], data[13], data[14], data[15] = 1, 0, 1, 1
	data[16], data[17], data[18], data[19] = 1, 1, 0, 0
	data[20], data[21], data[22], data[23] = 1, 1, 0, 1

	fragments := buildValidShortLCFragments(byte(enums.SLCOActivityUpdate), data)

	var a layer2.ShortLCAssembler
	addShortLCFragments(t, &a, fragments)

	slc, err := a.Complete()
	fecResult := slc.FEC
	if err != nil {
		t.Fatal(err)
	}
	if fecResult.Uncorrectable {
		t.Fatal("unexpected uncorrectable error")
	}
	if slc.SLCO != enums.SLCOActivityUpdate {
		t.Errorf("expected SLCOActivityUpdate, got %d", slc.SLCO)
	}
	if slc.ActivityUpdate == nil {
		t.Fatal("ActivityUpdate should not be nil")
	}
	if slc.ActivityUpdate.TS1ActivityID != 5 {
		t.Errorf("expected TS1ActivityID=5, got %d", slc.ActivityUpdate.TS1ActivityID)
	}
	if slc.ActivityUpdate.TS2ActivityID != 3 {
		t.Errorf("expected TS2ActivityID=3, got %d", slc.ActivityUpdate.TS2ActivityID)
	}
	if slc.ActivityUpdate.HashTS1 != 0xAB {
		t.Errorf("expected HashTS1=0xAB, got 0x%02X", slc.ActivityUpdate.HashTS1)
	}
	if slc.ActivityUpdate.HashTS2 != 0xCD {
		t.Errorf("expected HashTS2=0xCD, got 0x%02X", slc.ActivityUpdate.HashTS2)
	}
}

func TestShortLCAssembler_Add(t *testing.T) {
	t.Parallel()
	var data [24]bit.Bit
	fragments := buildValidShortLCFragments(byte(enums.SLCONullMessage), data)

	slc := decodeShortLCFragments(t, fragments)
	fecResult := slc.FEC
	if fecResult.Uncorrectable {
		t.Fatal("unexpected uncorrectable error")
	}
	if slc.SLCO != enums.SLCONullMessage {
		t.Errorf("expected SLCONullMessage, got %d", slc.SLCO)
	}
}

func TestShortLCAssembler_RoundTrip_EncodeShortLC(t *testing.T) {
	t.Parallel()
	original := pdu.ShortLC{
		SLCO: enums.SLCOActivityUpdate,
		ActivityUpdate: &pdu.ShortLCActivityUpdate{
			TS1ActivityID: 15,
			TS2ActivityID: 8,
			HashTS1:       0x12,
			HashTS2:       0x34,
		},
	}
	encoded := must(original.Encode())
	fragments := bptc.EncodeCACH(encoded)

	slc := decodeShortLCFragments(t, fragments)
	fecResult := slc.FEC
	if fecResult.Uncorrectable {
		t.Fatal("round-trip failed: uncorrectable")
	}
	if slc.ActivityUpdate == nil {
		t.Fatal("ActivityUpdate is nil after round-trip")
	}
	if slc.ActivityUpdate.TS1ActivityID != 15 {
		t.Errorf("TS1ActivityID: got %d, want 15", slc.ActivityUpdate.TS1ActivityID)
	}
	if slc.ActivityUpdate.TS2ActivityID != 8 {
		t.Errorf("TS2ActivityID: got %d, want 8", slc.ActivityUpdate.TS2ActivityID)
	}
	if slc.ActivityUpdate.HashTS1 != 0x12 {
		t.Errorf("HashTS1: got 0x%02X, want 0x12", slc.ActivityUpdate.HashTS1)
	}
	if slc.ActivityUpdate.HashTS2 != 0x34 {
		t.Errorf("HashTS2: got 0x%02X, want 0x34", slc.ActivityUpdate.HashTS2)
	}
}

func TestShortLCAssembler_CombinedFEC(t *testing.T) {
	t.Parallel()
	var data [24]bit.Bit
	fragments := buildValidShortLCFragments(byte(enums.SLCONullMessage), data)

	slc := decodeShortLCFragments(t, fragments)
	fecResult := slc.FEC
	// The CRC-8 only detects errors, so BitsChecked is the BPTC's 68.
	expectedBitsChecked := 68
	if fecResult.BitsChecked != expectedBitsChecked {
		t.Errorf("BitsChecked: got %d, want %d", fecResult.BitsChecked, expectedBitsChecked)
	}
	if slc.FEC.BitsChecked != fecResult.BitsChecked {
		t.Errorf("ShortLC.FEC.BitsChecked mismatch: got %d, want %d", slc.FEC.BitsChecked, fecResult.BitsChecked)
	}
}
