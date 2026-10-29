# C header

This page holds the C headers of libdmrgo: `dmrgo.h`, with the rules of the API, and `dmrgo_gen.h`, with the C form of each package.

[C API](../targets/c.md) shows how to build the library and link a program with it. `v2/capi/internal/capigen` writes `dmrgo_gen.h` from the Go API, with the doc comment of each Go name on its C form. The comment of a type or function starts with `Go:` and the Go name that it maps to. See the [Go API](go-api.md) for those packages.

## dmrgo.h

<!-- dmrgo:code file=v2/capi/dmrgo.h lines=1-162 -->
<!-- /dmrgo:code -->

## dmrgo_gen.h

<!-- dmrgo:code file=v2/capi/dmrgo_gen.h lines=1-11889 -->
<!-- /dmrgo:code -->
