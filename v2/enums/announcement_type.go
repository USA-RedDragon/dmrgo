package enums

import "fmt"

// AnnouncementType represents the 5-bit announcement type in a C_BCAST PDU.
// ETSI TS 102 361-4 — §7.2.19, Table 7.68
type AnnouncementType int

const (
	AnnouncementAnnWDTSCC     AnnouncementType = 0b00000
	AnnouncementCallTimer     AnnouncementType = 0b00001
	AnnouncementVoteNow       AnnouncementType = 0b00010
	AnnouncementLocalTime     AnnouncementType = 0b00011
	AnnouncementMassReg       AnnouncementType = 0b00100
	AnnouncementChanFreq      AnnouncementType = 0b00101
	AnnouncementAdjacentSite  AnnouncementType = 0b00110
	AnnouncementGenSiteParams AnnouncementType = 0b00111

	AnnouncementManufacturerSpecific1 AnnouncementType = 0b11110
	AnnouncementManufacturerSpecific2 AnnouncementType = 0b11111
)

func AnnouncementTypeToName(a AnnouncementType) string {
	switch a {
	case AnnouncementAnnWDTSCC:
		return "Ann_WD_TSCC"
	case AnnouncementCallTimer:
		return "CallTimer_Parms"
	case AnnouncementVoteNow:
		return "Vote_Now"
	case AnnouncementLocalTime:
		return "Local_Time"
	case AnnouncementMassReg:
		return "MassReg"
	case AnnouncementChanFreq:
		return "Chan_Freq"
	case AnnouncementAdjacentSite:
		return "Adjacent_Site"
	case AnnouncementGenSiteParams:
		return "Gen_Site_Params"
	case AnnouncementManufacturerSpecific1, AnnouncementManufacturerSpecific2:
		return manufacturerSpecificName
	default:
		return fmt.Sprintf("Reserved AnnouncementType(%d)", int(a))
	}
}

func AnnouncementTypeFromInt(i int) AnnouncementType {
	return AnnouncementType(i)
}
