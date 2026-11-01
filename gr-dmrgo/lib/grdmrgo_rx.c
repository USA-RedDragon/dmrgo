/* SPDX-License-Identifier: GPL-3.0-or-later */
/* Copyright 2026 Jacob McSwain */

#include <math.h>
#include <stdbool.h>
#include <stdlib.h>
#include <string.h>

#include "dmrgo.h"
#include "grdmrgo_rx.h"

#define DEFAULT_QUEUE 256
#define DEFAULT_AUDIO_SCALE 16384.0f
#define IQ_CHUNK 16384
#define AUDIO_CHUNK 4096

struct channel {
	dmrgo_codec2_codec_t *c2;
};

struct grdmrgo_rx {
	grdmrgo_rx_config cfg;
	dmrgo_rf_receiver_t *iq;
	dmrgo_rf_audio_receiver_t *audio;
	dmrgo_rf_monitor_t *mon;
	dmrgo_layer2_burst_t *l2;
	struct channel chans[3];
	bool has_c2;
	dmrgo_rf_burst_t *bursts;
	size_t bursts_cap;
	int16_t s16[AUDIO_CHUNK];
	grdmrgo_burst *queue;
	int head, n;
	uint64_t dropped;
};

int grdmrgo_abi_version(void) { return GRDMRGO_ABI_VERSION; }

size_t grdmrgo_burst_size(void) { return sizeof(grdmrgo_burst); }

const char *grdmrgo_strerror(int status)
{
	switch (status) {
	case GRDMRGO_OK:
		return "ok";
	case GRDMRGO_ERR_CONFIG:
		return "gr-dmrgo: config out of range";
	case GRDMRGO_ERR_INPUT:
		return "gr-dmrgo: write does not match the receiver input";
	case GRDMRGO_ERR_LIBRARY:
		return "gr-dmrgo: libdmrgo failed";
	}
	return "gr-dmrgo: unknown status";
}

static dmrgo_rf_tier_t tier_of(int32_t t)
{
	switch (t) {
	case 0:
		return DMRGO_RF_TIER0;
	case 1:
		return DMRGO_RF_TIER1;
	case 2:
		return DMRGO_RF_TIER2;
	case 3:
		return DMRGO_RF_TIER3;
	}
	return DMRGO_RF_TIER_DEFAULT;
}

static int new_codec2(grdmrgo_rx *r, int32_t mode)
{
	if (mode == -1)
		return GRDMRGO_OK;
	if (mode != 0 && mode != 2400 && mode != 3200)
		return GRDMRGO_ERR_CONFIG;
	for (int i = 0; i < 3; i++) {
		dmrgo_codec2_config_t c = {.size = sizeof c, .mode = (dmrgo_codec2_mode_t)mode};
		if (dmrgo_codec2_new(&c, &r->chans[i].c2) != DMRGO_OK)
			return GRDMRGO_OK;
	}
	r->has_c2 = true;
	return GRDMRGO_OK;
}

int grdmrgo_rx_new(const grdmrgo_rx_config *cfg, grdmrgo_rx **out)
{
	if (cfg == NULL || out == NULL)
		return GRDMRGO_ERR_CONFIG;
	*out = NULL;
	dmrgo_rf_tier_t tier = tier_of(cfg->tier);
	if (tier == DMRGO_RF_TIER_DEFAULT || cfg->queue < 0 || cfg->lookback_slots < 0 || !(cfg->rate >= 0) ||
	    cfg->rate != floor(cfg->rate))
		return GRDMRGO_ERR_CONFIG;
	grdmrgo_rx *r = calloc(1, sizeof *r);
	if (r == NULL)
		return GRDMRGO_ERR_LIBRARY;
	r->cfg = *cfg;
	if (r->cfg.queue == 0)
		r->cfg.queue = DEFAULT_QUEUE;
	if (r->cfg.audio_scale == 0)
		r->cfg.audio_scale = DEFAULT_AUDIO_SCALE;
	int st = GRDMRGO_ERR_CONFIG;
	r->queue = calloc((size_t)r->cfg.queue, sizeof *r->queue);
	if (r->queue == NULL || dmrgo_rf_monitor_new(&r->mon) != DMRGO_OK || dmrgo_layer2_new_burst(&r->l2) != DMRGO_OK) {
		st = GRDMRGO_ERR_LIBRARY;
		goto fail;
	}
	switch (cfg->input) {
	case GRDMRGO_INPUT_IQ: {
		dmrgo_rf_rx_config_t c = {.size = sizeof c, .tier = tier, .rate = (int64_t)(cfg->rate == 0 ? 48000 : cfg->rate),
					  .format = DMRGO_RF_IQ_FORMAT_CF32, .lookback_slots = cfg->lookback_slots};
		if (dmrgo_rf_new_receiver(&c, &r->iq) != DMRGO_OK)
			goto fail;
		break;
	}
	case GRDMRGO_INPUT_AUDIO: {
		dmrgo_rf_audio_rx_config_t c = {.size = sizeof c, .tier = tier, .rate = (int64_t)cfg->rate};
		if (dmrgo_rf_new_audio_receiver(&c, &r->audio) != DMRGO_OK)
			goto fail;
		break;
	}
	default:
		goto fail;
	}
	if ((st = new_codec2(r, cfg->codec2_mode)) != GRDMRGO_OK)
		goto fail;
	*out = r;
	return GRDMRGO_OK;
fail:
	grdmrgo_rx_free(r);
	return st;
}

