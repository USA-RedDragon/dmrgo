# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright 2026 Jacob McSwain

"""AMBE+2 decoding on a DVSI AMBE-3000R or AMBE-3003 chip.

The chip is on a serial port, such as a ThumbDV or a DVstick, or behind an
AMBEserver on UDP. The packet protocol and the codec are the vocoder/dvsi
package of dmrgo, through libdmrgo; this module opens the port and passes
its reads and writes to libdmrgo.
"""

import ctypes
import os
import select
import socket
import termios

FRAME_SAMPLES = 160
AMBESERVER_PORT = 2460
DEFAULT_BAUD = 460800
TIMEOUT_NS = 1_000_000_000
POLL_SECONDS = 0.1

BAUDS = {57600: termios.B57600, 115200: termios.B115200, 230400: termios.B230400,
         460800: termios.B460800, 921600: termios.B921600}

READ_FN = ctypes.CFUNCTYPE(ctypes.c_int, ctypes.c_void_p, ctypes.POINTER(ctypes.c_uint8), ctypes.c_size_t,
                           ctypes.POINTER(ctypes.c_int64))
WRITE_FN = READ_FN


class PortVtable(ctypes.Structure):

    _fields_ = [("size", ctypes.c_size_t), ("user", ctypes.c_void_p), ("read", READ_FN), ("write", WRITE_FN)]


class Config(ctypes.Structure):

    _fields_ = [("size", ctypes.c_size_t), ("channel", ctypes.c_int64), ("timeout", ctypes.c_int64),
                ("no_reset", ctypes.c_bool), ("no_parity", ctypes.c_bool)]


class Error(Exception):
    pass


def _bind(lib):
    p = ctypes.c_void_p
    lib.dmrgo_dvsi_port_from_vtable.argtypes = [ctypes.POINTER(PortVtable), ctypes.POINTER(p)]
    lib.dmrgo_dvsi_port_free.argtypes = [p]
    lib.dmrgo_dvsi_new.argtypes = [p, ctypes.POINTER(Config), ctypes.POINTER(p)]
    lib.dmrgo_dvsi_codec_decode.argtypes = [p, ctypes.POINTER(ctypes.c_int16), ctypes.c_size_t,
                                            ctypes.POINTER(ctypes.c_size_t), ctypes.POINTER(ctypes.c_size_t),
                                            ctypes.POINTER(ctypes.c_uint8 * 9), ctypes.c_void_p]
    lib.dmrgo_dvsi_codec_product.argtypes = [p, ctypes.c_char_p, ctypes.c_size_t, ctypes.POINTER(ctypes.c_size_t),
                                             ctypes.POINTER(ctypes.c_size_t)]
    lib.dmrgo_dvsi_codec_free.argtypes = [p]
    lib.dmrgo_layer2_raw_burst_voice_slots.argtypes = [ctypes.POINTER(ctypes.c_uint8 * 33),
                                                       ctypes.POINTER((ctypes.c_uint8 * 9) * 3)]


class SerialPort:

    def __init__(self, path, baud=DEFAULT_BAUD):
        if baud not in BAUDS:
            raise Error(f"baud {baud}, want one of {sorted(BAUDS)}")
        self._fd = os.open(path, os.O_RDWR | os.O_NOCTTY)
        try:
            attr = termios.tcgetattr(self._fd)
            attr[0:6] = [0, 0, termios.CS8 | termios.CREAD | termios.CLOCAL | termios.CRTSCTS, 0,
                         BAUDS[baud], BAUDS[baud]]
            attr[6][termios.VMIN] = 0
            attr[6][termios.VTIME] = 1
            termios.tcsetattr(self._fd, termios.TCSANOW, attr)
        except termios.error:
            os.close(self._fd)
            raise

    def read(self, n):
        if not select.select([self._fd], [], [], POLL_SECONDS)[0]:
            return b""
        return os.read(self._fd, n)

    def write(self, b):
        return os.write(self._fd, b)

    def close(self):
        if self._fd >= 0:
            os.close(self._fd)
            self._fd = -1


