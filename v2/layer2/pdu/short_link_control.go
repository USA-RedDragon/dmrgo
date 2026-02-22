package pdu

import (
	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/fec"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/elements"
)

// ETSI TS 102 361-2 - 7.1.1 Short LC Null Message PDU
type ShortLCNullMessage struct {
	Reserved [24]bit.Bit `dmr:"bits:0-23,raw"`
}

// ETSI TS 102 361-2 - 7.1.2 Activity Update PDU
type ShortLCActivityUpdate struct {
	TS1ActivityID enums.ActivityID `dmr:"bits:0-3,enum"`
	TS2ActivityID enums.ActivityID `dmr:"bits:4-7,enum"`
	HashTS1       byte             `dmr:"bits:8-15"`
	HashTS2       byte             `dmr:"bits:16-23"`
}

// ETSI TS 102 361-4 §7.1.2 C_SYS_Parms Short LC PDU
// MODEL-dependent NET/SITE split: use NetID()/SiteID() helpers to interpret.
type ShortLCCSysParms struct {
	MODEL             uint8       `dmr:"bits:0-1"`
	NetSiteRaw        [12]bit.Bit `dmr:"bits:2-13,raw"`
	Reg               bool        `dmr:"bit:14"`
	CommonSlotCounter uint16      `dmr:"bits:15-23"`
}

// NetID returns the network ID based on the MODEL field.
// MODEL 00 (Tiny): 6-bit NET, MODEL 01 (Small): 7-bit NET,
// MODEL 10 (Large): 9-bit NET, MODEL 11 (Huge): 12-bit NET.
func (s *ShortLCCSysParms) NetID() uint16 {
	switch s.MODEL {
	case 0: // Tiny: bits 0-5
		return uint16(bit.BitsToUint16(s.NetSiteRaw[:], 0, 6))
	case 1: // Small: bits 0-6
		return uint16(bit.BitsToUint16(s.NetSiteRaw[:], 0, 7))
	case 2: // Large: bits 0-8
		return uint16(bit.BitsToUint16(s.NetSiteRaw[:], 0, 9))
	case 3: // Huge: bits 0-11
		return uint16(bit.BitsToUint16(s.NetSiteRaw[:], 0, 12))
	default:
		return 0
	}
}

// SiteID returns the site ID based on the MODEL field.
// MODEL 00 (Tiny): 4-bit SITE, MODEL 01 (Small): 5-bit SITE,
// MODEL 10 (Large): 3-bit SITE, MODEL 11 (Huge): no site bits (returns 0).
func (s *ShortLCCSysParms) SiteID() uint8 {
	switch s.MODEL {
	case 0: // Tiny: bits 6-9
		return bit.BitsToUint8(s.NetSiteRaw[:], 6, 4)
	case 1: // Small: bits 7-11
		return bit.BitsToUint8(s.NetSiteRaw[:], 7, 5)
	case 2: // Large: bits 9-11
		return bit.BitsToUint8(s.NetSiteRaw[:], 9, 3)
	case 3: // Huge: no site bits
		return 0
	default:
		return 0
	}
}

// ETSI TS 102 361-4 §7.1.2 P_SYS_Parms Short LC PDU
// MODEL-dependent NET/SITE split: use NetID()/SiteID() helpers to interpret.
type ShortLCPSysParms struct {
	MODEL              uint8       `dmr:"bits:0-1"`
	NetSiteRaw         [12]bit.Bit `dmr:"bits:2-13,raw"`
	PayloadChannelType bool        `dmr:"bit:14"`
	CommonSlotCounter  uint16      `dmr:"bits:15-23"`
}

// NetID returns the network ID based on the MODEL field.
func (s *ShortLCPSysParms) NetID() uint16 {
	switch s.MODEL {
	case 0:
		return uint16(bit.BitsToUint16(s.NetSiteRaw[:], 0, 6))
	case 1:
		return uint16(bit.BitsToUint16(s.NetSiteRaw[:], 0, 7))
	case 2:
		return uint16(bit.BitsToUint16(s.NetSiteRaw[:], 0, 9))
	case 3:
		return uint16(bit.BitsToUint16(s.NetSiteRaw[:], 0, 12))
	default:
		return 0
	}
}

// SiteID returns the site ID based on the MODEL field.
func (s *ShortLCPSysParms) SiteID() uint8 {
	switch s.MODEL {
	case 0:
		return bit.BitsToUint8(s.NetSiteRaw[:], 6, 4)
	case 1:
		return bit.BitsToUint8(s.NetSiteRaw[:], 7, 5)
	case 2:
		return bit.BitsToUint8(s.NetSiteRaw[:], 9, 3)
	case 3:
		return 0
	default:
		return 0
	}
}

// dmr:crc crc_8
// dmr:input_size 36
// ETSI TS 102 361-1 - 9.1.7 Short Link Control (SHORT_LC) PDU
type ShortLC struct {
	DataType elements.DataType `dmr:"-"`

	SLCO enums.SLCO    `dmr:"bits:0-3,enum"`
	FEC  fec.FECResult `dmr:"-"`

	NullMessage    *ShortLCNullMessage    `dmr:"bits:4-27,dispatch:SLCO=enums.SLCONullMessage"`
	ActivityUpdate *ShortLCActivityUpdate `dmr:"bits:4-27,dispatch:SLCO=enums.SLCOActivityUpdate"`
	CSysParms      *ShortLCCSysParms      `dmr:"bits:4-27,dispatch:SLCO=enums.SLCOCSysParms"`
	PSysParms      *ShortLCPSysParms      `dmr:"bits:4-27,dispatch:SLCO=enums.SLCOPSysParms"`

	crc uint8 `dmr:"-"` //nolint:unused
}

func (slc *ShortLC) GetDataType() elements.DataType {
	return slc.DataType
}
