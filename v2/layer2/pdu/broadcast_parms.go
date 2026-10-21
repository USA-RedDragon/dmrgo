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

// CallTimerParms is the C_BCAST payload with AnnouncementType
// CallTimer_Parms, which sets the call timers of the MSs. For each timer 0
// means the internal timer of the MS and the largest value means infinity
// (ETSI TS 102 361-4 clause 7.2.19.2, Table 7.70).
//
//dmr:subpdu
type CallTimerParms struct {
	AnnouncementType enums.AnnouncementType `dmr:"bits:0-4,enum"` // AnnouncementCallTimer
	EmergencyTimer   uint16                 `dmr:"bits:5-13"`     // T_EMERG_TIMER, 1 to 510, 511 for infinity
	PacketTimer      uint8                  `dmr:"bits:14-18"`    // T_PACKET_TIMER, 1 to 30, 31 for infinity
	Reg              bool                   `dmr:"bit:19"`        // true when MSs must register
	Backoff          uint8                  `dmr:"bits:20-23"`    // backoff number, 1 to 15 (0 is reserved)
	SysIdentCode     uint16                 `dmr:"bits:24-39"`    // system identity code (C_SYScode) of the TSCC
	MSMSTimer        uint16                 `dmr:"bits:40-51"`    // T_MS-MS_TIMER, 1 to 4094, 4095 for infinity
	LineTimer        uint16                 `dmr:"bits:52-63"`    // T_MS-LINE_TIMER, 1 to 4094, 4095 for infinity
}

// VoteNowParms is the C_BCAST payload with AnnouncementType Vote_Now,
// which asks the MSs to assess a TSCC. A Channel of 0xFFF means that a
// VN_AP block with the frequencies follows (ETSI TS 102 361-4 clause
// 7.2.19.3, Table 7.71).
//
//dmr:subpdu
type VoteNowParms struct {
	AnnouncementType  enums.AnnouncementType `dmr:"bits:0-4,enum"` // AnnouncementVoteNow
	VoteSysCode       uint16                 `dmr:"bits:5-18"`     // 14 most significant bits of the C_SYScode of the TSCC to assess
	Reg               bool                   `dmr:"bit:19"`        // true when MSs must register
	Backoff           uint8                  `dmr:"bits:20-23"`    // backoff number, 1 to 15 (0 is reserved)
	SysIdentCode      uint16                 `dmr:"bits:24-39"`    // system identity code (C_SYScode) of the TSCC that sends it
	HasActiveConn     bool                   `dmr:"bit:40"`        // true when ActiveConn is valid
	ActiveConn        bool                   `dmr:"bit:41"`        // true when the assessed site has a network connection
	ConfirmedPriority uint8                  `dmr:"bits:42-44"`    // priority of the confirmed channel
	AdjacentPriority  uint8                  `dmr:"bits:45-47"`    // priority of the adjacent channel
	Reserved          uint8                  `dmr:"bits:48-51"`    // reserved, 0
	Channel           uint16                 `dmr:"bits:52-63"`    // CH_VOTE, the 12-bit channel number to assess, 0xFFF when a VN_AP block follows
}

// LocalTimeParms is the C_BCAST payload with AnnouncementType Local_Time,
// which broadcasts the local date and time and the offset from UTC (ETSI
// TS 102 361-4 clause 7.2.19.4, Tables 7.73 to 7.75).
//
//dmr:subpdu
type LocalTimeParms struct {
	AnnouncementType enums.AnnouncementType `dmr:"bits:0-4,enum"` // AnnouncementLocalTime
	Day              uint8                  `dmr:"bits:5-9"`      // B_DAY, day of the month 1 to 31, 0 when not broadcast
	Month            uint8                  `dmr:"bits:10-13"`    // B_MONTH, 1 for January to 12 for December, 0 when not broadcast
	UTCOffset        uint8                  `dmr:"bits:14-17"`    // UTC_OFFSET, hours 0 to 14, 15 when not broadcast
	UTCOffsetLags    bool                   `dmr:"bit:18"`        // UTC_OFFSET_SIGN: true when local time lags UTC (negative offset)
	Reg              bool                   `dmr:"bit:19"`        // true when MSs must register
	Backoff          uint8                  `dmr:"bits:20-23"`    // backoff number, 1 to 15 (0 is reserved)
	SysIdentCode     uint16                 `dmr:"bits:24-39"`    // system identity code (C_SYScode) of the TSCC
	Hours            uint8                  `dmr:"bits:40-44"`    // B_HOURS, 0 to 23
	Minutes          uint8                  `dmr:"bits:45-50"`    // B_MINS, 0 to 59
	Seconds          uint8                  `dmr:"bits:51-56"`    // B_SECS, 0 to 59
	DayOfWeek        uint8                  `dmr:"bits:57-59"`    // DAYOF_WEEK, 1 for Sunday to 7 for Saturday, 0 when not broadcast
	UTCOffsetQuarter uint8                  `dmr:"bits:60-61"`    // UTC_OFFSET_FRACTION: quarter hours added to UTCOffset, 0 to 3
	Reserved         uint8                  `dmr:"bits:62-63"`    // reserved, 0
}

