package enums

import "fmt"

// ETSI TS 102 361-2 - 7.1.3.2 / Table 7.10 Activity ID
type ActivityID int

const (
	ActivityNoActivity               ActivityID = 0b0000
	ActivityGroupCSBK                ActivityID = 0b0010
	ActivityIndividualCSBK           ActivityID = 0b0011
	ActivityGroupVoice               ActivityID = 0b1000
	ActivityIndividualVoice          ActivityID = 0b1001
	ActivityIndividualData           ActivityID = 0b1010
	ActivityGroupData                ActivityID = 0b1011
	ActivityEmergencyGroupVoice      ActivityID = 0b1100
	ActivityEmergencyIndividualVoice ActivityID = 0b1101
)

func ActivityIDToName(a ActivityID) string {
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

func ActivityIDFromInt(i int) ActivityID {
	switch ActivityID(i) {
	case ActivityNoActivity:
		return ActivityNoActivity
	case ActivityGroupCSBK:
		return ActivityGroupCSBK
	case ActivityIndividualCSBK:
		return ActivityIndividualCSBK
	case ActivityGroupVoice:
		return ActivityGroupVoice
	case ActivityIndividualVoice:
		return ActivityIndividualVoice
	case ActivityGroupData:
		return ActivityGroupData
	case ActivityIndividualData:
		return ActivityIndividualData
	case ActivityEmergencyGroupVoice:
		return ActivityEmergencyGroupVoice
	case ActivityEmergencyIndividualVoice:
		return ActivityEmergencyIndividualVoice
	}
	return ActivityID(i)
}
