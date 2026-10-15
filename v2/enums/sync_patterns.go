package enums

import "math/bits"

// ETSI TS 102 361-1 - 9.1.1 Synchronization (SYNC) PDU - Table 9.2: SYNC patterns
type SyncPattern int64

const embeddedSignallingName = "Embedded Signalling"

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

// SyncMatchThreshold is the largest Hamming distance, in bits, at which a
// received 48-bit SYNC field still matches a Table 9.2 pattern. Common DMR
// decoders accept up to 4 bit errors. The Table 9.2 patterns are at least
// 10 bits apart, so a field can be within the threshold of one pattern only.
const SyncMatchThreshold = 4

//nolint:gochecknoglobals
var syncPatterns = [...]SyncPattern{
	BsSourcedVoice,
	BsSourcedData,
	MsSourcedVoice,
	MsSourcedData,
	MsSourcedRcSync,
	Tdma1Voice,
	Tdma1Data,
	Tdma2Voice,
	Tdma2Data,
	Reserved,
}

// SyncPatternFromBytes returns the SyncPattern that matches the given 48-bit
// SYNC field. See SyncPatternFromBytesWithDistance.
func SyncPatternFromBytes(syncOrEmbeddedSignalling [6]byte) SyncPattern {
	pattern, _ := SyncPatternFromBytesWithDistance(syncOrEmbeddedSignalling)
	return pattern
}

// SyncPatternFromBytesWithDistance returns the Table 9.2 pattern nearest to
// the given 48-bit SYNC field and its Hamming distance from that pattern.
// If the distance is more than SyncMatchThreshold, the field is taken as
// embedded signalling and the pattern is EmbeddedSignallingPattern. The
// distance is always to the nearest Table 9.2 pattern.
func SyncPatternFromBytesWithDistance(syncOrEmbeddedSignalling [6]byte) (SyncPattern, int) {
	var value uint64
	for i := 0; i < 6; i++ {
		value = value<<8 | uint64(syncOrEmbeddedSignalling[i])
	}

	best := EmbeddedSignallingPattern
	bestDistance := 49
	for _, pattern := range syncPatterns {
		distance := bits.OnesCount64(value ^ uint64(pattern)) //nolint:gosec // patterns are 48-bit positive constants
		if distance < bestDistance {
			best = pattern
			bestDistance = distance
		}
	}
	if bestDistance > SyncMatchThreshold {
		return EmbeddedSignallingPattern, bestDistance
	}
	return best, bestDistance
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
		return embeddedSignallingName
	case Reserved:
		return reservedName
	default:
		return embeddedSignallingName
	}
}
