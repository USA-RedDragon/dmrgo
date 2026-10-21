//go:build tinygo

package allocs_test

import (
	"math/rand"
	"runtime"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/bs"
	"github.com/USA-RedDragon/dmrgo/v2/csbk"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/internal/bits"
	"github.com/USA-RedDragon/dmrgo/v2/internal/dvsisim"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/framer"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/packet"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/layer3"
	"github.com/USA-RedDragon/dmrgo/v2/internal/digest"
	"github.com/USA-RedDragon/dmrgo/v2/transport/dmrd"
	"github.com/USA-RedDragon/dmrgo/v2/transport/dmrd/auth"
	"github.com/USA-RedDragon/dmrgo/v2/transport/hytera"
	"github.com/USA-RedDragon/dmrgo/v2/transport/ipsc"
	"github.com/USA-RedDragon/dmrgo/v2/transport/mmdvm"
	ipscauth "github.com/USA-RedDragon/dmrgo/v2/transport/ipsc/auth"
	"github.com/USA-RedDragon/dmrgo/v2/vocoder"
	"github.com/USA-RedDragon/dmrgo/v2/vocoder/dvsi"
	"github.com/USA-RedDragon/dmrgo/v2/voice"
)

const tinyRuns = 20

type tinyOp struct {
	name string
	run  func()
}

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
		for j := range 49 {
			bits.SetBit(frames[i][:], j, byte(r.Intn(2)))
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
	slot, _ := vocoder.EncodeAMBE72(frames[0])
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
		{"vocoder.EncodeAMBE72", func() { slot, err = vocoder.EncodeAMBE72(frame) }},
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

	vtx := tinyOK(voice.NewTransmitter(&voice.TransmitterConfig{ColorCode: 1, Silence: vocoder.AMBESilenceSlot(), Queue: 3 * 20}))
	outs := make([]voice.Output, 0, voice.MaxOutputs)
	gps := pdu.FLCGPSInfo{Latitude: 41.75}
	callOpts := &voice.CallOptions{TalkerAlias: "KI5VMF JACOB", TalkerAliasFormat: layer3.TalkerAliasISOEightBit, GPS: &gps}

	rep := tinyOK(bs.New(&bs.Config{ColorCode: 1, CACH: true}))
	bsOut := make([]bs.Output, 0, bs.MaxOutputs)
	csbk := tinyDataBurst(t, r, enums.DataTypeCSBK)

	b := packet.NewBuilder(127)
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
		{"voice.Transmitter", func() {
			if err = vtx.Start(now, voice.Call{Group: true, Target: 91, Source: 3191868}, callOpts); err != nil {
				return
			}
			for range 2 * layer2.VoiceBurstsPerSuperframe {
				f := tinyFrames(r)
				_ = vtx.Frames(f[:]...)
				now += 60 * time.Millisecond
				outs = vtx.Tick(now, outs[:0])
			}
			vtx.Stop()
			for {
				at, ok := vtx.Deadline()
				if !ok {
					break
				}
				now = max(now, at)
				outs = vtx.Tick(now, outs[:0])
			}
		}},
		{"bs.Repeater", func() {
			rep.Reset()
			from := bs.Source{Peer: 1}
			bsOut, _ = rep.Inbound(now, &csbk, layer2.Hint{Slot: enums.Slot2}, from, bsOut[:0])
			for i := range bursts {
				bsOut, _ = rep.Inbound(now, &bursts[i], layer2.Hint{Slot: enums.Slot1}, from, bsOut[:0])
				bsOut = rep.Tick(now, bsOut[:0])
				now += bs.SlotTime
				bsOut = rep.Tick(now, bsOut[:0])
				now += bs.SlotTime
			}
			for range 200 {
				bsOut = rep.Tick(now, bsOut[:0])
				now += bs.SlotTime
			}
		}},
		{"layer2.VoiceTransmitter.Reset", func() { err = tx.Reset(opts) }},
		{"packet.Builder.Retry", func() { _, err = b.Retry(&p, &rd) }},
	}
}

