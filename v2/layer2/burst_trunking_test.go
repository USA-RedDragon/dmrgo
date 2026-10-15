package layer2_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

func buildOpcode0x38Burst(t *testing.T) [33]byte {
	t.Helper()
	csbk := &pdu.CSBK{
		LastBlock:  true,
		CSBKOpcode: pdu.CSBKBSOutboundActivationPDU,
		BSOutboundActivationPDU: &pdu.BSOutboundActivationPDU{
			BSAddress:     [24]bit.Bit{23: 1},
			SourceAddress: [24]bit.Bit{22: 1},
		},
	}
	encBits := must(csbk.Encode())
	var lcBytes [12]byte
	copy(lcBytes[:], bit.PackBits(encBits[:]))
	return must(layer2.BuildLCDataBurst(lcBytes, enums.DataTypeCSBK, 1))
}

func TestBurst_TrunkingModeSelectsOpcode0x38Variant(t *testing.T) {
	raw := buildOpcode0x38Burst(t)

	tests := []struct {
		name     string
		trunking bool
	}{
		{"Conventional", false},
		{"Trunking", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var b layer2.Burst
			b.SetTrunkingMode(tt.trunking)
			if err := b.DecodeFromBytes(raw); err != nil {
				t.Fatalf("DecodeFromBytes: %v", err)
			}
			csbk, ok := b.Data.(*pdu.CSBK)
			if !ok {
				t.Fatalf("Data is %T, want *pdu.CSBK", b.Data)
			}
			if csbk.TrunkingMode != tt.trunking {
				t.Errorf("TrunkingMode = %v, want %v", csbk.TrunkingMode, tt.trunking)
			}
			if got := csbk.TalkgroupDataGrantMultiItemPDU != nil; got != tt.trunking {
				t.Errorf("TalkgroupDataGrantMultiItemPDU set = %v, want %v", got, tt.trunking)
			}
			if got := csbk.BSOutboundActivationPDU != nil; got == tt.trunking {
				t.Errorf("BSOutboundActivationPDU set = %v, want %v", got, !tt.trunking)
			}
		})
	}
}
