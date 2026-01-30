package enums

// ETSI TS 102 361-1 V2.5.1 (2017-10) - 9.1.1 Synchronization (SYNC) PDU - Table 9.2: SYNC patterns
type SyncPattern int64

const (
	BsSourcedVoice            SyncPattern = 0x755FD7DF75F7
	BsSourcedData             SyncPattern = 0xDFF57D75DF5D
	MsSourcedVoice            SyncPattern = 0x7F7D5DD57DFD
	MsSourcedData             SyncPattern = 0xD5D7F77FD757
	MsSourcedRcSync           SyncPattern = 0x77D55F7DFD77
	Tdma1Voice                SyncPattern = 0x5D577F7757FF
	Tdma1Data                 SyncPattern = 0xF7FDD5DDFD55
	Tdma2Voice                SyncPattern = 0x7DFFD5F55D5F
	Tdma2Data                 SyncPattern = 0xD7557F5FF7F5
	Reserved                  SyncPattern = 0xDD7FF5D757DD
	EmbeddedSignallingPattern SyncPattern = -1
)

// SyncPatternFromBytes returns the SyncPattern that matches the given burst.
func SyncPatternFromBytes(syncOrEmbeddedSignalling [6]byte) SyncPattern {
	// Convert 6 bytes into an int64
	var syncOrEmbeddedSignallingInt64 int64
	for i := 0; i < 6; i++ {
		syncOrEmbeddedSignallingInt64 |= int64(syncOrEmbeddedSignalling[i]) << (8 * (5 - i))
	}

	switch SyncPattern(syncOrEmbeddedSignallingInt64) {
	case BsSourcedVoice:
		return BsSourcedVoice
	case BsSourcedData:
		return BsSourcedData
	case MsSourcedVoice:
		return MsSourcedVoice
	case MsSourcedData:
		return MsSourcedData
	case MsSourcedRcSync:
		return MsSourcedRcSync
	case Tdma1Voice:
		return Tdma1Voice
	case Tdma1Data:
		return Tdma1Data
	case Tdma2Voice:
		return Tdma2Voice
	case Tdma2Data:
		return Tdma2Data
	case EmbeddedSignallingPattern:
		return EmbeddedSignallingPattern
	case Reserved:
		return Reserved
	default:
		return EmbeddedSignallingPattern
	}
}

// SyncPatternToName returns the name of the SyncPattern that matches the given burst.
func SyncPatternToName(sync SyncPattern) string {
	switch sync {
	case BsSourcedVoice:
		return "BS-sourced voice"
	case BsSourcedData:
		return "BS-sourced data"
	case MsSourcedVoice:
		return "MS-sourced voice"
	case MsSourcedData:
		return "MS-sourced data"
	case MsSourcedRcSync:
		return "MS-sourced RC sync"
	case Tdma1Voice:
		return "TDMA1 voice"
	case Tdma1Data:
		return "TDMA1 data"
	case Tdma2Voice:
		return "TDMA2 voice"
	case Tdma2Data:
		return "TDMA2 data"
	case EmbeddedSignallingPattern:
		return "Embedded Signalling"
	case Reserved:
		return "Reserved"
	default:
		return "Embedded Signalling"
	}
}
