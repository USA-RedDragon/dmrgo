package pdu_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/internal/bits"
	"github.com/USA-RedDragon/dmrgo/v2/internal/crc"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/layer3"
)

func buildContinuationBits96(pf bool, csbko byte, colourCode byte, cdefParms [8]byte) [12]byte {
	var data [12]byte

	bits.SetBit(data[:], 0, 1)
	if pf {
		bits.SetBit(data[:], 1, 1)
	}
	bits.SetField(data[:], 2, 6, uint64(csbko))
	bits.SetField(data[:], 12, 4, uint64(colourCode))
	bits.Copy(data[:], 22, cdefParms[:], 0, 58)

	// ETSI TS 102 361-1 clause 7.4.0, Table B.21
	c := crc.CCITT(data[:10])
	bits.SetField(data[:], 80, 16, uint64(c))

	return data
}

func TestCGAPContinuation_Decode(t *testing.T) {
	t.Parallel()

	var cdefParms [8]byte
	for _, i := range []int{3, 6, 10, 11, 13, 14, 16, 17, 18} {
		bits.SetBit(cdefParms[:], i, 1)
	}

	data := buildContinuationBits96(false, byte(pdu.CSBKPrivateVoiceGrant), 5, cdefParms)

	decoded := pdu.DecodeCGAPContinuation(data)

	if !decoded.LastBlock {
		t.Error("LastBlock should be true")
	}
	if decoded.ProtectFlag {
		t.Error("ProtectFlag should be false")
	}
	if decoded.Opcode != pdu.CSBKPrivateVoiceGrant {
		t.Errorf("CSBKOpcode = %d, want PV_GRANT(%d)", decoded.Opcode, pdu.CSBKPrivateVoiceGrant)
	}
	if decoded.ColorCode != 5 {
		t.Errorf("ColorCode = %d, want 5", decoded.ColorCode)
	}
	if decoded.Cdeftype != 0 {
		t.Errorf("Cdeftype = %d, want 0", decoded.Cdeftype)
	}

	parms := &decoded.CdefParms
	if parms.Channel != 291 {
		t.Errorf("Channel = %d, want 291", parms.Channel)
	}
}

func TestCGAPContinuation_EncodeDecodeCycle(t *testing.T) {
	t.Parallel()

	var cdefParms [8]byte
	bits.SetBit(cdefParms[:], 0, 1)
	bits.SetBit(cdefParms[:], 20, 1)
	bits.SetBit(cdefParms[:], 57, 1)

	original := pdu.CGAPContinuation{
		LastBlock:   true,
		ProtectFlag: false,
		Opcode:      pdu.CSBKTalkgroupVoiceGrant,
		ColorCode:   7,
		Cdeftype:    0,
		CdefParms:   cdefOf(cdefParms),
	}

	encoded := must(original.Encode())
	decoded := pdu.DecodeCGAPContinuation(encoded)

	if decoded.LastBlock != original.LastBlock {
		t.Error("LastBlock mismatch")
	}
	if decoded.Opcode != original.Opcode {
		t.Error("CSBKOpcode mismatch")
	}
	if decoded.ColorCode != original.ColorCode {
		t.Errorf("ColorCode = %d, want %d", decoded.ColorCode, original.ColorCode)
	}
	if decoded.CdefParms != original.CdefParms {
		t.Error("CdefParms mismatch")
	}
}

func TestMVAPContinuation_Decode(t *testing.T) {
	t.Parallel()

	var cdefParms [8]byte
	bits.SetBit(cdefParms[:], 0, 1)

	data := buildContinuationBits96(true, byte(pdu.CSBKMove), 3, cdefParms)

	decoded := pdu.DecodeMVAPContinuation(data)

	if !decoded.LastBlock {
		t.Error("LastBlock should be true")
	}
	if !decoded.ProtectFlag {
		t.Error("ProtectFlag should be true")
	}
	if decoded.Opcode != pdu.CSBKMove {
		t.Errorf("CSBKOpcode = %d, want C_MOVE(%d)", decoded.Opcode, pdu.CSBKMove)
	}
	if decoded.ColorCode != 3 {
		t.Errorf("ColorCode = %d, want 3", decoded.ColorCode)
	}
}

func TestBCAPContinuation_Decode(t *testing.T) {
	t.Parallel()

	var cdefParms [8]byte
	bits.SetBit(cdefParms[:], 5, 1)

	var data [12]byte
	bits.SetBit(data[:], 0, 1)
	bits.SetBit(data[:], 2, 1)
	bits.SetBit(data[:], 4, 1)
	bits.SetField(data[:], 8, 8, 0xFF)
	bits.Copy(data[:], 22, cdefParms[:], 0, 58)

	decoded := pdu.DecodeBCAPContinuation(data)

	if !decoded.LastBlock {
		t.Error("LastBlock should be true")
	}
	if decoded.Opcode != pdu.CSBKBroadcast {
		t.Errorf("CSBKOpcode = %d, want C_BCAST(%d)", decoded.Opcode, pdu.CSBKBroadcast)
	}
	if decoded.Reserved1 != 0xFF {
		t.Errorf("Reserved1 = 0x%02X, want 0xFF", decoded.Reserved1)
	}
	if decoded.CdefParms != cdefOf(cdefParms) {
		t.Error("CdefParms mismatch")
	}
}

