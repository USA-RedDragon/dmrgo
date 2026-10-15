package layer2_test

import (
	"math/rand"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/elements"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

func TestMMDVMHostSeal_PairsWithAccepts(t *testing.T) {
	rng := rand.New(rand.NewSource(6))
	sealed := []elements.DataType{
		elements.DataTypeVoiceLCHeader, elements.DataTypeTerminatorWithLC, elements.DataTypePIHeader,
		elements.DataTypeCSBK, elements.DataTypeMBCHeader, elements.DataTypeDataHeader, elements.DataTypeMBCContinuation,
		elements.DataTypeRate12, elements.DataTypeRate34, elements.DataTypeRate1,
	}
	for _, dt := range sealed {
		for n := 0; n < 200; n++ {
			var p [12]byte
			rng.Read(p[:])
			before := p
			if err := layer2.MMDVMHostSeal(dt, &p); err != nil {
				t.Fatalf("dt %d: %v", dt, err)
			}
			if !layer2.MMDVMHostAccepts(dt, p) {
				t.Fatalf("dt %d: sealed payload %x rejected", dt, p)
			}
			if [9]byte(p[:9]) != [9]byte(before[:9]) {
				t.Fatalf("dt %d: seal changed the data octets", dt)
			}
			switch dt {
			case elements.DataTypeVoiceLCHeader, elements.DataTypeTerminatorWithLC:
				if !pdu.FullLCParityOK(p, dt) {
					t.Fatalf("dt %d: not a Full LC codeword", dt)
				}
			case elements.DataTypePIHeader, elements.DataTypeCSBK, elements.DataTypeMBCHeader, elements.DataTypeDataHeader:
				if !pdu.BlockCRCOK(p, dt) {
					t.Fatalf("dt %d: block CRC fails", dt)
				}
			case elements.DataTypeRate12, elements.DataTypeRate34, elements.DataTypeRate1:
				if p != before {
					t.Fatalf("dt %d: payload changed", dt)
				}
			case elements.DataTypeMBCContinuation, elements.DataTypeIdle, elements.DataTypeUnifiedSingleBlock, elements.DataTypeReserved:
			}
		}
	}
	for _, dt := range []elements.DataType{elements.DataTypeIdle, elements.DataTypeUnifiedSingleBlock, elements.DataTypeReserved, 15} {
		p := [12]byte{1, 2, 3}
		if err := layer2.MMDVMHostSeal(dt, &p); err == nil || p != [12]byte{1, 2, 3} {
			t.Errorf("dt %d: sealed", dt)
		}
	}
}

func TestMMDVMHostSeal_IdentityOnAcceptedCaptures(t *testing.T) {
	n := 0
	for _, frame := range captureFrames(t) {
		r := layer2.RawBurst(frame)
		st, res := r.SlotType()
		if res.HasErrors() || st.DataType == elements.DataTypeRate34 || st.DataType == elements.DataTypeRate1 || st.DataType.IsReserved() {
			continue
		}
		if b, _ := layer2.NewBurstFromBytes(frame); !b.IsData {
			continue
		}
		p, _ := r.BPTCPayload()
		if !layer2.MMDVMHostAccepts(st.DataType, p) {
			continue
		}
		got := p
		if err := layer2.MMDVMHostSeal(st.DataType, &got); err != nil || got != p {
			t.Errorf("dt %d: seal changed accepted payload %x to %x (%v)", st.DataType, p, got, err)
		}
		n++
	}
	if n == 0 {
		t.Fatal("no accepted data bursts")
	}
}
