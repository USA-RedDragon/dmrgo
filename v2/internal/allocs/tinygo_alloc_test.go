//go:build tinygo

package allocs_test

import (
	"math/rand"
	"runtime"
	"slices"
	"testing"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/framer"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/packet"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/layer3"
	"github.com/USA-RedDragon/dmrgo/v2/vocoder"
	"github.com/USA-RedDragon/dmrgo/v2/voice"
)

// tinyRuns is the number of measured calls of each operation.
const tinyRuns = 20

// tinyOp is one operation that TestTinyGoAllocs measures.
type tinyOp struct {
	name string
	run  func()
}

// tinyOK returns v and panics on err, for setup calls that must succeed.
func tinyOK[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}

func tinyDataBurst(t *testing.T, r *rand.Rand, dt enums.DataType) layer2.RawBurst {
	t.Helper()
	switch dt {
	case enums.DataTypeRate34:
		var p [18]byte
		r.Read(p[:])
		return tinyOK(layer2.BuildRate34DataBurst(p, 1, layer2.SyncSourceBS))
	case enums.DataTypeRate1:
		var p [24]byte
		r.Read(p[:])
		return tinyOK(layer2.BuildRate1DataBurst(p, 1, layer2.SyncSourceBS))
	default:
		var p [12]byte
		r.Read(p[:])
		_ = pdu.Seal(&p, dt)
		return tinyOK(layer2.BuildDataBurst(p, dt, 1, layer2.SyncSourceBS))
	}
}

func tinyFrames(r *rand.Rand) [layer2.VoiceSlotsPerBurst]vocoder.AMBEFrame {
	var frames [layer2.VoiceSlotsPerBurst]vocoder.AMBEFrame
	for i := range frames {
		for j := range frames[i] {
			frames[i][j] = bit.Bit(r.Intn(2))
		}
	}
	return frames
}

var tinyDataTypes = []enums.DataType{
	enums.DataTypePIHeader, enums.DataTypeVoiceLCHeader, enums.DataTypeTerminatorWithLC, enums.DataTypeCSBK,
	enums.DataTypeMBCHeader, enums.DataTypeMBCContinuation, enums.DataTypeDataHeader, enums.DataTypeRate12,
	enums.DataTypeRate34, enums.DataTypeIdle, enums.DataTypeRate1, enums.DataTypeUnifiedSingleBlock,
}

func tinyBurstOps(t *testing.T, r *rand.Rand) []tinyOp {
	t.Helper()
	ops := make([]tinyOp, 0, 4+3*len(tinyDataTypes))
	var b layer2.Burst
	var err error
	voice := tinyOK(layer2.BuildVoiceBurst(tinyFrames(r), enums.VoiceBurstC, pdu.EmbeddedSignalling{ColorCode: 1}, [4]byte{1, 2, 3, 4}, layer2.SyncSourceBS))
	voiceA := tinyOK(layer2.BuildVoiceBurst(tinyFrames(r), enums.VoiceBurstA, pdu.EmbeddedSignalling{ColorCode: 1}, [4]byte{}, layer2.SyncSourceBS))
	for _, v := range []struct {
		name string
		raw  layer2.RawBurst
	}{{"voice A", voiceA}, {"voice C", voice}} {
		var dec layer2.Burst
		_ = tinyOK(struct{}{}, dec.DecodeFromBytes(v.raw))
		ops = append(ops,
			tinyOp{"layer2.Burst.DecodeFromBytes " + v.name, func() { err = b.DecodeFromBytes(v.raw) }},
			tinyOp{"layer2.Burst.Encode " + v.name, func() { _, err = dec.Encode() }},
		)
	}
	for _, dt := range tinyDataTypes {
		raw := tinyDataBurst(t, r, dt)
		dec := new(layer2.Burst)
		_ = tinyOK(struct{}{}, dec.DecodeFromBytes(raw))
		data := dec.Data
		ops = append(ops,
			tinyOp{"layer2.Burst.DecodeFromBytes " + dt.String(), func() { err = b.DecodeFromBytes(raw) }},
			tinyOp{"layer2.Burst.Encode " + dt.String(), func() { _, err = dec.Encode() }},
			tinyOp{"layer2.BuildBurst " + dt.String(), func() { _, err = layer2.BuildBurst(data, 1, layer2.SyncSourceBS) }},
		)
	}
	_ = err
	return ops
}

