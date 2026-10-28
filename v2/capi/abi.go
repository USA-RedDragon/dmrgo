package main

/*
#include <stdlib.h>
#define DMRGO_USER_T uintptr_t
#include "dmrgo.h"
*/
import "C"

import (
	"encoding/json"
	"errors"
	"fmt"
	"math/rand/v2"
	"time"
	"unsafe"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/framer"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/layer3"
	"github.com/USA-RedDragon/dmrgo/v2/vocoder"
	"github.com/USA-RedDragon/dmrgo/v2/vocoder/codec2"
	"github.com/USA-RedDragon/dmrgo/v2/voice"
)

const harnessArena = 64 << 20

type harness struct {
	rng    *rand.Rand
	arena  unsafe.Pointer
	afill  C.size_t
	in     *outc
	inMem  unsafe.Pointer
	keep   []func() error
	noFill bool
	calls  int
	shorts int
}

func newHarness(seed uint64) *harness {
	h := &harness{rng: rand.New(rand.NewPCG(seed, 99)), arena: C.malloc(harnessArena), inMem: C.malloc(1 << 20), in: &outc{}}
	h.afill = C.size_t(h.rng.IntN(7))
	h.in.setArena(h.inMem, 1<<20)
	return h
}

func (h *harness) free() {
	C.free(h.arena)
	C.free(h.inMem)
}

func sized[T any](p *T) *T {
	*(*C.size_t)(unsafe.Pointer(p)) = C.size_t(unsafe.Sizeof(*p))
	return p
}

func jsonOf(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "error: " + err.Error()
	}
	return string(b)
}

func goCode(err error) C.int {
	if err == nil {
		return C.DMRGO_OK
	}
	return errCode(err)
}

func ptr[T any](s []T) *T {
	if len(s) == 0 {
		return nil
	}
	return &s[0]
}

type appendCall[CT any] func(dst *CT, capx C.size_t, n, nn *C.size_t, ar unsafe.Pointer, acap C.size_t, an, ann *C.size_t) C.int

type needCall func(items, abytes *C.size_t) C.int

func runAppend[CT, GT any](h *harness, name string, want []GT, wantErr error, conv func(*CT) GT, need needCall, call appendCall[CT]) error {
	h.calls++
	var items, ab C.size_t
	if rc := need(&items, &ab); rc != C.DMRGO_OK {
		return fmt.Errorf("%s: need returned %d", name, rc)
	}
	if len(want) > int(items) {
		return fmt.Errorf("%s: Go returned %d items, need gave %d", name, len(want), items)
	}
	fill := h.rng.IntN(3)
	if h.noFill {
		fill = 0
	}
	if items > 0 && h.rng.IntN(4) == 0 {
		dst := make([]CT, fill+int(items)-1)
		n, an := C.size_t(fill), h.afill
		var nn C.size_t
		if rc := call(ptr(dst), C.size_t(len(dst)), &n, &nn, h.arena, harnessArena, &an, nil); rc != C.DMRGO_ERR_SHORT || int(n) != fill || an != h.afill || nn != C.size_t(fill)+items {
			return fmt.Errorf("%s: short call gave status %d fill %d arena %d need %d", name, rc, n, an, nn)
		}
		h.shorts++
	}
	dst := make([]CT, fill+int(items)+h.rng.IntN(2))
	n, an := C.size_t(fill), h.afill
	if int(h.afill)+int(ab) > harnessArena {
		return errors.New("harness arena exhausted")
	}
	rc := call(ptr(dst), C.size_t(len(dst)), &n, nil, h.arena, harnessArena, &an, nil)
	if rc == C.DMRGO_ERR_SHORT {
		return fmt.Errorf("%s: DMRGO_ERR_SHORT with the room that need gave", name)
	}
	if rc != goCode(wantErr) {
		return fmt.Errorf("%s: status %d, Go error %w", name, rc, wantErr)
	}
	if int(an-h.afill) > int(ab) {
		return fmt.Errorf("%s: used %d arena bytes, need gave %d", name, an-h.afill, ab)
	}
	h.afill = an
	got := dst[fill:n]
	wantJ := jsonOf(want)
	if len(want) == 0 {
		wantJ = "null"
	}
	check := func() error {
		var g []GT
		if len(got) > 0 {
			g = make([]GT, 0, len(got))
		}
		for i := range got {
			g = append(g, conv(&got[i]))
		}
		if gj := jsonOf(g); gj != wantJ {
			return fmt.Errorf("%s: C gave %s, Go %s", name, gj, wantJ)
		}
		return nil
	}
	h.keep = append(h.keep, check)
	return check()
}

