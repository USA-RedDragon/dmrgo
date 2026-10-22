package pdu

import (
	"github.com/USA-RedDragon/dmrgo/v2"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/fec"
)

// ShortLCNullMessage is the Null Message Short LC payload (ETSI TS 102 361-2
// clause 7.1.3.1).
type ShortLCNullMessage struct {
	Reserved uint32 `dmr:"bits:0-23"` // reserved
}

// ShortLCActivityUpdate is the Activity Update Short LC payload (ETSI TS 102
// 361-2 clause 7.1.3.2).
type ShortLCActivityUpdate struct {
	TS1ActivityID enums.ActivityID `dmr:"bits:0-3,enum"` // activity of timeslot 1
	TS2ActivityID enums.ActivityID `dmr:"bits:4-7,enum"` // activity of timeslot 2
	HashTS1       byte             `dmr:"bits:8-15"`     // 8-bit address hash for timeslot 1
	HashTS2       byte             `dmr:"bits:16-23"`    // 8-bit address hash for timeslot 2
}

// ShortLCCSysParms is the C_SYS_Parms Short LC payload. NetID, SiteID and
// SysCode split NetSiteRaw by MODEL. (ETSI TS 102 361-4 clause 7.1.2.1)
type ShortLCCSysParms struct {
	Model             NetworkModel `dmr:"bits:0-1"`   // network model (MODEL)
	NetSiteRaw        uint16       `dmr:"bits:2-13"`  // NET and SITE, split by MODEL
	Reg               bool         `dmr:"bit:14"`     // true when the TSCC requires the MS to register
	CommonSlotCounter uint16       `dmr:"bits:15-23"` // 9-bit Common_Slot_Counter
}

// ETSI TS 102 361-4 Table 6.5
func netSiteBits(model NetworkModel) int {
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

// NetID returns the network ID based on the MODEL field: Tiny 9 bits, Small 7
// bits, Large 4 bits, Huge 2 bits. It returns a *dmrgo.FieldRangeError when
// MODEL or NetSiteRaw does not fit its bits. (ETSI TS 102 361-4 Table 6.5)
func (s *ShortLCCSysParms) NetID() (uint16, error) {
	if s == nil {
		return 0, dmrgo.ErrNilReceiver
	}
	if err := checkNetSite(s.Model, s.NetSiteRaw, errShortLCCSysParmsModelRange, errShortLCCSysParmsNetSiteRawRange); err != nil {
		return 0, err
	}
	return netID(s.Model, s.NetSiteRaw), nil
}

// SiteID returns the site ID based on the MODEL field: Tiny 3 bits, Small 5
// bits, Large 8 bits, Huge 10 bits. It returns the errors of NetID.
// (ETSI TS 102 361-4 Table 6.5)
func (s *ShortLCCSysParms) SiteID() (uint16, error) {
	if s == nil {
		return 0, dmrgo.ErrNilReceiver
	}
	if err := checkNetSite(s.Model, s.NetSiteRaw, errShortLCCSysParmsModelRange, errShortLCCSysParmsNetSiteRawRange); err != nil {
		return 0, err
	}
	return siteID(s.Model, s.NetSiteRaw), nil
}

func netID(model NetworkModel, netSite uint16) uint16 {
	return netSite >> (netSiteLen - netSiteBits(model))
}

func siteID(model NetworkModel, netSite uint16) uint16 {
	return netSite & (1<<(netSiteLen-netSiteBits(model)) - 1)
}

// ShortLCPSysParms is the P_SYS_Parms Short LC payload. NetID, SiteID and
// SysCode split NetSiteRaw by MODEL. (ETSI TS 102 361-4 clause 7.1.2.2)
type ShortLCPSysParms struct {
	Model              NetworkModel `dmr:"bits:0-1"`   // network model (MODEL)
	NetSiteRaw         uint16       `dmr:"bits:2-13"`  // NET and SITE, split by MODEL
	PayloadChannelType bool         `dmr:"bit:14"`     // true when the payload channel is a composite control channel
	CommonSlotCounter  uint16       `dmr:"bits:15-23"` // 9-bit Common_Slot_Counter
}

// NetID returns the network ID based on the MODEL field
// (ETSI TS 102 361-4 Table 6.5), with the errors of ShortLCCSysParms.NetID.
func (s *ShortLCPSysParms) NetID() (uint16, error) {
	if s == nil {
		return 0, dmrgo.ErrNilReceiver
	}
	if err := checkNetSite(s.Model, s.NetSiteRaw, errShortLCPSysParmsModelRange, errShortLCPSysParmsNetSiteRawRange); err != nil {
		return 0, err
	}
	return netID(s.Model, s.NetSiteRaw), nil
}

// SiteID returns the site ID based on the MODEL field
// (ETSI TS 102 361-4 Table 6.5), with the errors of ShortLCCSysParms.NetID.
func (s *ShortLCPSysParms) SiteID() (uint16, error) {
	if s == nil {
		return 0, dmrgo.ErrNilReceiver
	}
	if err := checkNetSite(s.Model, s.NetSiteRaw, errShortLCPSysParmsModelRange, errShortLCPSysParmsNetSiteRawRange); err != nil {
		return 0, err
	}
	return siteID(s.Model, s.NetSiteRaw), nil
}

// ShortLC is the Short Link Control (Short LC) PDU. SLCO selects the view
// of RawPayload: AsActivityUpdate, for example, decodes it for SLCO
// enums.SLCOActivityUpdate. (ETSI TS 102 361-1 clause 9.1.7)
//
//dmr:crc crc_8
//dmr:input_size 36
//dmr:variant NullMessage ShortLCNullMessage SLCO=enums.SLCONullMessage
//dmr:variant ActivityUpdate ShortLCActivityUpdate SLCO=enums.SLCOActivityUpdate
//dmr:variant CSysParms ShortLCCSysParms SLCO=enums.SLCOCSysParms
//dmr:variant PSysParms ShortLCPSysParms SLCO=enums.SLCOPSysParms
type ShortLC struct {
	SLCO enums.SLCO `dmr:"bits:0-3,enum"` // Short Link Control Opcode (SLCO)
	FEC  fec.Result `dmr:"-"`             // result of the CRC-8 check

	// RawPayload holds bits 4 to 27, the payload that SLCO selects, packed
	// most significant bit first: bit 4 is the top bit of byte 0.
	RawPayload [3]byte `dmr:"bits:4-27,raw,union"`
}
