# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright 2026 Jacob McSwain

"""GNU Radio blocks of dmrgo, a DMR library: a receiver of DMR bursts,
voice frames and Codec2 audio."""

from .rx import rx

__all__ = ["rx"]