func runBuffer[CT, GT any](h *harness, name string, want []GT, wantErr error, conv func(*CT) GT, need needCall, call appendCall[CT]) error {
	h.noFill = true
	defer func() { h.noFill = false }()
	return runAppend(h, name, want, wantErr, conv, need, call)
}

func (h *harness) recheck() error {
	for _, c := range h.keep {
		if err := c(); err != nil {
			return fmt.Errorf("after later calls: %w", err)
		}
	}
	return nil
}

func identity[T any](p *T) T { return *p }

func cHint(h layer2.Hint) *C.dmrgo_layer2_hint_t {
	c := new(C.dmrgo_layer2_hint_t)
	out_layer2_Hint(c, &h, &outc{})
	return c
}

func cRaw(r *layer2.RawBurst) *C.dmrgo_layer2_raw_burst_t {
	return (*C.dmrgo_layer2_raw_burst_t)(unsafe.Pointer(r))
}

func hintAt(i int) layer2.Hint {
	return layer2.Hint{Slot: enums.Slot1, Stream: 7, HasStream: true, Seq: uint16(i), HasSeq: true, SeqBits: 8}
}

func voiceABI(seed uint64) error {
	h := newHarness(seed)
	defer h.free()
	gcfg := voice.TransmitterConfig{Slot: enums.Slot1, ColorCode: 1, Source: layer2.SyncSourceMS, Queue: 192}
	gtx, err := voice.NewTransmitter(&gcfg)
	if err != nil {
		return err
	}
	ccfg := sized(new(C.dmrgo_voice_transmitter_config_t))
	out_voice_TransmitterConfig(ccfg, &gcfg, h.in)
	var ctx *C.dmrgo_voice_transmitter_t
	if rc := C.dmrgo_voice_new_transmitter(ccfg, &ctx); rc != C.DMRGO_OK {
		return fmt.Errorf("new transmitter %d", rc)
	}
	defer C.dmrgo_voice_transmitter_free(ctx)
	call := voice.Call{Group: true, Target: 9990, Source: 3191868}
	opts := voice.CallOptions{TalkerAlias: "dmrgo abi alias"}
	if err := gtx.Start(0, call, &opts); err != nil {
		return err
	}
	cc, co := new(C.dmrgo_voice_call_t), sized(new(C.dmrgo_voice_call_options_t))
	out_voice_Call(cc, &call, h.in)
	out_voice_CallOptions(co, &opts, h.in)
	if rc := C.dmrgo_voice_transmitter_start(ctx, 0, cc, co); rc != C.DMRGO_OK {
		return fmt.Errorf("start %d", rc)
	}
	frames := make([]vocoder.AMBEFrame, 180)
	for i := range frames {
		frames[i] = vocoder.AMBESilence()
		frames[i][0] = byte(i)
	}
	if err := gtx.Frames(frames...); err != nil {
		return err
	}
	if rc := C.dmrgo_voice_transmitter_frames(ctx, (*C.dmrgo_vocoder_ambe_frame_t)(unsafe.Pointer(&frames[0])), C.size_t(len(frames))); rc != C.DMRGO_OK {
		return fmt.Errorf("frames %d", rc)
	}
	gtx.Stop(0)
	C.dmrgo_voice_transmitter_stop(ctx, 0)
	var bursts []layer2.RawBurst
	gout := make([]voice.Output, 0, voice.MaxOutputs)
	for now := time.Duration(0); now < 20*time.Second; now += 10 * time.Millisecond {
		gout = gtx.Tick(now, gout[:0])
		for _, o := range gout {
			if o.Kind == voice.OutputBurst {
				bursts = append(bursts, o.Burst)
			}
		}
		cn := C.int64_t(now)
		err := runAppend(h, "voice.Transmitter.Tick", gout, nil, fromC_voice_Output,
			func(i, a *C.size_t) C.int { return C.dmrgo_voice_transmitter_tick_need(ctx, cn, i, a) },
			func(d *C.dmrgo_voice_output_t, c C.size_t, n, nn *C.size_t, _ unsafe.Pointer, _ C.size_t, _, _ *C.size_t) C.int {
				return C.dmrgo_voice_transmitter_tick(ctx, cn, d, c, n, nn)
			})
		if err != nil {
			return err
		}
	}
	grx, err := voice.NewReceiver(nil)
	if err != nil {
		return err
	}
	var crx *C.dmrgo_voice_receiver_t
	if rc := C.dmrgo_voice_new_receiver(nil, &crx); rc != C.DMRGO_OK {
		return fmt.Errorf("new receiver %d", rc)
	}
	defer C.dmrgo_voice_receiver_free(crx)
	ev := make([]voice.Event, 0, voice.MaxEvents)
	texts := 0
	for i := range bursts {
		now := time.Duration(i+1) * 60 * time.Millisecond
		hint := hintAt(i)
		ev = grx.Burst(now, &bursts[i], hint, ev[:0])
		for _, e := range ev {
			if len(e.Text) > 0 {
				texts++
			}
		}
		cn, ch, cr := C.int64_t(now), cHint(hint), cRaw(&bursts[i])
		err := runAppend(h, "voice.Receiver.Burst", ev, nil, fromC_voice_Event,
			func(it, a *C.size_t) C.int { return C.dmrgo_voice_receiver_burst_need(crx, cn, cr, ch, it, a) },
			func(d *C.dmrgo_voice_event_t, c C.size_t, n, nn *C.size_t, ar unsafe.Pointer, ac C.size_t, an, ann *C.size_t) C.int {
				return C.dmrgo_voice_receiver_burst(crx, cn, cr, ch, d, c, n, nn, ar, ac, an, ann)
			})
		if err != nil {
			return err
		}
		if i%7 == 0 {
			ev = grx.Tick(now+time.Millisecond, ev[:0])
			cn := C.int64_t(now + time.Millisecond)
			err := runAppend(h, "voice.Receiver.Tick", ev, nil, fromC_voice_Event,
				func(it, a *C.size_t) C.int { return C.dmrgo_voice_receiver_tick_need(crx, cn, it, a) },
				func(d *C.dmrgo_voice_event_t, c C.size_t, n, nn *C.size_t, ar unsafe.Pointer, ac C.size_t, an, ann *C.size_t) C.int {
					return C.dmrgo_voice_receiver_tick(crx, cn, d, c, n, nn, ar, ac, an, ann)
				})
			if err != nil {
				return err
			}
		}
	}
	if texts == 0 {
		return errors.New("voice: no talker alias text in the run")
	}
	return h.recheck()
}

