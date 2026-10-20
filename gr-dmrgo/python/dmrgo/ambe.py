# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright 2026 Jacob McSwain

"""Run-time hook for a separately installed AMBE+2 decoder.

gr-dmrgo contains no AMBE+2 vocoder. When the rx block gets the path of a
shared library, it loads it at run time and decodes the AMBE+2 frames of
calls that are not Codec2 calls with it. The library must export:

    void *dmrgo_ambe_new(void);
    int dmrgo_ambe_decode(void *state, const uint8_t frame[7], int bad,
                          int16_t pcm[160]);
    void dmrgo_ambe_free(void *state);

frame holds the 49 bits of one frame, the first bit in the most significant
bit of frame[0], as dmrgo_burst.ambe does. bad is 1 when the FEC of the
frame failed. dmrgo_ambe_decode writes 20 ms of 8 kHz audio and returns 0, or
returns a negative value on error.
"""

import ctypes

FRAME_SAMPLES = 160


class Decoder:

    def __init__(self, path):
        self._lib = ctypes.CDLL(path)
        self._lib.dmrgo_ambe_new.restype = ctypes.c_void_p
        self._lib.dmrgo_ambe_decode.argtypes = [
            ctypes.c_void_p, ctypes.POINTER(ctypes.c_uint8), ctypes.c_int, ctypes.POINTER(ctypes.c_int16)]
        self._lib.dmrgo_ambe_free.argtypes = [ctypes.c_void_p]
        self._lib.dmrgo_ambe_free.restype = None
        self._state = self._lib.dmrgo_ambe_new()
        if not self._state:
            raise RuntimeError(f"{path}: dmrgo_ambe_new failed")
        self._pcm = (ctypes.c_int16 * FRAME_SAMPLES)()

    def decode(self, frame, bad):
        if self._lib.dmrgo_ambe_decode(self._state, frame, int(bad), self._pcm) < 0:
            return None
        return self._pcm

    def close(self):
        if self._state:
            self._lib.dmrgo_ambe_free(self._state)
            self._state = None

    def __del__(self):
        self.close()
