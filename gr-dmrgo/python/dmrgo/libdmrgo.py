# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright 2026 Jacob McSwain

"""ctypes binding of the gr-dmrgo receiver (lib/grdmrgo_rx.h) and libdmrgo,
the C API of dmrgo (dmrgo.h)."""

import ctypes
import ctypes.util
import os

ABI_VERSION = 1
PCM_SAMPLES = 480

INPUT_IQ = 0
INPUT_AUDIO = 1

KIND_UNKNOWN = 0
KIND_VOICE = 1
KIND_DATA = 2

FLAG_TIMED = 1 << 0
FLAG_BS = 1 << 1
FLAG_FIXED = 1 << 2
FLAG_LC = 1 << 3
FLAG_LC_DONE = 1 << 4
FLAG_GROUP = 1 << 5
FLAG_FEC_BAD = 1 << 6
FLAG_END = 1 << 7

CODEC2_NONE = 0
CODEC2_2400 = 1
CODEC2_3200 = 2


class RxConfig(ctypes.Structure):

    _fields_ = [
        ("tier", ctypes.c_int32),
        ("input", ctypes.c_int32),
        ("rate", ctypes.c_double),
        ("codec2_mode", ctypes.c_int32),
        ("queue", ctypes.c_int32),
        ("audio_scale", ctypes.c_float),
        ("lookback_slots", ctypes.c_int32),
    ]


class Burst(ctypes.Structure):

    _fields_ = [
        ("seconds", ctypes.c_double),
        ("slot", ctypes.c_int64),
        ("stream", ctypes.c_int32),
        ("flags", ctypes.c_uint32),
        ("timeslot", ctypes.c_int32),
        ("color_code", ctypes.c_int32),
        ("kind", ctypes.c_int32),
        ("data_type", ctypes.c_int32),
        ("voice_pos", ctypes.c_int32),
        ("sync_distance", ctypes.c_int32),
        ("sync", ctypes.c_uint64),
        ("fec_corrected", ctypes.c_int32),
        ("src", ctypes.c_uint32),
        ("dst", ctypes.c_uint32),
        ("flco", ctypes.c_uint8),
        ("fid", ctypes.c_uint8),
        ("ambe_bad", ctypes.c_uint8 * 3),
        ("bits", ctypes.c_uint8 * 33),
        ("ambe", (ctypes.c_uint8 * 7) * 3),
        ("codec2_mode", ctypes.c_uint8),
        ("ambe_errors", ctypes.c_int32 * 3),
        ("pcm_len", ctypes.c_int32),
        ("pcm", ctypes.c_int16 * PCM_SAMPLES),
    ]


class Error(Exception):
    pass


def find_library(path=""):
    for p in (path, os.environ.get("DMRGO_LIB", ""), ctypes.util.find_library("dmrgo")):
        if p:
            return p
    return "libdmrgo.so"


def find_rx_library():
    p = os.environ.get("DMRGO_RX_LIB", "")
    if p:
        return p
    here = os.path.join(os.path.dirname(os.path.abspath(__file__)), "libgrdmrgo_rx.so")
    return here if os.path.exists(here) else "libgrdmrgo_rx.so"


def load(path=""):
    ctypes.CDLL(find_library(path), mode=ctypes.RTLD_GLOBAL)
    lib = ctypes.CDLL(find_rx_library())
    rx = ctypes.c_void_p
    lib.grdmrgo_abi_version.restype = ctypes.c_int
    lib.grdmrgo_burst_size.restype = ctypes.c_size_t
    lib.grdmrgo_strerror.restype = ctypes.c_char_p
    lib.grdmrgo_strerror.argtypes = [ctypes.c_int]
    lib.dmrgo_strerror.restype = ctypes.c_char_p
    lib.dmrgo_strerror.argtypes = [ctypes.c_int]
    lib.grdmrgo_rx_new.argtypes = [ctypes.POINTER(RxConfig), ctypes.POINTER(rx)]
    lib.grdmrgo_rx_write_cf32.argtypes = [rx, ctypes.c_void_p, ctypes.c_size_t]
    lib.grdmrgo_rx_write_f32.argtypes = [rx, ctypes.c_void_p, ctypes.c_size_t]
    lib.grdmrgo_rx_flush.argtypes = [rx]
    lib.grdmrgo_rx_poll.argtypes = [rx, ctypes.POINTER(Burst)]
    lib.grdmrgo_rx_dropped.argtypes = [rx]
    lib.grdmrgo_rx_dropped.restype = ctypes.c_uint64
    lib.grdmrgo_rx_has_codec2.argtypes = [rx]
    lib.grdmrgo_rx_free.argtypes = [rx]
    lib.grdmrgo_rx_free.restype = None
    if lib.grdmrgo_abi_version() != ABI_VERSION:
        raise Error(f"gr-dmrgo receiver ABI {lib.grdmrgo_abi_version()}, want {ABI_VERSION}")
    if lib.grdmrgo_burst_size() != ctypes.sizeof(Burst):
        raise Error(f"gr-dmrgo burst size {lib.grdmrgo_burst_size()}, want {ctypes.sizeof(Burst)}")
    return lib


class Receiver:

    def __init__(self, lib, tier=2, input_kind=INPUT_IQ, rate=0, codec2_mode=0, queue=0):
        self._lib = lib
        self._rx = ctypes.c_void_p()
        self._burst = Burst()
        cfg = RxConfig(tier=tier, input=input_kind, rate=rate, codec2_mode=codec2_mode, queue=queue)
        self._check(lib.grdmrgo_rx_new(ctypes.byref(cfg), ctypes.byref(self._rx)))
        self._write = lib.grdmrgo_rx_write_cf32 if input_kind == INPUT_IQ else lib.grdmrgo_rx_write_f32

    def _check(self, status):
        if status < 0:
            raise Error(self._lib.grdmrgo_strerror(status).decode())
        return status

    def write(self, samples):
        self._check(self._write(self._rx, samples.ctypes.data, len(samples)))

    def flush(self):
        self._check(self._lib.grdmrgo_rx_flush(self._rx))

    def poll(self):
        if self._check(self._lib.grdmrgo_rx_poll(self._rx, ctypes.byref(self._burst))) == 0:
            return None
        return self._burst

    def dropped(self):
        return self._lib.grdmrgo_rx_dropped(self._rx)

    def has_codec2(self):
        return self._lib.grdmrgo_rx_has_codec2(self._rx) == 1

    def close(self):
        if self._rx:
            self._lib.grdmrgo_rx_free(self._rx)
            self._rx = ctypes.c_void_p()

    def __del__(self):
        self.close()