func tinyRawBurstOps(t *testing.T, r *rand.Rand) []tinyOp {
	t.Helper()
	csbk := tinyDataBurst(t, r, enums.DataTypeCSBK)
	r34 := tinyDataBurst(t, r, enums.DataTypeRate34)
	voice := tinyOK(layer2.BuildVoiceBurst(tinyFrames(r), enums.VoiceBurstB, pdu.EmbeddedSignalling{ColorCode: 1}, [4]byte{9, 8, 7, 6}, layer2.SyncSourceBS))
	frames := tinyFrames(r)
	var rb layer2.RawBurst
	p12 := [12]byte{1, 2, 3}
	p18 := [18]byte{4, 5, 6}
	st := csbk.SlotType()
	emb := voice.EMB()
	return []tinyOp{
		{"layer2.RawBurst.SlotType", func() { st = csbk.SlotType() }},
		{"layer2.RawBurst.SetSlotType", func() { rb.SetSlotType(st) }},
		{"layer2.RawBurst.Sync", func() { _, _ = csbk.Sync() }},
		{"layer2.RawBurst.EMB", func() { emb = voice.EMB() }},
		{"layer2.RawBurst.SetEMB", func() { rb.SetEMB(emb) }},
		{"layer2.RawBurst.BPTCPayload", func() { p12, _ = csbk.BPTCPayload() }},
		{"layer2.RawBurst.SetBPTCPayload", func() { rb.SetBPTCPayload(p12) }},
		{"layer2.RawBurst.Rate34Payload", func() { p18, _ = r34.Rate34Payload() }},
		{"layer2.RawBurst.SetRate34Payload", func() { rb.SetRate34Payload(p18) }},
		{"layer2.RawBurst.AMBEFrames", func() { frames, _ = voice.AMBEFrames() }},
		{"layer2.RawBurst.SetAMBEFrames", func() { rb.SetAMBEFrames(frames) }},
	}
}

func tinyVoiceOps(t *testing.T, r *rand.Rand) []tinyOp {
	t.Helper()
	call := tinyOK(pdu.NewVoiceLC(true, 91, 3191868, layer3.ServiceOptions{}))
	opts := &layer2.VoiceTransmitterOptions{LC: call, ColorCode: 1, Silence: vocoder.AMBESilenceSlot()}
	tx := tinyOK(layer2.NewVoiceTransmitter(opts))
	buf := make([]layer2.RawBurst, 0, layer2.VoiceBurstsPerSuperframe+1)
	var frames [layer2.VoiceSlotsPerBurst * layer2.VoiceBurstsPerSuperframe]vocoder.AMBEFrame
	for i := range layer2.VoiceBurstsPerSuperframe {
		f := tinyFrames(r)
		copy(frames[i*layer2.VoiceSlotsPerBurst:], f[:])
	}
	superframe := tinyOK(tx.AppendFrames(nil, frames[:]...))
	bursts := make([]layer2.Burst, len(superframe))
	for i := range superframe {
		_ = tinyOK(struct{}{}, bursts[i].DecodeFromBytes(superframe[i]))
	}
	lc9 := [9]byte{0, 0, 0, 0, 0x26, 0x0f, 0x30, 0xb3, 0x3c}
	frags := layer2.EncodeEmbeddedLC(lc9)
	lcss := [4]enums.LCSS{enums.LCSSFirstFragment, enums.LCSSContinuationFragment, enums.LCSSContinuationFragment, enums.LCSSLastFragment}
	var (
		asm    layer2.EmbeddedLCAssembler
		rx     layer2.VoiceReceiver
		flcDst pdu.FullLinkControl
		flcP   pdu.FullLinkControlPayloads
		err    error
	)
	_ = err
	slot := vocoder.EncodeAMBE72(frames[0])
	frame := frames[0]
	return []tinyOp{
		{"layer2.VoiceTransmitter.AppendFrames", func() { _, err = tx.AppendFrames(buf[:0], frames[:layer2.VoiceSlotsPerBurst]...) }},
		{"layer2.VoiceReceiver.ReceiveInto", func() {
			for i := range bursts {
				_, err = rx.ReceiveInto(&flcDst, &flcP, &bursts[i])
			}
		}},
		{"layer2.EncodeEmbeddedLC", func() { frags = layer2.EncodeEmbeddedLC(lc9) }},
		{"layer2.DecodeEmbeddedLC", func() { _, _ = layer2.DecodeEmbeddedLC(frags, false) }},
		{"layer2.DecodeEmbeddedLC strict", func() { _, _ = layer2.DecodeEmbeddedLC(frags, true) }},
		{"layer2.EmbeddedLCAssembler.Add", func() {
			for i := range frags {
				_, _, _, err = asm.Add(frags[i], lcss[i])
			}
		}},
		{"vocoder.DecodeAMBE72", func() { frame, _ = vocoder.DecodeAMBE72(slot) }},
		{"vocoder.EncodeAMBE72", func() { slot = vocoder.EncodeAMBE72(frame) }},
	}
}

