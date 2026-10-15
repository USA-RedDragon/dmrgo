package enums

import "fmt"

// AnnouncementType is the 5-bit announcement type of a C_BCAST PDU. Values
// that have no constant are reserved (ETSI TS 102 361-4 clause 7.2.19,
// Table 7.68).
type AnnouncementType uint8

const (
	AnnouncementAnnWDTSCC     AnnouncementType = 0b00000 // Ann_WD_TSCC, announce or withdraw a TSCC
	AnnouncementCallTimer     AnnouncementType = 0b00001 // CallTimer_Parms, call timer parameters
	AnnouncementVoteNow       AnnouncementType = 0b00010 // Vote_Now, vote now advice
	AnnouncementLocalTime     AnnouncementType = 0b00011 // Local_Time, local date and time
	AnnouncementMassReg       AnnouncementType = 0b00100 // MassReg, mass registration
	AnnouncementChanFreq      AnnouncementType = 0b00101 // Chan_Freq, logical channel frequency
	AnnouncementAdjacentSite  AnnouncementType = 0b00110 // Adjacent_Site, adjacent site information
	AnnouncementGenSiteParams AnnouncementType = 0b00111 // Gen_Site_Params, general site parameters

	AnnouncementManufacturerSpecific1 AnnouncementType = 0b11110 // manufacturer specific
	AnnouncementManufacturerSpecific2 AnnouncementType = 0b11111 // manufacturer specific
)

func (a AnnouncementType) String() string {
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
