#!/usr/bin/env python3
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright 2026 Jacob McSwain

import os

import numpy as np
import pmt
from gnuradio import blocks, filter, gr, gr_unittest

try:
    from gnuradio import dmrgo
except ImportError:
    import sys
    sys.path.insert(0, os.path.join(os.path.dirname(__file__), ".."))
    import dmrgo

TESTDATA = os.environ.get("DMRGO_TESTDATA", os.path.join(os.path.dirname(__file__), "..", "..", "..", "v2"))
WEAK = os.path.join(TESTDATA, "cmd", "dmrrf", "testdata", "xpr8400_bs_voice_weak.cu8")
CODEC2 = os.path.join(TESTDATA, "capi", "testdata", "codec2_call.cu8")


def cu8_source(tb, path):
    src = blocks.file_source(gr.sizeof_char, path, False)
    to_f = blocks.uchar_to_float()
    shift = blocks.add_const_ff(-127.5)
    scale = blocks.multiply_const_ff(1 / 128)
    deint = blocks.deinterleave(gr.sizeof_float)
    to_c = blocks.float_to_complex()
    tb.connect(src, to_f, shift, scale, deint)
    tb.connect((deint, 0), (to_c, 0))
    tb.connect((deint, 1), (to_c, 1))
    return to_c


def pitch_lag(x):
    best, lag = -np.inf, 0
    for k in range(20, 160):
        r = float(np.dot(x[k:], x[:-k]))
        if r > best:
            best, lag = r, k
    return lag


class qa_rx(gr_unittest.TestCase):

    def setUp(self):
        self.tb = gr.top_block()

    def tearDown(self):
        self.tb = None

    def run_rx(self, src, rx):
        bursts = blocks.message_debug()
        voice = blocks.message_debug()
        self.tb.connect(src, rx)
        self.tb.msg_connect(rx, "bursts", bursts, "store")
        self.tb.msg_connect(rx, "voice", voice, "store")
        self.tb.run()
        return ([bursts.get_message(i) for i in range(bursts.num_messages())],
                [voice.get_message(i) for i in range(voice.num_messages())])

    def test_weak_capture(self):
        src = cu8_source(self.tb, WEAK)
        resamp = filter.rational_resampler_ccc(interpolation=3, decimation=16)
        self.tb.connect(src, resamp)
        rx = dmrgo.rx(input_type="complex", samp_rate=48000, tier=2, codec2_mode=-1)
        bursts, voice = self.run_rx(resamp, rx)

        self.assertEqual(len(voice), 18)
        for i, m in enumerate(voice):
            meta, frame = pmt.car(m), pmt.cdr(m)
            self.assertEqual(pmt.length(frame), 7)
            self.assertEqual(pmt.to_long(pmt.dict_ref(meta, pmt.intern("timeslot"), pmt.PMT_NIL)), 2)
            self.assertEqual(pmt.to_long(pmt.dict_ref(meta, pmt.intern("index"), pmt.PMT_NIL)), i % 3)
            self.assertFalse(pmt.to_bool(pmt.dict_ref(meta, pmt.intern("bad"), pmt.PMT_NIL)))
            self.assertEqual(pmt.to_long(pmt.dict_ref(meta, pmt.intern("src"), pmt.PMT_NIL)), 3191868)
            self.assertEqual(pmt.to_long(pmt.dict_ref(meta, pmt.intern("dst"), pmt.PMT_NIL)), 9)
        headers = 0
        for m in bursts:
            meta = pmt.car(m)
            self.assertEqual(pmt.length(pmt.cdr(m)), 33)
            kind = pmt.symbol_to_string(pmt.dict_ref(meta, pmt.intern("kind"), pmt.PMT_NIL))
            if kind == "data":
                self.assertEqual(pmt.to_long(pmt.dict_ref(meta, pmt.intern("color_code"), pmt.PMT_NIL)), 7)
                if pmt.to_long(pmt.dict_ref(meta, pmt.intern("data_type"), pmt.PMT_NIL)) == 1:
                    headers += 1
        self.assertEqual(headers, 3)

    def test_codec2_call(self):
        u8 = np.fromfile(CODEC2, dtype=np.uint8).astype(np.float32)
        iq = (u8[0::2] - 127.5) / 128 + 1j * (u8[1::2] - 127.5) / 128
        iq = np.concatenate([iq, np.zeros(24000)]).astype(np.complex64)
        src = blocks.vector_source_c(iq.tolist(), False)
        rx = dmrgo.rx(input_type="complex", samp_rate=48000, tier=1)
        if not rx.has_codec2():
            self.skipTest("libdmrgo built without libcodec2")
        sink = blocks.vector_sink_f()
        self.tb.connect(rx, sink)
        _, voice = self.run_rx(src, rx)

        self.assertEqual(len(voice), 54)
        for m in voice:
            fid = pmt.to_long(pmt.dict_ref(pmt.car(m), pmt.intern("fid"), pmt.PMT_NIL))
            self.assertEqual(fid, 0xC2)
        audio = np.array(sink.data(), dtype=np.float32)
        self.assertAlmostEqual(len(audio), len(iq) * 8000 / 48000, delta=2)
        voiced = np.flatnonzero(audio)
        self.assertGreaterEqual(len(voiced), 54 * 160 - 20)
        self.assertLessEqual(voiced[-1] - voiced[0], 54 * 160 + 20)
        seg = audio[voiced[0] + 800:voiced[-1]]
        lag = pitch_lag(seg)
        self.assertTrue(50 <= lag <= 57, f"pitch lag {lag} samples, want about 53 (150 Hz)")


if __name__ == "__main__":
    gr_unittest.run(qa_rx)
