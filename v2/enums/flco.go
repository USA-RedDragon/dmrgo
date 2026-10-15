package enums

import (
	"fmt"
	"math"
)

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

// FLCOFromInt converts i to a FLCO. Unknown values in 0-255 are returned
// as-is with a non-nil error so callers can keep the raw value.
func FLCOFromInt(i int) (FLCO, error) {
	if i < 0 || i > math.MaxUint8 {
		return 0, fmt.Errorf("FLCO value out of range: %d", i)
	}
	v := FLCO(i)
	switch v {
	case FLCOGroupVoiceChannelUser,
		FLCOUnitToUnitVoiceChannelUser,
		FLCOTalkerAliasHeader,
		FLCOTalkerAliasBlock1,
		FLCOTalkerAliasBlock2,
		FLCOTalkerAliasBlock3,
		FLCOGPSInfo,
		FLCOTerminatorDataLinkControl:
		return v, nil
	}
	return v, fmt.Errorf("invalid FLCO value: %d", i)
}
