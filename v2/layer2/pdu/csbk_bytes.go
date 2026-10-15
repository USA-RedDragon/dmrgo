package pdu

import (
	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/fec"
)

// CSBKBytes is the 12-octet BPTC(196,96) payload of a CSBK burst (ETSI TS
// 102 361-1 §9.1.8), as layer2.RawBurst.BPTCPayload returns it: 80 CSBK
// bits followed by the CRC-CCITT XORed with the CSBK mask 0xA5A5. Its
// methods read or write one field in place and leave every other bit as it
// is. The setters do not update the CRC: call Reseal after the last edit,
// or the CSBK fails its CRC check.
//
// Octet 0 holds the LB and PF bits and the 6-bit CSBKO, octet 1 the FID
// and octets 2 to 9 the CSBK data. Address fields follow the payload
// layouts that DecodeCSBK applies, so they apply only to the standard FID.
type CSBKBytes [12]byte

// Opcode returns the 6-bit CSBKO, the low 6 bits of octet 0.
func (c CSBKBytes) Opcode() CSBKOpcode {
	return CSBKOpcode(c[0] & 0x3F)
}

// FID returns the Feature set ID, octet 1.
func (c CSBKBytes) FID() enums.FeatureSetID {
	return enums.FeatureSetID(c[1])
}

func (c CSBKBytes) addressOffsets() (target, source int, ok bool) {
	if c.FID() != enums.FIDStandardized {
		return 0, 0, false
	}
	switch c.Opcode() {
	case CSBKNegativeAcknowledgement:
		return 7, 4, true
	case CSBKUnitToUnitVoiceServiceRequest, CSBKUnitToUnitVoiceServiceAnswerResponse,
		CSBKPreamble, CSBKPrivateVoiceGrant, CSBKTalkgroupVoiceGrant,
		CSBKBroadcastTalkgroupVoiceGrant, CSBKPrivateDataGrant, CSBKTalkgroupDataGrant,
		CSBKDuplexPrivateVoiceGrant, CSBKDuplexPrivateDataGrant,
		CSBKPrivateDataGrantMultiItem, CSBKClear, CSBKProtect, CSBKAhoy,
		CSBKRandomAccess, CSBKAckvitation, CSBKMaintenance:
		return 4, 7, true
	case CSBKChannelTiming, CSBKBSOutboundActivation, CSBKAloha, CSBKAckOutbound,
		CSBKAckInbound, CSBKAckOutboundPayload, CSBKAckInboundPayload,
		CSBKBroadcast, CSBKMove:
		return 0, 0, false
	default:
		return 0, 0, false
	}
}

// Addresses returns the 24-bit target and source addresses of the CSBKs
// for which CSBK.Addresses returns them, with the standard FID: octets 4 to
// 6 hold the target and octets 7 to 9 the source, except in NACK_Rsp (ETSI
// TS 102 361-2 Table 7.6), where octets 4 to 6 hold the source and octets 7
// to 9 the target. ok is false for every other CSBK, as for CSBK.Addresses.
func (c CSBKBytes) Addresses() (target, source uint32, ok bool) {
	t, s, ok := c.addressOffsets()
	if !ok {
		return 0, 0, false
	}
	return uint24(c[t:]), uint24(c[s:]), true
}

// SetAddresses writes target and source into the address octets of a CSBK
// for which Addresses returns ok. It does not update the CRC; call Reseal.
// It returns false, and changes nothing, for other CSBKs or when an address
// does not fit in 24 bits, as CSBK.SetAddresses does.
func (c *CSBKBytes) SetAddresses(target, source uint32) bool {
	t, s, ok := c.addressOffsets()
	if !ok || target>>24 != 0 || source>>24 != 0 {
		return false
	}
	putUint24(c[t:], target)
	putUint24(c[s:], source)
	return true
}

