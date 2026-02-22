package pdu_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/layer3/elements"
)

// buildContinuationBits96 constructs a [96]bit.Bit array for an MBC continuation block.
// It sets LB, PF, CSBKO, and writes the given CdefParms bits at bits 22-79.
func buildContinuationBits96(lb bool, pf bool, csbko byte, colourCode byte, cdeftype byte, cdefParms [58]bit.Bit) [96]bit.Bit {
	var bits [96]bit.Bit

	if lb {
		bits[0] = 1
	}
	if pf {
		bits[1] = 1
	}

	// CSBKO (bits 2-7)
	for i := 0; i < 6; i++ {
		if (csbko>>(5-i))&1 == 1 {
			bits[2+i] = 1
		}
	}

	// ColourCode (bits 12-15) — for CG_AP, MV_AP, VN_AP
	for i := 0; i < 4; i++ {
		if (colourCode>>(3-i))&1 == 1 {
			bits[12+i] = 1
		}
	}

	// Cdeftype (bits 16-19)
	for i := 0; i < 4; i++ {
		if (cdeftype>>(3-i))&1 == 1 {
			bits[16+i] = 1
		}
	}

	// CdefParms (bits 22-79)
	copy(bits[22:80], cdefParms[:])

	return bits
}

func TestCGAPContinuation_Decode(t *testing.T) {
	var cdefParms [58]bit.Bit
	// Channel (bits 0-11) = 291 = 0b000100100011
	cdefParms[3] = 1
	cdefParms[6] = 1
	cdefParms[10] = 1
	cdefParms[11] = 1
	// TXMHz (bits 12-21) = 440 = 0b0110111000
	cdefParms[13] = 1
	cdefParms[14] = 1
	cdefParms[16] = 1
	cdefParms[17] = 1
	cdefParms[18] = 1

	bits := buildContinuationBits96(true, false, byte(pdu.CSBKPrivateVoiceGrant), 5, 0, cdefParms)

	decoded, _ := pdu.DecodeCGAPContinuation(bits)

	if !decoded.LastBlock {
		t.Error("LastBlock should be true")
	}
	if decoded.ProtectFlag {
		t.Error("ProtectFlag should be false")
	}
	if decoded.CSBKOpcode != pdu.CSBKPrivateVoiceGrant {
		t.Errorf("CSBKOpcode = %d, want PV_GRANT(%d)", decoded.CSBKOpcode, pdu.CSBKPrivateVoiceGrant)
	}
	if decoded.ColourCode != 5 {
		t.Errorf("ColourCode = %d, want 5", decoded.ColourCode)
	}
	if decoded.Cdeftype != 0 {
		t.Errorf("Cdeftype = %d, want 0", decoded.Cdeftype)
	}

	// Verify CdefParms can be parsed
	parms := elements.NewCdefParmsFromBits(decoded.CdefParms)
	if parms.Channel != 291 {
		t.Errorf("Channel = %d, want 291", parms.Channel)
	}
}

func TestCGAPContinuation_EncodeDecodeCycle(t *testing.T) {
	var cdefParms [58]bit.Bit
	cdefParms[0] = 1
	cdefParms[20] = 1
	cdefParms[57] = 1

	original := pdu.CGAPContinuation{
		LastBlock:   true,
		ProtectFlag: false,
		CSBKOpcode:  pdu.CSBKTalkgroupVoiceGrant,
		ColourCode:  7,
		Cdeftype:    0,
		CdefParms:   cdefParms,
	}

	encoded := pdu.EncodeCGAPContinuation(&original)
	decoded, _ := pdu.DecodeCGAPContinuation(encoded)

	if decoded.LastBlock != original.LastBlock {
		t.Error("LastBlock mismatch")
	}
	if decoded.CSBKOpcode != original.CSBKOpcode {
		t.Error("CSBKOpcode mismatch")
	}
	if decoded.ColourCode != original.ColourCode {
		t.Errorf("ColourCode = %d, want %d", decoded.ColourCode, original.ColourCode)
	}
	if decoded.CdefParms != original.CdefParms {
		t.Error("CdefParms mismatch")
	}
}