func tinyServiceOps(t *testing.T, r *rand.Rand) []tinyOp {
	t.Helper()
	call := tinyOK(pdu.NewVoiceLC(true, 91, 3191868, layer3.ServiceOptions{}))
	opts := &layer2.VoiceTransmitterOptions{LC: call, ColorCode: 1, Silence: vocoder.AMBESilenceSlot()}
	tx := tinyOK(layer2.NewVoiceTransmitter(opts))
	bursts := tx.AppendHeaders(nil)
	for range 2 * layer2.VoiceBurstsPerSuperframe {
		f := tinyFrames(r)
		bursts = tinyOK(tx.AppendFrames(bursts, f[:]...))
	}
	bursts = tinyOK(tx.AppendTerminate(bursts))
	rx := tinyOK(voice.NewReceiver(nil))
	events := make([]voice.Event, 0, voice.MaxEvents)
	var now time.Duration

	var b packet.Builder
	h := pdu.DataHeader{Format: pdu.FormatConfirmed, ConfirmedDataHeader: &pdu.ConfirmedDataHeader{SAP: 4, LLIDDestination: 2, LLIDSource: 1, FullMessageFlag: true}}
	p := tinyOK(b.Segment(h, make([]byte, 100), enums.DataTypeRate12))
	var rd packet.ResponseData
	rd.Retry[2], rd.Retry[5] = true, true
	var err error
	_ = err
	return []tinyOp{
		{"voice.Receiver.Burst", func() {
			for i := range bursts {
				now += 60 * time.Millisecond
				events = rx.Burst(now, &bursts[i], layer2.Hint{}, events[:0])
			}
			now += 10 * time.Second
			events = rx.Tick(now, events[:0])
		}},
		{"layer2.VoiceTransmitter.Reset", func() { err = tx.Reset(opts) }},
		{"packet.Builder.Retry", func() { _, err = b.Retry(&p, &rd) }},
	}
}

func tinyCACHOps(t *testing.T) []tinyOp {
	t.Helper()
	slc := pdu.ShortLC{SLCO: enums.SLCOActivityUpdate, ActivityUpdate: &pdu.ShortLCActivityUpdate{}}
	tact := pdu.TACT{}
	fields := tinyOK(layer2.EncodeShortLCToCACH(&slc, tact))
	var cachs [4]layer2.CACH
	for i := range fields {
		cachs[i] = layer2.DecodeCACHField(fields[i])
	}
	var (
		asm    layer2.ShortLCAssembler
		slcDst pdu.ShortLC
		slcP   pdu.ShortLCPayloads
		err    error
	)
	_ = err
	return []tinyOp{
		{"layer2.DecodeCACHField", func() { cachs[0] = layer2.DecodeCACHField(fields[0]) }},
		{"layer2.EncodeShortLCToCACH", func() { fields, err = layer2.EncodeShortLCToCACH(&slc, tact) }},
		{"layer2.ShortLCAssembler.AddInto", func() {
			for _, c := range cachs {
				_, err = asm.AddInto(&slcDst, &slcP, c)
			}
		}},
	}
}

func tinyFramerOps(t *testing.T, r *rand.Rand) []tinyOp {
	t.Helper()
	stream := make([]bit.Bit, 0, 8*264)
	for i := range 8 {
		raw := tinyDataBurst(t, r, tinyDataTypes[i%len(tinyDataTypes)])
		stream = bit.AppendUnpack(stream, raw[:])
	}
	f := framer.New(framer.Config{Period: 264})
	dst := make([]framer.Burst, 0, 16)
	return []tinyOp{
		{"layer2/framer.Framer.AppendBursts", func() { dst = f.AppendBursts(dst[:0], stream...) }},
	}
}

