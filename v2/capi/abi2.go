package main

/*
#include <stdlib.h>
#define DMRGO_USER_T uintptr_t
#include "dmrgo.h"
*/
import "C"

import (
	"errors"
	"fmt"
	"time"
	"unsafe"

	"github.com/USA-RedDragon/dmrgo/v2/bs"
	"github.com/USA-RedDragon/dmrgo/v2/csbk"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/packet"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/layer3"
	"github.com/USA-RedDragon/dmrgo/v2/trunk"
	"github.com/USA-RedDragon/dmrgo/v2/vocoder"
)

type valueCall[CT any] func(out *CT, ar unsafe.Pointer, acap C.size_t, an, ann *C.size_t) C.int

func runValue[CT, GT any](h *harness, name string, want GT, wantErr error, conv func(*CT) GT, need needCall, call valueCall[CT]) (*CT, error) {
	h.calls++
	var items, ab C.size_t
	if rc := need(&items, &ab); rc != C.DMRGO_OK {
		return nil, fmt.Errorf("%s: need returned %d", name, rc)
	}
	out := new(CT)
	if ab > 0 && h.rng.IntN(4) == 0 {
		an := h.afill
		var ann C.size_t
		if rc := call(out, h.arena, h.afill+ab-1, &an, &ann); rc != C.DMRGO_ERR_SHORT || an != h.afill || ann != h.afill+ab {
			return nil, fmt.Errorf("%s: short call gave status %d arena %d need %d", name, rc, an, ann)
		}
		h.shorts++
	}
	an := h.afill
	if int(h.afill)+int(ab) > harnessArena {
		return nil, errors.New("harness arena exhausted")
	}
	rc := call(out, h.arena, harnessArena, &an, nil)
	if rc == C.DMRGO_ERR_SHORT {
		return nil, fmt.Errorf("%s: DMRGO_ERR_SHORT with the room that need gave", name)
	}
	if rc != goCode(wantErr) {
		return nil, fmt.Errorf("%s: status %d, Go error %w", name, rc, wantErr)
	}
	if int(an-h.afill) > int(ab) {
		return nil, fmt.Errorf("%s: used %d arena bytes, need gave %d", name, an-h.afill, ab)
	}
	h.afill = an
	wantJ := jsonOf(want)
	check := func() error {
		if gj := jsonOf(conv(out)); gj != wantJ {
			return fmt.Errorf("%s: C gave %s, Go %s", name, gj, wantJ)
		}
		return nil
	}
	h.keep = append(h.keep, check)
	return out, check()
}

