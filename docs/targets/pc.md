# PC

This page shows how to build and run dmrgo with the standard Go toolchain on a PC or a server, which platforms CI checks, and which build tags change the build.

## What you need

- Go, at the version that the `go` line of the module gives, or later:

<!-- dmrgo:code file=v2/go.mod lines=3-3 -->
<!-- /dmrgo:code -->

- No C compiler. The `v2` module is pure Go, and its only cgo code is the optional system libcodec2 backend of [vocoder/codec2](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/vocoder/codec2).

## Build

Add the module to your program:

```sh
go get github.com/USA-RedDragon/dmrgo/v2
```

Then import the packages that you need, such as `github.com/USA-RedDragon/dmrgo/v2/layer2`. The [Go API](../reference/go-api.md) page lists every package.

## Run

The example programs run offline on the captures in the repository. From the root of a clone, run the conformance example:

```sh
go run -C v2 ./examples/conformance
```

It decodes each capture, encodes each burst again and compares the bytes. It prints:

<!-- dmrgo:output example=conformance -->
<!-- /dmrgo:output -->

[Examples](../reference/examples.md) lists the other programs.

## Platforms in CI

CI runs the tests of the module at the Go version of `v2/go.mod`, with cgo off:

<!-- dmrgo:code file=.github/workflows/test.yaml lines=40-44 -->
<!-- /dmrgo:code -->

It also builds and vets the module for 32-bit arm and 386, and tests the Codec2 engine on them:

<!-- dmrgo:code file=.github/workflows/test.yaml lines=76-92 -->
<!-- /dmrgo:code -->

## Build tags

The tags change only the backend of `vocoder/codec2`. The package doc gives each one:

<!-- dmrgo:doc pkg=vocoder/codec2 -->
<!-- /dmrgo:doc -->

## Limits

- AMBE+2 voice is not in the library. The library has the AMBE+2 frame FEC, and [vocoder/dvsi](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/vocoder/dvsi) uses a DVSI chip for the codec.
- The library has no I/O of its own for radios and networks. Your program reads and writes the sockets, serial ports and sample streams, and gives the bytes to the packages.
