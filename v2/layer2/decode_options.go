package layer2

import (
	"errors"
	"fmt"
	"math/bits"

	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
)

// ErrColorCodeMismatch is returned, wrapped, by DecodeFromBytesWith when the
// burst's color code differs from DecodeOptions.ColorCode.
var ErrColorCodeMismatch = errors.New("color code mismatch")

// FrameKind is the kind of a burst as the caller knows it from outside the
// burst, for example from TDMA timing or a network header.
type FrameKind int

const (
	// FrameKindUnknown classifies the burst by its SYNC field.
	FrameKindUnknown FrameKind = iota
	// FrameKindVoice is a voice burst.
	FrameKindVoice
	// FrameKindData is a data or control burst.
	FrameKindData
	// FrameKindReverseChannel is a standalone inbound RC burst (§6.4.1).
	FrameKindReverseChannel
)

// FrameType is the type of a burst as the caller knows it. The zero value
// is an unknown type.
type FrameType struct {
	Kind FrameKind
	// VoiceBurst is the superframe position of a FrameKindVoice burst.
	// VoiceBurstA takes the SYNC field as the nearest voice SYNC pattern and
	// VoiceBurstB to VoiceBurstF take it as EMB and embedded data, whatever
	// the field holds. VoiceBurstUnknown takes it as a voice SYNC pattern
	// within the SYNC threshold and as EMB otherwise, so the burst is never
	// taken as data or RC.
	VoiceBurst enums.VoiceBurstType
	// DataType, when set, is the data type of a FrameKindData burst. It
	// replaces the data type of the decoded slot type in Burst.SlotType
	// and selects the payload decode; SlotType.ColorCode and FEC.SlotType
	// still come from the slot type bits. When nil, the slot type decides.
	DataType *enums.DataType
}

// DecodeOptions are receive-side settings for DecodeFromBytesWith.
type DecodeOptions struct {
	// SyncThreshold is the largest Hamming distance at which the SYNC field
	// of a burst of unknown type matches a Table 9.2 pattern. 0 selects
	// enums.SyncMatchThreshold; a negative value accepts exact matches only.
	SyncThreshold int
	// ColorCode, when set, is the expected color code. DecodeFromBytesWith
	// returns ErrColorCodeMismatch when the slot type or EMB of the burst
	// carries another one. A voice burst A carries no color code and is not
	// checked.
	ColorCode *uint8
	// Frame is the burst type, when the caller knows it. A known type
	// overrides the classification by the SYNC field.
	Frame FrameType
}

//nolint:gochecknoglobals
var (
	allSyncPatterns = []enums.SyncPattern{
		enums.SyncBSSourcedVoice, enums.SyncBSSourcedData, enums.SyncMSSourcedVoice, enums.SyncMSSourcedData,
		enums.SyncMSSourcedRC, enums.SyncTDMA1Voice, enums.SyncTDMA1Data, enums.SyncTDMA2Voice, enums.SyncTDMA2Data,
		enums.SyncReserved,
	}
	voiceSyncPatterns = []enums.SyncPattern{enums.SyncBSSourcedVoice, enums.SyncMSSourcedVoice, enums.SyncTDMA1Voice, enums.SyncTDMA2Voice}
	dataSyncPatterns  = []enums.SyncPattern{enums.SyncBSSourcedData, enums.SyncMSSourcedData, enums.SyncTDMA1Data, enums.SyncTDMA2Data}
)

// DecodeFromBytesWith is DecodeFromBytes with receive-side options. With the
// zero DecodeOptions it decodes exactly as DecodeFromBytes. SyncPattern is
// the pattern the burst is taken to carry and SyncDistance its distance
// from the SYNC field; for a voice burst B to F, SyncPattern is
// EmbeddedSignallingPattern and SyncDistance is the distance to the nearest
// Table 9.2 pattern. The burst is populated even when an error is
// returned; a color code mismatch is joined with any decode error.
func (b *Burst) DecodeFromBytesWith(data [33]byte, opts DecodeOptions) error {
	var err error
	if opts.Frame.Kind == FrameKindUnknown && opts.SyncThreshold == 0 {
		err = b.DecodeFromBytes(data)
	} else {
		err = b.decodeWith(data, opts)
	}
	return errors.Join(err, b.checkColorCode(opts.ColorCode))
}

