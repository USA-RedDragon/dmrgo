package layer2_test

import (
	"encoding/hex"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

// Capacity Max bursts from sdrtrunk recordings of a Motorola Capacity Max
// control channel (CC 6). They decode with 0 FEC corrections and R(2)=1.
// The CSBK CRC fails because RAS replaces it with a keyed check.
const (
	capMaxCSBKAloha = "0226983139d852c195e80d7018edff57d75df5da04188040c1588182010209e53a"
	capMaxCSBK      = "00429eb534f051e9a4b84b3018edff57d75df5da047981c0e2a881860cca09f528"
	// capMaxVoiceLCHeader has 0 BPTC corrections and 1 corrected slot type
	// bit.
	capMaxVoiceLCHeader = "09089bea12c010685560cb80984dff57d75df5daff18283015700b410413544161"
)

func hexBurst(t *testing.T, s string) [33]byte {
	t.Helper()
	b, err := hex.DecodeString(s)
	if err != nil || len(b) != 33 {
		t.Fatalf("bad burst hex %q: %v", s, err)
	}
	return [33]byte(b)
}

func TestBPTCReserved_CapacityMaxRoundTrip(t *testing.T) {
	t.Parallel()
	ras := [3]bit.Bit{0, 0, 1}
	for _, s := range []string{capMaxCSBKAloha, capMaxCSBK} {
		frame := hexBurst(t, s)
		var b layer2.Burst
		if err := b.DecodeFromBytes(frame); err != nil {
			t.Fatal(err)
		}
		if b.Kind != layer2.KindData || b.SlotType.DataType != enums.DataTypeCSBK {
			t.Fatalf("%s: got %s %s, want a CSBK", s, b.Kind, b.SlotType.DataType)
		}
		if b.FEC.Payload.ErrorsCorrected != 0 || b.FEC.Payload.Uncorrectable {
			t.Fatalf("%s: BPTC FEC %+v, want clean", s, b.FEC.Payload)
		}
		if b.BPTCReserved != ras {
			t.Errorf("%s: BPTCReserved = %v, want %v", s, b.BPTCReserved, ras)
		}
		if got := must(b.Encode()); got != frame {
			t.Errorf("%s: Encode\ngot  %x\nwant %x", s, got, frame)
		}

		r := layer2.RawBurst(frame)
		res, fr := r.BPTCReserved()
		if res != ras || fr.ErrorsCorrected != 0 || fr.Uncorrectable {
			t.Errorf("%s: RawBurst.BPTCReserved = %v %+v, want %v clean", s, res, fr, ras)
		}
		p, _ := r.BPTCPayload()
		built := must(layer2.BuildDataBurst(p, enums.DataTypeCSBK, b.SlotType.ColorCode, layer2.SyncSourceBS))
		built.SetBPTCReserved(ras)
		if built != r {
			t.Errorf("%s: BuildDataBurst and SetBPTCReserved\ngot  %x\nwant %x", s, built, frame)
		}
		built.SetBPTCReserved([3]bit.Bit{})
		if zero := must(layer2.BuildDataBurst(p, enums.DataTypeCSBK, b.SlotType.ColorCode, layer2.SyncSourceBS)); built != zero {
			t.Errorf("%s: SetBPTCReserved to zero\ngot  %x\nwant %x", s, built, zero)
		}
	}
}

func TestBPTCReserved_CapacityMaxVoiceLCHeader(t *testing.T) {
	t.Parallel()
	frame := hexBurst(t, capMaxVoiceLCHeader)
	var b layer2.Burst
	if err := b.DecodeFromBytes(frame); err != nil {
		t.Fatal(err)
	}
	if b.SlotType.DataType != enums.DataTypeVoiceLCHeader || b.FEC.Payload.ErrorsCorrected != 0 {
		t.Fatalf("got %s with BPTC FEC %+v, want a clean Voice LC Header", b.SlotType.DataType, b.FEC.Payload)
	}
	if want := [3]bit.Bit{0, 0, 1}; b.BPTCReserved != want {
		t.Errorf("BPTCReserved = %v, want %v", b.BPTCReserved, want)
	}
	got := must(b.Encode())
	want := layer2.RawBurst(frame)
	want.SetSlotType(pdu.SlotType{ColorCode: b.SlotType.ColorCode, DataType: b.SlotType.DataType})
	if got != want {
		t.Errorf("Encode\ngot  %x\nwant %x", got, want)
	}
}

func TestBPTCReserved_EncodeWritesField(t *testing.T) {
	t.Parallel()
	frame := must(lcBurst([12]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12}, enums.DataTypeRate12, 1))
	for v := range 8 {
		res := [3]bit.Bit{bit.Bit(v & 1), bit.Bit(v >> 1 & 1), bit.Bit(v >> 2 & 1)}
		var b layer2.Burst
		if err := b.DecodeFromBytes(frame); err != nil {
			t.Fatal(err)
		}
		b.BPTCReserved = res
		enc := must(b.Encode())
		var d layer2.Burst
		if err := d.DecodeFromBytes(enc); err != nil {
			t.Fatal(err)
		}
		if d.BPTCReserved != res || d.FEC.Payload.ErrorsCorrected != 0 {
			t.Errorf("R %v: decoded %v with %d corrections", res, d.BPTCReserved, d.FEC.Payload.ErrorsCorrected)
		}
		r := layer2.RawBurst(frame)
		r.SetBPTCReserved(res)
		if r != enc {
			t.Errorf("R %v: SetBPTCReserved\ngot  %x\nwant %x", res, r, enc)
		}
	}
}

func TestBPTCReserved_SetKeepsBitErrors(t *testing.T) {
	t.Parallel()
	frame := hexBurst(t, capMaxCSBKAloha)
	r := layer2.RawBurst(flipBurstBits(frame, 3, 40, 200))
	p, before := r.BPTCPayload()
	r.SetBPTCReserved([3]bit.Bit{1, 1, 0})
	q, after := r.BPTCPayload()
	if p != q || before != after {
		t.Errorf("payload %x %+v, after SetBPTCReserved %x %+v", p, before, q, after)
	}
	if res, _ := r.BPTCReserved(); res != [3]bit.Bit{1, 1, 0} {
		t.Errorf("BPTCReserved = %v, want [1 1 0]", res)
	}
}
