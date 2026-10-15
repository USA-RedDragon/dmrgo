package enums

import (
	"fmt"
	"math/bits"
)

// ETSI TS 102 361-1 - 9.1.1 Synchronization (SYNC) PDU - Table 9.2: SYNC patterns
type SyncPattern uint64

const embeddedSignallingName = "Embedded Signalling"

const (
	SyncBSSourcedVoice SyncPattern = 0x755FD7DF75F7
	SyncBSSourcedData  SyncPattern = 0xDFF57D75DF5D
	SyncMSSourcedVoice SyncPattern = 0x7F7D5DD57DFD
	SyncMSSourcedData  SyncPattern = 0xD5D7F77FD757
	SyncMSSourcedRC    SyncPattern = 0x77D55F7DFD77
	SyncTDMA1Voice     SyncPattern = 0x5D577F7757FF
	SyncTDMA1Data      SyncPattern = 0xF7FDD5DDFD55
	SyncTDMA2Voice     SyncPattern = 0x7DFFD5F55D5F
	SyncTDMA2Data      SyncPattern = 0xD7557F5FF7F5
	SyncReserved       SyncPattern = 0xDD7FF5D757DD
	// SyncNone is the SyncPattern of a burst whose centre carries embedded
	// signalling instead of a SYNC pattern.
	SyncNone SyncPattern = 0
)

//nolint:gochecknoglobals
var syncPatterns = [...]SyncPattern{
	SyncBSSourcedVoice,
	SyncBSSourcedData,
	SyncMSSourcedVoice,
	SyncMSSourcedData,
	SyncMSSourcedRC,
	SyncTDMA1Voice,
	SyncTDMA1Data,
	SyncTDMA2Voice,
	SyncTDMA2Data,
	SyncReserved,
}

// NearestSyncPattern returns the Table 9.2 pattern nearest to the 48-bit
// field and its Hamming distance from that pattern.
func NearestSyncPattern(field [6]byte) (SyncPattern, int) {
	var value uint64
	for i := 0; i < 6; i++ {
		value = value<<8 | uint64(field[i])
	}

	best, bestDistance := syncPatterns[0], 49
	for _, pattern := range syncPatterns {
		if distance := bits.OnesCount64(value ^ uint64(pattern)); distance < bestDistance {
			best, bestDistance = pattern, distance
		}
	}
	return best, bestDistance
}

// String returns the name of the SyncPattern.
func (sync SyncPattern) String() string {
	switch sync {
	case SyncBSSourcedVoice:
		return "BS-sourced voice"
	case SyncBSSourcedData:
		return "BS-sourced data"
	case SyncMSSourcedVoice:
		return "MS-sourced voice"
	case SyncMSSourcedData:
		return "MS-sourced data"
	case SyncMSSourcedRC:
		return "MS-sourced RC sync"
	case SyncTDMA1Voice:
		return "TDMA1 voice"
	case SyncTDMA1Data:
		return "TDMA1 data"
	case SyncTDMA2Voice:
		return "TDMA2 voice"
	case SyncTDMA2Data:
		return "TDMA2 data"
	case SyncNone:
		return embeddedSignallingName
	case SyncReserved:
		return reservedName
	default:
		return fmt.Sprintf("Unknown SyncPattern %#012x", uint64(sync))
	}
}
