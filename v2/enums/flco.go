package enums

// FLCO is the 6-bit Full Link Control Opcode of a full LC (ETSI TS 102 361-2
// Table B.1, ETSI TS 102 361-3 clause 7.1.1).
type FLCO byte

// The FLCO values.
const (
	FLCOGroupVoiceChannelUser      FLCO = 0b000000 // Grp_V_Ch_Usr, group voice channel user
	FLCOUnitToUnitVoiceChannelUser FLCO = 0b000011 // UU_V_Ch_Usr, unit to unit voice channel user
	FLCOTalkerAliasHeader          FLCO = 0b000100 // Talker_Alias_hdr
	FLCOTalkerAliasBlock1          FLCO = 0b000101 // Talker_Alias_blk1
	FLCOTalkerAliasBlock2          FLCO = 0b000110 // Talker_Alias_blk2
	FLCOTalkerAliasBlock3          FLCO = 0b000111 // Talker_Alias_blk3
	FLCOGPSInfo                    FLCO = 0b001000 // GPS_Info
	FLCOTerminatorDataLinkControl  FLCO = 0b110000 // TD_LC, terminator data link control
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
