package main

/*
#include <stdlib.h>
#define DMRGO_NO_PROTOTYPES
#include "dmrgo.h"
*/
import "C"

import (
	"errors"
	"fmt"
	"math/rand/v2"
	"runtime"
	"unsafe"
)

var errPath = errors.New("capi: a call of the path failed")

func check(rc ...C.int) error {
	for _, r := range rc {
		if r != C.DMRGO_OK {
			return errPath
		}
	}
	return nil
}

func pduPath() (func() error, error) {
	c := new(C.dmrgo_pdu_csbk_t)
	d := new(C.dmrgo_pdu_csbk_t)
	lc := new(C.dmrgo_pdu_full_link_control_t)
	lc2 := new(C.dmrgo_pdu_full_link_control_t)
	so := new(C.dmrgo_layer3_service_options_t)
	bits := new([12]C.uint8_t)
	if err := check(dmrgo_pdu_new_bs_outbound_activation(0x123456, 3191868, c),
		dmrgo_pdu_new_voice_lc(true, 91, 3191868, so, lc)); err != nil {
		return nil, err
	}
	lc.burst_data_type = 1
	var cp *C.dmrgo_pdu_csbk_payloads_t
	var lp *C.dmrgo_pdu_full_link_control_payloads_t
	if err := check(dmrgo_pdu_csbk_payloads_new(&cp), dmrgo_pdu_full_link_control_payloads_new(&lp)); err != nil {
		return nil, err
	}
	var target, source C.uint32_t
	var ok C.bool
	return func() error {
		return check(dmrgo_pdu_csbk_encode(c, &bits[0]), dmrgo_pdu_decode_csbk_into(d, cp, &bits[0]),
			dmrgo_pdu_csbk_addresses(d, &target, &source, &ok),
			dmrgo_pdu_full_link_control_encode(lc, &bits[0]), dmrgo_pdu_decode_full_link_control_into(lc2, lp, &bits[0], 1))
	}, nil
}

func decodePath() (func() error, error) {
	c := new(C.dmrgo_pdu_csbk_t)
	d := new(C.dmrgo_pdu_csbk_t)
	bits := new([12]C.uint8_t)
	if err := check(dmrgo_pdu_new_bs_outbound_activation(0x123456, 3191868, c), dmrgo_pdu_csbk_encode(c, &bits[0])); err != nil {
		return nil, err
	}
	return func() error {
		return check(dmrgo_pdu_decode_csbk(&bits[0], d))
	}, nil
}

func layer2Path() (func() error, error) {
	data := new(C.dmrgo_layer2_data_t)
	data.tag = C.DMRGO_LAYER2_DATA_PDU_CSBK
	if err := check(dmrgo_pdu_new_bs_outbound_activation(0x123456, 3191868, (*C.dmrgo_pdu_csbk_t)(unsafe.Pointer(&data.v)))); err != nil {
		return nil, err
	}
	raw := new(C.dmrgo_layer2_raw_burst_t)
	again := new(C.dmrgo_layer2_raw_burst_t)
	got := new(C.dmrgo_layer2_data_t)
	st := new(C.dmrgo_pdu_slot_type_t)
	var b *C.dmrgo_layer2_burst_t
	if err := check(dmrgo_layer2_new_burst(&b)); err != nil {
		return nil, err
	}
	var k C.dmrgo_layer2_kind_t
	return func() error {
		return check(dmrgo_layer2_build_burst(data, 5, C.DMRGO_LAYER2_SYNC_SOURCE_BS, raw),
			dmrgo_layer2_burst_decode_from_bytes(b, raw), dmrgo_layer2_burst_get_kind(b, &k),
			dmrgo_layer2_burst_get_data(b, got), dmrgo_layer2_burst_get_slot_type(b, st), dmrgo_layer2_burst_encode(b, again))
	}, nil
}

func txPath() (func() error, error) {
	opts := new(C.dmrgo_layer2_voice_transmitter_options_t)
	so := new(C.dmrgo_layer3_service_options_t)
	if err := check(dmrgo_pdu_new_voice_lc(true, 9990, 3191868, so, &opts.lc)); err != nil {
		return nil, err
	}
	opts.color_code = 1
	opts.source = C.DMRGO_LAYER2_SYNC_SOURCE_MS
	var tx *C.dmrgo_layer2_voice_transmitter_t
	if err := check(dmrgo_layer2_new_voice_transmitter(opts, &tx)); err != nil {
		return nil, err
	}
	frames := new([3]C.dmrgo_vocoder_ambe_frame_t)
	bursts := new([4]C.dmrgo_layer2_raw_burst_t)
	for i := range frames {
		if err := check(dmrgo_vocoder_ambe_silence(&frames[i])); err != nil {
			return nil, err
		}
	}
	var n C.size_t
	return func() error {
		n = 0
		return check(dmrgo_layer2_voice_transmitter_append_frames(tx, &bursts[0], 4, &n, nil, &frames[0], 3))
	}, nil
}

