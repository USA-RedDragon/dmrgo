package rf

import (
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/internal/c2mode"
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

const (
	c2Other uint8 = iota
	c2Voice
	c2Reset
)

func c2MarkOf(b *Burst, raw bool) uint8 {
	_, kind, known := layer2.SyncSourceOf(b.Sync)
	switch {
	case b.Voice != enums.VoiceBurstUnknown || known && kind == layer2.KindVoice:
		if raw {
			return c2Voice
		}
		return c2Reset
	case known && kind == layer2.KindData:
		return c2Reset
	default:
		return c2Other
	}
}

func c2Note(d *c2mode.Detector, b *Burst, mark uint8) {
	b.Codec2 = 0
	switch mark {
	case c2Voice:
		slots := b.Data.VoiceSlots()
		for i := range slots {
			d.Add(&slots[i])
		}
		b.Codec2 = d.Mode()
	case c2Reset:
		d.Reset()
	}
}
