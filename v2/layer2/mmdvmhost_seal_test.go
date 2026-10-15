package layer2_test

import (
	"math/rand"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

func TestMMDVMHostSeal_PairsWithAccepts(t *testing.T) {
	rng := rand.New(rand.NewSource(6))
	sealed := []enums.DataType{
		enums.DataTypeVoiceLCHeader, enums.DataTypeTerminatorWithLC, enums.DataTypePIHeader,
		enums.DataTypeCSBK, enums.DataTypeMBCHeader, enums.DataTypeDataHeader, enums.DataTypeMBCContinuation,
		enums.DataTypeRate12, enums.DataTypeRate34, enums.DataTypeRate1,
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
			case enums.DataTypeVoiceLCHeader, enums.DataTypeTerminatorWithLC:
				if !pdu.FullLCParityOK(p, dt) {
					t.Fatalf("dt %d: not a Full LC codeword", dt)
				}
			case enums.DataTypePIHeader, enums.DataTypeCSBK, enums.DataTypeMBCHeader, enums.DataTypeDataHeader:
				if !pdu.BlockCRCOK(p, dt) {
					t.Fatalf("dt %d: block CRC fails", dt)
				}
			case enums.DataTypeRate12, enums.DataTypeRate34, enums.DataTypeRate1:
				if p != before {
					t.Fatalf("dt %d: payload changed", dt)
				}
			case enums.DataTypeMBCContinuation, enums.DataTypeIdle, enums.DataTypeUnifiedSingleBlock, enums.DataTypeReserved:
			}
		}
	}
	for _, dt := range []enums.DataType{enums.DataTypeIdle, enums.DataTypeUnifiedSingleBlock, enums.DataTypeReserved, 15} {
		p := [12]byte{1, 2, 3}
		if err := layer2.MMDVMHostSeal(dt, &p); err == nil || p != [12]byte{1, 2, 3} {
			t.Errorf("dt %d: sealed", dt)
		}
	}
}

func TestMMDVMHostSeal_IdentityOnAcceptedCaptures(t *testing.T) {
	n := 0
	for _, frame := range captureFrames(t) {
		r := frame
		st := r.SlotType()
		res := st.FEC
		if res.HasErrors() || st.DataType == enums.DataTypeRate34 || st.DataType == enums.DataTypeRate1 || st.DataType.IsReserved() {
			continue
		}
		if b, _ := layer2.NewBurstFromBytes(frame); b.Kind != layer2.KindData {
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