func tinyPDUOps(t *testing.T, r *rand.Rand) []tinyOp {
	t.Helper()
	payload := func(dt enums.DataType) [96]bit.Bit {
		var p [12]byte
		r.Read(p[:])
		_ = pdu.Seal(&p, dt)
		return bit.Unpack96(p)
	}
	csbkBits := payload(enums.DataTypeCSBK)
	lcBits := payload(enums.DataTypeVoiceLCHeader)
	dhBits := payload(enums.DataTypeDataHeader)
	mbcBits := payload(enums.DataTypeMBCHeader)
	var slcBits [36]bit.Bit
	var (
		csbk   pdu.CSBK
		csbkP  pdu.CSBKPayloads
		lc     pdu.FullLinkControl
		lcP    pdu.FullLinkControlPayloads
		dh     pdu.DataHeader
		dhP    pdu.DataHeaderPayloads
		mbc    pdu.MBCHeader
		mbcP   pdu.MBCHeaderPayloads
		slc    pdu.ShortLC
		slcP   pdu.ShortLCPayloads
		err    error
		pi     = pdu.DecodePIHeader(payload(enums.DataTypePIHeader))
		usbd   = pdu.DecodeUnifiedSingleBlockData(payload(enums.DataTypeUnifiedSingleBlock))
		st     = pdu.SlotType{ColorCode: 1, DataType: enums.DataTypeCSBK}
		emb    = pdu.EmbeddedSignalling{ColorCode: 1}
		enc96  [96]bit.Bit
		stBits [20]bit.Bit
		eBits  [16]bit.Bit
	)
	_ = err
	pdu.DecodeCSBKInto(&csbk, &csbkP, csbkBits)
	pdu.DecodeFullLinkControlInto(&lc, &lcP, lcBits, enums.DataTypeVoiceLCHeader)
	pdu.DecodeDataHeaderInto(&dh, &dhP, dhBits)
	pdu.DecodeMBCHeaderInto(&mbc, &mbcP, mbcBits)
	pdu.DecodeShortLCInto(&slc, &slcP, slcBits)
	return []tinyOp{
		{"layer2/pdu.DecodeCSBKInto", func() { pdu.DecodeCSBKInto(&csbk, &csbkP, csbkBits) }},
		{"layer2/pdu.CSBK.Encode", func() { enc96, err = csbk.Encode() }},
		{"layer2/pdu.DecodeFullLinkControlInto", func() { pdu.DecodeFullLinkControlInto(&lc, &lcP, lcBits, enums.DataTypeVoiceLCHeader) }},
		{"layer2/pdu.FullLinkControl.Encode", func() { enc96, err = lc.Encode() }},
		{"layer2/pdu.DecodeDataHeaderInto", func() { pdu.DecodeDataHeaderInto(&dh, &dhP, dhBits) }},
		{"layer2/pdu.DataHeader.Encode", func() { enc96, err = dh.Encode() }},
		{"layer2/pdu.DecodeMBCHeaderInto", func() { pdu.DecodeMBCHeaderInto(&mbc, &mbcP, mbcBits) }},
		{"layer2/pdu.MBCHeader.Encode", func() { enc96, err = mbc.Encode() }},
		{"layer2/pdu.DecodeShortLCInto", func() { pdu.DecodeShortLCInto(&slc, &slcP, slcBits) }},
		{"layer2/pdu.ShortLC.Encode", func() { _, err = slc.Encode() }},
		{"layer2/pdu.PIHeader.Encode", func() { enc96, err = pi.Encode() }},
		{"layer2/pdu.UnifiedSingleBlockData.Encode", func() { enc96, err = usbd.Encode() }},
		{"layer2/pdu.SlotType.Encode", func() { stBits, err = st.Encode() }},
		{"layer2/pdu.DecodeSlotType", func() { st = pdu.DecodeSlotType(stBits) }},
		{"layer2/pdu.EmbeddedSignalling.Encode", func() { eBits, err = emb.Encode() }},
		{"layer2/pdu.DecodeEmbeddedSignalling", func() { emb = pdu.DecodeEmbeddedSignalling(eBits) }},
		{"layer2/pdu.DecodePIHeader", func() { pi = pdu.DecodePIHeader(enc96) }},
	}
}

// tinyAlloc returns the heap bytes and allocations per call of run, after
// two warm-up calls. The host target of TinyGo uses the Boehm collector,
// which leaves MemStats.Mallocs at 0, so only TotalAlloc decides; run with
// -gc=conservative, the collector of the microcontroller targets, to count
// allocations too.
func tinyAlloc(run func()) (bytes, mallocs float64) {
	run()
	run()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	for range tinyRuns {
		run()
	}
	runtime.ReadMemStats(&after)
	return float64(after.TotalAlloc-before.TotalAlloc) / tinyRuns, float64(after.Mallocs-before.Mallocs) / tinyRuns
}

// TestTinyGoAllocs checks that the decode and encode paths make no heap
// garbage under TinyGo, whose testing.AllocsPerRun always reports 0. It
// reads runtime.MemStats around the calls instead.
func TestTinyGoAllocs(t *testing.T) {
	t.Parallel()

	r := rand.New(rand.NewSource(1))
	ops := slices.Concat(tinyBurstOps(t, r), tinyRawBurstOps(t, r), tinyVoiceOps(t, r), tinyServiceOps(t, r), tinyCACHOps(t), tinyFramerOps(t, r), tinyPDUOps(t, r))

	probe := make([]byte, 0, 1)
	if b, _ := tinyAlloc(func() { probe = make([]byte, 64) }); b < 64 {
		t.Errorf("runtime.MemStats.TotalAlloc does not count a 64-byte allocation: %.0f bytes", b)
		return
	}
	_ = probe

	for _, op := range ops {
		b, n := tinyAlloc(op.run)
		if b != 0 {
			t.Errorf("%s: %.0f bytes, %.1f allocs per call, want 0", op.name, b, n)
		}
	}
}
