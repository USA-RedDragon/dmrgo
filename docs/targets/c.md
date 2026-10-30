# C API

This page shows how to build libdmrgo, the C form of dmrgo in `v2/capi`, and how to call it from a C program.

## What you need

- Go, at the version of the `go` line of `v2/capi/go.mod`:

<!-- dmrgo:code file=v2/capi/go.mod lines=3-3 -->
```
go 1.24.0
```
<!-- /dmrgo:code -->

- A C compiler and GNU make. `make static` also needs `ld`, `objcopy` and `ar` from binutils. `make test` also needs a C++ compiler and `nm`.
- Optional: libcodec2 and its pkg-config file, for the native Codec2 backend (`GOFLAGS=-tags=codec2native`). The default build runs libcodec2 in WebAssembly and needs no system libcodec2.

`v2/capi` is a module of its own, so its cgo code stays out of the `v2` module. It builds against the `v2` directory of the same checkout.

## The ABI version

libdmrgo has a C ABI number of its own, `DMRGO_ABI_VERSION`. The generated header defines it:

<!-- dmrgo:code file=v2/capi/dmrgo_gen.h lines=14-14 -->
```c
#define DMRGO_ABI_VERSION 4
```
<!-- /dmrgo:code -->

The soname is `libdmrgo.so.N`, with N the ABI number. `dmrgo.h` gives the rules of the number:

<!-- dmrgo:code file=v2/capi/dmrgo.h lines=71-78 -->
```c
* The ABI version: DMRGO_ABI_VERSION numbers the C ABI of libdmrgo. It is
* independent of the version of the Go module and changes only when a
* declaration changes in a way that breaks a program built against the
* older header, such as a removed function or a changed layout of a
* struct that is not sized. The soname of the library follows it:
* libdmrgo.so.1 for ABI version 1, and the symbols have the version node
* DMRGO_1. New functions, codes and members at the end of a sized struct
* keep the ABI version.
```
<!-- /dmrgo:code -->

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

<!-- dmrgo:code file=.github/workflows/test.yaml lines=211-221 -->
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

<!-- dmrgo:code file=v2/capi/test/api_test.c lines=475-512 -->
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

These rules of `dmrgo.h` apply to threads, callback storage and processes:

<!-- dmrgo:code file=v2/capi/dmrgo.h lines=58-69 -->
```c
* - Use a handle from one thread at a time. A handle that a call takes,
*   such as the port of dmrgo_dvsi_new or the authenticator of a config,
*   stays in use by the handle that the call makes: freeing the handle
*   that was taken does not stop that use. The user value of a vtable or
*   callback must stay valid until the handle made from it and every
*   handle that took that handle or callback are freed.
* - A callback that gets an array, such as the bursts of
*   dmrgo_rf_rx_config_t.on_bursts or the samples of
*   dmrgo_rf_tx_config_t.on_samples, gets it in storage of the library
*   that is valid only during the call: copy what it keeps.
* - libdmrgo runs the Go runtime, whose threads a child of fork() does not
*   have, so a child process calls libdmrgo only after an exec.
```
<!-- /dmrgo:code -->

- `dmrgo_version` gives the `v2` tag that the library was built from. The capi module has no tags of its own.