void grdmrgo_rx_free(grdmrgo_rx *r)
{
	if (r == NULL)
		return;
	for (int i = 0; i < 3; i++)
		dmrgo_codec2_codec_free(r->chans[i].c2);
	dmrgo_rf_receiver_free(r->iq);
	dmrgo_rf_audio_receiver_free(r->audio);
	dmrgo_rf_monitor_free(r->mon);
	dmrgo_layer2_burst_free(r->l2);
	free(r->bursts);
	free(r->queue);
	free(r);
}

int grdmrgo_rx_has_codec2(grdmrgo_rx *r) { return r != NULL && r->has_c2; }

uint64_t grdmrgo_rx_dropped(grdmrgo_rx *r) { return r == NULL ? 0 : r->dropped; }

int grdmrgo_rx_poll(grdmrgo_rx *r, grdmrgo_burst *out)
{
	if (r == NULL || out == NULL)
		return GRDMRGO_ERR_CONFIG;
	if (r->n == 0)
		return 0;
	*out = r->queue[r->head];
	r->head = (r->head + 1) % r->cfg.queue;
	r->n--;
	return 1;
}

static grdmrgo_burst *next(grdmrgo_rx *r)
{
	if (r->n == r->cfg.queue) {
		r->head = (r->head + 1) % r->cfg.queue;
		r->n--;
		r->dropped++;
	}
	grdmrgo_burst *e = &r->queue[(r->head + r->n) % r->cfg.queue];
	r->n++;
	memset(e, 0, sizeof *e);
	return e;
}

static void set_lc(grdmrgo_burst *e, const dmrgo_pdu_lc_bytes_t *lc)
{
	e->flags |= GRDMRGO_FLAG_LC;
	e->flco = lc->v[0] & 0x3f;
	e->fid = lc->v[1];
	if (e->flco == 0)
		e->flags |= GRDMRGO_FLAG_GROUP;
	e->dst = (uint32_t)lc->v[3] << 16 | (uint32_t)lc->v[4] << 8 | lc->v[5];
	e->src = (uint32_t)lc->v[6] << 16 | (uint32_t)lc->v[7] << 8 | lc->v[8];
}

static void voice(grdmrgo_rx *r, struct channel *ch, const dmrgo_rf_burst_t *b, grdmrgo_burst *e)
{
	uint8_t slots[3][9];
	if (dmrgo_layer2_burst_get_voice_slots(r->l2, slots) != DMRGO_OK)
		return;
	for (int i = 0; i < 3; i++) {
		dmrgo_vocoder_ambe_frame_t f;
		dmrgo_fec_result_t res;
		if (dmrgo_vocoder_decode_ambe72(slots[i], &f, &res) != DMRGO_OK) {
			e->ambe_bad[i] = 1;
			continue;
		}
		memcpy(e->ambe[i], f.v, sizeof f.v);
		e->ambe_errors[i] = (int32_t)res.errors_corrected;
		e->ambe_bad[i] = res.uncorrectable;
	}
	if (!r->has_c2 || !(e->flags & GRDMRGO_FLAG_LC) || e->fid != DMRGO_CODEC2_DEFAULT_FID)
		return;
	uint8_t raw[3][9];
	if (dmrgo_layer2_raw_burst_voice_slots(&b->data, raw) != DMRGO_OK)
		return;
	size_t len = 0;
	for (int i = 0; i < 3; i++) {
		size_t need = 0;
		dmrgo_vocoder_frame_report_t rep;
		if (dmrgo_codec2_codec_decode(ch->c2, e->pcm, GRDMRGO_PCM_SAMPLES, &len, &need, raw[i], &rep) != DMRGO_OK)
			return;
	}
	e->pcm_len = (int32_t)len;
	dmrgo_codec2_mode_t mode = 0;
	dmrgo_codec2_codec_decode_mode(ch->c2, &mode);
	if (mode == DMRGO_CODEC2_MODE2400)
		e->codec2_mode = GRDMRGO_CODEC2_2400;
	else if (mode == DMRGO_CODEC2_MODE3200)
		e->codec2_mode = GRDMRGO_CODEC2_3200;
}