func packetABI(seed uint64) error {
	h := newHarness(seed)
	defer h.free()
	gb := packet.NewBuilder(127)
	ga := packet.NewAssembler(127)
	var cb *C.dmrgo_packet_builder_t
	var ca *C.dmrgo_packet_assembler_t
	if rc := C.dmrgo_packet_new_builder(127, &cb); rc != C.DMRGO_OK {
		return fmt.Errorf("new builder %d", rc)
	}
	defer C.dmrgo_packet_builder_free(cb)
	if rc := C.dmrgo_packet_new_assembler(127, &ca); rc != C.DMRGO_OK {
		return fmt.Errorf("new assembler %d", rc)
	}
	defer C.dmrgo_packet_assembler_free(ca)
	rates := []enums.DataType{enums.DataTypeRate12, enums.DataTypeRate34, enums.DataTypeRate1}
	messages := 0
	for i := range 12 {
		var header pdu.DataHeader
		if err := header.SetConfirmed(pdu.ConfirmedDataHeader{
			ResponseRequested: true, SAP: enums.SAPIPBasedPacketData, LLIDDestination: 3120002, LLIDSource: 3120001,
			FullMessageFlag: true, SendSequenceNumber: uint8(i % 8)}); err != nil {
			return err
		}
		payload := make([]byte, 1+h.rng.IntN(900))
		for k := range payload {
			payload[k] = byte(k*31 + i)
		}
		rate := rates[i%3]
		gp, gerr := gb.Segment(header, payload, rate)
		ch := new(C.dmrgo_pdu_data_header_t)
		out_layer2_pdu_DataHeader(ch, &header, h.in)
		cpay := C.CBytes(payload)
		cp, err := runValue(h, "Builder.Segment", gp, gerr, fromC_layer2_packet_Packet,
			func(it, a *C.size_t) C.int {
				return C.dmrgo_packet_builder_segment_need(cb, ch, (*C.uint8_t)(cpay), C.size_t(len(payload)), C.dmrgo_enums_data_type_t(rate), it, a)
			},
			func(o *C.dmrgo_packet_t, ar unsafe.Pointer, ac C.size_t, an, ann *C.size_t) C.int {
				return C.dmrgo_packet_builder_segment(cb, ch, (*C.uint8_t)(cpay), C.size_t(len(payload)), C.dmrgo_enums_data_type_t(rate), o, ar, ac, an, ann)
			})
		C.free(cpay)
		if err != nil {
			return err
		}
		if gerr != nil {
			continue
		}
		feed := func(name string, gd layer2.Data, cd *C.dmrgo_layer2_data_t) error {
			gm, gok, gerr := ga.Add(gd)
			if gok {
				messages++
			}
			var cok C.bool
			_, err := runValue(h, name, gm, gerr, fromC_layer2_packet_Message,
				func(it, a *C.size_t) C.int { return C.dmrgo_packet_assembler_add_need(ca, cd, it, a) },
				func(o *C.dmrgo_packet_message_t, ar unsafe.Pointer, ac C.size_t, an, ann *C.size_t) C.int {
					return C.dmrgo_packet_assembler_add(ca, cd, o, &cok, ar, ac, an, ann)
				})
			if err == nil && bool(cok) != gok {
				err = fmt.Errorf("%s: ok %v, Go %v", name, cok, gok)
			}
			return err
		}
		chd := new(C.dmrgo_layer2_data_t)
		out_layer2_Data(chd, &gp.Header, h.in)
		if err := feed("Assembler.Add header", &gp.Header, chd); err != nil {
			return err
		}
		blocks := unsafe.Slice(cp.blocks, int(cp.blocks_len))
		for k := range gp.Blocks {
			if i%2 == 1 && k%7 == i%7 {
				continue
			}
			if err := feed("Assembler.Add block", gp.Blocks[k], &blocks[k]); err != nil {
				return err
			}
		}
		gmiss := ga.Missing()
		gmissC := make([]int64, len(gmiss))
		for k, v := range gmiss {
			gmissC[k] = int64(v)
		}
		err = runBuffer(h, "Assembler.Missing", gmissC, nil, func(p *C.int64_t) int64 { return int64(*p) },
			func(it, a *C.size_t) C.int { return C.dmrgo_packet_assembler_missing_need(ca, it, a) },
			func(d *C.int64_t, c C.size_t, n, nn *C.size_t, _ unsafe.Pointer, _ C.size_t, _, _ *C.size_t) C.int {
				return C.dmrgo_packet_assembler_missing(ca, d, c, n, nn)
			})
		if err != nil {
			return err
		}
		gr, gerr := ga.Response()
		_, err = runValue(h, "Assembler.Response", gr, gerr, fromC_layer2_packet_Packet,
			func(it, a *C.size_t) C.int { return C.dmrgo_packet_assembler_response_need(ca, it, a) },
			func(o *C.dmrgo_packet_t, ar unsafe.Pointer, ac C.size_t, an, ann *C.size_t) C.int {
				return C.dmrgo_packet_assembler_response(ca, o, ar, ac, an, ann)
			})
		if err != nil {
			return err
		}
		var rd packet.ResponseData
		for k := range len(gp.Blocks) {
			rd.Retry[k] = k%7 == i%7
		}
		crd := new(C.dmrgo_packet_response_data_t)
		out_layer2_packet_ResponseData(crd, &rd, h.in)
		gq, gerr := gb.Retry(&gp, &rd)
		_, err = runValue(h, "Builder.Retry", gq, gerr, fromC_layer2_packet_Packet,
			func(it, a *C.size_t) C.int { return C.dmrgo_packet_builder_retry_need(cb, cp, crd, it, a) },
			func(o *C.dmrgo_packet_t, ar unsafe.Pointer, ac C.size_t, an, ann *C.size_t) C.int {
				return C.dmrgo_packet_builder_retry(cb, cp, crd, o, ar, ac, an, ann)
			})
		if err != nil {
			return err
		}
		gm, gok, gerr := ga.Lost()
		var cok C.bool
		_, err = runValue(h, "Assembler.Lost", gm, gerr, fromC_layer2_packet_Message,
			func(it, a *C.size_t) C.int { return C.dmrgo_packet_assembler_lost_need(ca, it, a) },
			func(o *C.dmrgo_packet_message_t, ar unsafe.Pointer, ac C.size_t, an, ann *C.size_t) C.int {
				return C.dmrgo_packet_assembler_lost(ca, o, &cok, ar, ac, an, ann)
			})
		if err != nil {
			return err
		}
		if bool(cok) != gok {
			return fmt.Errorf("Assembler.Lost: ok %v, Go %v", cok, gok)
		}
	}
	if messages == 0 {
		return errors.New("packet: no message in the run")
	}
	return h.recheck()
}

