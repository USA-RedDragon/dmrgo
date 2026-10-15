package pdu_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/internal/crc"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/layer3"
)

// buildContinuationBits96 returns the 96 information bits of a last MBC
// continuation block with the given PF, CSBKO, color code and CdefParms bits.
func buildContinuationBits96(pf bool, csbko byte, colourCode byte, cdefParms [58]bit.Bit) [96]bit.Bit {
	var bits [96]bit.Bit

	bits[0] = 1
	if pf {
		bits[1] = 1
	}

	for i := 0; i < 6; i++ {
		if (csbko>>(5-i))&1 == 1 {
			bits[2+i] = 1
		}
	}

	for i := 0; i < 4; i++ {
		if (colourCode>>(3-i))&1 == 1 {
			bits[12+i] = 1
		}
	}

	copy(bits[22:80], cdefParms[:])

	// The last-block CRC-CCITT covers bits 0 to 79 and has no mask
	// (ETSI TS 102 361-1 clause 7.4.0, Table B.21).
	c := crc.CCITT(bit.Pack(bits[:80]))
	bit.PutUint(bits[80:96], uint64(c))

	return bits
}

func TestCGAPContinuation_Decode(t *testing.T) {
	t.Parallel()

	var cdefParms [58]bit.Bit
	cdefParms[3] = 1
	cdefParms[6] = 1
	cdefParms[10] = 1
	cdefParms[11] = 1
	cdefParms[13] = 1
	cdefParms[14] = 1
	cdefParms[16] = 1
	cdefParms[17] = 1
	cdefParms[18] = 1

	bits := buildContinuationBits96(false, byte(pdu.CSBKPrivateVoiceGrant), 5, cdefParms)

	decoded := pdu.DecodeCGAPContinuation(bits)

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

	var cdefParms [58]bit.Bit
	cdefParms[0] = 1
	cdefParms[20] = 1
	cdefParms[57] = 1

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

	var cdefParms [58]bit.Bit
	cdefParms[0] = 1

	bits := buildContinuationBits96(true, byte(pdu.CSBKMove), 3, cdefParms)

	decoded := pdu.DecodeMVAPContinuation(bits)

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

	var cdefParms [58]bit.Bit
	cdefParms[5] = 1

	var bits [96]bit.Bit
	bits[0] = 1
	bits[2] = 1
	bits[4] = 1
	for i := 8; i <= 15; i++ {
		bits[i] = 1
	}
	copy(bits[22:80], cdefParms[:])

	decoded := pdu.DecodeBCAPContinuation(bits)

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

	var cdefParms [58]bit.Bit
	cdefParms[0] = 1
	cdefParms[57] = 1

	bits := buildContinuationBits96(false, byte(pdu.CSBKBroadcast), 10, cdefParms)

	decoded := pdu.DecodeVNAPContinuation(bits)

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

	var cdefParms [58]bit.Bit
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

	var cdefParms [58]bit.Bit
	cdefParms[10] = 1
	cdefParms[30] = 1

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

	var zeroBits [96]bit.Bit

	pdu.DecodeCGAPContinuation(zeroBits)
	pdu.DecodeMVAPContinuation(zeroBits)
	pdu.DecodeBCAPContinuation(zeroBits)
	pdu.DecodeVNAPContinuation(zeroBits)
}

func TestMBCAbsoluteParms_LastBlockCRC(t *testing.T) {
	t.Parallel()

	var parms [58]bit.Bit
	parms[0] = 1
	parms[57] = 1
	bits := buildContinuationBits96(false, 0x30, 5, parms)

	decoders := map[string]func([96]bit.Bit) bool{
		"CG_AP": func(b [96]bit.Bit) bool { return pdu.DecodeCGAPContinuation(b).FEC.Uncorrectable },
		"MV_AP": func(b [96]bit.Bit) bool { return pdu.DecodeMVAPContinuation(b).FEC.Uncorrectable },
		"BC_AP": func(b [96]bit.Bit) bool { return pdu.DecodeBCAPContinuation(b).FEC.Uncorrectable },
		"VN_AP": func(b [96]bit.Bit) bool { return pdu.DecodeVNAPContinuation(b).FEC.Uncorrectable },
	}
	for name, dec := range decoders {
		if dec(bits) {
			t.Errorf("%s: valid CRC reported uncorrectable", name)
		}
		bad := bits
		bad[40] ^= 1
		if !dec(bad) {
			t.Errorf("%s: corrupted block not detected", name)
		}
	}

	if !pdu.MBCLastBlockCRCOK(bits) {
		t.Error("MBCLastBlockCRCOK should accept a single valid last block")
	}

	cg := pdu.DecodeCGAPContinuation(bits)
	if must(cg.Encode()) != bits {
		t.Error("CGAPContinuation.Encode should reproduce the CRC")
	}
}

func TestMBCLastBlockCRC_MultiBlock(t *testing.T) {
	t.Parallel()

	var inter, last [96]bit.Bit
	inter[5] = 1
	inter[95] = 1
	last[0] = 1
	last[10] = 1
	blocks := [][96]bit.Bit{inter, last}
	pdu.SetMBCLastBlockCRC(blocks)

	want := make([]byte, 0, 22)
	want = append(want, bit.Pack(inter[:])...)
	want = append(want, bit.Pack(last[:80])...)
	c := crc.CCITT(want)
	if got := uint16(bit.Uint(blocks[1][80 : 80+16])); got != c {
		t.Errorf("last-block CRC = 0x%04X, want 0x%04X", got, c)
	}
	if !pdu.MBCLastBlockCRCOK(blocks...) {
		t.Error("MBCLastBlockCRCOK should accept valid blocks")
	}
	blocks[0][50] ^= 1
	if pdu.MBCLastBlockCRCOK(blocks...) {
		t.Error("MBCLastBlockCRCOK should reject a corrupted intermediate block")
	}
	if pdu.MBCLastBlockCRCOK() {
		t.Error("MBCLastBlockCRCOK with no blocks should be false")
	}
}

func cdefOf(bits [58]bit.Bit) layer3.CdefParms {
	p := layer3.DecodeCdefParms(bits)
	return p
}