static void on_burst(grdmrgo_rx *r, const dmrgo_rf_burst_t *b)
{
	dmrgo_rf_monitor_burst_t mb;
	memset(&mb, 0, sizeof mb);
	int st = dmrgo_rf_monitor_burst(r->mon, r->l2, b, &mb);
	grdmrgo_burst *e = next(r);
	int ts = mb.hint.slot == 1 || mb.hint.slot == 2 ? mb.hint.slot : 0;
	struct channel *ch = &r->chans[ts];
	e->seconds = b->seconds;
	e->slot = b->slot;
	e->stream = (int32_t)b->stream;
	e->timeslot = ts;
	e->color_code = -1;
	e->data_type = -1;
	e->voice_pos = -1;
	e->sync_distance = (int32_t)b->distance;
	e->sync = b->sync;
	dmrgo_layer2_raw_burst_t data;
	uint8_t cach[3];
	dmrgo_rf_burst_decoded(b, &data, cach);
	memcpy(e->bits, data.v, sizeof e->bits);
	bool timed = false;
	dmrgo_rf_burst_timed(b, &timed);
	if (timed)
		e->flags |= GRDMRGO_FLAG_TIMED;
	if (b->bs)
		e->flags |= GRDMRGO_FLAG_BS;
	if (b->has_fixed && memcmp(b->fixed.v, b->data.v, sizeof b->data.v) != 0)
		e->flags |= GRDMRGO_FLAG_FIXED;
	if (st != DMRGO_OK) {
		e->flags |= GRDMRGO_FLAG_FEC_BAD;
		return;
	}
	dmrgo_fec_burst_fec_stats_t stats;
	dmrgo_fec_result_t agg;
	if (dmrgo_layer2_burst_get_fec(r->l2, &stats) == DMRGO_OK && dmrgo_fec_burst_fec_stats_aggregate(&stats, &agg) == DMRGO_OK) {
		e->fec_corrected = (int32_t)agg.errors_corrected;
		if (agg.uncorrectable)
			e->flags |= GRDMRGO_FLAG_FEC_BAD;
	}
	bool emb = false;
	dmrgo_pdu_embedded_signalling_t es;
	if (dmrgo_layer2_burst_has_embedded_signalling(r->l2, &emb) == DMRGO_OK && emb &&
	    dmrgo_layer2_burst_get_embedded_signalling(r->l2, &es) == DMRGO_OK)
		e->color_code = es.color_code;
	if (mb.lc_done)
		e->flags |= GRDMRGO_FLAG_LC_DONE;
	if (mb.has_lc)
		set_lc(e, &mb.lc);
	dmrgo_layer2_kind_t kind = DMRGO_LAYER2_KIND_UNKNOWN;
	dmrgo_layer2_burst_get_kind(r->l2, &kind);
	if (kind == DMRGO_LAYER2_KIND_VOICE) {
		e->kind = GRDMRGO_KIND_VOICE;
		dmrgo_enums_voice_burst_type_t pos = DMRGO_ENUMS_VOICE_BURST_UNKNOWN;
		int64_t idx = 0;
		bool ok = false;
		dmrgo_layer2_burst_get_voice_burst(r->l2, &pos);
		if (dmrgo_enums_voice_burst_type_index(pos, &idx, &ok) == DMRGO_OK && ok)
			e->voice_pos = (int32_t)idx;
		voice(r, ch, b, e);
	} else if (kind == DMRGO_LAYER2_KIND_DATA) {
		e->kind = GRDMRGO_KIND_DATA;
		dmrgo_pdu_slot_type_t stype;
		if (dmrgo_layer2_burst_get_slot_type(r->l2, &stype) == DMRGO_OK) {
			e->color_code = stype.color_code;
			e->data_type = (int32_t)stype.data_type;
			if ((stype.data_type == DMRGO_ENUMS_DATA_TYPE_VOICE_LC_HEADER ||
			     stype.data_type == DMRGO_ENUMS_DATA_TYPE_TERMINATOR_WITH_LC) &&
			    mb.lc_done && ch->c2 != NULL)
				dmrgo_codec2_codec_reset(ch->c2);
		}
	}
	if (mb.end)
		e->flags |= GRDMRGO_FLAG_END;
}