func (c CSBKBytes) groupBit() (octet int, mask byte, ok bool) {
	if c.FID() != enums.FIDStandardized {
		return 0, 0, false
	}
	switch c.Opcode() {
	case CSBKPreamble:
		return 2, 0x40, true
	case CSBKClear, CSBKProtect:
		return 3, 0x01, true
	case CSBKAhoy:
		return 3, 0x40, true
	case CSBKUnitToUnitVoiceServiceRequest, CSBKUnitToUnitVoiceServiceAnswerResponse,
		CSBKChannelTiming, CSBKNegativeAcknowledgement, CSBKBSOutboundActivation,
		CSBKAloha, CSBKAckvitation, CSBKRandomAccess, CSBKAckOutbound,
		CSBKAckInbound, CSBKAckOutboundPayload, CSBKAckInboundPayload,
		CSBKBroadcast, CSBKMaintenance, CSBKPrivateVoiceGrant,
		CSBKTalkgroupVoiceGrant, CSBKBroadcastTalkgroupVoiceGrant,
		CSBKPrivateDataGrant, CSBKTalkgroupDataGrant, CSBKDuplexPrivateVoiceGrant,
		CSBKDuplexPrivateDataGrant, CSBKPrivateDataGrantMultiItem, CSBKMove:
		return 0, 0, false
	default:
		return 0, 0, false
	}
}

// Group returns the flag that says the target address is a talkgroup, for
// the standard FID CSBKs that carry one: the Group bit of Pre (ETSI TS 102
// 361-2 Table 7.7, octet 2 bit 6) and the G/I bit of P_CLEAR and P_PROTECT
// (octet 3 bit 0) and of C_AHOY/P_AHOY (octet 3 bit 6), ETSI TS 102 361-4
// §7.1.1. Bits count from 0 at the least significant. 1 means a talkgroup
// and 0 an individual address. ok is false for every other CSBK: in the
// grants the opcode tells group from individual, and the others have no
// such flag.
func (c CSBKBytes) Group() (group, ok bool) {
	o, m, ok := c.groupBit()
	if !ok {
		return false, false
	}
	return c[o]&m != 0, true
}

// SetGroup writes the flag that Group returns. It does not update the CRC;
// call Reseal. It returns false, and changes nothing, for a CSBK for which
// Group returns no flag.
func (c *CSBKBytes) SetGroup(group bool) bool {
	o, m, ok := c.groupBit()
	if !ok {
		return false
	}
	if group {
		c[o] |= m
	} else {
		c[o] &^= m
	}
	return true
}

// CRCOK reports whether octets 10 and 11 hold the CRC of octets 0 to 9 with
// the CSBK mask, as BlockCRCOK does.
func (c CSBKBytes) CRCOK() bool {
	return BlockCRCOK(c, enums.DataTypeCSBK)
}

// Reseal writes the CRC of octets 0 to 9 with the CSBK mask into octets 10
// and 11, as SealBlock does.
func (c *CSBKBytes) Reseal() {
	_ = SealBlock((*[12]byte)(c), enums.DataTypeCSBK)
}

// DecodeCSBKBytes decodes the 12-octet payload b of a CSBK burst as
// DecodeCSBK does.
func DecodeCSBKBytes(b [12]byte) (CSBK, fec.Result) {
	return DecodeCSBK(bit.Unpack96(b))
}

// Bytes returns the 12-octet payload of s with its CRC, as Encode returns
// it packed most significant bit first, and the errors that Encode returns.
func (s *CSBK) Bytes() ([12]byte, error) {
	bits, err := s.Encode()
	if err != nil {
		return [12]byte{}, err
	}
	return bit.Pack96(bits), nil
}

func uint24(b []byte) uint32 {
	return uint32(b[0])<<16 | uint32(b[1])<<8 | uint32(b[2])
}

func putUint24(b []byte, v uint32) {
	b[0], b[1], b[2] = byte(v>>16), byte(v>>8), byte(v) //nolint:gosec // 24-bit address split into octets
}