func (b *Burst) decodeWith(data [33]byte, opts DecodeOptions) error {
	trunkingMode := b.TrunkingMode
	*b = Burst{TrunkingMode: trunkingMode}
	b.bitData = bit.UnpackBytesToBits264(data)

	threshold := opts.SyncThreshold
	if threshold == 0 {
		threshold = enums.SyncMatchThreshold
	} else if threshold < 0 {
		threshold = 0
	}
	nearest, distance := b.nearestSync(allSyncPatterns)

	switch opts.Frame.Kind {
	case FrameKindVoice:
		return b.decodeVoiceFrame(opts.Frame.VoiceBurst, nearest, distance, threshold)
	case FrameKindData:
		return b.decodeDataFrame(opts.Frame.DataType)
	case FrameKindReverseChannel:
		b.SyncPattern = enums.SyncMSSourcedRC
		b.SyncDistance = b.syncDistance(enums.SyncMSSourcedRC)
		b.HasEmbeddedSignalling = true
		b.decodeStandaloneRC()
		return nil
	case FrameKindUnknown:
	default:
		return fmt.Errorf("unknown frame kind %d", opts.Frame.Kind)
	}

	if distance > threshold {
		b.decodeVoiceAt(enums.VoiceBurstUnknown, enums.SyncEmbeddedSignalling, distance)
		return nil
	}
	switch {
	case isDataSync(nearest):
		return b.decodeDataFrame(nil)
	case isVoiceSync(nearest):
		b.decodeVoiceAt(enums.VoiceBurstA, nearest, distance)
	case nearest == enums.SyncMSSourcedRC:
		b.SyncPattern, b.SyncDistance = nearest, distance
		b.HasEmbeddedSignalling = true
		b.decodeStandaloneRC()
	default:
		b.decodeVoiceAt(enums.VoiceBurstUnknown, nearest, distance)
	}
	return nil
}

func (b *Burst) decodeVoiceFrame(pos enums.VoiceBurstType, nearest enums.SyncPattern, distance, threshold int) error {
	switch pos {
	case enums.VoiceBurstA:
		p, d := b.nearestSync(voiceSyncPatterns)
		b.decodeVoiceAt(pos, p, d)
	case enums.VoiceBurstB, enums.VoiceBurstC, enums.VoiceBurstD, enums.VoiceBurstE, enums.VoiceBurstF:
		b.decodeVoiceAt(pos, enums.SyncEmbeddedSignalling, distance)
	case enums.VoiceBurstUnknown:
		if distance <= threshold && isVoiceSync(nearest) {
			b.decodeVoiceAt(enums.VoiceBurstA, nearest, distance)
		} else {
			b.decodeVoiceAt(pos, enums.SyncEmbeddedSignalling, distance)
		}
	default:
		return fmt.Errorf("voice burst position %d out of range", pos)
	}
	return nil
}

func (b *Burst) decodeVoiceAt(pos enums.VoiceBurstType, sync enums.SyncPattern, distance int) {
	b.SyncPattern, b.SyncDistance, b.VoiceBurst = sync, distance, pos
	if sync == enums.SyncEmbeddedSignalling {
		b.HasEmbeddedSignalling = true
		b.EmbeddedSignalling, b.EmbeddedSignallingData = parseEmbedded(b.bitData)
		b.FEC.EMB = b.EmbeddedSignalling.FEC
		if b.EmbeddedSignalling.PreemptionAndPowerControlIndicator &&
			b.EmbeddedSignalling.LCSS == enums.LCSSSingleFragment {
			rc, rcFEC := DecodeEmbeddedRC(b.EmbeddedSignallingData)
			b.HasReverseChannel = true
			b.ReverseChannel = &rc
			b.FEC.RC = rcFEC
		}
	}
	b.VoiceSlots = parseVoiceSlots(b.bitData)
}

func (b *Burst) decodeDataFrame(dataType *enums.DataType) error {
	b.SyncPattern, b.SyncDistance = b.nearestSync(dataSyncPatterns)
	b.IsData = true
	b.HasSlotType = true
	b.SlotType = parseSlotType(b.bitData)
	b.FEC.SlotType = b.SlotType.FEC
	if dataType != nil {
		b.SlotType.DataType = *dataType
	}
	b.deinterleavedInfoLen, b.FEC.Payload = b.deinterleave(extractDataBits(b.bitData), b.SlotType.DataType)
	var err error
	b.Data, err = b.extractData()
	b.decodedData = b.Data
	b.decodedDataType = b.SlotType.DataType
	return err
}

func (b *Burst) syncField() uint64 {
	var v uint64
	for _, x := range b.bitData[SyncStart:SyncEnd] {
		v = v<<1 | uint64(x&1)
	}
	return v
}

func (b *Burst) syncDistance(p enums.SyncPattern) int {
	return bits.OnesCount64(b.syncField() ^ uint64(p)) //nolint:gosec // Table 9.2 patterns are positive 48-bit constants
}

func (b *Burst) nearestSync(candidates []enums.SyncPattern) (enums.SyncPattern, int) {
	best, bestDistance := candidates[0], 49
	for _, p := range candidates {
		if d := b.syncDistance(p); d < bestDistance {
			best, bestDistance = p, d
		}
	}
	return best, bestDistance
}

func (b *Burst) checkColorCode(want *uint8) error {
	if want == nil {
		return nil
	}
	var got int
	switch {
	case b.HasSlotType:
		got = b.SlotType.ColorCode
	case b.HasEmbeddedSignalling:
		got = b.EmbeddedSignalling.ColorCode
	default:
		return nil
	}
	if got != int(*want) {
		return fmt.Errorf("%w: burst has %d, want %d", ErrColorCodeMismatch, got, *want)
	}
	return nil
}