func layer2txABI(seed uint64) error {
	h := newHarness(seed)
	defer h.free()
	lc, err := pdu.NewVoiceLC(true, 9990, 3191868, layer3.ServiceOptions{})
	if err != nil {
		return err
	}
	gopts := layer2.VoiceTransmitterOptions{LC: lc, ColorCode: 1, Source: layer2.SyncSourceMS, PIHeader: &pdu.PIHeader{AlgorithmID: 1}}
	gtx, err := layer2.NewVoiceTransmitter(&gopts)
	if err != nil {
		return err
	}
	copts := sized(new(C.dmrgo_layer2_voice_transmitter_options_t))
	out_layer2_VoiceTransmitterOptions(copts, &gopts, h.in)
	var ctx *C.dmrgo_layer2_voice_transmitter_t
	if rc := C.dmrgo_layer2_new_voice_transmitter(copts, &ctx); rc != C.DMRGO_OK {
		return fmt.Errorf("new %d", rc)
	}
	defer C.dmrgo_layer2_voice_transmitter_free(ctx)
	conv := identity[layer2.RawBurst]
	asRaw := func(d *C.dmrgo_layer2_raw_burst_t) layer2.RawBurst {
		return conv((*layer2.RawBurst)(unsafe.Pointer(d)))
	}
	for i := range 300 {
		switch h.rng.IntN(4) {
		case 0:
			n := h.rng.IntN(11)
			fr := make([]vocoder.AMBEFrame, n)
			for k := range fr {
				fr[k] = vocoder.AMBESilence()
				fr[k][0] = byte(i + k)
				if h.rng.IntN(40) == 0 {
					fr[k][6] |= 1
				}
			}
			want, gerr := gtx.AppendFrames(nil, fr...)
			cf := (*C.dmrgo_vocoder_ambe_frame_t)(unsafe.Pointer(ptr(fr)))
			err = runAppend(h, "VoiceTransmitter.AppendFrames", want, gerr, asRaw,
				func(it, a *C.size_t) C.int {
					return C.dmrgo_layer2_voice_transmitter_append_frames_need(ctx, cf, C.size_t(n), it, a)
				},
				func(d *C.dmrgo_layer2_raw_burst_t, c C.size_t, nn, nd *C.size_t, _ unsafe.Pointer, _ C.size_t, _, _ *C.size_t) C.int {
					return C.dmrgo_layer2_voice_transmitter_append_frames(ctx, d, c, nn, nd, cf, C.size_t(n))
				})
		case 1:
			n := h.rng.IntN(11)
			sl := make([][9]byte, n)
			for k := range sl {
				sl[k][0] = byte(i * k)
			}
			want, gerr := gtx.AppendSlots(nil, sl...)
			cs := (*[9]C.uint8_t)(unsafe.Pointer(ptr(sl)))
			err = runAppend(h, "VoiceTransmitter.AppendSlots", want, gerr, asRaw,
				func(it, a *C.size_t) C.int {
					return C.dmrgo_layer2_voice_transmitter_append_slots_need(ctx, cs, C.size_t(n), it, a)
				},
				func(d *C.dmrgo_layer2_raw_burst_t, c C.size_t, nn, nd *C.size_t, _ unsafe.Pointer, _ C.size_t, _, _ *C.size_t) C.int {
					return C.dmrgo_layer2_voice_transmitter_append_slots(ctx, d, c, nn, nd, cs, C.size_t(n))
				})
		case 2:
			want, gerr := gtx.AppendTerminate(nil)
			err = runAppend(h, "VoiceTransmitter.AppendTerminate", want, gerr, asRaw,
				func(it, a *C.size_t) C.int { return C.dmrgo_layer2_voice_transmitter_append_terminate_need(ctx, it, a) },
				func(d *C.dmrgo_layer2_raw_burst_t, c C.size_t, nn, nd *C.size_t, _ unsafe.Pointer, _ C.size_t, _, _ *C.size_t) C.int {
					return C.dmrgo_layer2_voice_transmitter_append_terminate(ctx, d, c, nn, nd)
				})
			if err == nil && h.rng.IntN(2) == 0 {
				if gerr := gtx.Reset(&gopts); gerr != nil {
					return gerr
				}
				if rc := C.dmrgo_layer2_voice_transmitter_reset(ctx, copts); rc != C.DMRGO_OK {
					return fmt.Errorf("reset %d", rc)
				}
			}
		default:
			want := gtx.AppendHeaders(nil)
			err = runAppend(h, "VoiceTransmitter.AppendHeaders", want, nil, asRaw,
				func(it, a *C.size_t) C.int { return C.dmrgo_layer2_voice_transmitter_append_headers_need(ctx, it, a) },
				func(d *C.dmrgo_layer2_raw_burst_t, c C.size_t, nn, nd *C.size_t, _ unsafe.Pointer, _ C.size_t, _, _ *C.size_t) C.int {
					return C.dmrgo_layer2_voice_transmitter_append_headers(ctx, d, c, nn, nd)
				})
		}
		if err != nil {
			return err
		}
	}
	return h.recheck()
}

