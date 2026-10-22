package trunk_test

import (
	"encoding/hex"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/trunk"
)

func TestFollower_RealUDT(t *testing.T) {
	t.Parallel()
	caps := readCapture(t, "testdata/aselsan_udt.bits")
	f, err := trunk.NewFollower(&trunk.FollowerConfig{NoFollow: true})
	if err != nil {
		t.Fatal(err)
	}
	out := make([]trunk.Output, 0, trunk.MaxOutputs)
	udts := 0
	for i := range caps {
		c := &caps[i]
		out = f.Burst(c.at, &c.raw, c.hint, c.cach, out[:0])
		for j := range out {
			if out[j].Kind != trunk.OutputUDT {
				continue
			}
			udts++
			u, _ := f.UDT()
			want := pdu.UDTHeader{SAP: enums.SAPUnifiedDataTransport, UDTFormat: 9, LLIDDestination: 3109823, LLIDSource: 3169855, Opcode: pdu.UDTOpcodeOutboundHeader}
			if !u.CRCOK || u.Header != want || hex.EncodeToString(u.Data[:12*int(u.Header.AppendedBlocks+1)-2]) != "060008c02f73cc305e35" {
				t.Errorf("UDT %+v, CRCOK %v, data %x", u.Header, u.CRCOK, u.Data[:12*int(u.Header.AppendedBlocks+1)-2])
			}
		}
	}
	if udts != 1 {
		t.Errorf("%d UDTs, want 1", udts)
	}
	if s := f.Site(); !s.HasSysCode || s.ColorCode != 2 {
		t.Errorf("Site %+v", s)
	}
}