func voiceBursts(n int) ([]layer2.RawBurst, error) {
	lc, err := pdu.NewVoiceLC(true, 9990, 3191868, layer3.ServiceOptions{})
	if err != nil {
		return nil, err
	}
	tx, err := layer2.NewVoiceTransmitter(&layer2.VoiceTransmitterOptions{LC: lc, ColorCode: 1, Source: layer2.SyncSourceMS})
	if err != nil {
		return nil, err
	}
	out := tx.AppendHeaders(nil)
	fr := make([]vocoder.AMBEFrame, n)
	for i := range fr {
		fr[i] = vocoder.AMBESilence()
	}
	out, err = tx.AppendFrames(out, fr...)
	if err != nil {
		return nil, err
	}
	return tx.AppendTerminate(out)
}

func bsABI(seed uint64) error {
	h := newHarness(seed)
	defer h.free()
	cfg := bs.Config{ColorCode: 1, CACH: true}
	gr, err := bs.New(&cfg)
	if err != nil {
		return err
	}
	ccfg := sized(new(C.dmrgo_bs_config_t))
	out_bs_Config(ccfg, &cfg, h.in)
	var cr *C.dmrgo_bs_repeater_t
	if rc := C.dmrgo_bs_new(ccfg, &cr); rc != C.DMRGO_OK {
		return fmt.Errorf("new %d", rc)
	}
	defer C.dmrgo_bs_repeater_free(cr)
	bursts, err := voiceBursts(60)
	if err != nil {
		return err
	}
	src := bs.Source{RF: true}
	csrc := new(C.dmrgo_bs_source_t)
	out_bs_Source(csrc, &src, h.in)
	out := make([]bs.Output, 0, bs.MaxOutputs)
	total := 0
	for i := range bursts {
		now := time.Duration(i) * 60 * time.Millisecond
		hint := layer2.Hint{Slot: enums.Slot(1 + h.rng.IntN(2))}
		if i%3 != 0 {
			hint.Slot = enums.Slot1
		}
		out = gr.Inbound(now, &bursts[i], hint, src, out[:0])
		total += len(out)
		cn, ch, craw := C.int64_t(now), cHint(hint), cRaw(&bursts[i])
		err := runAppend(h, "Repeater.Inbound", out, nil, fromC_bs_Output,
			func(it, a *C.size_t) C.int { return C.dmrgo_bs_repeater_inbound_need(cr, cn, craw, ch, csrc, it, a) },
			func(d *C.dmrgo_bs_output_t, c C.size_t, n, nn *C.size_t, _ unsafe.Pointer, _ C.size_t, _, _ *C.size_t) C.int {
				return C.dmrgo_bs_repeater_inbound(cr, cn, craw, ch, csrc, d, c, n, nn)
			})
		if err != nil {
			return err
		}
		for k := range 2 {
			t := now + time.Duration(k)*30*time.Millisecond
			out = gr.Tick(t, out[:0])
			total += len(out)
			ct := C.int64_t(t)
			err := runAppend(h, "Repeater.Tick", out, nil, fromC_bs_Output,
				func(it, a *C.size_t) C.int { return C.dmrgo_bs_repeater_tick_need(cr, ct, it, a) },
				func(d *C.dmrgo_bs_output_t, c C.size_t, n, nn *C.size_t, _ unsafe.Pointer, _ C.size_t, _, _ *C.size_t) C.int {
					return C.dmrgo_bs_repeater_tick(cr, ct, d, c, n, nn)
				})
			if err != nil {
				return err
			}
		}
	}
	if total == 0 {
		return errors.New("bs: no outputs in the run")
	}
	return h.recheck()
}