// MassRegParms is the C_BCAST payload with AnnouncementType MassReg,
// which asks the MSs that match Address and AlohaMask to register within
// the registration window (ETSI TS 102 361-4 clause 7.2.19.5, Tables 7.76
// and 7.77).
//
//dmr:subpdu
type MassRegParms struct {
	AnnouncementType enums.AnnouncementType `dmr:"bits:0-4,enum"` // AnnouncementMassReg
	Reserved         uint8                  `dmr:"bits:5-9"`      // reserved, 0
	RegWindow        uint8                  `dmr:"bits:10-13"`    // Reg_Window code: 0 cancels, 1 to 15 for 0.5 s to 200000 s
	AlohaMask        uint8                  `dmr:"bits:14-18"`    // address mask, 0 to 24
	Reg              bool                   `dmr:"bit:19"`        // true when MSs must register
	Backoff          uint8                  `dmr:"bits:20-23"`    // backoff number, 1 to 15 (0 is reserved)
	SysIdentCode     uint16                 `dmr:"bits:24-39"`    // system identity code (C_SYScode) of the TSCC
	Address          uint32                 `dmr:"bits:40-63"`    // ADRNULL or a 24-bit individual MS address
}

// GenSiteParms is the C_BCAST payload with AnnouncementType
// Gen_Site_Params, which broadcasts the current site information and the
// network information (ETSI TS 102 361-4 clauses 7.2.19.8, 7.2.40 and
// 7.2.41, Tables 7.80, 7.101 and 7.102).
//
//dmr:subpdu
type GenSiteParms struct {
	AnnouncementType enums.AnnouncementType `dmr:"bits:0-4,enum"` // AnnouncementGenSiteParams
	Reserved1        uint16                 `dmr:"bits:5-18"`     // reserved, 0
	Reg              bool                   `dmr:"bit:19"`        // true when MSs must register
	Backoff          uint8                  `dmr:"bits:20-23"`    // backoff number, 1 to 15 (0 is reserved)
	SysIdentCode     uint16                 `dmr:"bits:24-39"`    // system identity code (C_SYScode) of the TSCC
	Reserved2        bool                   `dmr:"bit:40"`        // reserved, 0
	Hibernating      bool                   `dmr:"bit:41"`        // Hibernating_Flag: true when the TSCC is about to hibernate
	Reserved3        uint8                  `dmr:"bits:42-47"`    // reserved, 0
	Reserved4        uint8                  `dmr:"bits:48-55"`    // reserved, 0
	RegWithoutTG     bool                   `dmr:"bit:56"`        // true when MSs register without talkgroup Subscription/Attachment
	Reserved5        uint8                  `dmr:"bits:57-63"`    // reserved, 0
}

// AsCallTimer returns p as CallTimer_Parms parameters, as AsAnnWDTSCC
// does.
func (p *BroadcastPDU) AsCallTimer() (v CallTimerParms, ok bool) {
	b, err := p.Encode()
	if err != nil || p.AnnouncementType != enums.AnnouncementCallTimer {
		return v, false
	}
	return decodeCallTimerParms(b), true
}

// AsVoteNow returns p as Vote_Now parameters, as AsAnnWDTSCC does.
func (p *BroadcastPDU) AsVoteNow() (v VoteNowParms, ok bool) {
	b, err := p.Encode()
	if err != nil || p.AnnouncementType != enums.AnnouncementVoteNow {
		return v, false
	}
	return decodeVoteNowParms(b), true
}

// AsLocalTime returns p as Local_Time parameters, as AsAnnWDTSCC does.
func (p *BroadcastPDU) AsLocalTime() (v LocalTimeParms, ok bool) {
	b, err := p.Encode()
	if err != nil || p.AnnouncementType != enums.AnnouncementLocalTime {
		return v, false
	}
	return decodeLocalTimeParms(b), true
}

// AsMassReg returns p as MassReg parameters, as AsAnnWDTSCC does.
func (p *BroadcastPDU) AsMassReg() (v MassRegParms, ok bool) {
	b, err := p.Encode()
	if err != nil || p.AnnouncementType != enums.AnnouncementMassReg {
		return v, false
	}
	return decodeMassRegParms(b), true
}

// AsGenSite returns p as Gen_Site_Params parameters, as AsAnnWDTSCC does.
func (p *BroadcastPDU) AsGenSite() (v GenSiteParms, ok bool) {
	b, err := p.Encode()
	if err != nil || p.AnnouncementType != enums.AnnouncementGenSiteParams {
		return v, false
	}
	return decodeGenSiteParms(b), true
}