func TestVNAPContinuation_Decode(t *testing.T) {
	t.Parallel()

	var cdefParms [8]byte
	bits.SetBit(cdefParms[:], 0, 1)
	bits.SetBit(cdefParms[:], 57, 1)

	data := buildContinuationBits96(false, byte(pdu.CSBKBroadcast), 10, cdefParms)

	decoded := pdu.DecodeVNAPContinuation(data)

	if !decoded.LastBlock {
		t.Error("LastBlock should be true")
	}
	if decoded.Opcode != pdu.CSBKBroadcast {
		t.Errorf("CSBKOpcode = %d, want C_BCAST(%d)", decoded.Opcode, pdu.CSBKBroadcast)
	}
	if decoded.ColorCode != 10 {
		t.Errorf("ColorCode = %d, want 10", decoded.ColorCode)
	}
	if decoded.CdefParms != cdefOf(cdefParms) {
		t.Error("CdefParms mismatch")
	}
}

func TestCGAPContinuation_String(t *testing.T) {
	t.Parallel()

	var cdefParms [8]byte
	cgap := &pdu.CGAPContinuation{
		LastBlock:   true,
		ProtectFlag: false,
		Opcode:      pdu.CSBKPrivateVoiceGrant,
		ColorCode:   5,
		Cdeftype:    0,
		CdefParms:   cdefOf(cdefParms),
	}

	str := cgap.String()
	if str == "" {
		t.Error("String should not be empty")
	}
}

func TestMVAPContinuation_EncodeDecodeCycle(t *testing.T) {
	t.Parallel()

	var cdefParms [8]byte
	bits.SetBit(cdefParms[:], 10, 1)
	bits.SetBit(cdefParms[:], 30, 1)

	original := pdu.MVAPContinuation{
		LastBlock:   true,
		ProtectFlag: true,
		Opcode:      pdu.CSBKMove,
		ColorCode:   15,
		Cdeftype:    0,
		CdefParms:   cdefOf(cdefParms),
	}

	encoded := must(original.Encode())
	decoded := pdu.DecodeMVAPContinuation(encoded)

	if decoded.ColorCode != original.ColorCode {
		t.Errorf("ColorCode = %d, want %d", decoded.ColorCode, original.ColorCode)
	}
	if decoded.CdefParms != original.CdefParms {
		t.Error("CdefParms mismatch")
	}
}

func TestBit_ContinuationTypes_AllDecode(t *testing.T) {
	t.Parallel()

	var zeroBits [12]byte

	pdu.DecodeCGAPContinuation(zeroBits)
	pdu.DecodeMVAPContinuation(zeroBits)
	pdu.DecodeBCAPContinuation(zeroBits)
	pdu.DecodeVNAPContinuation(zeroBits)
}

func TestMBCAbsoluteParms_LastBlockCRC(t *testing.T) {
	t.Parallel()

	var parms [8]byte
	bits.SetBit(parms[:], 0, 1)
	bits.SetBit(parms[:], 57, 1)
	data := buildContinuationBits96(false, 0x30, 5, parms)

	decoders := map[string]func([12]byte) bool{
		"CG_AP": func(b [12]byte) bool { return pdu.DecodeCGAPContinuation(b).FEC.Uncorrectable },
		"MV_AP": func(b [12]byte) bool { return pdu.DecodeMVAPContinuation(b).FEC.Uncorrectable },
		"BC_AP": func(b [12]byte) bool { return pdu.DecodeBCAPContinuation(b).FEC.Uncorrectable },
		"VN_AP": func(b [12]byte) bool { return pdu.DecodeVNAPContinuation(b).FEC.Uncorrectable },
	}
	for name, dec := range decoders {
		if dec(data) {
			t.Errorf("%s: valid CRC reported uncorrectable", name)
		}
		bad := data
		bad[5] ^= 0x80
		if !dec(bad) {
			t.Errorf("%s: corrupted block not detected", name)
		}
	}

	if !pdu.MBCLastBlockCRCOK(data) {
		t.Error("MBCLastBlockCRCOK should accept a single valid last block")
	}

	cg := pdu.DecodeCGAPContinuation(data)
	if must(cg.Encode()) != data {
		t.Error("CGAPContinuation.Encode should reproduce the CRC")
	}
}

func TestMBCLastBlockCRC_MultiBlock(t *testing.T) {
	t.Parallel()

	var inter, last [12]byte
	bits.SetBit(inter[:], 5, 1)
	bits.SetBit(inter[:], 95, 1)
	bits.SetBit(last[:], 0, 1)
	bits.SetBit(last[:], 10, 1)
	blocks := [][12]byte{inter, last}
	pdu.SetMBCLastBlockCRC(blocks)

	want := make([]byte, 0, 22)
	want = append(want, inter[:]...)
	want = append(want, last[:10]...)
	c := crc.CCITT(want)
	if got := uint16(bits.Field(blocks[1][:], 80, 16)); got != c {
		t.Errorf("last-block CRC = 0x%04X, want 0x%04X", got, c)
	}
	if !pdu.MBCLastBlockCRCOK(blocks...) {
		t.Error("MBCLastBlockCRCOK should accept valid blocks")
	}
	blocks[0][6] ^= 0x20
	if pdu.MBCLastBlockCRCOK(blocks...) {
		t.Error("MBCLastBlockCRCOK should reject a corrupted intermediate block")
	}
	if pdu.MBCLastBlockCRCOK() {
		t.Error("MBCLastBlockCRCOK with no blocks should be false")
	}
}

func cdefOf(data [8]byte) layer3.CdefParms {
	p := layer3.DecodeCdefParms(data)
	return p
}