func framerABI(seed uint64) error {
	h := newHarness(seed)
	defer h.free()
	b, err := layer2.BuildDataBurst(pdu.PRFillBits(), enums.DataTypeIdle, 1, layer2.SyncSourceBS)
	if err != nil {
		return err
	}
	var stream []byte
	for range 60 {
		stream = append(stream, 0x5a, 0x0f, 0xa5)
		stream = append(stream, b[:]...)
	}
	cuts := make([]byte, 16)
	for i := range cuts {
		cuts[i] = byte(h.rng.IntN(200))
	}
	period := byte(0)
	if seed%2 == 1 {
		period = byte(h.rng.IntN(256))
		for range 1 + h.rng.IntN(40) {
			stream[h.rng.IntN(len(stream))] ^= byte(1 << h.rng.IntN(8))
		}
	}
	total, err := framerRun(h, stream, cuts, period)
	if err != nil {
		return err
	}
	if seed%2 == 0 && total == 0 {
		return errors.New("framer: no bursts in the run")
	}
	return h.recheck()
}

func framerPeriod(p byte) int {
	if p == 0 {
		return 0
	}
	return layer2.BurstBits + int(p)
}

func framerRun(h *harness, stream, cuts []byte, period byte) (int, error) {
	cfg := framer.Config{Period: framerPeriod(period), CACH: period&1 == 0, Threshold: int(period % 5)}
	gf, err := framer.New(&cfg)
	if err != nil {
		return 0, err
	}
	ccfg := sized(new(C.dmrgo_framer_config_t))
	out_layer2_framer_Config(ccfg, &cfg, h.in)
	var cf *C.dmrgo_framer_t
	if rc := C.dmrgo_framer_new(ccfg, &cf); rc != C.DMRGO_OK {
		return 0, fmt.Errorf("new %d", rc)
	}
	defer C.dmrgo_framer_free(cf)
	cs := C.CBytes(stream)
	defer C.free(cs)
	total := 0
	for off, i := 0, 0; off < len(stream); i++ {
		n := 1
		if len(cuts) > 0 {
			n = int(cuts[i%len(cuts)])%97 + 1
		}
		n = min(n, len(stream)-off)
		nbits := 8*n - int(period%8)
		p := (*C.uint8_t)(unsafe.Add(cs, off))
		want := gf.AppendBursts(nil, stream[off:off+n], nbits)
		total += len(want)
		err := runAppend(h, "Framer.AppendBursts", want, nil, fromC_layer2_framer_Burst,
			func(it, a *C.size_t) C.int {
				return C.dmrgo_framer_append_bursts_need(cf, p, C.size_t(n), C.int64_t(nbits), it, a)
			},
			func(d *C.dmrgo_framer_burst_t, c C.size_t, nn, nd *C.size_t, _ unsafe.Pointer, _ C.size_t, _, _ *C.size_t) C.int {
				return C.dmrgo_framer_append_bursts(cf, d, c, nn, nd, p, C.size_t(n), C.int64_t(nbits))
			})
		if err != nil {
			return total, err
		}
		off += n
	}
	return total, nil
}

