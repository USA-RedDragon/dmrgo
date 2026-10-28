# Tooling

- `go generate ./...` from the repository root runs these generators. Do not edit the files they write.
  - `cmd/tablegen` writes the FEC and CRC lookup tables (`tables_gen.go`).
  - `cmd/dmrgen` writes the decoder and encoder of each struct with `dmr` field tags (`*_decode_gen.go`).
  - The table tests of `v2/rf` and `v2/internal/fsktx` write their `tables_gen.go`.
  - `v2/internal/allocgate/apigen` writes the program that calls every export for the allocation gate (`allapi_gen.go`).
  - `v2/capi/internal/capigen` writes the C API of libdmrgo: the `*_gen*` files, `libdmrgo.map` and `abi.txt` in `v2/capi`.
  - wasm2go translates the WebAssembly build of libcodec2 to Go for the platforms where wazero has no compiler.
- Generated pages. Each generator has a check that fails when the page differs from what it writes:
  - `env CGO_ENABLED=0 go run -C cmd/speccheck . -update-readme` runs the tests that `spec_matrix.yaml` maps to each ETSI clause and writes the table of [ETSI coverage](etsi.md). `-check-readme` fails when the table is out of date.
  - `go run -C cmd/rxbench . -update-readme` runs the receiver sweep of `v2/rf` (about an hour on 32 cores) and writes the tables and graphs of [Receiver](rx/README.md). `-check-readme` fails when they differ from the sweep data in `v2/rf/testdata`.
  - `go test ./internal/fsktx -run TestTxDoc -update-txdoc` in v2 measures the transmitter margins into `v2/internal/fsktx/testdata/txmargins.json` and writes the tables of [Transmitter](tx/README.md). Without the flag the test fails when the tables differ from `minrate_gen.go` and that data. With `DMRGO_LONG=1` it measures the margins again and fails when they differ from the data.
  - `DMRGO_README=write go test -run TestHeapAllocations ./internal/allocgate` in v2 writes the table of [Allocation](allocation.md), and `DMRGO_README=check` fails when it differs.
