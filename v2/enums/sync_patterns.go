package enums

import (
	"math/bits"

	"github.com/USA-RedDragon/dmrgo/v2/internal/text"
)

// SyncPattern is the 48-bit SYNC pattern in the center of a burst, as an
// integer with the first bit most significant (ETSI TS 102 361-1 clause
// 9.1.1, Table 9.2).
type SyncPattern uint64

const embeddedSignallingName = "Embedded Signalling"

// The SyncPattern values.
const (
	SyncBSSourcedVoice SyncPattern = 0x755FD7DF75F7 // BS sourced voice
	SyncBSSourcedData  SyncPattern = 0xDFF57D75DF5D // BS sourced data
	SyncMSSourcedVoice SyncPattern = 0x7F7D5DD57DFD // MS sourced voice
	SyncMSSourcedData  SyncPattern = 0xD5D7F77FD757 // MS sourced data
	SyncMSSourcedRC    SyncPattern = 0x77D55F7DFD77 // MS sourced RC sync
	SyncTDMA1Voice     SyncPattern = 0x5D577F7757FF // TDMA direct mode time slot 1 voice
	SyncTDMA1Data      SyncPattern = 0xF7FDD5DDFD55 // TDMA direct mode time slot 1 data
	SyncTDMA2Voice     SyncPattern = 0x7DFFD5F55D5F // TDMA direct mode time slot 2 voice
	SyncTDMA2Data      SyncPattern = 0xD7557F5FF7F5 // TDMA direct mode time slot 2 data
	SyncReserved       SyncPattern = 0xDD7FF5D757DD // reserved pattern
	// SyncNone is the SyncPattern of a burst whose center carries embedded
	// signalling instead of a SYNC pattern.
	SyncNone SyncPattern = 0
)

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

// String returns the name of the SyncPattern. It allocates only for a value
// without a name.
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
		return "Unknown SyncPattern 0x" + text.Digits(uint64(sync), 12, 4)
	}
}
