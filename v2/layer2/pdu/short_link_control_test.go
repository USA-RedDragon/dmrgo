package pdu_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/fec"
	"github.com/USA-RedDragon/dmrgo/v2/internal/bits"
	"github.com/USA-RedDragon/dmrgo/v2/internal/crc"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

// buildShortLCBits constructs a valid 36-bit Short LC PDU with correct CRC-8.
// slco is the 4-bit SLCO value, data is the 24-bit payload.
func buildShortLCBits(slco byte, data [3]byte) [5]byte {
	var info [5]byte
	bits.SetField(info[:], 0, 4, uint64(slco))
	bits.Copy(info[:], 4, data[:], 0, 24)
	bits.SetField(info[:], 28, 8, uint64(crc.CRC8(info[:], 28)))
	return info
}

func TestShortLC_NullMessage_Decode(t *testing.T) {
	t.Parallel()
	var data [3]byte
	info := buildShortLCBits(byte(enums.SLCONullMessage), data)

	slc := pdu.DecodeShortLC(info)
	fecResult := slc.FEC
	if fecResult.Uncorrectable {
		t.Fatal("unexpected uncorrectable error")
	}
	if slc.SLCO != enums.SLCONullMessage {
		t.Errorf("expected SLCONullMessage, got %d", slc.SLCO)
	}
	if slc.NullMessage == nil {
		t.Fatal("NullMessage should not be nil")
	}
	if slc.ActivityUpdate != nil {
		t.Error("ActivityUpdate should be nil for NullMessage")
	}
}

func TestShortLC_ActivityUpdate_Decode(t *testing.T) {
	t.Parallel()
	// TS1ActivityID=5, TS2ActivityID=3, HashTS1=0xAB, HashTS2=0xCD
	data := [3]byte{0b0101_0011, 0b1010_1011, 0b1100_1101}

	info := buildShortLCBits(byte(enums.SLCOActivityUpdate), data)

	slc := pdu.DecodeShortLC(info)
	fecResult := slc.FEC
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

func TestShortLC_EncodeDecodeRoundTrip(t *testing.T) {
	t.Parallel()
	original := pdu.ShortLC{
		SLCO: enums.SLCOActivityUpdate,
		ActivityUpdate: &pdu.ShortLCActivityUpdate{
			TS1ActivityID: 7,
			TS2ActivityID: 2,
			HashTS1:       0x42,
			HashTS2:       0xFF,
		},
	}

	encoded := must(original.Encode())
	decoded := pdu.DecodeShortLC(encoded)
	fecResult := decoded.FEC
	if fecResult.Uncorrectable {
		t.Fatal("round-trip CRC failed")
	}
	if decoded.SLCO != enums.SLCOActivityUpdate {
		t.Errorf("SLCO mismatch: got %d", decoded.SLCO)
	}
	if decoded.ActivityUpdate == nil {
		t.Fatal("ActivityUpdate is nil after round-trip")
	}
	if decoded.ActivityUpdate.TS1ActivityID != 7 {
		t.Errorf("TS1ActivityID: got %d, want 7", decoded.ActivityUpdate.TS1ActivityID)
	}
	if decoded.ActivityUpdate.TS2ActivityID != 2 {
		t.Errorf("TS2ActivityID: got %d, want 2", decoded.ActivityUpdate.TS2ActivityID)
	}
	if decoded.ActivityUpdate.HashTS1 != 0x42 {
		t.Errorf("HashTS1: got 0x%02X, want 0x42", decoded.ActivityUpdate.HashTS1)
	}
	if decoded.ActivityUpdate.HashTS2 != 0xFF {
		t.Errorf("HashTS2: got 0x%02X, want 0xFF", decoded.ActivityUpdate.HashTS2)
	}
}

func TestShortLC_CRCFailure(t *testing.T) {
	t.Parallel()
	var data [3]byte
	info := buildShortLCBits(byte(enums.SLCONullMessage), data)

	info[3] ^= 0x02

	fecResult := pdu.DecodeShortLC(info).FEC
	if !fecResult.Uncorrectable {
		t.Error("expected uncorrectable after CRC corruption")
	}
}

func TestShortLC_BitsChecked(t *testing.T) {
	t.Parallel()
	var data [3]byte
	info := buildShortLCBits(byte(enums.SLCONullMessage), data)

	fecResult := pdu.DecodeShortLC(info).FEC
	if fecResult != (fec.Result{}) {
		t.Errorf("CRC-8 only PDU: got %+v, want a zero result", fecResult)
	}
}

func TestShortLC_String(t *testing.T) {
	t.Parallel()
	var data [3]byte
	info := buildShortLCBits(byte(enums.SLCOActivityUpdate), data)

	slc := pdu.DecodeShortLC(info)
	s := slc.String()
	if s == "" {
		t.Error("String should not be empty")
	}
}

func TestShortLC_UnknownSLCO_RoundTrip(t *testing.T) {
	t.Parallel()

	var payload [3]byte
	for i := range 24 {
		bits.SetBit(payload[:], i, byte((i*7+3)%2^i%3%2))
	}
	for _, slco := range []byte{0b0101, 0b1111} {
		in := buildShortLCBits(slco, payload)
		slc := pdu.DecodeShortLC(in)
		fecResult := slc.FEC
		if fecResult.Uncorrectable {
			t.Fatalf("SLCO %04b: CRC failed", slco)
		}
		if slc.RawPayload != payload {
			t.Errorf("SLCO %04b: RawPayload = %x, want %x", slco, slc.RawPayload, payload)
		}
		if got := must(slc.Encode()); got != in {
			t.Errorf("SLCO %04b: Encode(Decode(x)) = %v, want %v", slco, got, in)
		}
	}
}
