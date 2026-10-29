# Versioning and compatibility

This page gives the version rules of the `v2` Go module and of the C ABI of libdmrgo, and the Go version that the module needs.

## The Go module

- The `v2` module has versions of the form v2.MINOR.PATCH.
- A patch release, such as v2.1.1, never breaks callers. Code that builds against v2.1.0 builds against v2.1.1 with no change.
- A minor release, such as v2.2.0, adds packages, functions, methods, struct fields, constants and enum values. A minor release can also break callers. The breaking-changes page of that release lists each break and the change to make in your code.
- A minor release can add a value to an enum type, so give a switch over an enum a default case.

The breaking-changes page of each minor release:

- [v2.1 breaking changes](breaking-v2.1.md): the changes after v2.0.8 that break code that compiles against v2.0.8.

## The Go version

The `go` line of `v2/go.mod` gives the oldest Go release that builds the module:

<!-- dmrgo:code file=v2/go.mod lines=3-3 -->
<!-- /dmrgo:code -->

The minimum Go version rises only in a minor release, with a note in the release notes. CI runs the tests at this version. See [PC](../targets/pc.md#platforms-in-ci).

## The C ABI

- libdmrgo, the C API, has its own ABI number (`DMRGO_ABI_VERSION`) and soname (`libdmrgo.so.N`). It is built from the `v2` tag, and `v2/capi` has no tags of its own.
- A change that breaks C code that compiles against `dmrgo.h`, or a program that links an older libdmrgo, gives a new ABI number and soname.
- A config, options or vtable struct of the C API whose first member is `size_t size` is sized. The caller sets `size` to `sizeof` the struct, and the library reads only the members that `size` covers and takes the rest as zero. Thus a minor release can add members at the end of such a struct without a new ABI number, and a program built against older headers works with the newer library. Other structs have a fixed layout.

The ABI number of each release:

| Releases | ABI | soname |
| --- | --- | --- |
| v2.0.0 to v2.0.3 | 1 | `libdmrgo.so.1` |
| v2.0.4 and v2.0.5 | 2 | `libdmrgo.so.2` |
| v2.0.6 to v2.0.8 | 3 | `libdmrgo.so.3` |
| the release after v2.0.8 | 4 | `libdmrgo.so.4` |

`v2/capi/abi.txt` holds the ABI number and each C declaration of the current ABI. Its first line is the number:

<!-- dmrgo:code file=v2/capi/abi.txt lines=1-1 -->
<!-- /dmrgo:code -->

`capigen`, the generator of the C API, fails when a declaration of the lock changes or goes away, unless it runs with `-break`, which gives a new ABI number. While `abi.txt` has a `prerelease` line, a change does not need `-break`, and CI fails a release tag such as `v2.1.0` until the release commit removes that line. [C API](../targets/c.md#the-abi-version) gives the rules of `dmrgo.h`.
