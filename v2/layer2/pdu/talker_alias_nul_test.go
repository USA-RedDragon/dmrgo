package pdu_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/layer3"
)

func TestTalkerAliasTrailingNUL(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		format layer3.TalkerAliasDataFormat
		data   uint64
		blk    [7]byte
		length uint8
		want   string
	}{
		{layer3.TalkerAliasISOEightBit, 0x414200000000, [7]byte{}, 6, "AB"},
		{layer3.TalkerAliasUTF8, 0x412042200000, [7]byte{}, 6, "A B "},
		{layer3.TalkerAliasUTF8, 0x410042000000, [7]byte{}, 6, "A\x00B"},
		{layer3.TalkerAliasUTF16, 0x004100420000, [7]byte{}, 3, "AB"},
		{layer3.TalkerAliasSevenBit, 0x41<<42 | 0x42<<35, [7]byte{}, 7, "AB"},
		{layer3.TalkerAliasISOEightBit, 0x414243444546, [7]byte{0x47}, 13, "ABCDEFG"},
		{layer3.TalkerAliasISOEightBit, 0, [7]byte{}, 6, ""},
	} {
		h := pdu.FLCTalkerAliasHeader{TalkerAliasDataFormat: tt.format, TalkerAliasDataLength: tt.length, TalkerAliasData: tt.data}
		b := pdu.FLCTalkerAliasBlock{TalkerAliasData: tt.blk}
		got, err := pdu.TalkerAlias(&h, &b)
		if err != nil || got != tt.want {
			t.Errorf("format %v data %#x length %d: %q, %v, want %q", tt.format, tt.data, tt.length, got, err, tt.want)
		}
	}
}
