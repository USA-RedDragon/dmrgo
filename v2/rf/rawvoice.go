package rf

import (
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
)

const codec2FID = 0xC2

func rawVoiceLC(lc []byte) (raw, ok bool) {
	switch enums.FLCO(lc[0] & 0x3F) {
	case enums.FLCOGroupVoiceChannelUser, enums.FLCOUnitToUnitVoiceChannelUser:
		return lc[1] == codec2FID, true
	default:
		return false, false
	}
}

func noteCallLC(raw *bool, b *layer2.RawBurst, check func([12]byte) bool) {
	st := b.SlotType()
	if st.FEC.Uncorrectable {
		return
	}
	switch st.DataType {
	case enums.DataTypeVoiceLCHeader:
		p, _ := b.BPTCPayload()
		if check != nil && !check(p) {
			return
		}
		if r, ok := rawVoiceLC(p[:]); ok {
			*raw = r
		}
	case enums.DataTypeTerminatorWithLC:
		*raw = false
	default:
	}
}

func keepVoiceSlots(fixed, data *layer2.RawBurst) {
	for i := range fixed {
		m := voiceSlotMask(i)
		fixed[i] = fixed[i]&^m | data[i]&m
	}
}

func voiceSlotMask(i int) byte {
	const half = layer2.VoiceSlotsPerBurst * 72 / 2
	switch {
	case i < half/8:
		return 0xFF
	case i == half/8:
		return 0xF0
	case i == (layer2.BurstBits-half)/8:
		return 0x0F
	case i > (layer2.BurstBits-half)/8:
		return 0xFF
	default:
		return 0
	}
}