func csbkABI(seed uint64) error {
	h := newHarness(seed)
	defer h.free()
	qcfg := csbk.RequesterConfig{ColorCode: 7, Source: layer2.SyncSourceMS, HoldoffMax: 200 * time.Millisecond}
	rcfg := csbk.ResponderConfig{Address: 3120002, ColorCode: 7}
	wcfg := csbk.WakeupConfig{ColorCode: 7}
	gq, err := csbk.NewRequester(&qcfg)
	if err != nil {
		return err
	}
	gr, err := csbk.NewResponder(&rcfg)
	if err != nil {
		return err
	}
	gw, err := csbk.NewWakeup(&wcfg)
	if err != nil {
		return err
	}
	gp, err := csbk.NewPreambles(&csbk.PreamblesConfig{})
	if err != nil {
		return err
	}
	cq, cr, cw, cp := new(*C.dmrgo_csbk_requester_t), new(*C.dmrgo_csbk_responder_t), new(*C.dmrgo_csbk_wakeup_t), new(*C.dmrgo_csbk_preambles_t)
	cqc, crc, cwc, cpc := sized(new(C.dmrgo_csbk_requester_config_t)), sized(new(C.dmrgo_csbk_responder_config_t)), sized(new(C.dmrgo_csbk_wakeup_config_t)), sized(new(C.dmrgo_csbk_preambles_config_t))
	out_csbk_RequesterConfig(cqc, &qcfg, h.in)
	out_csbk_ResponderConfig(crc, &rcfg, h.in)
	out_csbk_WakeupConfig(cwc, &wcfg, h.in)
	if C.dmrgo_csbk_new_requester(cqc, cq) != 0 || C.dmrgo_csbk_new_responder(crc, cr) != 0 || C.dmrgo_csbk_new_wakeup(cwc, cw) != 0 || C.dmrgo_csbk_new_preambles(cpc, cp) != 0 {
		return errors.New("csbk: new failed")
	}
	defer func() {
		C.dmrgo_csbk_requester_free(*cq)
		C.dmrgo_csbk_responder_free(*cr)
		C.dmrgo_csbk_wakeup_free(*cw)
		C.dmrgo_csbk_preambles_free(*cp)
	}()
	gerr := gq.RequestRadioCheck(0, 3120002, 3120001, nil)
	if rc := C.dmrgo_csbk_requester_request_radio_check(*cq, 0, 3120002, 3120001, nil); rc != goCode(gerr) {
		return fmt.Errorf("request radio check %d, Go %w", rc, gerr)
	}
	gerr = gw.Wake(0, 310123, 3120001)
	if rc := C.dmrgo_csbk_wakeup_wake(*cw, 0, 310123, 3120001); rc != goCode(gerr) {
		return fmt.Errorf("wake %d, Go %w", rc, gerr)
	}
	out := make([]csbk.Output, 0, csbk.MaxOutputs)
	conv := fromC_csbk_Output
	tick := func(name string, now time.Duration, g func(time.Duration, []csbk.Output) []csbk.Output, need func(C.int64_t, *C.size_t, *C.size_t) C.int,
		call func(C.int64_t, *C.dmrgo_csbk_output_t, C.size_t, *C.size_t, *C.size_t) C.int) ([]csbk.Output, error) {
		o := append([]csbk.Output(nil), g(now, out[:0])...)
		cn := C.int64_t(now)
		return o, runAppend(h, name, o, nil, conv,
			func(it, a *C.size_t) C.int { return need(cn, it, a) },
			func(d *C.dmrgo_csbk_output_t, c C.size_t, n, nn *C.size_t, _ unsafe.Pointer, _ C.size_t, _, _ *C.size_t) C.int {
				return call(cn, d, c, n, nn)
			})
	}
	feed := func(name string, now time.Duration, raw layer2.RawBurst, g func(time.Duration, *layer2.RawBurst, layer2.Hint, []csbk.Output) []csbk.Output,
		need func(C.int64_t, *C.dmrgo_layer2_raw_burst_t, *C.dmrgo_layer2_hint_t, *C.size_t, *C.size_t) C.int,
		call func(C.int64_t, *C.dmrgo_layer2_raw_burst_t, *C.dmrgo_layer2_hint_t, *C.dmrgo_csbk_output_t, C.size_t, *C.size_t, *C.size_t) C.int) ([]csbk.Output, error) {
		hint := layer2.Hint{Slot: enums.Slot1, Kind: layer2.KindData, DataType: enums.DataTypeCSBK, HasDataType: true}
		r := raw
		o := append([]csbk.Output(nil), g(now, &r, hint, out[:0])...)
		cn, ch := C.int64_t(now), cHint(hint)
		cr := raw
		return o, runAppend(h, name, o, nil, conv,
			func(it, a *C.size_t) C.int { return need(cn, cRaw(&cr), ch, it, a) },
			func(d *C.dmrgo_csbk_output_t, c C.size_t, n, nn *C.size_t, _ unsafe.Pointer, _ C.size_t, _, _ *C.size_t) C.int {
				return call(cn, cRaw(&cr), ch, d, c, n, nn)
			})
	}
	total := 0
	for i := range 200 {
		now := time.Duration(i) * 30 * time.Millisecond
		qo, err := tick("Requester.Tick", now, gq.Tick, func(n C.int64_t, a, b *C.size_t) C.int { return C.dmrgo_csbk_requester_tick_need(*cq, n, a, b) },
			func(n C.int64_t, d *C.dmrgo_csbk_output_t, c C.size_t, l, nn *C.size_t) C.int {
				return C.dmrgo_csbk_requester_tick(*cq, n, d, c, l, nn)
			})
		if err != nil {
			return err
		}
		ro, err := tick("Responder.Tick", now, gr.Tick, func(n C.int64_t, a, b *C.size_t) C.int { return C.dmrgo_csbk_responder_tick_need(*cr, n, a, b) },
			func(n C.int64_t, d *C.dmrgo_csbk_output_t, c C.size_t, l, nn *C.size_t) C.int {
				return C.dmrgo_csbk_responder_tick(*cr, n, d, c, l, nn)
			})
		if err != nil {
			return err
		}
		wo, err := tick("Wakeup.Tick", now, gw.Tick, func(n C.int64_t, a, b *C.size_t) C.int { return C.dmrgo_csbk_wakeup_tick_need(*cw, n, a, b) },
			func(n C.int64_t, d *C.dmrgo_csbk_output_t, c C.size_t, l, nn *C.size_t) C.int {
				return C.dmrgo_csbk_wakeup_tick(*cw, n, d, c, l, nn)
			})
		if err != nil {
			return err
		}
		total += len(qo) + len(ro) + len(wo)
		for _, o := range qo {
			if o.Kind != csbk.OutputBurst {
				continue
			}
			if _, err := feed("Responder.Burst", now, o.Burst, gr.Burst,
				func(n C.int64_t, r *C.dmrgo_layer2_raw_burst_t, hh *C.dmrgo_layer2_hint_t, a, b *C.size_t) C.int {
					return C.dmrgo_csbk_responder_burst_need(*cr, n, r, hh, a, b)
				},
				func(n C.int64_t, r *C.dmrgo_layer2_raw_burst_t, hh *C.dmrgo_layer2_hint_t, d *C.dmrgo_csbk_output_t, c C.size_t, l, nn *C.size_t) C.int {
					return C.dmrgo_csbk_responder_burst(*cr, n, r, hh, d, c, l, nn)
				}); err != nil {
				return err
			}
			if _, err := feed("Preambles.Burst", now, o.Burst, gp.Burst,
				func(n C.int64_t, r *C.dmrgo_layer2_raw_burst_t, hh *C.dmrgo_layer2_hint_t, a, b *C.size_t) C.int {
					return C.dmrgo_csbk_preambles_burst_need(*cp, n, r, hh, a, b)
				},
				func(n C.int64_t, r *C.dmrgo_layer2_raw_burst_t, hh *C.dmrgo_layer2_hint_t, d *C.dmrgo_csbk_output_t, c C.size_t, l, nn *C.size_t) C.int {
					return C.dmrgo_csbk_preambles_burst(*cp, n, r, hh, d, c, l, nn)
				}); err != nil {
				return err
			}
		}
		for _, o := range append(ro, wo...) {
			if o.Kind != csbk.OutputBurst {
				continue
			}
			if _, err := feed("Requester.Burst", now, o.Burst, gq.Burst,
				func(n C.int64_t, r *C.dmrgo_layer2_raw_burst_t, hh *C.dmrgo_layer2_hint_t, a, b *C.size_t) C.int {
					return C.dmrgo_csbk_requester_burst_need(*cq, n, r, hh, a, b)
				},
				func(n C.int64_t, r *C.dmrgo_layer2_raw_burst_t, hh *C.dmrgo_layer2_hint_t, d *C.dmrgo_csbk_output_t, c C.size_t, l, nn *C.size_t) C.int {
					return C.dmrgo_csbk_requester_burst(*cq, n, r, hh, d, c, l, nn)
				}); err != nil {
				return err
			}
			if _, err := feed("Wakeup.Burst", now, o.Burst, gw.Burst,
				func(n C.int64_t, r *C.dmrgo_layer2_raw_burst_t, hh *C.dmrgo_layer2_hint_t, a, b *C.size_t) C.int {
					return C.dmrgo_csbk_wakeup_burst_need(*cw, n, r, hh, a, b)
				},
				func(n C.int64_t, r *C.dmrgo_layer2_raw_burst_t, hh *C.dmrgo_layer2_hint_t, d *C.dmrgo_csbk_output_t, c C.size_t, l, nn *C.size_t) C.int {
					return C.dmrgo_csbk_wakeup_burst(*cw, n, r, hh, d, c, l, nn)
				}); err != nil {
				return err
			}
		}
	}
	if total == 0 {
		return errors.New("csbk: no outputs in the run")
	}
	return h.recheck()
}

