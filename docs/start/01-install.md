# Install

This step adds dmrgo to a Go module and gets a clone of the repository, so that you can run the code of the next steps.

## What you need

- Go <!-- dmrgo:gomod field=go -->`1.24.0`<!-- /dmrgo:gomod --> or later, the version of the `go` line of the module.
- No C compiler. The library is pure Go. A C compiler is necessary only for two optional parts: the system libcodec2 backend of `vocoder/codec2` (build tag `codec2native`) and the C API in `v2/capi`.

## Add the module to your program

The import path of the module is `github.com/USA-RedDragon/dmrgo/v2`. Each package is under it, such as `github.com/USA-RedDragon/dmrgo/v2/layer2`.

```sh
go mod init example.com/dmrdemo
go get github.com/USA-RedDragon/dmrgo/v2
```

## Get the captures

The steps of this guide run on captures of real radios that the repository holds in its `testdata` directories. Clone the repository to get them:

```sh
git clone https://github.com/USA-RedDragon/dmrgo.git
cd dmrgo/v2
```

Each step shows a Go example function of the library. `go test` runs each example and compares its output with the output that the step shows. To run the example of a step, use the `go test` command that the step gives, from the `v2` directory.

## Check the setup

The root package `dmrgo` holds only the overview of the module and its examples. Run its first example. It decodes one burst that the source code holds as bytes:

```sh
go test -run '^Example$' -v .
```

The example prints:

<!-- dmrgo:output test=.Example -->
```text
Voice LC Header <nil>
call from 3191868 to 9990
true <nil>
```
<!-- /dmrgo:output -->

The [package documentation](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2) lists every package. [The layer model](../concepts/layers.md) shows how the packages fit together.

Next: [Decode a burst](02-decode-a-burst.md).
