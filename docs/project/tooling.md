# Tooling

This page lists the generators of the repository and the checks of the generated files and pages. [Tools](../reference/tools.md) gives the flags of each command.

- `go generate ./...` from the repository root runs these generators. Do not edit the files they write.
  - `cmd/tablegen` writes the FEC and CRC lookup tables (`tables_gen.go`).
  - `cmd/dmrgen` writes the decoder and encoder of each struct with `dmr` field tags (`*_decode_gen.go`).
  - The table tests of `v2/rf` and `v2/internal/fsktx` write their `tables_gen.go`.
  - `v2/internal/allocgate/apigen` writes the program that calls every export for the allocation gate (`allapi_gen.go`).
  - `v2/capi/internal/capigen` writes the C API of libdmrgo: the `*_gen*` files, `libdmrgo.map` and `abi.txt` in `v2/capi`.
  - wasm2go translates the WebAssembly build of libcodec2 to Go for the platforms where wazero has no compiler.
- Generated pages. Each generator has a check that fails when the page differs from what it writes:
  - `env CGO_ENABLED=0 go run -C cmd/speccheck . -update-readme` runs the tests that `spec_matrix.yaml` maps to each ETSI clause and writes the table of [ETSI coverage](../etsi.md). `-check-readme` fails when the table is out of date, or when a relative link in `README.md` or `docs/` names a file or heading that does not exist.
  - `go run -C cmd/rxbench . -update-readme` runs the receiver sweep of `v2/rf` (about an hour on 32 cores) and writes the tables and graphs of [Receiver](../rx/README.md). `-check-readme` fails when they differ from the sweep data in `v2/rf/testdata`.
  - `DMRRF_RECORDINGS=<dir> go test -timeout 0 -run '^TestRxBenchRecordings$' ./rf` in v2 runs I/Q tiers 0 to 3 on the real recordings and writes `v2/rf/testdata/rxbench_recordings.json`, the data of the recordings table of [Receiver](../rx/README.md). `<dir>` holds the `dmrgo-rfcap` and `ipsc2hbrp-rig` capture directories. The test needs `zstd`, and it is skipped when `DMRRF_RECORDINGS` is not set or the directory does not exist.
  - `go test ./internal/fsktx -run TestTxDoc -update-txdoc` in v2 measures the transmitter margins into `v2/internal/fsktx/testdata/txmargins.json` and writes the tables of [Transmitter](../tx/README.md). Without the flag the test fails when the tables differ from `minrate_gen.go` and that data. With `DMRGO_LONG=1` it measures the margins again and fails when they differ from the data.
  - `DMRGO_README=write go test -run TestHeapAllocations ./internal/allocgate` in v2 writes the table of [Allocation](../allocation.md), and `DMRGO_README=check` fails when it differs.
  - `go run -C cmd/docgen . -update` writes the generated facts of every page under `docs/`. `-check` fails and lists each stale marker. See [Generated facts](#generated-facts).

## Generated facts

Facts that come from the code go inside markers in the Markdown, so that the pages change when the code changes. Write the prose by hand and leave the marker body empty; `cmd/docgen` fills it. Do not edit a marker body by hand.

```text
<!-- dmrgo:config pkg=csbk type=RequesterConfig -->
<!-- /dmrgo:config -->
```

Package paths are relative to `v2`: `csbk`, `transport/dmrd/dmrdsession`, and `.` for the root package. The last elements of one package path are enough (`dmrdsession`). An argument with spaces takes double quotes: `section="Build tags"`.

| Marker | Arguments | Body |
|---|---|---|
| `dmrgo:output` | `example=<dir under v2/examples>` | The standard output of `go run` on the example. |
| `dmrgo:output` | `test=<pkg>.<ExampleName>` | The `// Output:` of a Go Example, after `go test` runs it. `<pkg>.Example` is the package Example. |
| `dmrgo:code` | `file=<repo path> symbol=<Name>` | One func, method (`Type.Method`), type, or const or var block, without its doc comment. An Example has no `// Output:` comment in the excerpt. |
| `dmrgo:code` | `file=<YAML file> symbol=<key.key>` | The block of the key at that dotted path, such as `jobs.test` of a workflow. A last segment `key[A:B]` gives the items of the list at `key` from the item named `A` to the item named `B`, and `key[A]` the one item `A`. Quote a value that has spaces. |
| `dmrgo:code` | `file=<C file> symbol=<Name>` | The `#define` of `Name`, or the definition of the function `Name`. |
| `dmrgo:code` | `file=<repo path> region=<name>` | The lines between the line that holds `docgen:region <name>` and the next line that holds `docgen:end`, for example a part of a comment. Other excerpts of the file leave out those two lines. |
| `dmrgo:code` | `file=<repo path>` | The whole file. |
| `dmrgo:code` | `file=<repo path> lines=<a>-<b>` | Those lines. For Go, the lines must hold whole declarations or statements. It fails when the file changes under the excerpt, so prefer `symbol=`. |
| `dmrgo:config` | `pkg=<path> type=<Config>` | A table of the exported fields: type, default and doc text. The default comes from the code that applies it. |
| `dmrgo:const` | `pkg=<path> name=<Name>` | Inline: the value of a constant, or of a package-level var with a constant value, exported or not. Durations print as Go prints them (`60ms`, `2m0s`). |
| `dmrgo:errors` | `pkg=<path> type=<Type>` | A table of the errors that the doc comments of the constructors and methods of the type name. |
| `dmrgo:states` | `pkg=<path> type=<Type>` | The states, the transitions seen in the tests and an SVG diagram in `states/` next to the page. `type` is the machine type or its state type (`bs` `SlotState`). |
| `dmrgo:doc` | `pkg=<path>` and optional `name=<Name>`, `section=<heading>` | The package doc, or the doc of a type, func, method, const or var, as Markdown, or one `# heading` section of it. A `pkg` outside v2, such as `cmd/rxbench`, is a directory from the repository root. |
| `dmrgo:flags` | `cmd=<dir>` | The usage that the command in that directory prints for `-h`. |
| `dmrgo:gomod` | `field=go`, `toolchain` or `module`, optional `file=<repo path>` | Inline: that line of `v2/go.mod`, or of the `go.mod` at `file=`. `field=go` is the minimum Go version. |

docgen records the transitions of a state machine from its tests: a go build overlay adds a hook to the top of each exported method, and the hook records the state before and after each call. The package doc of `cmd/docgen` (`go doc` in `cmd/docgen`) gives the details. A transition that no test makes is not in the table, so a missing transition needs a test, not a hand edit.

docgen needs the Go module cache and no network. It runs the go commands with `-p=2` and `GOMAXPROCS=2`. `go test` in `cmd/docgen` runs it on a fixture page that uses every marker kind.

## Docs site

`docs/site/build.sh` builds an HTML site from `README.md` and `docs/` into `artifacts/docs-site/` (git ignores `artifacts/`). It needs pandoc 3.11 and no Go. Run it from any directory:

```sh
docs/site/build.sh
```

Open `artifacts/docs-site/index.html` in a browser. A path as the first argument sets another output directory. The site works from the file system and needs no server.

- `docs/nav.md` holds the sections and pages of the site in reading order. `README.md` is the Overview. A page that `docs/nav.md` names by path in code is planned: the build log lists it as not yet written, and the navigation leaves it out. A comment `<!-- old: <path> -->` after a link gives the path of a page before it moved; the build uses the old file until the new one exists.
- The build log also lists each Markdown file under `docs/` that `docs/nav.md` does not name. The build makes a page for it, but the navigation does not show it.
- Each page has the sections in a left navigation, an "On this page" list of its headings, links to the previous and next page in outline order, and a § link on each heading.
- The search box finds pages by title, heading and text. The build writes the index to `search-index.js`, and a short script in each page searches it in the browser. No other service is used.
- pandoc reads each page as GitHub-flavored Markdown (`-f gfm`), so tables, code blocks and heading anchors match GitHub.
- `docs/site/template.html` is the page around each doc. `docs/site/nav.lua` reads `docs/nav.md`, `docs/site/pages.lua` lists the pages to build, `docs/site/site.lua` is the pandoc filter of each page, and `docs/site/index.lua` writes the search index. The filter rewrites links to other docs as links to their pages, links to other files of the repository as GitHub links, and copies each image into the output.