func talkerAliasABI(seed uint64) error {
	h := newHarness(seed)
	defer h.free()
	var ga pdu.TalkerAliasAssembler
	var ca *C.dmrgo_pdu_talker_alias_assembler_t
	if rc := C.dmrgo_pdu_talker_alias_assembler_new(&ca); rc != C.DMRGO_OK {
		return fmt.Errorf("new %d", rc)
	}
	defer C.dmrgo_pdu_talker_alias_assembler_free(ca)
	aliases := 0
	for i := range 40 {
		alias := fmt.Sprintf("alias %d %s", i, string(make([]byte, h.rng.IntN(20))))
		lcs, err := pdu.EncodeTalkerAlias(alias, layer3.TalkerAliasDataFormat(i%3))
		if err != nil {
			continue
		}
		for k := range lcs {
			clc := new(C.dmrgo_pdu_full_link_control_t)
			out_layer2_pdu_FullLinkControl(clc, &lcs[k], h.in)
			want, ok, gerr := ga.AppendAdd(nil, &lcs[k])
			if ok {
				aliases++
			}
			var cok C.bool
			err := runAppend(h, "TalkerAliasAssembler.AppendAdd", want, gerr, func(p *C.uint8_t) byte { return byte(*p) },
				func(it, a *C.size_t) C.int { return C.dmrgo_pdu_talker_alias_assembler_append_add_need(ca, clc, it, a) },
				func(d *C.uint8_t, c C.size_t, nn, nd *C.size_t, _ unsafe.Pointer, _ C.size_t, _, _ *C.size_t) C.int {
					return C.dmrgo_pdu_talker_alias_assembler_append_add(ca, d, c, nn, nd, clc, &cok)
				})
			if err != nil {
				return err
			}
			if bool(cok) != ok {
				return fmt.Errorf("talker alias: ok %v, Go %v", cok, ok)
			}
		}
	}
	if aliases == 0 {
		return errors.New("talker alias: no complete alias")
	}
	return h.recheck()
}

