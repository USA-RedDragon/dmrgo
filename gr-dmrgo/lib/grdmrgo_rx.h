/* SPDX-License-Identifier: GPL-3.0-or-later */
/* Copyright 2026 Jacob McSwain */

/*
 * grdmrgo_rx.h is the receiver of gr-dmrgo: an rf receiver of libdmrgo with
 * the TDMA channel, call LC, AMBE+2 frames and Codec2 audio of each burst,
 * which the rx block polls one burst at a time.
 */

#ifndef GRDMRGO_RX_H
#define GRDMRGO_RX_H

#include <stddef.h>
#include <stdint.h>

#ifdef __cplusplus
extern "C" {
#endif

#define GRDMRGO_ABI_VERSION 1
#define GRDMRGO_PCM_SAMPLES 480

enum grdmrgo_status {
	GRDMRGO_OK = 0,
	GRDMRGO_ERR_CONFIG = -1,
	GRDMRGO_ERR_INPUT = -2,
	GRDMRGO_ERR_LIBRARY = -3,
};

enum grdmrgo_input {
	GRDMRGO_INPUT_IQ = 0,
	GRDMRGO_INPUT_AUDIO = 1,
};

enum grdmrgo_kind {
	GRDMRGO_KIND_UNKNOWN = 0,
	GRDMRGO_KIND_VOICE = 1,
	GRDMRGO_KIND_DATA = 2,
};

enum grdmrgo_flag {
	GRDMRGO_FLAG_TIMED = 1 << 0,
	GRDMRGO_FLAG_BS = 1 << 1,
	GRDMRGO_FLAG_FIXED = 1 << 2,
	GRDMRGO_FLAG_LC = 1 << 3,
	GRDMRGO_FLAG_LC_DONE = 1 << 4,
	GRDMRGO_FLAG_GROUP = 1 << 5,
	GRDMRGO_FLAG_FEC_BAD = 1 << 6,
	GRDMRGO_FLAG_END = 1 << 7,
};

enum grdmrgo_codec2 {
	GRDMRGO_CODEC2_NONE = 0,
	GRDMRGO_CODEC2_2400 = 1,
	GRDMRGO_CODEC2_3200 = 2,
};

/* grdmrgo_rx_config sets up a receiver. tier is 0 to 3. input is
 * GRDMRGO_INPUT_IQ for complex float samples or GRDMRGO_INPUT_AUDIO for
 * discriminator samples, at rate Hz. codec2_mode is 2400, 3200, 0 to detect
 * the mode of each call, or -1 for no Codec2 decode. queue is the number of
 * bursts that the receiver holds for grdmrgo_rx_poll, 0 for 256. audio_scale
 * is the factor from float audio to 16-bit codes, 0 for 16384.
 * lookback_slots is passed to libdmrgo, 0 for its default. */
typedef struct grdmrgo_rx_config {
	int32_t tier;
	int32_t input;
	double rate;
	int32_t codec2_mode;
	int32_t queue;
	float audio_scale;
	int32_t lookback_slots;
} grdmrgo_rx_config;

/* grdmrgo_burst is one burst. timeslot is 1, 2, or 0 when unknown.
 * color_code, data_type and voice_pos are -1 when the burst has none.
 * bits holds the burst after FEC. ambe holds the three AMBE+2 frames of a
 * voice burst, 49 bits each from the top bit of the first byte. pcm holds
 * pcm_len samples of 8 kHz audio of a Codec2 call. */
typedef struct grdmrgo_burst {
	double seconds;
	int64_t slot;
	int32_t stream;
	uint32_t flags;
	int32_t timeslot;
	int32_t color_code;
	int32_t kind;
	int32_t data_type;
	int32_t voice_pos;
	int32_t sync_distance;
	uint64_t sync;
	int32_t fec_corrected;
	uint32_t src;
	uint32_t dst;
	uint8_t flco;
	uint8_t fid;
	uint8_t ambe_bad[3];
	uint8_t bits[33];
	uint8_t ambe[3][7];
	uint8_t codec2_mode;
	int32_t ambe_errors[3];
	int32_t pcm_len;
	int16_t pcm[GRDMRGO_PCM_SAMPLES];
} grdmrgo_burst;

typedef struct grdmrgo_rx grdmrgo_rx;

int grdmrgo_abi_version(void);
size_t grdmrgo_burst_size(void);
const char *grdmrgo_strerror(int status);
int grdmrgo_rx_new(const grdmrgo_rx_config *cfg, grdmrgo_rx **out);
int grdmrgo_rx_write_cf32(grdmrgo_rx *rx, const float *iq, size_t n);
int grdmrgo_rx_write_f32(grdmrgo_rx *rx, const float *x, size_t n);
int grdmrgo_rx_flush(grdmrgo_rx *rx);
int grdmrgo_rx_poll(grdmrgo_rx *rx, grdmrgo_burst *out);
uint64_t grdmrgo_rx_dropped(grdmrgo_rx *rx);
int grdmrgo_rx_has_codec2(grdmrgo_rx *rx);
void grdmrgo_rx_free(grdmrgo_rx *rx);

#ifdef __cplusplus
}
#endif

#endif
