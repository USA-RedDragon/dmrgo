package pdu

import (
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/fec"
)

// ShortLCNullMessage is the Null Message Short LC payload (ETSI TS 102 361-2
// clause 7.1.1).
type ShortLCNullMessage struct {
	Reserved uint32 `dmr:"bits:0-23"`
}

// ShortLCActivityUpdate is the Activity Update Short LC payload (ETSI TS 102
// 361-2 clause 7.1.2).
type ShortLCActivityUpdate struct {
	TS1ActivityID enums.ActivityID `dmr:"bits:0-3,enum"`
	TS2ActivityID enums.ActivityID `dmr:"bits:4-7,enum"`
	HashTS1       byte             `dmr:"bits:8-15"`
	HashTS2       byte             `dmr:"bits:16-23"`
}

// ShortLCCSysParms is the C_SYS_Parms Short LC payload (ETSI TS 102 361-4
// clause 7.1.2).
// MODEL-dependent NET/SITE split: use NetID()/SiteID() helpers to interpret.
type ShortLCCSysParms struct {
	MODEL             uint8  `dmr:"bits:0-1"`
	NetSiteRaw        uint16 `dmr:"bits:2-13"`
	Reg               bool   `dmr:"bit:14"`
	CommonSlotCounter uint16 `dmr:"bits:15-23"`
}

// netSiteBits returns the NET field width for a MODEL value per
// ETSI TS 102 361-4 Table 6.5. The SITE field takes the remaining bits.
func netSiteBits(model uint8) int {
	switch model {
	case 0:
		return 9
	case 1:
		return 7
	case 2:
		return 4
	default:
		return 2
	}
}

const netSiteLen = 12

// NetID returns the network ID based on the MODEL field
// (ETSI TS 102 361-4 Table 6.5): Tiny 9 bits, Small 7 bits,
// Large 4 bits, Huge 2 bits.
func (s *ShortLCCSysParms) NetID() uint16 {
	return netID(s.MODEL, s.NetSiteRaw)
}

// SiteID returns the site ID based on the MODEL field
// (ETSI TS 102 361-4 Table 6.5): Tiny 3 bits, Small 5 bits,
// Large 8 bits, Huge 10 bits.
func (s *ShortLCCSysParms) SiteID() uint16 {
	return siteID(s.MODEL, s.NetSiteRaw)
}

func netID(model uint8, netSite uint16) uint16 {
	return netSite >> (netSiteLen - netSiteBits(model))
}

func siteID(model uint8, netSite uint16) uint16 {
	return netSite & (1<<(netSiteLen-netSiteBits(model)) - 1)
}

// ShortLCPSysParms is the P_SYS_Parms Short LC payload (ETSI TS 102 361-4
// clause 7.1.2).
// MODEL-dependent NET/SITE split: use NetID()/SiteID() helpers to interpret.
type ShortLCPSysParms struct {
	MODEL              uint8  `dmr:"bits:0-1"`
	NetSiteRaw         uint16 `dmr:"bits:2-13"`
	PayloadChannelType bool   `dmr:"bit:14"`
	CommonSlotCounter  uint16 `dmr:"bits:15-23"`
}

// NetID returns the network ID based on the MODEL field
// (ETSI TS 102 361-4 Table 6.5).
func (s *ShortLCPSysParms) NetID() uint16 {
	return netID(s.MODEL, s.NetSiteRaw)
}

// SiteID returns the site ID based on the MODEL field
// (ETSI TS 102 361-4 Table 6.5).
func (s *ShortLCPSysParms) SiteID() uint16 {
	return siteID(s.MODEL, s.NetSiteRaw)
}

// ShortLC is the Short Link Control (Short LC) PDU (ETSI TS 102 361-1 clause
// 9.1.7).
//
// ShortLC.Encode returns an error if more than one payload field is set, or
// if SLCO does not select the payload field that is set.
//
//dmr:crc crc_8
//dmr:input_size 36
type ShortLC struct {
	SLCO enums.SLCO `dmr:"bits:0-3,enum"`
	FEC  fec.Result `dmr:"-"`

	NullMessage    *ShortLCNullMessage    `dmr:"bits:4-27,dispatch:SLCO=enums.SLCONullMessage"`
	ActivityUpdate *ShortLCActivityUpdate `dmr:"bits:4-27,dispatch:SLCO=enums.SLCOActivityUpdate"`
	CSysParms      *ShortLCCSysParms      `dmr:"bits:4-27,dispatch:SLCO=enums.SLCOCSysParms"`
	PSysParms      *ShortLCPSysParms      `dmr:"bits:4-27,dispatch:SLCO=enums.SLCOPSysParms"`

	// RawPayload holds bits 4 to 27 when no payload field is set, packed
	// most significant bit first: bit 4 is the top bit of byte 0.
	RawPayload [3]byte `dmr:"bits:4-27,raw,fallback"`
}
