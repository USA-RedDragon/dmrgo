# C API

This page shows how to build libdmrgo, the C form of dmrgo in `v2/capi`, and how to call it from a C program.

## What you need

- Go <!-- dmrgo:gomod field=go file=v2/capi/go.mod -->`1.24.0`<!-- /dmrgo:gomod --> or later, the version of the `go` line of `v2/capi/go.mod`.

- A C compiler and GNU make. `make static` also needs `ld`, `objcopy` and `ar` from binutils. `make test` also needs a C++ compiler and `nm`.
- Optional: libcodec2 and its pkg-config file, for the native Codec2 backend (`GOFLAGS=-tags=codec2native`). The default build runs libcodec2 in WebAssembly and needs no system libcodec2.

`v2/capi` is a module of its own, so its cgo code stays out of the `v2` module. It builds against the `v2` directory of the same checkout.

## The ABI version

libdmrgo has a C ABI number of its own, `DMRGO_ABI_VERSION`. The generated header defines it:

<!-- dmrgo:code file=v2/capi/dmrgo_gen.h symbol=DMRGO_ABI_VERSION -->
```c
#define DMRGO_ABI_VERSION 4
```
<!-- /dmrgo:code -->

The soname is `libdmrgo.so.N`, with N the ABI number. `dmrgo.h` gives the rules of the number.

[Versioning](../project/versioning.md) gives the ABI number of each release. [v2.1 breaking changes](../project/breaking-v2.1.md) lists the C declarations that ABI 4 changed.

## Build

From the root of a clone:

```sh
make -C v2/capi
```

This writes `v2/capi/build/libdmrgo.so.N` and the link `libdmrgo.so`. Other targets of the Makefile:

- `make -C v2/capi static` writes `build/static/libdmrgo.a`, a static archive whose only global symbols are the C API.
- `make -C v2/capi test` builds the shared and static libraries, checks the exported symbols against `libdmrgo.map`, and runs the C test program on a capture.
- `BUILD=<dir>` sets the output directory.

The CI job of the C API runs these builds, also with the native Codec2 backend and without libcodec2:

<!-- dmrgo:code file=.github/workflows/test.yaml symbol="jobs.capi.steps[Go tests of the C API:Build libdmrgo without libcodec2]" -->
```yaml
- name: Go tests of the C API
  run: go test -C v2/capi -race ./...

- name: Build libdmrgo and run the C test program
  run: make -C v2/capi test

- name: Build libdmrgo with the native Codec2 backend and run the C test program
  run: GOFLAGS=-tags=codec2native make -C v2/capi BUILD=build-native test

- name: Build libdmrgo without libcodec2
  run: go build -C v2/capi -tags nocodec2 -buildmode=c-shared -o build/libdmrgo-nocodec2.so .
```
<!-- /dmrgo:code -->

## Use the library

Include `dmrgo.h`. It includes `dmrgo_gen.h`, which holds the C form of each package. Compile with `-I v2/capi` and link with `-L v2/capi/build -ldmrgo`.

This part of the C test program builds a CSBK burst, decodes it, reads its addresses and encodes it again:

<!-- dmrgo:code file=v2/capi/test/api_test.c symbol=test_layer2 -->
```c
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
```
<!-- /dmrgo:code -->

Each function returns `DMRGO_OK` or a negative status. [C header](../reference/c-api.md) gives the full rules for outputs, inputs, sized structs and handles.

## Limits

`dmrgo.h` gives the rules for threads, callback storage and processes.

- `dmrgo_version` gives the `v2` tag that the library was built from. The capi module has no tags of its own.