class UDPPort:

    def __init__(self, addr):
        host, _, port = addr.rpartition(":")
        if not host or not port.isdigit():
            host, port = addr, AMBESERVER_PORT
        self._sock = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
        self._sock.settimeout(POLL_SECONDS)
        self._sock.connect((host.strip("[]"), int(port)))

    def read(self, n):
        try:
            return self._sock.recv(n)
        except (socket.timeout, BlockingIOError):
            return b""

    def write(self, b):
        return self._sock.send(b)

    def close(self):
        self._sock.close()


class Device:
    """A DVSI chip on port, with a codec on each channel that decode uses."""

    def __init__(self, lib, port):
        _bind(lib)
        self._lib = lib
        self._io = port
        self._read = READ_FN(self._on_read)
        self._write = WRITE_FN(self._on_write)
        self._vt = PortVtable(ctypes.sizeof(PortVtable), None, self._read, self._write)
        self._port = ctypes.c_void_p()
        self._check(lib.dmrgo_dvsi_port_from_vtable(ctypes.byref(self._vt), ctypes.byref(self._port)))
        self._codecs = {}
        self._codec(0)
        name = ctypes.create_string_buffer(17)
        n = ctypes.c_size_t()
        need = ctypes.c_size_t()
        self._check(lib.dmrgo_dvsi_codec_product(self._codecs[0], name, len(name), ctypes.byref(n), ctypes.byref(need)))
        self.product = name.value.decode()
        self.channels = 3 if self.product.startswith("AMBE3003") else 1
        self._pcm = (ctypes.c_int16 * FRAME_SAMPLES)()
        self._slot = (ctypes.c_uint8 * 9)()

    def _check(self, status):
        if status < 0:
            raise Error(self._lib.dmrgo_strerror(status).decode())

    def _on_read(self, _user, p, n, out):
        try:
            b = self._io.read(n)
        except OSError:
            return -1
        ctypes.memmove(p, b, len(b))
        out[0] = len(b)
        return 0

    def _on_write(self, _user, p, n, out):
        try:
            out[0] = self._io.write(ctypes.string_at(p, n))
        except OSError:
            return -1
        return 0

    def _codec(self, channel):
        if channel not in self._codecs:
            c = ctypes.c_void_p()
            cfg = Config(size=ctypes.sizeof(Config), channel=channel, timeout=TIMEOUT_NS, no_reset=bool(self._codecs))
            self._check(self._lib.dmrgo_dvsi_new(self._port, ctypes.byref(cfg), ctypes.byref(c)))
            self._codecs[channel] = c
        return self._codecs[channel]

    def decode(self, slot, channel=0):
        """Decode the 72 on-air bits of a vocoder slot, 9 bytes, into 160
        samples, or return None when the chip fails."""
        c = self._codec(channel % self.channels)
        ctypes.memmove(self._slot, bytes(slot), 9)
        n, need = ctypes.c_size_t(), ctypes.c_size_t()
        if self._lib.dmrgo_dvsi_codec_decode(c, self._pcm, FRAME_SAMPLES, ctypes.byref(n), ctypes.byref(need),
                                             ctypes.byref(self._slot), None) < 0 or n.value != FRAME_SAMPLES:
            return None
        return self._pcm

    def close(self):
        for c in self._codecs.values():
            self._lib.dmrgo_dvsi_codec_free(c)
        self._codecs = {}
        if self._port:
            self._lib.dmrgo_dvsi_port_free(self._port)
            self._port = ctypes.c_void_p()
        self._io.close()


def voice_slots(lib, bits):
    """Return the three 9-byte vocoder slots of a 33-byte voice burst."""
    _bind(lib)
    raw = (ctypes.c_uint8 * 33)(*bits)
    out = ((ctypes.c_uint8 * 9) * 3)()
    if lib.dmrgo_layer2_raw_burst_voice_slots(ctypes.byref(raw), ctypes.byref(out)) < 0:
        raise Error("dmrgo_layer2_raw_burst_voice_slots failed")
    return [bytes(s) for s in out]
