package pdu

import "github.com/USA-RedDragon/dmrgo/v2/enums"

// AnnWDTSCCParms is the C_BCAST payload with AnnouncementType Ann_WD_TSCC,
// which adds up to two TSCCs to the hunt list or withdraws them (ETSI TS
// 102 361-4 clause 7.2.19.1, Table 7.69).
//
//dmr:subpdu
type AnnWDTSCCParms struct {
	AnnouncementType enums.AnnouncementType `dmr:"bits:0-4,enum"` // AnnouncementAnnWDTSCC
	Reserved         uint8                  `dmr:"bits:5-8"`      // reserved, 0
	ColorCode1       enums.ColorCode        `dmr:"bits:9-12"`     // color code of Channel1
	ColorCode2       enums.ColorCode        `dmr:"bits:13-16"`    // color code of Channel2
	Withdraw1        bool                   `dmr:"bit:17"`        // AW_FLAG1: true withdraws Channel1 from the hunt list, false adds it
	Withdraw2        bool                   `dmr:"bit:18"`        // AW_FLAG2: true withdraws Channel2 from the hunt list, false adds it
	Reg              bool                   `dmr:"bit:19"`        // true when MSs must register
	Backoff          uint8                  `dmr:"bits:20-23"`    // backoff number, 1 to 15 (0 is reserved)
	SysIdentCode     uint16                 `dmr:"bits:24-39"`    // system identity code (C_SYScode) of the TSCC
	Channel1         uint16                 `dmr:"bits:40-51"`    // BCAST_CH1, a 12-bit channel number, 0 for CHNULL
	Channel2         uint16                 `dmr:"bits:52-63"`    // BCAST_CH2, a 12-bit channel number, 0 for CHNULL
}

// ChanFreqParms is the C_BCAST header payload with AnnouncementType
// Chan_Freq, which announces the frequencies of a channel number in the
// CdefParms of the BC_AP block that follows (ETSI TS 102 361-4 clause
// 7.2.19.6, Table 7.78).
//
//dmr:subpdu
type ChanFreqParms struct {
	AnnouncementType enums.AnnouncementType `dmr:"bits:0-4,enum"` // AnnouncementChanFreq
	Reserved1        uint16                 `dmr:"bits:5-18"`     // reserved, 0
	Reg              bool                   `dmr:"bit:19"`        // true when MSs must register
	Backoff          uint8                  `dmr:"bits:20-23"`    // backoff number, 1 to 15 (0 is reserved)
	SysIdentCode     uint16                 `dmr:"bits:24-39"`    // system identity code (C_SYScode) of the TSCC
	Reserved2        uint16                 `dmr:"bits:40-51"`    // reserved, 0
	Channel          uint16                 `dmr:"bits:52-63"`    // CH_ADJ, the 12-bit channel number announced
}

// AdjacentSiteParms is the C_BCAST payload with AnnouncementType
// Adjacent_Site, which announces the TSCC of an adjacent site (ETSI TS 102
// 361-4 clause 7.2.19.7, Table 7.79).
//
//dmr:subpdu
type AdjacentSiteParms struct {
	AnnouncementType  enums.AnnouncementType `dmr:"bits:0-4,enum"` // AnnouncementAdjacentSite
	AdjacentSysCode   uint16                 `dmr:"bits:5-18"`     // 14 most significant bits of the C_SYScode of the adjacent TSCC
	Reg               bool                   `dmr:"bit:19"`        // true when MSs must register
	Backoff           uint8                  `dmr:"bits:20-23"`    // backoff number, 1 to 15 (0 is reserved)
	SysIdentCode      uint16                 `dmr:"bits:24-39"`    // system identity code (C_SYScode) of the TSCC that sends it
	HasActiveConn     bool                   `dmr:"bit:40"`        // true when ActiveConn is valid
	ActiveConn        bool                   `dmr:"bit:41"`        // true when the adjacent site has a network connection
	ConfirmedPriority uint8                  `dmr:"bits:42-44"`    // priority of the confirmed channel
	AdjacentPriority  uint8                  `dmr:"bits:45-47"`    // priority of the adjacent channel
	Reserved          uint8                  `dmr:"bits:48-51"`    // reserved, 0
	Channel           uint16                 `dmr:"bits:52-63"`    // CH_ADJ, the 12-bit channel number of the adjacent TSCC
}

// AsAnnWDTSCC returns p as Ann_WD_TSCC parameters. ok is false for another
// AnnouncementType, or when a field of p does not fit its bits.
func (p *BroadcastPDU) AsAnnWDTSCC() (v AnnWDTSCCParms, ok bool) {
	b, err := p.Encode()
	if err != nil || p.AnnouncementType != enums.AnnouncementAnnWDTSCC {
		return v, false
	}
	return decodeAnnWDTSCCParms(b), true
}

// AsChanFreq returns p as Chan_Freq parameters, as AsAnnWDTSCC does.
func (p *BroadcastPDU) AsChanFreq() (v ChanFreqParms, ok bool) {
	b, err := p.Encode()
	if err != nil || p.AnnouncementType != enums.AnnouncementChanFreq {
		return v, false
	}
	return decodeChanFreqParms(b), true
}

// AsAdjacentSite returns p as Adjacent_Site parameters, as AsAnnWDTSCC
// does.
func (p *BroadcastPDU) AsAdjacentSite() (v AdjacentSiteParms, ok bool) {
	b, err := p.Encode()
	if err != nil || p.AnnouncementType != enums.AnnouncementAdjacentSite {
		return v, false
	}
	return decodeAdjacentSiteParms(b), true
}
