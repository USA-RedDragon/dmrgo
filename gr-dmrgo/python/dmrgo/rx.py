# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright 2026 Jacob McSwain

"""The dmrgo DMR receiver as a GNU Radio block."""

import numpy as np
import pmt
from gnuradio import gr

from . import ambe, dvsi, libdmrgo

AUDIO_RATE = 8000
AUDIO_DELAY = 0.3
KINDS = {libdmrgo.KIND_UNKNOWN: "unknown", libdmrgo.KIND_VOICE: "voice", libdmrgo.KIND_DATA: "data"}
CODEC2_RATES = {libdmrgo.CODEC2_2400: 2400, libdmrgo.CODEC2_3200: 3200}


class rx(gr.basic_block):
    """DMR receiver of dmrgo.

    The input is complex baseband at samp_rate, 48000 or with tier 0 or 1
    24000, or with input_type "float" the output of an FM discriminator at
    24000, 12000 or 9600. Put a resampler before the block for other rates.

    The "bursts" message port sends one PDU per burst: the 33 burst bytes
    after FEC, and a dict with seconds, stream, slot, timeslot (1, 2 or 0
    when unknown), color_code (-1 when none), kind ("voice", "data" or
    "unknown"), data_type, voice_pos (0 to 5 for A to F, or -1),
    sync_distance, fec_corrected, fec_bad, flags, codec2_mode (2400 or 3200
    for a voice burst of a Codec2 call once the mode is known, else 0), and
    for a burst of a call with a known LC src, dst, flco, fid and group.

    The "voice" message port sends one PDU per AMBE+2 frame of a voice
    burst: the 7 frame bytes, and a dict with timeslot, voice_pos, index (0
    to 2), bad, errors, and the call LC fields when known.

    The optional stream output is 8 kHz float audio in -1 to 1: the audio of
    Codec2 calls (FID 0xC2), and of other calls with an AMBE+2 backend. It is
    silence between calls. ambe_backend selects the AMBE+2 decoder:
    "library" for the separately installed library that ambe_lib names (see
    gnuradio.dmrgo.ambe), none when ambe_lib is empty; "serial" for a DVSI
    AMBE-3000R or AMBE-3003 on the serial port dvsi_device at dvsi_baud,
    such as a ThumbDV or a DVstick; "ambeserver" for an AMBEserver at
    ambeserver, "host" or "host:port" (default port 2460). An AMBE-3003
    decodes each timeslot on its own channel (see gnuradio.dmrgo.dvsi). It
    plays the audio of each burst 0.3 s after the burst in the input, so
    that the receiver has the time to decode it.
    audio_slot selects the timeslot, 1 or 2, or 0 for both, which the block
    adds. codec2_mode is the Codec2 bit rate, 2400 or 3200, or 0 to detect
    it for each call, or -1 to turn the Codec2 decode off.
    """

    def __init__(self, input_type="complex", samp_rate=48000, tier=2, codec2_mode=0,
                 audio_slot=0, lib_path="", ambe_lib="", ambe_backend="library", dvsi_device="",
                 dvsi_baud=dvsi.DEFAULT_BAUD, ambeserver=""):
        iq = input_type == "complex"
        gr.basic_block.__init__(
            self, name="dmrgo_rx",
            in_sig=[np.complex64 if iq else np.float32],
            out_sig=gr.py_io_signature(0, 1, [np.float32]))
        self._lib = libdmrgo.load(lib_path)
        self._rx = libdmrgo.Receiver(
            self._lib, tier=int(tier), input_kind=libdmrgo.INPUT_IQ if iq else libdmrgo.INPUT_AUDIO,
            rate=float(samp_rate), codec2_mode=int(codec2_mode))
        self._rate = float(samp_rate)
        self._audio_slot = int(audio_slot)
        self._ambe_lib = ambe_lib
        self._ambe = {}
        self._dvsi = None
        if ambe_backend == "serial":
            self._dvsi = dvsi.Device(self._lib, dvsi.SerialPort(dvsi_device, int(dvsi_baud)))
        elif ambe_backend == "ambeserver":
            self._dvsi = dvsi.Device(self._lib, dvsi.UDPPort(ambeserver))
        elif ambe_backend != "library":
            raise ValueError(f"ambe_backend {ambe_backend!r}, want library, serial or ambeserver")
        self._ring = np.zeros(4 * AUDIO_RATE, dtype=np.float32)
        self._in_total = 0
        self._out_total = 0
        self.message_port_register_out(pmt.intern("bursts"))
        self.message_port_register_out(pmt.intern("voice"))

    def ambe_device(self):
        """Return the product name of the DVSI chip, or "" without one."""
        return self._dvsi.product if self._dvsi else ""

    def has_codec2(self):
        """Report whether the library decodes Codec2 audio."""
        return self._rx.has_codec2()

    def dropped(self):
        """Return the number of bursts that a full queue lost."""
        return self._rx.dropped()

    def forecast(self, noutput_items, ninputs):
        return [max(1, int(noutput_items * self._rate / AUDIO_RATE))] * ninputs

    def general_work(self, input_items, output_items):
        x = input_items[0]
        n = len(x)
        if output_items:
            n = min(n, int(len(output_items[0]) * self._rate / AUDIO_RATE))
        if n > 0:
            self._rx.write(x[:n])
            self.consume(0, n)
            self._in_total += n
        self._drain()
        if not output_items:
            return 0
        return self._play(output_items[0])

    def stop(self):
        self._rx.flush()
        self._drain()
        return True

    def _play(self, out):
        due = min(len(out), int(self._in_total * AUDIO_RATE / self._rate) - self._out_total)
        if due <= 0:
            return 0
        i = self._out_total % len(self._ring)
        k = min(due, len(self._ring) - i)
        out[:k] = self._ring[i:i + k]
        out[k:due] = self._ring[:due - k]
        self._ring[i:i + k] = 0
        self._ring[:due - k] = 0
        self._out_total += due
        return due

    def _queue_audio(self, seconds, pcm):
        start = round((seconds + AUDIO_DELAY) * AUDIO_RATE)
        skip = max(0, self._out_total - start)
        pcm = np.asarray(pcm[skip:], dtype=np.float32) / 32768
        start += skip
        if len(pcm) == 0 or start + len(pcm) > self._out_total + len(self._ring):
            return
        i = start % len(self._ring)
        k = min(len(pcm), len(self._ring) - i)
        self._ring[i:i + k] += pcm[:k]
        self._ring[:len(pcm) - k] += pcm[k:]

    def _ambe_decoder(self, ts):
        if self._dvsi:
            return _DVSIDecoder(self._lib, self._dvsi, ts)
        if not self._ambe_lib:
            return None
        if ts not in self._ambe:
            self._ambe[ts] = _LibraryDecoder(ambe.Decoder(self._ambe_lib))
        return self._ambe[ts]

    def _drain(self):
        while (b := self._rx.poll()) is not None:
            self._publish(b)

    def _publish(self, b):
        meta = pmt.make_dict()
        lc = []
        if b.flags & libdmrgo.FLAG_LC:
            lc = [("src", pmt.from_long(b.src)), ("dst", pmt.from_long(b.dst)), ("flco", pmt.from_long(b.flco)),
                  ("fid", pmt.from_long(b.fid)), ("group", pmt.from_bool(bool(b.flags & libdmrgo.FLAG_GROUP)))]
        fields = [
            ("seconds", pmt.from_double(b.seconds)), ("stream", pmt.from_long(b.stream)),
            ("slot", pmt.from_long(b.slot)), ("timeslot", pmt.from_long(b.timeslot)),
            ("color_code", pmt.from_long(b.color_code)), ("kind", pmt.intern(KINDS.get(b.kind, "unknown"))),
            ("data_type", pmt.from_long(b.data_type)), ("voice_pos", pmt.from_long(b.voice_pos)),
            ("sync_distance", pmt.from_long(b.sync_distance)), ("fec_corrected", pmt.from_long(b.fec_corrected)),
            ("fec_bad", pmt.from_bool(bool(b.flags & libdmrgo.FLAG_FEC_BAD))), ("flags", pmt.from_long(b.flags)),
            ("codec2_mode", pmt.from_long(CODEC2_RATES.get(b.codec2_mode, 0))),
        ] + lc
        for k, v in fields:
            meta = pmt.dict_add(meta, pmt.intern(k), v)
        self.message_port_pub(pmt.intern("bursts"), pmt.cons(meta, pmt.init_u8vector(33, list(b.bits))))
        if b.kind != libdmrgo.KIND_VOICE:
            return

        play = self._audio_slot in (0, b.timeslot)
        dec = None
        if play and b.pcm_len == 0 and b.flags & libdmrgo.FLAG_LC:
            dec = self._ambe_decoder(b.timeslot)
        for i in range(3):
            v = pmt.make_dict()
            for k, val in [("timeslot", pmt.from_long(b.timeslot)), ("voice_pos", pmt.from_long(b.voice_pos)),
                           ("index", pmt.from_long(i)), ("bad", pmt.from_bool(bool(b.ambe_bad[i]))),
                           ("errors", pmt.from_long(b.ambe_errors[i]))] + lc:
                v = pmt.dict_add(v, pmt.intern(k), val)
            self.message_port_pub(pmt.intern("voice"), pmt.cons(v, pmt.init_u8vector(7, list(b.ambe[i]))))
            if dec is not None and (pcm := dec.decode(b, i)) is not None:
                self._queue_audio(b.seconds + i * ambe.FRAME_SAMPLES / AUDIO_RATE, np.ctypeslib.as_array(pcm))
        if play and b.pcm_len:
            self._queue_audio(b.seconds, np.ctypeslib.as_array(b.pcm)[:b.pcm_len])


class _LibraryDecoder:

    def __init__(self, dec):
        self._dec = dec

    def decode(self, b, i):
        return self._dec.decode(b.ambe[i], b.ambe_bad[i])


class _DVSIDecoder:

    def __init__(self, lib, dev, ts):
        self._lib, self._dev, self._ts = lib, dev, ts
        self._slots = None

    def decode(self, b, i):
        if i == 0 or self._slots is None:
            self._slots = dvsi.voice_slots(self._lib, b.bits)
        return self._dev.decode(self._slots[i], max(self._ts - 1, 0))