func tinyCSBKOps(t *testing.T) []tinyOp {
	t.Helper()
	scratch := layer2.NewBurst()
	q := tinyOK(csbk.NewRequester(&csbk.RequesterConfig{ColorCode: 1, Source: layer2.SyncSourceMS, Scratch: scratch}))
	r := tinyOK(csbk.NewResponder(&csbk.ResponderConfig{Address: 2, ColorCode: 1, Source: layer2.SyncSourceMS, Scratch: scratch}))
	p := csbk.NewPreambles(&csbk.PreamblesConfig{Scratch: scratch})
	w := tinyOK(csbk.NewWakeup(&csbk.WakeupConfig{ColorCode: 1, Scratch: scratch}))
	out := make([]csbk.Output, 0, csbk.MaxOutputs)
	out2 := make([]csbk.Output, 0, csbk.MaxOutputs)
	opts := &csbk.RequestOptions{Preambles: 2}
	var now time.Duration
	var err error
	_ = err
	exchange := func() {
		for range 8 {
			at, ok := q.Deadline()
			if !ok {
				return
			}
			now = max(now, at)
			out = q.Tick(now, out[:0])
			for i := range out {
				if out[i].Kind != csbk.OutputBurst {
					continue
				}
				out2 = p.Burst(now, &out[i].Burst, layer2.Hint{}, out2[:0])
				out2 = r.Burst(now, &out[i].Burst, layer2.Hint{}, out2[:0])
				if at, ok := r.Deadline(); ok {
					out2 = r.Tick(at, out2[:0])
					for j := range out2 {
						out = q.Burst(at, &out2[j].Burst, layer2.Hint{}, out[:0])
					}
				}
			}
		}
	}
	return []tinyOp{
		{"csbk.Requester.RequestOACSU", func() {
			err = q.RequestOACSU(now, 2, 1, layer3.ServiceOptions{}, opts)
			exchange()
		}},
		{"csbk.Requester.RequestRadioCheck", func() {
			err = q.RequestRadioCheck(now, 2, 1, opts)
			exchange()
		}},
		{"csbk.Wakeup.Wake", func() {
			err = w.Wake(now, 3, 1)
			for range 4 {
				if at, ok := w.Deadline(); ok {
					now = max(now, at)
					out = w.Tick(now, out[:0])
				}
			}
		}},
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
		gen    bs.CACH
		slcDst pdu.ShortLC
		slcP   pdu.ShortLCPayloads
		err    error
	)
	_ = err
	return []tinyOp{
		{"layer2.DecodeCACHField", func() { cachs[0] = layer2.DecodeCACHField(fields[0]) }},
		{"layer2.EncodeShortLCToCACH", func() { fields, err = layer2.EncodeShortLCToCACH(&slc, tact) }},
		{"bs.CACH.Next", func() {
			for range 4 {
				fields[0], err = gen.Next(enums.Slot2, true, &slc)
			}
		}},
		{"layer2.ShortLCAssembler.AddInto", func() {
			for _, c := range cachs {
				_, err = asm.AddInto(&slcDst, &slcP, c)
			}
		}},
	}
}

func tinyFramerOps(t *testing.T, r *rand.Rand) []tinyOp {
	t.Helper()
	stream := make([]byte, 0, 8*33)
	for i := range 8 {
		raw := tinyDataBurst(t, r, tinyDataTypes[i%len(tinyDataTypes)])
		stream = append(stream, raw[:]...)
	}
	f := framer.New(framer.Config{Period: 264})
	dst := make([]framer.Burst, 0, 16)
	return []tinyOp{
		{"layer2/framer.Framer.AppendBursts", func() { dst = f.AppendBursts(dst[:0], stream, 8*len(stream)) }},
	}
}

