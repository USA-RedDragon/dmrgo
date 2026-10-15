package layer2_test

import (
	"encoding/hex"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/elements"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

// TestMMDVMHostAccepts_Vectors checks MMDVMHostAccepts against
// CDMRFullLC::decode, CDMRCSBK::put and CDMRDataHeader::put of MMDVMHost
// 0acb758, given each payload BPTC encoded. Per data type the vectors are a
// valid payload, the same with one bit flipped, one with a wrong CRC mask
// and a random payload.
func TestMMDVMHostAccepts_Vectors(t *testing.T) {
	for _, v := range []struct {
		dt      elements.DataType
		payload string
		accept  bool
	}{
		{1, "1f43dd8657a82d0a2c0ba7ee", true},
		{1, "58bc4a2f3112954e7d8cd8cb", false},
		{1, "b2a9701e69081bcbf8db1930", false},
		{1, "a496cc3d1c62db6d84a1eee4", false},
		{2, "82964d8462854f0415c48b81", true},
		{2, "7f0ecb4ec18d538b17356af7", false},
		{2, "a6c6c53c1c29d85fe0e45d43", false},
		{2, "cee02c261c5326d1109b53a6", false},
		{3, "15a03fc01c6bee74fa0e2601", true},
		{3, "1ab5b8bfb414cbfba5aabebd", false},
		{3, "4c10e4ea148442b6e8b7641c", false},
		{3, "ce5fa12de3fed2cde8a39ea4", false},
		{4, "c1a7a64b660881989fe0ca01", true},
		{4, "874792ce92430c6b7333ba50", false},
		{4, "a2430ade1468f39863537a89", false},
		{4, "20ca0c30a3e0cd16acbca946", false},
		{5, "c65ba926d753da127f9e026f", true},
		{5, "634e11ba08469a7d01f529ac", false},
		{5, "6a062de5a7fce53e6249a309", false},
		{5, "771a52d60a646e1ab9417bb3", false},
		{6, "3740a132a2f5c22dd00c1d46", true},
		{6, "60c57e0247be86b5e03791e3", false},
		{6, "960c71538f2de644e672a49c", false},
		{6, "a1afe0c63b1a43487887754f", false},
	} {
		var payload [12]byte
		copy(payload[:], must(hex.DecodeString(v.payload)))
		burst, _ := layer2.NewBurstFromBytes(must(layer2.BuildLCDataBurst(payload, v.dt, 1)))
		got, _, ok := burst.BPTCPayload()
		if !ok || got != payload {
			t.Fatalf("%s %s: BPTCPayload %x ok=%v", elements.DataTypeToName(v.dt), v.payload, got, ok)
		}
		if layer2.MMDVMHostAccepts(v.dt, got) != v.accept {
			t.Errorf("%s %s: accepted %v, MMDVMHost %v", elements.DataTypeToName(v.dt), v.payload, !v.accept, v.accept)
		}
	}
}

func TestMMDVMHostAccepts_UncheckedDataTypes(t *testing.T) {
	var payload [12]byte
	for _, dt := range []elements.DataType{elements.DataTypePIHeader, elements.DataTypeRate12, elements.DataTypeRate34, elements.DataTypeRate1} {
		if !layer2.MMDVMHostAccepts(dt, payload) {
			t.Errorf("%s rejected", elements.DataTypeToName(dt))
		}
	}
	for dt := elements.DataTypeIdle; dt <= 0xF; dt++ {
		if dt != elements.DataTypeRate1 && layer2.MMDVMHostAccepts(dt, payload) {
			t.Errorf("%s accepted", elements.DataTypeToName(dt))
		}
	}
}

// TestMMDVMHostAccepts_MBCContinuation checks that MMDVMHost accepts the
// last block of an MBC with one continuation block, whose last-block CRC
// covers only that block, and rejects that block when it follows another.
func TestMMDVMHostAccepts_MBCContinuation(t *testing.T) {
	var first, last [96]bit.Bit
	for i := range last {
		first[i] = bit.Bit(i % 3 & 1)
		last[i] = bit.Bit(i % 5 & 1)
	}
	last[0] = 1
	blocks := [][96]bit.Bit{last}
	pdu.SetMBCLastBlockCRC(blocks)
	var payload [12]byte
	copy(payload[:], bit.PackBits(blocks[0][:]))
	if !layer2.MMDVMHostAccepts(elements.DataTypeMBCContinuation, payload) {
		t.Error("single continuation block rejected")
	}
	blocks = [][96]bit.Bit{first, last}
	pdu.SetMBCLastBlockCRC(blocks)
	copy(payload[:], bit.PackBits(blocks[1][:]))
	if !pdu.CheckMBCLastBlockCRC(blocks...) || layer2.MMDVMHostAccepts(elements.DataTypeMBCContinuation, payload) {
		t.Error("last of two continuation blocks accepted")
	}
}