func TestMVAPContinuation_Decode(t *testing.T) {
	var cdefParms [58]bit.Bit
	cdefParms[0] = 1

	bits := buildContinuationBits96(true, true, byte(pdu.CSBKMove), 3, 0, cdefParms)

	decoded, _ := pdu.DecodeMVAPContinuation(bits)

	if !decoded.LastBlock {
		t.Error("LastBlock should be true")
	}
	if !decoded.ProtectFlag {
		t.Error("ProtectFlag should be true")
	}
	if decoded.CSBKOpcode != pdu.CSBKMove {
		t.Errorf("CSBKOpcode = %d, want C_MOVE(%d)", decoded.CSBKOpcode, pdu.CSBKMove)
	}
	if decoded.ColourCode != 3 {
		t.Errorf("ColourCode = %d, want 3", decoded.ColourCode)
	}
}

func TestBCAPContinuation_Decode(t *testing.T) {
	var cdefParms [58]bit.Bit
	cdefParms[5] = 1

	// BC_AP has 8-bit Reserved1 (bits 8-15) instead of 4-bit Reserved + 4-bit CC
	var bits [96]bit.Bit
	bits[0] = 1 // LB
	// CSBKO = C_BCAST = 0b101000
	bits[2] = 1
	bits[4] = 1
	// Reserved1 (bits 8-15) = 0xFF → all 1s
	for i := 8; i <= 15; i++ {
		bits[i] = 1
	}
	// Cdeftype (bits 16-19) = 0
	// CdefParms (bits 22-79)
	copy(bits[22:80], cdefParms[:])

	decoded, _ := pdu.DecodeBCAPContinuation(bits)

	if !decoded.LastBlock {
		t.Error("LastBlock should be true")
	}
	if decoded.CSBKOpcode != pdu.CSBKBroadcast {
		t.Errorf("CSBKOpcode = %d, want C_BCAST(%d)", decoded.CSBKOpcode, pdu.CSBKBroadcast)
	}
	if decoded.Reserved1 != 0xFF {
		t.Errorf("Reserved1 = 0x%02X, want 0xFF", decoded.Reserved1)
	}
	if decoded.CdefParms != cdefParms {
		t.Error("CdefParms mismatch")
	}
}

func TestVNAPContinuation_Decode(t *testing.T) {
	var cdefParms [58]bit.Bit
	cdefParms[0] = 1
	cdefParms[57] = 1

	bits := buildContinuationBits96(true, false, byte(pdu.CSBKBroadcast), 10, 0, cdefParms)

	decoded, _ := pdu.DecodeVNAPContinuation(bits)

	if !decoded.LastBlock {
		t.Error("LastBlock should be true")
	}
	if decoded.CSBKOpcode != pdu.CSBKBroadcast {
		t.Errorf("CSBKOpcode = %d, want C_BCAST(%d)", decoded.CSBKOpcode, pdu.CSBKBroadcast)
	}
	if decoded.ColourCode != 10 {
		t.Errorf("ColourCode = %d, want 10", decoded.ColourCode)
	}
	if decoded.CdefParms != cdefParms {
		t.Error("CdefParms mismatch")
	}
}

func TestCGAPContinuation_ToString(t *testing.T) {
	var cdefParms [58]bit.Bit
	cgap := &pdu.CGAPContinuation{
		LastBlock:   true,
		ProtectFlag: false,
		CSBKOpcode:  pdu.CSBKPrivateVoiceGrant,
		ColourCode:  5,
		Cdeftype:    0,
		CdefParms:   cdefParms,
	}

	str := cgap.ToString()
	if str == "" {
		t.Error("ToString should not be empty")
	}
}

func TestMVAPContinuation_EncodeDecodeCycle(t *testing.T) {
	var cdefParms [58]bit.Bit
	cdefParms[10] = 1
	cdefParms[30] = 1

	original := pdu.MVAPContinuation{
		LastBlock:   true,
		ProtectFlag: true,
		CSBKOpcode:  pdu.CSBKMove,
		ColourCode:  15,
		Cdeftype:    0,
		CdefParms:   cdefParms,
	}

	encoded := pdu.EncodeMVAPContinuation(&original)
	decoded, _ := pdu.DecodeMVAPContinuation(encoded)

	if decoded.ColourCode != original.ColourCode {
		t.Errorf("ColourCode = %d, want %d", decoded.ColourCode, original.ColourCode)
	}
	if decoded.CdefParms != original.CdefParms {
		t.Error("CdefParms mismatch")
	}
}

func TestBit_ContinuationTypes_AllDecode(t *testing.T) {
	// Verify all four continuation types can decode a zero-filled 96-bit array without panic
	var zeroBits [96]bit.Bit

	pdu.DecodeCGAPContinuation(zeroBits)
	pdu.DecodeMVAPContinuation(zeroBits)
	pdu.DecodeBCAPContinuation(zeroBits)
	pdu.DecodeVNAPContinuation(zeroBits)
}