type abiMS struct {
	g       *trunk.MS
	c       *C.dmrgo_trunk_ms_t
	channel uint16
}

type abiInbound struct {
	due  time.Duration
	ch   uint16
	slot enums.Slot
	raw  layer2.RawBurst
}

func abiCall(h *harness, m *abiMS, now time.Duration, tg uint32) error {
	opts := layer3.VoiceServiceOptions{}
	gerr := m.g.Call(now, true, tg, opts)
	co := new(C.dmrgo_layer3_voice_service_options_t)
	out_layer3_VoiceServiceOptions(co, &opts, h.in)
	if rc := C.dmrgo_trunk_ms_call(m.c, C.int64_t(now), true, C.uint32_t(tg), co); rc != goCode(gerr) {
		return fmt.Errorf("ms call %d, Go %w", rc, gerr)
	}
	return nil
}

func abiInboundRun(h *harness, gt *trunk.TSCC, ct *C.dmrgo_trunk_tscc_t, now time.Duration, p abiInbound, tout []trunk.TSCCOutput) ([]trunk.TSCCOutput, error) {
	r := p.raw
	hint := layer2.Hint{Slot: p.slot, Kind: layer2.KindData}
	tout = gt.Inbound(now, p.ch, &r, hint, tout[:0])
	cn, ch := C.int64_t(now), cHint(hint)
	cr := p.raw
	pch := C.uint16_t(p.ch)
	return tout, runAppend(h, "TSCC.Inbound", tout, nil, fromC_trunk_TSCCOutput,
		func(it, a *C.size_t) C.int {
			return C.dmrgo_trunk_tscc_inbound_need(ct, cn, pch, cRaw(&cr), ch, it, a)
		},
		func(d *C.dmrgo_trunk_tscc_output_t, c C.size_t, n, nn *C.size_t, _ unsafe.Pointer, _ C.size_t, _, _ *C.size_t) C.int {
			return C.dmrgo_trunk_tscc_inbound(ct, cn, pch, cRaw(&cr), ch, d, c, n, nn)
		})
}

