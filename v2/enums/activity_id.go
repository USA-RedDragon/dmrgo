package enums

import "fmt"

// ActivityID is the 4-bit activity of one BS time slot in an Activity Update
// short LC. Values that have no constant are reserved (ETSI TS 102 361-2
// clause 7.1.3.2, Table 7.10).
type ActivityID uint8

// The ActivityID values.
const (
	ActivityNoActivity               ActivityID = 0b0000 // no activity
	ActivityGroupCSBK                ActivityID = 0b0010 // group CSBK activity
	ActivityIndividualCSBK           ActivityID = 0b0011 // individual CSBK activity
	ActivityGroupVoice               ActivityID = 0b1000 // group voice activity
	ActivityIndividualVoice          ActivityID = 0b1001 // individual voice activity
	ActivityIndividualData           ActivityID = 0b1010 // individual data activity
	ActivityGroupData                ActivityID = 0b1011 // group data activity
	ActivityEmergencyGroupVoice      ActivityID = 0b1100 // emergency group voice activity
	ActivityEmergencyIndividualVoice ActivityID = 0b1101 // emergency individual voice activity
)

// String returns the name of the ActivityID. It allocates only for a value
// without a name.
func (a ActivityID) String() string {
	switch a {
	case ActivityNoActivity:
		return "No Activity"
	case ActivityGroupCSBK:
		return "Group CSBK"
	case ActivityIndividualCSBK:
		return "Individual CSBK"
	case ActivityGroupVoice:
		return "Group Voice"
	case ActivityIndividualVoice:
		return "Individual Voice"
	case ActivityGroupData:
		return "Group Data"
	case ActivityIndividualData:
		return "Individual Data"
	case ActivityEmergencyGroupVoice:
		return "Emergency Group Voice"
	case ActivityEmergencyIndividualVoice:
		return "Emergency Individual Voice"
	}
	return fmt.Sprintf("Reserved ActivityID(%d)", int(a))
}
