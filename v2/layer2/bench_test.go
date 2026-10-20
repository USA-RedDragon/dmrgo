package layer2_test

import (
	"math/rand"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/internal/bits"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/vocoder"
)

func benchBursts(b *testing.B, dt enums.DataType) []layer2.RawBurst {
	b.Helper()
	r := rand.New(rand.NewSource(int64(dt) + 1))
	out := make([]layer2.RawBurst, 64)
	for i := range out {
		var err error
		switch dt {
		case enums.DataTypeRate34:
			var p [18]byte
			r.Read(p[:])
			out[i], err = layer2.BuildRate34DataBurst(p, 1, layer2.SyncSourceBS)
		case enums.DataTypeRate1:
			var p [24]byte
			r.Read(p[:])
			out[i], err = layer2.BuildRate1DataBurst(p, 1, layer2.SyncSourceBS)
		default:
			var p [12]byte
			r.Read(p[:])
			_ = pdu.Seal(&p, dt)
			out[i], err = layer2.BuildDataBurst(p, dt, 1, layer2.SyncSourceBS)
		}
		if err != nil {
			b.Fatal(err)
		}
	}
	return out
}

func benchVoiceBursts(b *testing.B) []layer2.RawBurst {
	b.Helper()
	r := rand.New(rand.NewSource(99))
	out := make([]layer2.RawBurst, 64)
	for i := range out {
		var frames [layer2.VoiceSlotsPerBurst]vocoder.AMBEFrame
		for j := range frames {
			for k := range 49 {
				bits.SetBit(frames[j][:], k, byte(r.Intn(2)))
			}
		}
		pos := enums.VoiceBurstType(int(enums.VoiceBurstA) + i%6)
		var err error
		out[i], err = layer2.BuildVoiceBurst(frames, pos, pdu.EmbeddedSignalling{ColorCode: 1}, [4]byte{byte(i), 2, 3, 4}, layer2.SyncSourceBS)
		if err != nil {
			b.Fatal(err)
		}
	}
	return out
}

var benchDataTypes = []enums.DataType{
	enums.DataTypePIHeader, enums.DataTypeVoiceLCHeader, enums.DataTypeTerminatorWithLC, enums.DataTypeCSBK,
	enums.DataTypeMBCHeader, enums.DataTypeMBCContinuation, enums.DataTypeDataHeader, enums.DataTypeRate12,
	enums.DataTypeRate34, enums.DataTypeIdle, enums.DataTypeRate1, enums.DataTypeUnifiedSingleBlock,
}

func benchRawBursts(b *testing.B, dt enums.DataType) []layer2.RawBurst {
	b.Helper()
	if dt == enums.DataTypeReserved {
		return benchVoiceBursts(b)
	}
	return benchBursts(b, dt)
}

func benchName(dt enums.DataType) string {
	if dt == enums.DataTypeReserved {
		return "Voice"
	}
	return dt.String()
}

func BenchmarkBurstDecode(b *testing.B) {
	for _, dt := range append([]enums.DataType{enums.DataTypeReserved}, benchDataTypes...) {
		bursts := benchRawBursts(b, dt)
		b.Run(benchName(dt), func(b *testing.B) {
			var burst layer2.Burst
			b.ReportAllocs()
			i := 0
			for b.Loop() {
				_ = burst.DecodeFromBytes(bursts[i%len(bursts)])
				i++
			}
		})
	}
}

func BenchmarkBurstEncode(b *testing.B) {
	for _, dt := range append([]enums.DataType{enums.DataTypeReserved}, benchDataTypes...) {
		raw := benchRawBursts(b, dt)
		bursts := make([]layer2.Burst, len(raw))
		for i := range raw {
			_ = bursts[i].DecodeFromBytes(raw[i])
		}
		b.Run(benchName(dt), func(b *testing.B) {
			b.ReportAllocs()
			i := 0
			for b.Loop() {
				_, _ = bursts[i%len(bursts)].Encode()
				i++
			}
		})
	}
}

func BenchmarkBuildBurst(b *testing.B) {
	for _, dt := range benchDataTypes {
		raw := benchBursts(b, dt)
		data := make([]layer2.Data, len(raw))
		for i := range raw {
			burst, _ := layer2.NewBurstFromBytes(raw[i])
			data[i] = burst.Data
		}
		b.Run(benchName(dt), func(b *testing.B) {
			b.ReportAllocs()
			i := 0
			for b.Loop() {
				_, _ = layer2.BuildBurst(data[i%len(data)], 1, layer2.SyncSourceBS)
				i++
			}
		})
	}
}