func abiFollow(h *harness, gf *trunk.Follower, cf *C.dmrgo_trunk_follower_t, now time.Duration, raw layer2.RawBurst, hint layer2.Hint, cach [3]byte) error {
	r := raw
	fo := gf.Burst(now, &r, hint, &cach, nil)
	fo = append([]trunk.Output(nil), fo...)
	cn, ch := C.int64_t(now), cHint(hint)
	cc := (*C.uint8_t)(unsafe.Pointer(&cach[0]))
	cr := raw
	return runAppend(h, "Follower.Burst", fo, nil, fromC_trunk_Output,
		func(it, a *C.size_t) C.int {
			return C.dmrgo_trunk_follower_burst_need(cf, cn, cRaw(&cr), ch, cc, it, a)
		},
		func(d *C.dmrgo_trunk_output_t, c C.size_t, n, nn *C.size_t, _ unsafe.Pointer, _ C.size_t, _, _ *C.size_t) C.int {
			return C.dmrgo_trunk_follower_burst(cf, cn, cRaw(&cr), ch, cc, d, c, n, nn)
		})
}

func newABIMS(h *harness, i int) (*abiMS, error) {
	mc := trunk.MSConfig{Address: uint32(3120001 + i), TalkgroupsLen: 1, Seed: uint64(i + 1)}
	mc.Talkgroups[0] = uint32(100 + i)
	g, err := trunk.NewMS(&mc)
	if err != nil {
		return nil, err
	}
	cmc := sized(new(C.dmrgo_trunk_ms_config_t))
	out_trunk_MSConfig(cmc, &mc, h.in)
	m := &abiMS{g: g, channel: 1}
	if rc := C.dmrgo_trunk_new_ms(cmc, &m.c); rc != C.DMRGO_OK {
		return nil, fmt.Errorf("new ms %d", rc)
	}
	return m, nil
}

