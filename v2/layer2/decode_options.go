package layer2

import (
	"errors"
	"fmt"
	"math/bits"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
)

// ErrColorCodeMismatch is returned, wrapped, by DecodeFromBytesWith when the
// burst's color code differs from DecodeOptions.ColorCode.
var ErrColorCodeMismatch = errors.New("color code mismatch")

// DefaultSyncThreshold is the largest Hamming distance, in bits, at which a
// received 48-bit SYNC field still matches a SYNC pattern. The patterns are
// at least 10 bits apart, so a field is within the threshold of one pattern
// at most (ETSI TS 102 361-1 Table 9.2).
const DefaultSyncThreshold = 4

// DecodeOptions are receive-side settings for DecodeFromBytesWith. The zero
// value classifies the burst by its SYNC field, as the package doc
// describes.
type DecodeOptions struct {
	// SyncThreshold is the largest Hamming distance at which the SYNC field
	// matches a SYNC pattern. 0 selects DefaultSyncThreshold; a negative
	// value accepts exact matches only.
	SyncThreshold int
	// ColorCode, when set, is the expected color code. DecodeFromBytesWith
	// returns ErrColorCodeMismatch when the slot type or EMB of the burst
	// carries another one. A voice burst A carries no color code and is not
	// checked.
	ColorCode *enums.ColorCode
	// Kind is the burst kind, when the caller knows it from outside the
	// burst, such as from TDMA timing or a network header. A known kind
	// overrides the classification by the SYNC field. KindUnknown, the
	// zero value, classifies by the SYNC field.
	Kind Kind
	// VoiceBurst is the superframe position of a KindVoice burst.
	// VoiceBurstA takes the centre as a voice SYNC pattern and VoiceBurstB to
	// VoiceBurstF take it as EMB and embedded data. VoiceBurstUnknown takes
	// it as a voice SYNC pattern within the SYNC threshold and as EMB
	// otherwise.
	VoiceBurst enums.VoiceBurstType
	// DataType, when set, is the data type of a KindData burst. It replaces
	// the data type of the decoded slot type and selects the payload decode.
	// When nil, the slot type decides.
	DataType *enums.DataType
}

var (
	allSyncPatterns = []enums.SyncPattern{
		enums.SyncBSSourcedVoice, enums.SyncBSSourcedData, enums.SyncMSSourcedVoice, enums.SyncMSSourcedData,
		enums.SyncMSSourcedRC, enums.SyncTDMA1Voice, enums.SyncTDMA1Data, enums.SyncTDMA2Voice, enums.SyncTDMA2Data,
		enums.SyncReserved,
	}
	voiceSyncPatterns = []enums.SyncPattern{enums.SyncBSSourcedVoice, enums.SyncMSSourcedVoice, enums.SyncTDMA1Voice, enums.SyncTDMA2Voice}
	dataSyncPatterns  = []enums.SyncPattern{enums.SyncBSSourcedData, enums.SyncMSSourcedData, enums.SyncTDMA1Data, enums.SyncTDMA2Data}
)

func (b *Burst) syncField() uint64 {
	var v uint64
	for _, x := range b.bitData[syncStart:syncEnd] {
		v = v<<1 | uint64(x&1)
	}
	return v
}

func (b *Burst) nearestSync(candidates []enums.SyncPattern) (enums.SyncPattern, int) {
	field := b.syncField()
	best, bestDistance := candidates[0], 49
	for _, p := range candidates {
		if d := bits.OnesCount64(field ^ uint64(p)); d < bestDistance {
			best, bestDistance = p, d
		}
	}
	return best, bestDistance
}

func (b *Burst) checkColorCode(want *enums.ColorCode) error {
	if want == nil {
		return nil
	}
	var got enums.ColorCode
	switch {
	case b.Kind == KindData:
		got = b.SlotType.ColorCode
	case b.HasEmbeddedSignalling():
		got = b.EmbeddedSignalling.ColorCode
	default:
		return nil
	}
	if got != *want {
		return fmt.Errorf("%w: burst has %d, want %d", ErrColorCodeMismatch, got, *want)
	}
	return nil
}
