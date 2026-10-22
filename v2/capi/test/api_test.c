// SPDX-License-Identifier: MIT

#include <stdio.h>
#include <stdlib.h>
#include <string.h>

#include "dmrgo.h"

static int failures;

#define CHECK(cond)                                                            \
	do {                                                                   \
		if (!(cond)) {                                                 \
			fprintf(stderr, "%s:%d: %s\n", __FILE__, __LINE__,     \
				#cond);                                        \
			failures++;                                            \
		}                                                              \
	} while (0)

#define OK(call) CHECK((call) == DMRGO_OK)

static unsigned char *read_file(const char *path, size_t *n) {
	FILE *f = fopen(path, "rb");
	if (!f) {
		perror(path);
		exit(1);
	}
	fseek(f, 0, SEEK_END);
	long size = ftell(f);
	fseek(f, 0, SEEK_SET);
	unsigned char *b = malloc((size_t)size);
	if (!b || fread(b, 1, (size_t)size, f) != (size_t)size) {
		perror(path);
		exit(1);
	}
	fclose(f);
	*n = (size_t)size;
	return b;
}

static void test_version(void) {
	char v[64];
	size_t n = 0;
	CHECK(dmrgo_abi_version() == DMRGO_ABI_VERSION);
	OK(dmrgo_version(v, sizeof v, &n, NULL));
	CHECK(n > 0 && strlen(v) == n);
}

static void test_errors(void) {
	dmrgo_enums_color_code_t cc = 0;
	CHECK(dmrgo_enums_new_color_code(16, &cc) == DMRGO_ERR_FIELD_RANGE_ERROR);
	CHECK(dmrgo_last_error_code() == DMRGO_ERR_FIELD_RANGE_ERROR);
	dmrgo_field_range_error_t e = {0};
	bool ok = false;
	OK(dmrgo_field_range_error_last(&e, &ok));
	CHECK(ok && e.bits == 4 && e.field_len == 9 && memcmp(e.field, "ColorCode", 9) == 0);
	char msg[128];
	size_t n = 0;
	OK(dmrgo_last_error_message(msg, sizeof msg, &n, NULL));
	CHECK(n > 0 && strstr(msg, "4 bits") != NULL);
	CHECK(strcmp(dmrgo_strerror(DMRGO_ERR_FIELD_RANGE_ERROR), "dmrgo.FieldRangeError") == 0);
	OK(dmrgo_enums_new_color_code(7, &cc));
	CHECK(cc == 7);

	dmrgo_layer2_burst_t *b = NULL;
	OK(dmrgo_layer2_burst_new(&b));
	OK(dmrgo_layer2_burst_free(b));
	dmrgo_layer2_kind_t k;
	CHECK(dmrgo_layer2_burst_get_kind(b, &k) == DMRGO_ERR_HANDLE);
	CHECK(dmrgo_layer2_burst_get_kind(NULL, &k) == DMRGO_ERR_HANDLE);
	CHECK(dmrgo_layer2_burst_new(NULL) == DMRGO_ERR_ARG);
	OK(dmrgo_layer2_burst_free(NULL));

	dmrgo_vocoder_codec_t *c = NULL;
	CHECK(dmrgo_vocoder_new("no such codec", 13, &c) == DMRGO_ERR_VOCODER_UNKNOWN_CODEC);
}

static void test_pdu(void) {
	dmrgo_pdu_csbk_t c;
	OK(dmrgo_pdu_new_bs_outbound_activation(0x123456, 3191868, &c));
	uint8_t bits[12];
	OK(dmrgo_pdu_csbk_encode(&c, bits));
	dmrgo_pdu_csbk_t d;
	memset(&d, 0xff, sizeof d);
	OK(dmrgo_pdu_decode_csbk(bits, &d));
	CHECK(d.opcode == c.opcode && !d.fec.uncorrectable);
	uint32_t target = 0, source = 0;
	bool ok = false;
	OK(dmrgo_pdu_csbk_addresses(&d, &target, &source, &ok));
	CHECK(ok && target == 0x123456 && source == 3191868);
	char s[512];
	size_t n = 0;
	OK(dmrgo_pdu_csbk_string(&d, s, sizeof s, &n, NULL));
	CHECK(n > 0 && strlen(s) == n);
	char small[4];
	size_t m = 0, need = 0;
	CHECK(dmrgo_pdu_csbk_string(&d, small, sizeof small, &m, &need) == DMRGO_ERR_SHORT);
	CHECK(m == 3 && need == n + 1 && strlen(small) == 3 && memcmp(small, s, 3) == 0);

	dmrgo_layer3_service_options_t so = {0};
	dmrgo_pdu_full_link_control_t lc;
	OK(dmrgo_pdu_new_voice_lc(true, 91, 3191868, &so, &lc));
	CHECK(lc.has_group_voice && lc.group_voice.group_address == 91 && !lc.has_unit_to_unit);
	lc.burst_data_type = DMRGO_ENUMS_DATA_TYPE_VOICE_LC_HEADER;
	uint8_t lcbits[12];
	OK(dmrgo_pdu_full_link_control_encode(&lc, lcbits));
	dmrgo_pdu_full_link_control_t lc2;
	OK(dmrgo_pdu_decode_full_link_control(lcbits, DMRGO_ENUMS_DATA_TYPE_VOICE_LC_HEADER, &lc2));
	OK(dmrgo_pdu_full_link_control_addresses(&lc2, &target, &source, &ok));
	CHECK(ok && target == 91 && source == 3191868 && lc2.has_group_voice && !lc2.fec.uncorrectable);
	printf("pdu: CSBK and Full LC round trips\n");
}

static void test_layer2(void) {
	dmrgo_layer2_data_t data = {0};
	data.tag = DMRGO_LAYER2_DATA_PDU_CSBK;
	OK(dmrgo_pdu_new_bs_outbound_activation(0x123456, 3191868, &data.v.pdu_csbk));
	dmrgo_layer2_raw_burst_t raw;
	OK(dmrgo_layer2_build_burst(&data, 5, DMRGO_LAYER2_SYNC_SOURCE_BS, &raw));
	dmrgo_pdu_slot_type_t st;
	OK(dmrgo_layer2_raw_burst_slot_type(&raw, &st));
	CHECK(st.color_code == 5 && st.data_type == DMRGO_ENUMS_DATA_TYPE_CSBK);

	dmrgo_layer2_burst_t *b = NULL;
	OK(dmrgo_layer2_new_burst(&b));
	OK(dmrgo_layer2_burst_decode_from_bytes(b, &raw));
	dmrgo_layer2_kind_t k = 0;
	OK(dmrgo_layer2_burst_get_kind(b, &k));
	CHECK(k == DMRGO_LAYER2_KIND_DATA);
	dmrgo_layer2_data_t got;
	OK(dmrgo_layer2_burst_get_data(b, &got));
	CHECK(got.tag == DMRGO_LAYER2_DATA_PDU_CSBK);
	uint32_t target = 0, source = 0;
	bool ok = false;
	OK(dmrgo_pdu_csbk_addresses(&got.v.pdu_csbk, &target, &source, &ok));
	CHECK(ok && target == 0x123456 && source == 3191868);
	dmrgo_layer2_raw_burst_t again;
	OK(dmrgo_layer2_burst_encode(b, &again));
	CHECK(memcmp(again.v, raw.v, sizeof raw.v) == 0);

	OK(dmrgo_pdu_csbk_set_addresses(&got.v.pdu_csbk, 0x654321, 3191868));
	OK(dmrgo_layer2_burst_set_data(b, &got));
	OK(dmrgo_layer2_burst_encode(b, &again));
	CHECK(memcmp(again.v, raw.v, sizeof raw.v) != 0);
	OK(dmrgo_layer2_burst_decode_from_bytes(b, &again));
	OK(dmrgo_layer2_burst_get_data(b, &got));
	OK(dmrgo_pdu_csbk_addresses(&got.v.pdu_csbk, &target, &source, &ok));
	CHECK(got.tag == DMRGO_LAYER2_DATA_PDU_CSBK && ok && target == 0x654321 && source == 3191868);
	OK(dmrgo_layer2_burst_free(b));
	printf("layer2: build, decode, set and encode a CSBK burst\n");
}

static void test_tx(void) {
	dmrgo_layer2_voice_transmitter_options_t opts;
	memset(&opts, 0, sizeof opts);
	dmrgo_layer3_service_options_t so = {0};
	OK(dmrgo_pdu_new_voice_lc(true, 9990, 3191868, &so, &opts.lc));
	opts.color_code = 1;
	opts.source = DMRGO_LAYER2_SYNC_SOURCE_MS;
	dmrgo_layer2_voice_transmitter_t *tx = NULL;
	OK(dmrgo_layer2_new_voice_transmitter(&opts, &tx));

	dmrgo_layer2_raw_burst_t bursts[64];
	size_t n = 0;
	OK(dmrgo_layer2_voice_transmitter_append_headers(tx, bursts, 64, &n, NULL));
	size_t headers = n;
	dmrgo_vocoder_ambe_frame_t frames[18];
	for (int i = 0; i < 18; i++)
		OK(dmrgo_vocoder_ambe_silence(&frames[i]));
	OK(dmrgo_layer2_voice_transmitter_append_frames(tx, bursts, 64, &n, NULL, frames, 18));
	CHECK(n == headers + 6);
	OK(dmrgo_layer2_voice_transmitter_append_terminate(tx, bursts, 64, &n, NULL));
	CHECK(n == headers + 7);

	dmrgo_layer2_voice_transmitter_t *tx2 = NULL;
	OK(dmrgo_layer2_new_voice_transmitter(&opts, &tx2));
	dmrgo_layer2_raw_burst_t few[2], more[16];
	size_t two = 1, need = 0, k = 0;
	CHECK(dmrgo_layer2_voice_transmitter_append_frames(tx2, few, 2, &two, &need, frames, 18) == DMRGO_ERR_SHORT);
	CHECK(two == 1 && need == 7);
	size_t items = 0, abytes = 1;
	OK(dmrgo_layer2_voice_transmitter_append_frames_need(tx2, frames, 18, &items, &abytes));
	CHECK(items == 6 && abytes == 0);
	OK(dmrgo_layer2_voice_transmitter_append_frames(tx2, more, 16, &k, &need, frames, 18));
	CHECK(k == 6 && need == 6);
	OK(dmrgo_layer2_voice_transmitter_append_terminate_need(tx2, &items, &abytes));
	CHECK(items == DMRGO_LAYER2_VOICE_BURSTS_PER_SUPERFRAME + 1);
	OK(dmrgo_layer2_voice_transmitter_free(tx2));

	dmrgo_voice_receiver_t *rx = NULL;
	OK(dmrgo_voice_new_receiver(NULL, &rx));
	dmrgo_voice_event_t ev[DMRGO_VOICE_MAX_EVENTS];
	unsigned char arena[1024];
	size_t used = 0;
	int starts = 0, voice = 0, ends = 0;
	for (size_t i = 0; i < n; i++) {
		size_t m = 0;
		dmrgo_layer2_hint_t h = {0};
		memset(ev, 0xab, sizeof ev);
		used = 0;
		OK(dmrgo_voice_receiver_burst(rx, (int64_t)(i + 1) * 60000000, &bursts[i], &h, ev, DMRGO_VOICE_MAX_EVENTS, &m, NULL, arena, sizeof arena, &used, NULL));
		for (size_t j = 0; j < m; j++) {
			if (ev[j].kind == DMRGO_VOICE_EVENT_CALL_START) {
				starts++;
				CHECK(ev[j].call.group && ev[j].call.target == 9990 && ev[j].call.source == 3191868);
			}
			if (ev[j].kind == DMRGO_VOICE_EVENT_VOICE)
				voice++;
			if (ev[j].kind == DMRGO_VOICE_EVENT_CALL_END) {
				ends++;
				CHECK(ev[j].reason == DMRGO_VOICE_END_TERMINATOR);
			}
		}
	}
	CHECK(starts == 1 && voice == 6 && ends == 1);
	OK(dmrgo_voice_receiver_free(rx));
	OK(dmrgo_layer2_voice_transmitter_free(tx));

	dmrgo_voice_transmitter_config_t cfg;
	memset(&cfg, 0, sizeof cfg);
	cfg.color_code = 1;
	cfg.source = DMRGO_LAYER2_SYNC_SOURCE_MS;
	cfg.queue = 27;
	dmrgo_voice_transmitter_t *vt = NULL;
	OK(dmrgo_voice_new_transmitter(&cfg, &vt));
	dmrgo_voice_call_t call;
	memset(&call, 0, sizeof call);
	call.group = true;
	call.target = 9990;
	call.source = 3191868;
	OK(dmrgo_voice_transmitter_start(vt, 0, &call, NULL));
	OK(dmrgo_voice_transmitter_frames(vt, frames, 18));
	OK(dmrgo_voice_transmitter_stop(vt));
	dmrgo_voice_output_t out[16];
	int outs = 0, ended = 0;
	for (int64_t now = 0; now < 2000000000 && !ended; now += 10000000) {
		size_t m = 0;
		OK(dmrgo_voice_transmitter_tick(vt, now, out, 16, &m, NULL));
		for (size_t j = 0; j < m; j++) {
			if (out[j].kind == DMRGO_VOICE_OUTPUT_BURST)
				outs++;
			if (out[j].kind == DMRGO_VOICE_OUTPUT_ENDED)
				ended = 1;
		}
	}
	CHECK(ended && outs >= 8);
	OK(dmrgo_voice_transmitter_free(vt));
	printf("tx: %zu layer2 bursts, %d voice.Transmitter bursts\n", n, outs);
}

static void test_voice_capture(const char *path) {
	size_t n;
	unsigned char *data = read_file(path, &n);
	dmrgo_voice_receiver_t *rx = NULL;
	OK(dmrgo_voice_new_receiver(NULL, &rx));
	dmrgo_voice_event_t ev[DMRGO_VOICE_MAX_EVENTS];
	unsigned char arena[1024];
	int64_t now = 0;
	int voice = 0, ends = 0, retries = 0;
	for (size_t off = 0; off + 33 <= n; off += 33) {
		dmrgo_layer2_raw_burst_t raw;
		memcpy(raw.v, data + off, 33);
		now += 60000000;
		size_t m = 0;
		dmrgo_layer2_hint_t h = {0};
		memset(ev, 0xab, sizeof ev);
		size_t need = 0, used = 0, aneed = 0, items = 0, abytes = 0;
		OK(dmrgo_voice_receiver_burst_need(rx, now, &raw, &h, &items, &abytes));
		CHECK(items == DMRGO_VOICE_MAX_EVENTS && abytes > 0 && abytes <= sizeof arena);
		int rc = dmrgo_voice_receiver_burst(rx, now, &raw, &h, ev, items - 1, &m, &need, arena, sizeof arena, &used, &aneed);
		if (rc == DMRGO_ERR_SHORT) {
			retries++;
			CHECK(m == 0 && used == 0 && need == items);
		}
		rc = dmrgo_voice_receiver_burst(rx, now, &raw, &h, ev, items, &m, &need, NULL, 0, &used, &aneed);
		CHECK(rc == DMRGO_ERR_SHORT && m == 0 && aneed == abytes);
		OK(dmrgo_voice_receiver_burst(rx, now, &raw, &h, ev, DMRGO_VOICE_MAX_EVENTS, &m, &need, arena, aneed, &used, &aneed));
		for (size_t j = 0; j < m; j++) {
			if (ev[j].kind == DMRGO_VOICE_EVENT_CALL_START)
				CHECK(ev[j].call.source == 3191868 && ev[j].call.target == 9990 && ev[j].call.origin == DMRGO_VOICE_ORIGIN_HEADER);
			if (ev[j].kind == DMRGO_VOICE_EVENT_VOICE)
				voice++;
			if (ev[j].kind == DMRGO_VOICE_EVENT_CALL_END) {
				ends++;
				CHECK(ev[j].reason == DMRGO_VOICE_END_TERMINATOR && ev[j].stats.duration == 840000000);
			}
		}
	}
	CHECK(voice == 13 && ends == 1 && retries > 0);
	OK(dmrgo_voice_receiver_free(rx));
	free(data);
	printf("voice: parrot capture, %d voice bursts\n", voice);
}

static int c_frame_samples(uintptr_t user, int64_t *out) {
	(void)user;
	*out = 160;
	return 0;
}

static int c_decode(uintptr_t user, const uint8_t slot[9], int16_t *out, size_t cap, size_t *len, size_t *need) {
	*need = 160;
	*len = 0;
	if (cap < 160)
		return DMRGO_ERR_SHORT;
	*len = 160;
	for (int i = 0; i < 160; i++)
		out[i] = (int16_t)(slot[0] + *(int *)user);
	return 0;
}

static int c_encode(uintptr_t user, const int16_t *pcm, size_t n, uint8_t out[9]) {
	(void)user;
	if (n != 160)
		return DMRGO_ERR_CODEC2_FRAME_LENGTH;
	memset(out, 0, 9);
	out[0] = (uint8_t)pcm[0];
	return 0;
}

static int c_silence(uintptr_t user, uint8_t out[9]) {
	(void)user;
	memset(out, 0, 9);
	return 0;
}

static int c_fid(uintptr_t user, dmrgo_enums_feature_set_id_t *out) {
	(void)user;
	*out = 0x42;
	return 0;
}

static int c_close(uintptr_t user) {
	(void)user;
	return 0;
}

static int bias = 1000;

static int liar_decode(uintptr_t user, const uint8_t slot[9], int16_t *out, size_t cap, size_t *len, size_t *need) {
	(void)user;
	(void)slot;
	*need = 200;
	*len = 0;
	if (cap < 200)
		return DMRGO_ERR_SHORT;
	memset(out, 0, 200 * sizeof *out);
	*len = 200;
	return 0;
}

static int liar_factory(uintptr_t user, dmrgo_vocoder_codec_t **out) {
	(void)user;
	dmrgo_vocoder_codec_vtable_t vt = {0, c_close, liar_decode, c_encode, c_fid, c_frame_samples, c_silence};
	return dmrgo_vocoder_codec_from_vtable(&vt, out);
}

static void test_liar(void) {
	dmrgo_vocoder_register_factory_fn_t f = {liar_factory, 0};
	OK(dmrgo_vocoder_register("liar", 4, f));
	dmrgo_vocoder_codec_t *c = NULL;
	OK(dmrgo_vocoder_new("liar", 4, &c));
	uint8_t slot[9] = {0};
	size_t items = 0, ab = 0, n = 0, need = 0;
	OK(dmrgo_vocoder_codec_decode_need(c, slot, &items, &ab));
	CHECK(items == 160);
	int16_t pcm[400];
	CHECK(dmrgo_vocoder_codec_decode(c, slot, pcm, items, &n, &need) == DMRGO_ERR_BOUND);
	CHECK(dmrgo_last_error_code() == DMRGO_ERR_BOUND);
	OK(dmrgo_vocoder_codec_free(c));
}

static int c_factory(uintptr_t user, dmrgo_vocoder_codec_t **out) {
	(void)user;
	dmrgo_vocoder_codec_vtable_t vt = {(uintptr_t)&bias, c_close, c_decode, c_encode, c_fid, c_frame_samples, c_silence};
	return dmrgo_vocoder_codec_from_vtable(&vt, out);
}

static void test_vocoder(void) {
	dmrgo_vocoder_register_factory_fn_t f = {c_factory, 0};
	OK(dmrgo_vocoder_register("ctest", 5, f));
	CHECK(dmrgo_vocoder_register("ctest", 5, f) == DMRGO_ERR_VOCODER_DUPLICATE_CODEC);
	dmrgo_vocoder_codec_t *c = NULL;
	OK(dmrgo_vocoder_new("ctest", 5, &c));
	int64_t fs = 0;
	OK(dmrgo_vocoder_codec_frame_samples(c, &fs));
	CHECK(fs == 160);
	dmrgo_enums_feature_set_id_t fid = 0;
	OK(dmrgo_vocoder_codec_fid(c, &fid));
	CHECK(fid == 0x42);
	uint8_t slot[9] = {7};
	int16_t pcm[200];
	size_t n = 0;
	OK(dmrgo_vocoder_codec_decode(c, slot, pcm, 200, &n, NULL));
	CHECK(n == 160 && pcm[0] == 1007 && pcm[159] == 1007);
	uint8_t enc[9];
	pcm[0] = 9;
	OK(dmrgo_vocoder_codec_encode(c, pcm, 160, enc));
	CHECK(enc[0] == 9);
	CHECK(dmrgo_vocoder_codec_encode(c, pcm, 10, enc) == DMRGO_ERR_CODEC2_FRAME_LENGTH);
	OK(dmrgo_vocoder_codec_close(c));
	OK(dmrgo_vocoder_codec_free(c));

	dmrgo_codec2_config_t cfg = {0};
	dmrgo_vocoder_register_factory_fn_t c2 = {0};
	OK(dmrgo_codec2_new_factory(&cfg, &c2));
	CHECK(c2.fn != NULL);
	OK(dmrgo_vocoder_register("c2test", 6, c2));
	int rc = dmrgo_vocoder_new("c2test", 6, &c);
	if (rc == DMRGO_ERR_CODEC2_UNAVAILABLE) {
		printf("vocoder: C codec ok, codec2 skipped (built without libcodec2)\n");
		return;
	}
	OK(rc);
	dmrgo_vocoder_codec_silence(c, slot);
	OK(dmrgo_vocoder_codec_decode(c, slot, pcm, 200, &n, NULL));
	CHECK(n == 160);
	OK(dmrgo_vocoder_codec_close(c));
	OK(dmrgo_vocoder_codec_free(c));
	printf("vocoder: C codec and codec2 through vocoder.Register\n");
}

static void test_last_error(void) {
	dmrgo_layer2_burst_t *b = NULL;
	OK(dmrgo_layer2_burst_new(&b));
	OK(dmrgo_layer2_burst_free(b));
	CHECK(dmrgo_layer2_burst_free(b) == DMRGO_ERR_HANDLE);
	CHECK(dmrgo_last_error_code() == DMRGO_ERR_HANDLE);
	CHECK(dmrgo_layer2_burst_new(NULL) == DMRGO_ERR_ARG);
	CHECK(dmrgo_last_error_code() == DMRGO_ERR_ARG);
	char small[2];
	size_t n = 0;
	size_t sneed = 0;
	CHECK(dmrgo_enums_data_type_string(DMRGO_ENUMS_DATA_TYPE_CSBK, small, sizeof small, &n, &sneed) == DMRGO_ERR_SHORT);
	CHECK(dmrgo_last_error_code() == DMRGO_ERR_SHORT && n == 1 && sneed > 2);
	dmrgo_voice_transmitter_config_t cfg;
	memset(&cfg, 0, sizeof cfg);
	cfg.color_code = 16;
	cfg.source = DMRGO_LAYER2_SYNC_SOURCE_MS;
	dmrgo_voice_transmitter_t *vt = NULL;
	CHECK(dmrgo_voice_new_transmitter(&cfg, &vt) == DMRGO_ERR_CONFIG_ERROR && vt == NULL);
	dmrgo_config_error_t ce = {0};
	bool ok = false;
	OK(dmrgo_config_error_last(&ce, &ok));
	CHECK(ok && ce.type__len == 5 && memcmp(ce.type_, "voice", 5) == 0 && ce.field_len == 9 && memcmp(ce.field, "ColorCode", 9) == 0);
	char msg[256];
	OK(dmrgo_last_error_message(msg, sizeof msg, &n, NULL));
	CHECK(strcmp(msg, "voice: ColorCode is out of range") == 0);
	CHECK(strcmp(dmrgo_strerror(DMRGO_ERR_PDU_NO_ADDRESSES), "pdu.ErrNoAddresses") == 0);
}

static void test_partial(void) {
	dmrgo_layer2_voice_transmitter_options_t opts;
	memset(&opts, 0, sizeof opts);
	dmrgo_layer3_service_options_t so = {0};
	OK(dmrgo_pdu_new_voice_lc(true, 9990, 3191868, &so, &opts.lc));
	opts.color_code = 1;
	opts.source = DMRGO_LAYER2_SYNC_SOURCE_MS;
	dmrgo_layer2_voice_transmitter_t *tx = NULL;
	OK(dmrgo_layer2_new_voice_transmitter(&opts, &tx));
	dmrgo_layer2_raw_burst_t bursts[64];
	size_t n = 0;
	OK(dmrgo_layer2_voice_transmitter_append_headers(tx, bursts, 64, &n, NULL));
	size_t before = n;
	dmrgo_vocoder_ambe_frame_t frames[7];
	for (int i = 0; i < 7; i++)
		OK(dmrgo_vocoder_ambe_silence(&frames[i]));
	frames[6].v[6] |= 0x01;
	int rc = dmrgo_layer2_voice_transmitter_append_frames(tx, bursts, 64, &n, NULL, frames, 7);
	CHECK(rc < 0 && rc == dmrgo_last_error_code());
	CHECK(n == before + 2);
	OK(dmrgo_layer2_voice_transmitter_free(tx));
}

static int small_factory(uintptr_t user, dmrgo_vocoder_codec_t **out) {
	*out = NULL;
	return user == 7 ? 0 : DMRGO_ERR_ARG;
}

static void test_small_user(void) {
	dmrgo_vocoder_register_factory_fn_t f = {small_factory, 7};
	OK(dmrgo_vocoder_register("small", 5, f));
	dmrgo_vocoder_codec_t *c = (dmrgo_vocoder_codec_t *)1;
	OK(dmrgo_vocoder_new("small", 5, &c));
	CHECK(c == NULL);
}

static int names_fac(uintptr_t user, dmrgo_vocoder_codec_t **out) {
	(void)user;
	*out = NULL;
	return 0;
}

static void test_arena(void) {
	dmrgo_vocoder_register_factory_fn_t f = {names_fac, 0};
	OK(dmrgo_vocoder_register("arena1", 6, f));
	dmrgo_string_t names[2 * DMRGO_VOCODER_MAX_CODECS];
	char arena[512];
	size_t n = 0, used = 0;
	OK(dmrgo_vocoder_append_names(names, 2 * DMRGO_VOCODER_MAX_CODECS, &n, NULL, arena, sizeof arena, &used, NULL));
	size_t first = n;
	char copy[DMRGO_VOCODER_MAX_CODECS][32];
	for (size_t i = 0; i < first; i++)
		snprintf(copy[i], sizeof copy[i], "%.*s", (int)names[i].len, names[i].ptr);
	OK(dmrgo_vocoder_append_names(names, 2 * DMRGO_VOCODER_MAX_CODECS, &n, NULL, arena, sizeof arena, &used, NULL));
	CHECK(n == 2 * first);
	for (size_t i = 0; i < first; i++) {
		CHECK(names[i].len == strlen(copy[i]) && memcmp(names[i].ptr, copy[i], names[i].len) == 0);
		CHECK(names[first + i].len == names[i].len && memcmp(names[first + i].ptr, copy[i], names[i].len) == 0);
	}
	size_t m = 0, need = 0, fill = 0;
	CHECK(dmrgo_vocoder_names(names, 2 * DMRGO_VOCODER_MAX_CODECS, &m, NULL, NULL, 0, &fill, &need) == DMRGO_ERR_SHORT);
	CHECK(fill == 0 && need > 0);
	char odd[600];
	OK(dmrgo_vocoder_names(names, 2 * DMRGO_VOCODER_MAX_CODECS, &m, NULL, odd + 1, need, &fill, NULL));
	CHECK(fill > 0 && fill <= need);
	CHECK(strcmp(dmrgo_strerror(DMRGO_ERR_HANDLE), "invalid handle") == 0);
	char small[4];
	size_t ml = 0;
	dmrgo_layer2_burst_t *b = NULL;
	CHECK(dmrgo_layer2_burst_free((dmrgo_layer2_burst_t *)(uintptr_t)0x100000) == DMRGO_ERR_HANDLE);
	(void)b;
	size_t mneed = 0;
	CHECK(dmrgo_last_error_message(small, sizeof small, &ml, &mneed) == DMRGO_ERR_SHORT);
	CHECK(ml == 3 && mneed == strlen("invalid handle") + 1 && strcmp(small, "inv") == 0);
}

struct chip {
	uint8_t in[1024];
	size_t in_n;
	uint8_t out[4096];
	size_t out_n;
	int parity;
	int decodes, encodes;
};

static const uint8_t chip_slot[9] = {0xB9, 0xE8, 0x81, 0x52, 0x61, 0x73, 0x00, 0x2A, 0x6B};

static void chip_reply(struct chip *c, uint8_t type, const uint8_t *f, size_t n) {
	size_t len = n + (c->parity ? 2 : 0);
	uint8_t *b = c->out + c->out_n;
	b[0] = 0x61;
	b[1] = (uint8_t)(len >> 8);
	b[2] = (uint8_t)len;
	b[3] = type;
	memcpy(b + 4, f, n);
	if (c->parity) {
		uint8_t x = 0;
		b[4 + n] = 0x2F;
		for (size_t i = 1; i < 5 + n; i++)
			x ^= b[i];
		b[5 + n] = x;
	}
	c->out_n += 4 + len;
}

static void chip_packet(struct chip *c) {
	dmrgo_dvsi_packet_t p;
	int64_t n = 0;
	uint8_t arena[1024];
	size_t used = 0;
	if (dmrgo_dvsi_parse(c->in, c->in_n, c->parity, &p, &n, arena, sizeof arena, &used, NULL) != DMRGO_OK) {
		failures++;
		return;
	}
	CHECK((size_t)n == c->in_n);
	if (p.type_ == DMRGO_DVSI_TYPE_CONTROL && p.fields_len > 0) {
		static const uint8_t ready[] = {0x39};
		static const uint8_t query[] = "\x30" "AMBE3000R\0" "\x31" "V120\0";
		static const uint8_t rate[] = {0x0A, 0x00, 0x0B, 0x00};
		uint8_t pm[2] = {0x3F, 0x00};
		switch (p.fields[0]) {
		case DMRGO_DVSI_FIELD_RESET:
			c->parity = 1;
			chip_reply(c, 0, ready, sizeof ready);
			break;
		case DMRGO_DVSI_FIELD_PARITY_MODE:
			c->parity = p.fields[1];
			chip_reply(c, 0, pm, sizeof pm);
			break;
		case DMRGO_DVSI_FIELD_PROD_ID:
			chip_reply(c, 0, query, sizeof query - 1);
			break;
		case DMRGO_DVSI_FIELD_RATE_P:
			CHECK(p.fields_len == 15 && p.fields[1] == 0x04 && p.fields[2] == 0x31);
			chip_reply(c, 0, rate, sizeof rate);
			break;
		default:
			failures++;
		}
		return;
	}
	size_t len = 0;
	if (p.type_ == DMRGO_DVSI_TYPE_CHANNEL) {
		uint8_t data[24];
		size_t dn = 0;
		int64_t bits = 0, ch = -1;
		OK(dmrgo_dvsi_packet_append_channel_data(&p, data, sizeof data, &dn, NULL, &bits, &ch));
		CHECK(bits == 72 && ch == 0 && dn == 9 && memcmp(data, chip_slot, 9) == 0);
		int16_t pcm[160];
		for (int i = 0; i < 160; i++)
			pcm[i] = (int16_t)(i * 100 - 8000);
		OK(dmrgo_dvsi_append_speech(c->out + c->out_n, sizeof c->out - c->out_n, &len, NULL, 0, pcm, 160, c->parity));
		c->decodes++;
	} else {
		int16_t pcm[164];
		size_t pn = 0;
		int64_t ch = -1;
		OK(dmrgo_dvsi_packet_append_samples(&p, pcm, 164, &pn, NULL, &ch));
		CHECK(pn == 160 && ch == 0 && pcm[1] == 7);
		OK(dmrgo_dvsi_append_channel(c->out + c->out_n, sizeof c->out - c->out_n, &len, NULL, 0, chip_slot, 9, 72, c->parity));
		c->encodes++;
	}
	c->out_n += len;
}

static int chip_write(uintptr_t user, const uint8_t *p, size_t n, int64_t *out) {
	struct chip *c = (struct chip *)user;
	for (size_t i = 0; i < n; i++) {
		if (c->in_n == 0 && p[i] != 0x61)
			continue;
		c->in[c->in_n++] = p[i];
		if (c->in_n >= 4 && c->in_n == 4 + (size_t)(c->in[1] << 8 | c->in[2])) {
			chip_packet(c);
			c->in_n = 0;
		}
	}
	*out = (int64_t)n;
	return DMRGO_OK;
}

static int chip_read(uintptr_t user, uint8_t *p, size_t n, int64_t *out) {
	struct chip *c = (struct chip *)user;
	size_t k = c->out_n < n ? c->out_n : n;
	if (k > 7)
		k = 7;
	memcpy(p, c->out, k);
	memmove(c->out, c->out + k, c->out_n - k);
	c->out_n -= k;
	*out = (int64_t)k;
	return DMRGO_OK;
}

static void test_dvsi(void) {
	static struct chip c;
	dmrgo_dvsi_port_vtable_t vt = {(uintptr_t)&c, chip_read, chip_write};
	dmrgo_dvsi_port_t *port = NULL;
	OK(dmrgo_dvsi_port_from_vtable(&vt, &port));
	dmrgo_dvsi_config_t cfg = {0};
	dmrgo_dvsi_codec_t *codec = NULL;
	OK(dmrgo_dvsi_new(port, &cfg, &codec));
	char name[DMRGO_DVSI_MAX_PRODUCT_LEN + 1];
	size_t nl = 0;
	OK(dmrgo_dvsi_codec_product(codec, name, sizeof name, &nl, NULL));
	CHECK(nl == 9 && strcmp(name, "AMBE3000R") == 0);
	int16_t pcm[160];
	size_t n = 0;
	OK(dmrgo_dvsi_codec_decode(codec, chip_slot, pcm, 160, &n, NULL));
	CHECK(n == 160 && pcm[0] == -8000 && pcm[159] == 7900);
	for (int i = 0; i < 160; i++)
		pcm[i] = (int16_t)(7 * i);
	uint8_t slot[9];
	OK(dmrgo_dvsi_codec_encode(codec, pcm, 160, slot));
	CHECK(memcmp(slot, chip_slot, 9) == 0);
	CHECK(dmrgo_dvsi_codec_encode(codec, pcm, 100, slot) == DMRGO_ERR_DVSI_SAMPLES);
	CHECK(c.decodes == 1 && c.encodes == 1 && c.parity == 1);
	OK(dmrgo_dvsi_codec_close(codec));
	CHECK(dmrgo_dvsi_codec_decode(codec, chip_slot, pcm, 160, &n, NULL) == DMRGO_ERR_DVSI_CLOSED);
	OK(dmrgo_dvsi_codec_free(codec));
	OK(dmrgo_dvsi_port_free(port));
}

int main(int argc, char **argv) {
	if (argc != 2) {
		fprintf(stderr, "usage: %s parrot_kerchunk.bin\n", argv[0]);
		return 2;
	}
	test_version();
	test_errors();
	test_pdu();
	test_layer2();
	test_tx();
	test_voice_capture(argv[1]);
	test_vocoder();
	test_last_error();
	test_partial();
	test_small_user();
	test_arena();
	test_liar();
	test_dvsi();
	if (failures) {
		fprintf(stderr, "%d checks failed\n", failures);
		return 1;
	}
	printf("ok\n");
	return 0;
}
