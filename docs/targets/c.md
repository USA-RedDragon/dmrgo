# C API

This page shows how to build libdmrgo, the C form of dmrgo in `v2/capi`, and how to call it from a C program.

## What you need

- Go, at the version of the `go` line of `v2/capi/go.mod`:

<!-- dmrgo:code file=v2/capi/go.mod lines=3-3 -->
<!-- /dmrgo:code -->

- A C compiler and GNU make. `make static` also needs `ld`, `objcopy` and `ar` from binutils. `make test` also needs a C++ compiler and `nm`.
- Optional: libcodec2 and its pkg-config file, for the native Codec2 backend (`GOFLAGS=-tags=codec2native`). The default build runs libcodec2 in WebAssembly and needs no system libcodec2.

`v2/capi` is a module of its own, so its cgo code stays out of the `v2` module. It builds against the `v2` directory of the same checkout.

## The ABI version

libdmrgo has a C ABI number of its own, `DMRGO_ABI_VERSION`. The generated header defines it:

<!-- dmrgo:code file=v2/capi/dmrgo_gen.h lines=14-14 -->
<!-- /dmrgo:code -->

The soname is `libdmrgo.so.N`, with N the ABI number. `dmrgo.h` gives the rules of the number:

<!-- dmrgo:code file=v2/capi/dmrgo.h lines=67-74 -->
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
<!-- /dmrgo:code -->

## Use the library

Include `dmrgo.h`. It includes `dmrgo_gen.h`, which holds the C form of each package. Compile with `-I v2/capi` and link with `-L v2/capi/build -ldmrgo`.

This part of the C test program builds a CSBK burst, decodes it, reads its addresses and encodes it again:

<!-- dmrgo:code file=v2/capi/test/api_test.c lines=468-505 -->
<!-- /dmrgo:code -->

Each function returns `DMRGO_OK` or a negative status. [C header](../reference/c-api.md) gives the full rules for outputs, inputs, sized structs and handles.

## Limits

These rules of `dmrgo.h` apply to threads and processes:

<!-- dmrgo:code file=v2/capi/dmrgo.h lines=58-65 -->
<!-- /dmrgo:code -->

- `dmrgo_version` gives the `v2` tag that the library was built from. The capi module has no tags of its own.