func codec2ABI(seed uint64) error {
	h := newHarness(seed)
	defer h.free()
	cfg := codec2.Config{}
	gc, err := codec2.New(&cfg)
	if errors.Is(err, codec2.ErrUnavailable) {
		return nil
	}
	if err != nil {
		return err
	}
	defer gc.Close()
	ccfg := sized(new(C.dmrgo_codec2_config_t))
	out_vocoder_codec2_Config(ccfg, &cfg, h.in)
	var cc *C.dmrgo_codec2_codec_t
	if rc := C.dmrgo_codec2_new(ccfg, &cc); rc != C.DMRGO_OK {
		return fmt.Errorf("new %d", rc)
	}
	defer func() {
		C.dmrgo_codec2_codec_close(cc)
		C.dmrgo_codec2_codec_free(cc)
	}()
	for range 30 {
		var slot [9]byte
		for k := range slot {
			slot[k] = byte(h.rng.IntN(256))
		}
		cs := (*C.uint8_t)(unsafe.Pointer(&slot[0]))
		pcm, grep, gerr := gc.Decode(nil, slot)
		want := make([]int16, len(pcm))
		crep := new(C.dmrgo_vocoder_frame_report_t)
		err := runAppend(h, "codec2.Codec.Decode", want, gerr, func(*C.int16_t) int16 { return 0 },
			func(it, a *C.size_t) C.int { return C.dmrgo_codec2_codec_decode_need(cc, cs, it, a) },
			func(d *C.int16_t, c C.size_t, nn, nd *C.size_t, _ unsafe.Pointer, _ C.size_t, _, _ *C.size_t) C.int {
				return C.dmrgo_codec2_codec_decode(cc, d, c, nn, nd, cs, crep)
			})
		if err != nil {
			return err
		}
		if got := in_vocoder_FrameReport(crep); gerr == nil && got != grep {
			return fmt.Errorf("codec2.Codec.Decode: report %+v, want %+v", got, grep)
		}
	}
	return h.recheck()
}
