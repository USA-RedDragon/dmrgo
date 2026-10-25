// Package dmrgo holds the go:generate directives of the repository: go
// generate ./... runs tablegen and then dmrgen, which write the _gen.go
// files of the v2 module, and then apigen, which writes the allapi program
// of the allocation gate from the exports of the library, capigen, which
// writes the C API of libdmrgo from the same exports, and wasm2go, which
// translates the embedded WebAssembly build of libcodec2 to Go for the
// platforms where wazero has no compiler.
package dmrgo

//go:generate go run -C cmd/tablegen .
//go:generate go run -C cmd/dmrgen .
//go:generate go run -C v2 ./internal/allocgate/apigen/gen
//go:generate go run -C v2/capi ./internal/capigen/gen
//go:generate go run github.com/ncruces/wasm2go@v0.4.17 -pkg aot -o v2/vocoder/codec2/internal/aot/codec2.go v2/vocoder/codec2/wasm/codec2.wasm