func tinyPDUOps(t *testing.T, r *rand.Rand) []tinyOp {
	t.Helper()
	payload := func(dt enums.DataType) [12]byte {
		var p [12]byte
		r.Read(p[:])
		_ = pdu.Seal(&p, dt)
		return p
	}
	csbkBits := payload(enums.DataTypeCSBK)
	lcBits := payload(enums.DataTypeVoiceLCHeader)
	dhBits := payload(enums.DataTypeDataHeader)
	mbcBits := payload(enums.DataTypeMBCHeader)
	var slcBits [5]byte
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
		enc96  [12]byte
		stBits [3]byte
		eBits  [2]byte
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

func tinyDVSIOps(t *testing.T) []tinyOp {
	t.Helper()
	pairs := dvsisim.Pairs(2)
	var ops []tinyOp
	for _, opt := range []dvsisim.Options{{}, {Chunk: 3}, {Datagram: true}} {
		d := dvsisim.New(pairs, opt)
		c := tinyOK(dvsi.New(d, dvsi.Config{}))
		pcm := make([]int16, 0, dvsi.FrameSamples)
		_, _ = c.AppendDecode(pcm, pairs[0].Slot)
		_, _ = c.Encode(pairs[0].PCM[:])
		ops = append(ops, tinyOp{"dvsi.Codec " + strconv.Itoa(opt.Chunk), func() {
			for i := range pairs {
				pcm, _ = c.AppendDecode(pcm[:0], pairs[i].Slot)
				_, _ = c.Encode(pairs[i].PCM[:])
			}
		}})
	}
	pkt := make([]byte, 0, dvsi.MaxPacketLen)
	pcm := make([]int16, 0, dvsi.FrameSamples)
	ops = append(ops, tinyOp{"dvsi packets", func() {
		pkt, _ = dvsi.AppendSpeech(pkt[:0], 1, pairs[0].PCM[:], true)
		q, _, _ := dvsi.Parse(pkt, true)
		pcm, _, _ = q.AppendSamples(pcm[:0])
		pkt, _ = dvsi.AppendChannel(pkt[:0], 2, pairs[0].Slot[:], dvsi.FrameBits, false)
		q, _, _ = dvsi.Parse(pkt, false)
		pkt, _, _, _ = q.AppendChannelData(pkt[:0])
	}})
	return ops
}

func tinyBridgeRun(q, buf []byte) []byte {
	b, _ := dmrd.DecodeBridge(q)
	buf, _ = b.AppendBinary(buf[:0])
	c := dmrd.BridgeControl{Command: dmrd.CommandBCSQ, TalkGroup: 91, Stream: 7}
	buf, _ = c.AppendBinary(buf[:0])
	got, _ := dmrd.DecodeBridgeControl(buf)
	buf, _ = got.AppendBinary(buf[:0])
	return buf
}

func tinyDMRDOps(t *testing.T) []tinyOp {
	t.Helper()
	d := dmrd.Data{Seq: 1, Source: 3191868, Destination: 91, Repeater: 3149999, Slot: enums.Slot2, Frame: dmrd.FrameVoice, Value: 3, Stream: 7, Size: 55}
	pkt := tinyOK(d.AppendBinary(make([]byte, 0, 128)))
	login := []byte("RPTK00040178cbf0e29abbd11c6573825d36a664e3064441f91fc815ac5dc5ca570d4c4a5f85")
	opts := []byte("StartRef=4013;RelinkTime=15;UserLink=1;TS1_1=262;")
	buf := make([]byte, 0, 128)
	signer := auth.NewBridge([]byte("passw0rd"))
	bridge := append(make([]byte, 0, 73), pkt[:53]...)
	bridge = append(bridge, make([]byte, 20)...)
	dmre := tinyOK((&dmrd.Bridge{Version: 5, Data: d}).AppendBinary(make([]byte, 0, 89)))
	return []tinyOp{{"dmrd", func() {
		got, _ := dmrd.DecodeData(pkt)
		buf, _ = got.AppendBinary(buf[:0])
		_ = got.Hint()
		c, _ := dmrd.DecodeControl(login, dmrd.DialectHBRP)
		buf, _ = c.AppendBinary(buf[:0], dmrd.DialectHBRP)
		_ = dmrd.Classify(pkt, dmrd.DialectMMDVM)
		o, _ := dmrd.ParseDMRPlusOptions(opts)
		buf = o.AppendText(buf[:0])
	}}, {"dmrd bridge", func() {
		buf = tinyBridgeRun(dmre, buf)
		buf = tinyBridgeRun(bridge, buf)
	}}, {"dmrd/auth", func() {
		tok, _ := auth.Token(dmrd.DialectHBRP, 0x0A7ED498, []byte("DL5DI"))
		_ = auth.VerifyToken(dmrd.DialectHBRP, 0x0A7ED498, []byte("DL5DI"), tok[:])
		_ = signer.Sign(bridge)
		_ = signer.Verify(bridge)
		_ = signer.Sign(dmre)
		_ = signer.Verify(dmre)
	}}}
}

func tinyDigestRun(s *tinyDigestState) bool {
	s.s1.Reset()
	s.s1.Write(s.msg[:])
	a := s.s1.Sum()
	s.s2.Reset()
	s.s2.Write(s.msg[:])
	c := s.s2.Sum()
	s.mac.Init(s.key[:])
	s.mac.Write(s.msg[:])
	h := s.mac.Sum()
	s.b2.Init(16, s.key[:64])
	s.b2.Write(s.msg[:])
	s.b2.Sum(s.out[:])
	return digest.Equal(a[:], h[:]) || digest.Equal(c[:16], s.out[:16])
}

type tinyDigestState struct {
	s1  digest.SHA1
	s2  digest.SHA256
	mac digest.HMACSHA1
	b2  digest.BLAKE2b
	msg [200]byte
	key [80]byte
	out [64]byte
}

func tinyDigestOps() []tinyOp {
	s := &tinyDigestState{}
	return []tinyOp{{"digest", func() {
		_ = tinyDigestRun(s)
	}}}
}

func tinyMMDVMRun(stream, buf, out []byte) ([]byte, []byte) {
	for len(stream) > 0 {
		f, n, st := mmdvm.Next(stream)
		if st == mmdvm.StatusMore {
			break
		}
		stream = stream[n:]
		if st != mmdvm.StatusFrame {
			continue
		}
		if d, err := mmdvm.DecodeDMRData(f); err == nil {
			buf, _ = d.AppendBinary(buf[:0])
			out, _ = mmdvm.AppendFrame(out[:0], d.Command(), buf)
			_ = d.Hint()
		}
		if s, err := mmdvm.DecodeStatusV1(f.Payload); err == nil {
			buf, _ = s.AppendBinary(buf[:0])
		}
		if c, err := mmdvm.DecodeConfigV2(f.Payload); err == nil {
			buf, _ = c.AppendBinary(buf[:0])
		}
		if v, err := mmdvm.DecodeVersion(f.Payload); err == nil {
			buf, _ = v.AppendBinary(buf[:0])
		}
	}
	return buf, out
}

func tinyMMDVMOps(t *testing.T) []tinyOp {
	t.Helper()
	var p [12]byte
	raw := tinyOK(layer2.BuildDataBurst(p, enums.DataTypeVoiceLCHeader, 1, layer2.SyncSourceBS))
	d := mmdvm.DMRData{Slot: enums.Slot2, Control: mmdvm.ControlDataSync | 1, Burst: raw, RSSI: 300, HasRSSI: true}
	pl := tinyOK(d.AppendBinary(make([]byte, 0, 40)))
	stream := tinyOK(mmdvm.AppendFrame(make([]byte, 0, 128), d.Command(), pl))
	stream = append(stream, 0xE0, 0x0E, 0x01, 0x02, 0, 0, 0, 0x1C, 0x1C, 0, 0, 0, 0, 0)
	buf := make([]byte, 0, 64)
	out := make([]byte, 0, 64)
	return []tinyOp{{"mmdvm", func() {
		buf, out = tinyMMDVMRun(stream, buf, out)
	}}}
}

func tinyHyteraRun(frame, hb, cmd, buf []byte) []byte {
	f, _ := hytera.DecodeFrame(frame)
	buf, _ = f.AppendBinary(buf[:0])
	h := f.Hint()
	_ = f.SetKind(h.Kind, h.VoiceBurst, h.DataType)
	b, _ := hytera.DecodeHeartbeat(hb)
	buf, _ = b.AppendBinary(buf[:0])
	c, _ := hytera.DecodeP2P(cmd)
	buf, _ = c.AppendBinary(buf[:0])
	_, _ = c.RedirectPort()
	_, _ = c.Service()
	return buf
}

func tinyHyteraOps(t *testing.T) []tinyOp {
	t.Helper()
	var p [12]byte
	raw := tinyOK(layer2.BuildDataBurst(p, enums.DataTypeVoiceLCHeader, 1, layer2.SyncSourceBS))
	f := hytera.Frame{Port: 50002, Slot: enums.Slot1, SlotType: hytera.SlotTypeVoiceLCHeader, ColorCode: 1, Burst: raw, Destination: 262, Source: 2628347}
	frame := tinyOK(f.AppendBinary(make([]byte, 0, hytera.FrameLen)))
	hb := []byte("ZZZZ\x0a\x00\x00\x00\x14\x00\x00\x00\x5a\x5a\x59\x5a\x00\x00\x00\x00")
	cmd := []byte("P2PP\x0b\x00\x00\x00\x14\x00\x00\x00\x42\xff\x01\x00\x00\x00\x00\x00\x11\x00\x00\x00\xff\x01\x51\xc3")
	buf := make([]byte, 0, 128)
	return []tinyOp{{"hytera", func() {
		buf = tinyHyteraRun(frame, hb, cmd, buf)
	}}}
}

func tinyIPSCRun(q, buf []byte) []byte {
	got, _ := ipsc.DecodeUser(q)
	buf, _ = got.AppendBinary(buf[:0])
	h := got.Hint()
	if r, err := got.RawBurst(7, layer2.SyncSourceBS); err == nil {
		_ = got.SetRawBurst(r, h.Kind, h.VoiceBurst, h.DataType)
	}
	return buf
}

func tinyIPSCOps(t *testing.T) []tinyOp {
	t.Helper()
	var p [12]byte
	raw := tinyOK(layer2.BuildDataBurst(p, enums.DataTypeCSBK, 7, layer2.SyncSourceBS))
	u := ipsc.User{Opcode: ipsc.OpGroupData, Info: 0x20}
	if err := u.SetRawBurst(raw, layer2.KindData, 0, enums.DataTypeCSBK); err != nil {
		t.Fatal(err)
	}
	pkt := tinyOK(u.AppendBinary(make([]byte, 0, 80)))
	voice := ipsc.User{Opcode: ipsc.OpGroupVoice, Burst: ipsc.BurstVoice, Voice: ipsc.Voice{Layout: ipsc.VoiceLayoutE, Length: 0x22, Control: 0x16}}
	vpkt := tinyOK(voice.AppendBinary(make([]byte, 0, 80)))
	key := ipscauth.NewKey([20]byte{1})
	buf := make([]byte, 0, 128)
	return []tinyOp{{"ipsc", func() {
		buf = tinyIPSCRun(pkt, buf)
		buf = tinyIPSCRun(vpkt, buf)
		buf = key.Sign(buf)
		_, _ = key.Verify(buf)
	}}}
}

func TestTinyGoAllocs(t *testing.T) {
	t.Parallel()

	r := rand.New(rand.NewSource(1))
	ops := slices.Concat(tinyBurstOps(t, r), tinyRawBurstOps(t, r), tinyVoiceOps(t, r), tinyServiceOps(t, r), tinyCSBKOps(t), tinyCACHOps(t), tinyFramerOps(t, r), tinyPDUOps(t, r), tinyDVSIOps(t), tinyAppOps(t), tinyDMRDOps(t), tinyIPSCOps(t), tinyHyteraOps(t), tinyMMDVMOps(t), tinyDigestOps())

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