type abiTrunk struct {
	h       *harness
	pending []abiInbound
	mout    []trunk.Output
}

func (st *abiTrunk) msOut(m *abiMS, o []trunk.Output) {
	for _, x := range o {
		switch x.Kind {
		case trunk.OutputTune:
			m.channel = x.Tune.Channel
			if m.channel == trunk.ChannelNull {
				m.channel = 1
			}
		case trunk.OutputBurst:
			st.pending = append(st.pending, abiInbound{due: x.Due, ch: m.channel, slot: x.Slot, raw: x.Burst})
		default:
		}
	}
}
func (st *abiTrunk) runMS(name string, m *abiMS, now time.Duration, raw *layer2.RawBurst, hint layer2.Hint, cach *[3]byte) error {
	cn := C.int64_t(now)
	if raw == nil {
		st.mout = m.g.Tick(now, st.mout[:0])
		st.msOut(m, st.mout)
		return runAppend(st.h, name, st.mout, nil, fromC_trunk_Output,
			func(it, a *C.size_t) C.int { return C.dmrgo_trunk_ms_tick_need(m.c, cn, it, a) },
			func(d *C.dmrgo_trunk_output_t, c C.size_t, n, nn *C.size_t, _ unsafe.Pointer, _ C.size_t, _, _ *C.size_t) C.int {
				return C.dmrgo_trunk_ms_tick(m.c, cn, d, c, n, nn)
			})
	}
	r := *raw
	st.mout = m.g.Burst(now, &r, hint, cach, st.mout[:0])
	st.msOut(m, st.mout)
	ch := cHint(hint)
	var cc *C.uint8_t
	if cach != nil {
		cc = (*C.uint8_t)(unsafe.Pointer(&cach[0]))
	}
	cr := *raw
	return runAppend(st.h, name, st.mout, nil, fromC_trunk_Output,
		func(it, a *C.size_t) C.int { return C.dmrgo_trunk_ms_burst_need(m.c, cn, cRaw(&cr), ch, cc, it, a) },
		func(d *C.dmrgo_trunk_output_t, c C.size_t, n, nn *C.size_t, _ unsafe.Pointer, _ C.size_t, _, _ *C.size_t) C.int {
			return C.dmrgo_trunk_ms_burst(m.c, cn, cRaw(&cr), ch, cc, d, c, n, nn)
		})
}