static int drain(grdmrgo_rx *r, size_t n)
{
	for (size_t i = 0; i < n; i++)
		on_burst(r, &r->bursts[i]);
	return GRDMRGO_OK;
}

static int room(grdmrgo_rx *r, size_t items)
{
	if (items <= r->bursts_cap)
		return GRDMRGO_OK;
	dmrgo_rf_burst_t *b = realloc(r->bursts, items * sizeof *b);
	if (b == NULL)
		return GRDMRGO_ERR_LIBRARY;
	r->bursts = b;
	r->bursts_cap = items;
	return GRDMRGO_OK;
}

static int write_iq(grdmrgo_rx *r, const uint8_t *x, size_t n)
{
	size_t items = 0, arena = 0, len = 0, need = 0;
	if (dmrgo_rf_receiver_append_bursts_need(r->iq, x, n, &items, &arena) != DMRGO_OK || room(r, items) != GRDMRGO_OK)
		return GRDMRGO_ERR_LIBRARY;
	if (dmrgo_rf_receiver_append_bursts(r->iq, r->bursts, r->bursts_cap, &len, &need, x, n) != DMRGO_OK)
		return GRDMRGO_ERR_LIBRARY;
	return drain(r, len);
}

static int write_audio(grdmrgo_rx *r, const int16_t *x, size_t n)
{
	size_t items = 0, arena = 0, len = 0, need = 0;
	if (dmrgo_rf_audio_receiver_append_bursts_need(r->audio, x, n, &items, &arena) != DMRGO_OK ||
	    room(r, items) != GRDMRGO_OK)
		return GRDMRGO_ERR_LIBRARY;
	if (dmrgo_rf_audio_receiver_append_bursts(r->audio, r->bursts, r->bursts_cap, &len, &need, x, n) != DMRGO_OK)
		return GRDMRGO_ERR_LIBRARY;
	return drain(r, len);
}

int grdmrgo_rx_write_cf32(grdmrgo_rx *r, const float *iq, size_t n)
{
	if (r == NULL || r->iq == NULL || (iq == NULL && n > 0))
		return GRDMRGO_ERR_INPUT;
	while (n > 0) {
		size_t k = n < IQ_CHUNK ? n : IQ_CHUNK;
		int st = write_iq(r, (const uint8_t *)iq, k * 2 * sizeof(float));
		if (st != GRDMRGO_OK)
			return st;
		iq += 2 * k;
		n -= k;
	}
	return GRDMRGO_OK;
}

int grdmrgo_rx_write_f32(grdmrgo_rx *r, const float *x, size_t n)
{
	if (r == NULL || r->audio == NULL || (x == NULL && n > 0))
		return GRDMRGO_ERR_INPUT;
	while (n > 0) {
		size_t k = n < AUDIO_CHUNK ? n : AUDIO_CHUNK;
		for (size_t i = 0; i < k; i++) {
			float v = roundf(x[i] * r->cfg.audio_scale);
			r->s16[i] = (int16_t)(v > 32767 ? 32767 : v < -32767 ? -32767 : v);
		}
		int st = write_audio(r, r->s16, k);
		if (st != GRDMRGO_OK)
			return st;
		x += k;
		n -= k;
	}
	return GRDMRGO_OK;
}

int grdmrgo_rx_flush(grdmrgo_rx *r)
{
	if (r == NULL)
		return GRDMRGO_ERR_INPUT;
	size_t items = 0, arena = 0, len = 0, need = 0;
	int st;
	if (r->iq != NULL) {
		if (dmrgo_rf_receiver_append_flush_need(r->iq, &items, &arena) != DMRGO_OK || room(r, items) != GRDMRGO_OK)
			return GRDMRGO_ERR_LIBRARY;
		st = dmrgo_rf_receiver_append_flush(r->iq, r->bursts, r->bursts_cap, &len, &need);
	} else {
		if (dmrgo_rf_audio_receiver_append_flush_need(r->audio, &items, &arena) != DMRGO_OK ||
		    room(r, items) != GRDMRGO_OK)
			return GRDMRGO_ERR_LIBRARY;
		st = dmrgo_rf_audio_receiver_append_flush(r->audio, r->bursts, r->bursts_cap, &len, &need);
	}
	if (st != DMRGO_OK)
		return GRDMRGO_ERR_LIBRARY;
	return drain(r, len);
}
