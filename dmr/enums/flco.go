package enums

import "fmt"

type FLCO byte

const (
	FLCOGroupVoiceChannelUser      FLCO = 0b000000
	FLCOUnitToUnitVoiceChannelUser FLCO = 0b000011
	FLCOTalkerAliasHeader          FLCO = 0b000100
	FLCOTalkerAliasBlock1          FLCO = 0b000101
	FLCOTalkerAliasBlock2          FLCO = 0b000110
	FLCOTalkerAliasBlock3          FLCO = 0b000111
	FLCOGPSInfo                    FLCO = 0b001000
	FLCOTerminatorDataLinkControl  FLCO = 0b110000
)

func FLCOToName(flco FLCO) string {
	switch flco {
	case FLCOGroupVoiceChannelUser:
		return "Group Voice Channel User"
	case FLCOUnitToUnitVoiceChannelUser:
		return "Unit to Unit Voice Channel User"
	case FLCOTalkerAliasHeader:
		return "Talker Alias Header"
	case FLCOTalkerAliasBlock1:
		return "Talker Alias Block 1"
	case FLCOTalkerAliasBlock2:
		return "Talker Alias Block 2"
	case FLCOTalkerAliasBlock3:
		return "Talker Alias Block 3"
	case FLCOGPSInfo:
		return "GPS Info"
	case FLCOTerminatorDataLinkControl:
		return "Terminator Data Link Control"
	}
	return "Unknown"
}

func FLCOFromInt(i int) (FLCO, error) {
	switch FLCO(i) {
	case FLCOGroupVoiceChannelUser:
		return FLCOGroupVoiceChannelUser, nil
	case FLCOUnitToUnitVoiceChannelUser:
		return FLCOUnitToUnitVoiceChannelUser, nil
	case FLCOTalkerAliasHeader:
		return FLCOTalkerAliasHeader, nil
	case FLCOTalkerAliasBlock1:
		return FLCOTalkerAliasBlock1, nil
	case FLCOTalkerAliasBlock2:
		return FLCOTalkerAliasBlock2, nil
	case FLCOTalkerAliasBlock3:
		return FLCOTalkerAliasBlock3, nil
	case FLCOGPSInfo:
		return FLCOGPSInfo, nil
	case FLCOTerminatorDataLinkControl:
		return FLCOTerminatorDataLinkControl, nil
	}
	return 0, fmt.Errorf("Invalid FLCO value: %d", i)
}