func voicePath(capture []byte) (func() error, error) {
	var rx *C.dmrgo_voice_receiver_t
	if err := check(dmrgo_voice_new_receiver(nil, &rx)); err != nil {
		return nil, err
	}
	raw := new(C.dmrgo_layer2_raw_burst_t)
	h := new(C.dmrgo_layer2_hint_t)
	ev := new([6]C.dmrgo_voice_event_t)
	arena := new([4096]byte)
	var now C.int64_t
	off := 0
	return func() error {
		if off+33 > len(capture) {
			off = 0
		}
		copy(unsafe.Slice((*byte)(unsafe.Pointer(&raw.v[0])), 33), capture[off:off+33])
		off += 33
		now += 60000000
		var n C.size_t
		an := C.size_t(0)
		return check(dmrgo_voice_receiver_burst(rx, now, raw, h, &ev[0], 6, &n, nil, unsafe.Pointer(&arena[0]), C.size_t(len(arena)), &an, nil))
	}, nil
}

func partialC() (int, int, error) {
	opts := new(C.dmrgo_layer2_voice_transmitter_options_t)
	so := new(C.dmrgo_layer3_service_options_t)
	if err := check(dmrgo_pdu_new_voice_lc(true, 9990, 3191868, so, &opts.lc)); err != nil {
		return 0, 0, err
	}
	opts.color_code = 1
	opts.source = C.DMRGO_LAYER2_SYNC_SOURCE_MS
	var tx *C.dmrgo_layer2_voice_transmitter_t
	if err := check(dmrgo_layer2_new_voice_transmitter(opts, &tx)); err != nil {
		return 0, 0, err
	}
	defer dmrgo_layer2_voice_transmitter_free(tx)
	frames := new([7]C.dmrgo_vocoder_ambe_frame_t)
	for i := range frames {
		if err := check(dmrgo_vocoder_ambe_silence(&frames[i])); err != nil {
			return 0, 0, err
		}
	}
	frames[6].v[6] |= 1
	bursts := new([64]C.dmrgo_layer2_raw_burst_t)
	var n C.size_t
	rc := dmrgo_layer2_voice_transmitter_append_frames(tx, &bursts[0], 64, &n, nil, &frames[0], 7)
	return int(n), int(rc), nil
}

func lastMessage(msg string, capx int) (int, int, int) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	setLast(C.DMRGO_ERR_OTHER, msg)
	buf := make([]C.char, capx)
	var n, need C.size_t
	rc := dmrgo_last_error_message(unsafe.SliceData(buf), C.size_t(capx), &n, &need)
	return int(n), int(need), int(rc)
}

func eventKeyC(e *C.dmrgo_voice_event_t) string {
	text := ""
	if e.text != nil {
		text = C.GoStringN((*C.char)(unsafe.Pointer(e.text)), C.int(e.text_len))
	}
	return fmt.Sprintf("%d %d %t %d %d %d %q %d %d %d", int(e.kind), int(e.position), bool(e.call.group), int(e.call.target),
		int(e.call.source), int(e.call.origin), text, int(e.reason), int(e.lost), int(e.stats.bursts))
}

