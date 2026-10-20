#!/usr/bin/env python3
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright 2026 Jacob McSwain

import os
import pty
import socket
import threading
import tty

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


class FakeChip:

    def __init__(self):
        self.parity = True
        self.slots = []
        self.errors = []
        self._in = b""

    @staticmethod
    def echo(slot):
        return [slot[j % 9] * 16 - 2048 + j for j in range(160)]

    def _packet(self, typ, fields):
        n = len(fields) + (2 if self.parity else 0)
        b = bytes([0x61, n >> 8, n & 0xFF, typ]) + fields
        if self.parity:
            x = 0
            for c in b[1:] + b"\x2f":
                x ^= c
            b += bytes([0x2F, x])
        return b

    def feed(self, data):
        self._in += data
        out = []
        while True:
            i = self._in.find(b"\x61")
            if i < 0:
                self._in = b""
                return out
            self._in = self._in[i:]
            if len(self._in) < 4:
                return out
            n = self._in[1] << 8 | self._in[2]
            if len(self._in) < 4 + n:
                return out
            pkt, self._in = self._in[:4 + n], self._in[4 + n:]
            fields = pkt[4:]
            if self.parity:
                x = 0
                for c in pkt[1:-1]:
                    x ^= c
                if fields[-2:-1] != b"\x2f" or fields[-1] != x:
                    self.errors.append("parity")
                    continue
                fields = fields[:-2]
            out.append(self._reply(pkt[3], fields))

    def _reply(self, typ, f):
        if typ == 0 and f[:1] == b"\x33":
            self.parity = True
            return self._packet(0, b"\x39")
        if typ == 0 and f[:1] == b"\x3f":
            self.parity = f[1] == 1
            return self._packet(0, b"\x3f\x00")
        if typ == 0 and f[:1] == b"\x30":
            return self._packet(0, b"\x30AMBE3000R\x00\x31V120.FAKE\x00")
        if typ == 0 and f[:1] == b"\x0a":
            if f[1:13] != bytes.fromhex("043107542400000000006f48"):
                self.errors.append("rate")
            return self._packet(0, b"\x0a\x00\x0b\x00")
        if typ == 1 and f[:2] == b"\x01\x48" and len(f) == 11:
            self.slots.append(f[2:])
            pcm = b"".join(int(v).to_bytes(2, "big", signed=True) for v in self.echo(f[2:]))
            return self._packet(2, b"\x00\xa0" + pcm)
        self.errors.append(f"packet {typ} {f.hex()}")
        return b""


def serve_udp(chip):
    sock = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
    sock.bind(("127.0.0.1", 0))

    def loop():
        while True:
            try:
                data, peer = sock.recvfrom(2048)
            except OSError:
                return
            for r in chip.feed(data):
                if r:
                    sock.sendto(r, peer)

    threading.Thread(target=loop, daemon=True).start()
    return sock


def serve_pty(chip):
    master, slave = pty.openpty()
    tty.setraw(slave)
    path = os.ttyname(slave)

    def loop():
        while True:
            try:
                data = os.read(master, 512)
            except OSError:
                return
            for r in chip.feed(data):
                os.write(master, r)

    threading.Thread(target=loop, daemon=True).start()
    return master, slave, path


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

    def run_ambe(self, **backend):
        u8 = np.fromfile(WEAK, dtype=np.uint8).astype(np.float32)
        iq = (u8[0::2] - 127.5) / 128 + 1j * (u8[1::2] - 127.5) / 128
        iq = np.concatenate([iq, np.zeros(256000)]).astype(np.complex64)
        src = blocks.vector_source_c(iq.tolist(), False)
        resamp = filter.rational_resampler_ccc(interpolation=3, decimation=16)
        self.tb.connect(src, resamp)
        rx = dmrgo.rx(input_type="complex", samp_rate=48000, tier=2, codec2_mode=-1, **backend)
        sink = blocks.vector_sink_f()
        self.tb.connect(rx, sink)
        _, voice = self.run_rx(resamp, rx)
        self.assertEqual(len(voice), 18)
        self.assertEqual(rx.ambe_device(), "AMBE3000R")
        return np.array(sink.data(), dtype=np.float32)

    def check_echo(self, chip, audio):
        self.assertEqual(chip.errors, [])
        self.assertEqual(len(chip.slots), 18)
        want = np.concatenate([FakeChip.echo(s) for s in chip.slots]).astype(np.float32) / 32768
        start = np.flatnonzero(audio)[0] - np.flatnonzero(want)[0]
        got = audio[start:start + len(want)]
        self.assertEqual(len(got), len(want))
        self.assertTrue(np.array_equal(got, want), "the audio is not the decoded frames in order")

    def test_ambeserver(self):
        chip = FakeChip()
        sock = serve_udp(chip)
        try:
            audio = self.run_ambe(ambe_backend="ambeserver", ambeserver="127.0.0.1:%d" % sock.getsockname()[1])
        finally:
            sock.close()
        self.check_echo(chip, audio)

    def test_serial(self):
        chip = FakeChip()
        master, slave, path = serve_pty(chip)
        try:
            audio = self.run_ambe(ambe_backend="serial", dvsi_device=path, dvsi_baud=921600)
        finally:
            os.close(slave)
            os.close(master)
        self.check_echo(chip, audio)

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
        bursts, voice = self.run_rx(src, rx)

        modes = [pmt.to_long(pmt.dict_ref(pmt.car(m), pmt.intern("codec2_mode"), pmt.PMT_NIL))
                 for m in bursts if pmt.symbol_to_string(pmt.dict_ref(pmt.car(m), pmt.intern("kind"), pmt.PMT_NIL)) == "voice"]
        self.assertEqual(modes, [0] + [2400] * 17)
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
