package enums

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

func (flco FLCO) String() string {
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

// Valid reports whether flco is a value that ETSI TS 102 361 defines.
func (flco FLCO) Valid() bool {
	switch flco {
	case FLCOGroupVoiceChannelUser,
		FLCOUnitToUnitVoiceChannelUser,
		FLCOTalkerAliasHeader,
		FLCOTalkerAliasBlock1,
		FLCOTalkerAliasBlock2,
		FLCOTalkerAliasBlock3,
		FLCOGPSInfo,
		FLCOTerminatorDataLinkControl:
		return true
	}
	return false
}
