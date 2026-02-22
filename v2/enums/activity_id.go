package enums

import "fmt"

// ETSI TS 102 361-2 - 7.1.3 / Table B.3 Activity ID
type ActivityID int

const (
	ActivityNoActivity      ActivityID = 0b0000
	ActivityGroupCSBK       ActivityID = 0b0010
	ActivityIndividualCSBK  ActivityID = 0b0011
	ActivityGroupVoice      ActivityID = 0b1000
	ActivityIndividualVoice ActivityID = 0b1001
	ActivityGroupData       ActivityID = 0b1010
	ActivityIndividualData  ActivityID = 0b1011
	ActivityAllEmergency    ActivityID = 0b1100
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
	case ActivityAllEmergency:
		return "All/Emergency"
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
	case ActivityAllEmergency:
		return ActivityAllEmergency
	}
	return ActivityID(i)
}
