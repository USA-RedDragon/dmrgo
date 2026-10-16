package layer2_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

func TestBurst_Opcode0x38HintFromSync(t *testing.T) {
	t.Parallel()

	act := pdu.BSOutboundActivationPDU{
		BSAddress:     1,
		SourceAddress: 2,
	}
	csbk := &pdu.CSBK{LastBlock: true, Opcode: pdu.CSBKBSOutboundActivation, RawPayload: must(act.Encode())}
	payload := must(csbk.Encode())

	tests := []struct {
		src  layer2.SyncSource
		want pdu.Opcode0x38Hint
	}{
		{layer2.SyncSourceMS, pdu.Opcode0x38BSOutboundActivation},
		{layer2.SyncSourceBS, pdu.Opcode0x38TalkgroupDataGrantMultiItem},
		{layer2.SyncSourceDirectTS1, pdu.Opcode0x38Unknown},
	}
	for _, tt := range tests {
		raw := must(layer2.BuildDataBurst(payload, enums.DataTypeCSBK, 1, tt.src))
		b := must(layer2.NewBurstFromBytes(raw))
		got, ok := b.Data.(*pdu.CSBK)
		if !ok {
			t.Fatalf("Data is %T, want *pdu.CSBK", b.Data)
		}
		if got.Hint0x38 != tt.want {
			t.Errorf("source %d: Hint0x38 = %d, want %d", tt.src, got.Hint0x38, tt.want)
		}
		if a, ok := got.AsBSOutboundActivation(); !ok || a != act {
			t.Errorf("source %d: AsBSOutboundActivation = %+v %v", tt.src, a, ok)
		}
	}
}