func trunkABI(seed uint64, payload, nms int) (int, error) {
	h := newHarness(seed)
	defer h.free()
	sys := pdu.SysCode{Model: pdu.NetworkModelSmall, Net: 12, Site: 3, Par: 2}
	cfg := trunk.TSCCConfig{SysCode: sys, ColorCode: 3, Channel: 1, PayloadLen: payload, Reg: true, NRandWait: 3, Backoff: 4,
		Hangtime: time.Minute, Inactive: time.Minute}
	for i := range cfg.Payload {
		cfg.Payload[i] = uint16(10 + i)
	}
	gt, err := trunk.NewTSCC(&cfg)
	if err != nil {
		return 0, err
	}
	ccfg := sized(new(C.dmrgo_trunk_tscc_config_t))
	out_trunk_TSCCConfig(ccfg, &cfg, h.in)
	var ct *C.dmrgo_trunk_tscc_t
	if rc := C.dmrgo_trunk_new_tscc(ccfg, &ct); rc != C.DMRGO_OK {
		return 0, fmt.Errorf("new tscc %d", rc)
	}
	defer C.dmrgo_trunk_tscc_free(ct)
	ms := make([]*abiMS, 0, nms)
	defer func() {
		for _, m := range ms {
			C.dmrgo_trunk_ms_free(m.c)
		}
	}()
	for i := range nms {
		m, err := newABIMS(h, i)
		if err != nil {
			return 0, err
		}
		ms = append(ms, m)
	}
	fcfg := trunk.FollowerConfig{}
	gf, err := trunk.NewFollower(&fcfg)
	if err != nil {
		return 0, err
	}
	cfc := sized(new(C.dmrgo_trunk_follower_config_t))
	out_trunk_FollowerConfig(cfc, &fcfg, h.in)
	var cf *C.dmrgo_trunk_follower_t
	if rc := C.dmrgo_trunk_new_follower(cfc, &cf); rc != C.DMRGO_OK {
		return 0, fmt.Errorf("new follower %d", rc)
	}
	defer C.dmrgo_trunk_follower_free(cf)
	most := 0
	tout := make([]trunk.TSCCOutput, 0, trunk.MaxTSCCOutputs)
	st := &abiTrunk{h: h, mout: make([]trunk.Output, 0, trunk.MaxOutputs)}
	called := 0
	for step := 0; step < 1400; step++ {
		now := time.Duration(step) * 30 * time.Millisecond
		if step > 300 && step%25 == 0 && called < len(ms) {
			called++
			if err := abiCall(h, ms[called-1], now, uint32(99+called)); err != nil {
				return 0, err
			}
		}
		keep := st.pending[:0]
		var due []abiInbound
		for _, p := range st.pending {
			if p.due <= now {
				due = append(due, p)
			} else {
				keep = append(keep, p)
			}
		}
		st.pending = keep
		for _, p := range due {
			if tout, err = abiInboundRun(h, gt, ct, now, p, tout); err != nil {
				return 0, err
			}
		}
		tout = gt.Tick(now, tout[:0])
		most = max(most, len(tout))
		bursts := append([]trunk.TSCCOutput(nil), tout...)
		cn := C.int64_t(now)
		err := runAppend(h, "TSCC.Tick", tout, nil, fromC_trunk_TSCCOutput,
			func(it, a *C.size_t) C.int { return C.dmrgo_trunk_tscc_tick_need(ct, cn, it, a) },
			func(d *C.dmrgo_trunk_tscc_output_t, c C.size_t, n, nn *C.size_t, _ unsafe.Pointer, _ C.size_t, _, _ *C.size_t) C.int {
				return C.dmrgo_trunk_tscc_tick(ct, cn, d, c, n, nn)
			})
		if err != nil {
			return 0, err
		}
		for _, m := range ms {
			if err := st.runMS("MS.Tick", m, now, nil, layer2.Hint{}, nil); err != nil {
				return 0, err
			}
		}
		for _, b := range bursts {
			if b.Kind != trunk.TSCCBurst {
				continue
			}
			cach := b.CACH
			hint := layer2.Hint{Slot: b.Slot}
			if b.Channel == 1 && step%3 == 0 {
				if err := abiFollow(h, gf, cf, now, b.Burst, hint, cach); err != nil {
					return 0, err
				}
			}
			for _, m := range ms {
				if m.channel != b.Channel {
					continue
				}
				if err := st.runMS("MS.Burst", m, now, &b.Burst, hint, &cach); err != nil {
					return 0, err
				}
			}
		}
	}
	return most, h.recheck()
}