func BenchmarkRawBurst(b *testing.B) {
	csbk := benchBursts(b, enums.DataTypeCSBK)
	r34 := benchBursts(b, enums.DataTypeRate34)
	voice := benchVoiceBursts(b)
	b.Run("SlotType", func(b *testing.B) {
		i := 0
		for b.Loop() {
			_ = csbk[i%64].SlotType()
			i++
		}
	})
	b.Run("Sync", func(b *testing.B) {
		i := 0
		for b.Loop() {
			_, _ = csbk[i%64].Sync()
			i++
		}
	})
	b.Run("EMB", func(b *testing.B) {
		i := 0
		for b.Loop() {
			_ = voice[i%64].EMB()
			i++
		}
	})
	b.Run("BPTCPayload", func(b *testing.B) {
		i := 0
		for b.Loop() {
			_, _ = csbk[i%64].BPTCPayload()
			i++
		}
	})
	b.Run("SetBPTCPayload", func(b *testing.B) {
		var rb layer2.RawBurst
		p := [12]byte{1, 2, 3}
		for b.Loop() {
			p[0]++
			rb.SetBPTCPayload(p)
		}
	})
	b.Run("Rate34Payload", func(b *testing.B) {
		i := 0
		for b.Loop() {
			_, _ = r34[i%64].Rate34Payload()
			i++
		}
	})
	b.Run("SetRate34Payload", func(b *testing.B) {
		var rb layer2.RawBurst
		p := [18]byte{1, 2, 3}
		for b.Loop() {
			p[0]++
			rb.SetRate34Payload(p)
		}
	})
	b.Run("AMBEFrames", func(b *testing.B) {
		i := 0
		for b.Loop() {
			_, _ = voice[i%64].AMBEFrames()
			i++
		}
	})
	b.Run("SetAMBEFrames", func(b *testing.B) {
		var rb layer2.RawBurst
		var frames [layer2.VoiceSlotsPerBurst]vocoder.AMBEFrame
		for b.Loop() {
			frames[0][0] ^= 0x80
			noErr(rb.SetAMBEFrames(frames))
		}
	})
}

func BenchmarkEmbeddedLC(b *testing.B) {
	lc := [9]byte{0, 0, 0, 0, 0x26, 0x0f, 0x30, 0xb3, 0x3c}
	f := layer2.EncodeEmbeddedLC(lc)
	b.Run("Encode", func(b *testing.B) {
		for b.Loop() {
			lc[8]++
			_ = layer2.EncodeEmbeddedLC(lc)
		}
	})
	b.Run("Decode", func(b *testing.B) {
		for b.Loop() {
			_, _ = layer2.DecodeEmbeddedLC(f, false)
		}
	})
	b.Run("DecodeStrict", func(b *testing.B) {
		for b.Loop() {
			_, _ = layer2.DecodeEmbeddedLC(f, true)
		}
	})
}

func BenchmarkCACH(b *testing.B) {
	slc := pdu.ShortLC{}
	frags, err := layer2.EncodeShortLCToCACH(&slc, pdu.TACT{})
	if err != nil {
		b.Fatal(err)
	}
	b.Run("DecodeField", func(b *testing.B) {
		for b.Loop() {
			_ = layer2.DecodeCACHField(frags[0])
		}
	})
	b.Run("EncodeShortLC", func(b *testing.B) {
		for b.Loop() {
			_, _ = layer2.EncodeShortLCToCACH(&slc, pdu.TACT{})
		}
	})
}

func BenchmarkReverseChannel(b *testing.B) {
	rc := pdu.ReverseChannel{}
	bits, err := layer2.EncodeEmbeddedRC(&rc)
	if err != nil {
		b.Fatal(err)
	}
	b.Run("Decode", func(b *testing.B) {
		for b.Loop() {
			_ = layer2.DecodeEmbeddedRC(bits)
		}
	})
	b.Run("Encode", func(b *testing.B) {
		for b.Loop() {
			_, _ = layer2.EncodeEmbeddedRC(&rc)
		}
	})
}