func receiverFuzzC(bursts [][33]byte, seed uint64) ([]string, int, error) {
	rng := rand.New(rand.NewPCG(seed, 1))
	var rx *C.dmrgo_voice_receiver_t
	if err := check(dmrgo_voice_new_receiver(nil, &rx)); err != nil {
		return nil, 0, err
	}
	defer dmrgo_voice_receiver_free(rx)
	ev := new([16]C.dmrgo_voice_event_t)
	arena := new([4096]byte)
	h := new(C.dmrgo_layer2_hint_t)
	raw := new(C.dmrgo_layer2_raw_burst_t)
	var keys []string
	shorts := 0
	for i, b := range bursts {
		copy(unsafe.Slice((*byte)(unsafe.Pointer(&raw.v[0])), 33), b[:])
		now := C.int64_t(int64(i+1) * 60000000)
		fill := rng.IntN(3)
		afill := rng.IntN(64)
		for {
			capx := fill + rng.IntN(C.DMRGO_VOICE_MAX_EVENTS+2)
			var items, need C.size_t
			if err := check(dmrgo_voice_receiver_burst_need(rx, now, raw, h, &items, &need)); err != nil {
				return nil, 0, err
			}
			if int(items) != C.DMRGO_VOICE_MAX_EVENTS {
				return nil, 0, fmt.Errorf("need gives %d items", items)
			}
			acap := afill + rng.IntN(int(need)+64)
			n, an := C.size_t(fill), C.size_t(afill)
			rc := dmrgo_voice_receiver_burst(rx, now, raw, h, &ev[0], C.size_t(capx), &n, nil, unsafe.Pointer(&arena[0]), C.size_t(acap), &an, nil)
			if rc == C.DMRGO_ERR_SHORT {
				if int(n) != fill || int(an) != afill {
					return nil, 0, fmt.Errorf("burst %d: SHORT moved the fill to %d and %d", i, n, an)
				}
				shorts++
				continue
			}
			if rc != C.DMRGO_OK {
				return nil, 0, fmt.Errorf("burst %d: status %d", i, rc)
			}
			for j := fill; j < int(n); j++ {
				keys = append(keys, eventKeyC(&ev[j]))
			}
			break
		}
	}
	return keys, shorts, nil
}

func outputKeyC(o *C.dmrgo_voice_output_t) string {
	return fmt.Sprintf("%d %x %d %d %d %t %d", int(o.kind), unsafe.Slice((*byte)(unsafe.Pointer(&o.burst.v[0])), 33), int64(o.due),
		int(o.hint.voice_burst), int(o.hint.data_type), bool(o.fill), int(o.reason))
}

func transmitterFuzzC(alias string, frames int, seed uint64) ([]string, int, error) {
	rng := rand.New(rand.NewPCG(seed, 2))
	cfg := new(C.dmrgo_voice_transmitter_config_t)
	cfg.color_code = 1
	cfg.source = C.DMRGO_LAYER2_SYNC_SOURCE_MS
	cfg.queue = 192
	var tx *C.dmrgo_voice_transmitter_t
	if err := check(dmrgo_voice_new_transmitter(cfg, &tx)); err != nil {
		return nil, 0, err
	}
	defer dmrgo_voice_transmitter_free(tx)
	call := new(C.dmrgo_voice_call_t)
	call.group, call.target, call.source = true, 9990, 3191868
	opts := new(C.dmrgo_voice_call_options_t)
	ab := C.CBytes([]byte(alias))
	defer C.free(ab)
	opts.talker_alias, opts.talker_alias_len = (*C.char)(ab), C.size_t(len(alias))
	if err := check(dmrgo_voice_transmitter_start(tx, 0, call, opts)); err != nil {
		return nil, 0, err
	}
	fs := make([]C.dmrgo_vocoder_ambe_frame_t, frames)
	for i := range fs {
		if err := check(dmrgo_vocoder_ambe_silence(&fs[i])); err != nil {
			return nil, 0, err
		}
		fs[i].v[0] = C.uint8_t(i)
	}
	if err := check(dmrgo_voice_transmitter_frames(tx, &fs[0], C.size_t(frames)), dmrgo_voice_transmitter_stop(tx)); err != nil {
		return nil, 0, err
	}
	out := new([8]C.dmrgo_voice_output_t)
	var keys []string
	shorts := 0
	for now := C.int64_t(0); now < 30000000000; now += 10000000 {
		for {
			fill := rng.IntN(2)
			n := C.size_t(fill)
			rc := dmrgo_voice_transmitter_tick(tx, now, &out[0], C.size_t(fill+rng.IntN(C.DMRGO_VOICE_MAX_OUTPUTS+2)), &n, nil)
			if rc == C.DMRGO_ERR_SHORT {
				if int(n) != fill {
					return nil, 0, fmt.Errorf("tick at %d: SHORT moved the fill", now)
				}
				shorts++
				continue
			}
			if rc != C.DMRGO_OK {
				return nil, 0, fmt.Errorf("tick at %d: status %d", now, rc)
			}
			for j := fill; j < int(n); j++ {
				keys = append(keys, outputKeyC(&out[j]))
			}
			break
		}
	}
	return keys, shorts, nil
}
