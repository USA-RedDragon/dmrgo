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
  - `env CGO_ENABLED=0 go run -C cmd/speccheck . -update-readme` runs the tests that `spec_matrix.yaml` maps to each ETSI clause and writes the table of [ETSI coverage](../reference/etsi.md). `-check-readme` fails when the table is out of date, or when a relative link in `README.md` or `docs/` names a file or heading that does not exist.
  - `go run -C cmd/rxbench . -update-readme` runs the receiver sweep of `v2/rf` (about an hour on 32 cores) and `TestRxSheet`, which measures the eye diagrams, carrier offset, sample clock error, late entry and rate probe into `v2/rf/testdata/rxsheet.json` in about a minute, and writes the tables and graphs of [Receiver](../rx.md). `-sheet` runs only `TestRxSheet`, and `-no-sweep` neither. `-check-readme` fails when the page or a graph differs from the data in `v2/rf/testdata`.
  - `DMRRF_RECORDINGS=<dir> go test -timeout 0 -run '^TestRxBenchRecordings$' ./rf` in v2 runs I/Q tiers 0 to 3 on the real recordings and writes `v2/rf/testdata/rxbench_recordings.json`, the data of the recordings table of [Receiver](../rx.md). `<dir>` holds the `dmrgo-rfcap` and `ipsc2hbrp-rig` capture directories. The test needs `zstd`, and it is skipped when `DMRRF_RECORDINGS` is not set or the directory does not exist.
  - The Datasheets workflow (`.github/workflows/datasheet.yaml`) runs on each push to main that changes the receiver or the transmitter, and on demand. It runs the receiver sweep in one job per tier (`DMRRF_RXBENCH_TIERS` picks the tier). Each job must finish in less than 2 hours on the hosted runner, so the deep sweep of the slow tiers (`benchShards` in `v2/rf/rxbench_shard_test.go`) runs in more jobs: each job takes a part of the C/N points and stream batches, and `TestRxBenchShardPlan` makes sure that the parts cover each cell one time. The floors run in jobs by C/N range (`floorShards` in `v2/rf/floor_shard_test.go`). `TestRxFloorMerge` and `TestRxBenchMerge` in `v2/rf` merge the jobs into the same files as one unsharded sweep (`TestRxBenchMerge` keeps the committed CPU times), and then the workflow runs the generators below: the least sample rates, the Cortex-M4F cost, the tables and graphs of [Receiver](../rx.md) and [Transmitter](../tx.md) and the facts from the code. It commits the changed files with `[skip ci]`.
  - `go run -C cmd/mcubench . -update` measures the Tier 0 receivers and transmitters on a Cortex-M4F and writes `v2/rf/testdata/rxbench_mcu.json` and `v2/internal/fsktx/testdata/tier0cost.json`, which the pages of [Receiver](../rx.md) and [Transmitter](../tx.md) show. For a receiver row it runs each test signal of `TestMCUBenchSignals` in `v2/rf` through the bench, checks that the bursts match the Go receiver, and subtracts the cycles of a build that only reads the input. For each row it builds one bench program with TinyGo `-opt z -gc=none`, runs it in `qemu-system-arm -machine mps2-an386` with an instruction counter plugin, and takes the CPU from a cycle model of the counted instructions, the RAM from the static data and a painted stack, and the flash from the ELF. It needs TinyGo 0.44, the version of the TinyGo image of the CI (`tinygo` on PATH), `qemu-system-arm` with plugin support, a C compiler (`cc`) with `qemu-plugin.h`, and `pkg-config` with glib 2.0. `-check` measures again and fails when the data differs, and skips with the reason when a tool is missing.
  - `DMRGO_DOCPARAMS=write go test ./rf -run TestDocParams` in v2 writes the facts, limits, rates and test parameters of [Receiver](../rx.md) and [Transmitter](../tx.md) from the code of `rf` and `fsktx`. Without the variable the test fails when they differ.
  - `go test ./internal/fsktx -run TestTxDoc -update-txdoc` in v2 measures the transmitter margins into `v2/internal/fsktx/testdata/txmargins.json` and the spectra, eye diagrams and FSK error into `v2/internal/fsktx/testdata/txsheet.json`, and writes the tables and graphs of [Transmitter](../tx.md). Without the flag the test fails when the page or a graph differs from `minrate_gen.go` and that data. With `DMRGO_LONG=1` it measures the margins again and fails when they differ from the data.
  - `DMRGO_README=write go test -run TestHeapAllocations ./internal/allocgate` in v2 writes the table of [Allocation](../concepts/allocation.md), and `DMRGO_README=check` fails when it differs.
  - <!-- docgen:text text="`go run github.com/USA-RedDragon/docgen@{docgen} update`" -->`go run github.com/USA-RedDragon/docgen@v0.0.0-20261109224744-ef1962a8c996 update`<!-- /docgen:text --> writes the generated facts of every page under `docs/`. `check` fails and lists each stale marker. See [Generated facts](#generated-facts).

## Receiver genie harness

The genie harness in `v2/rf` (`genie_test.go`, `genie_scenes_test.go`) tells, for each burst on the air, if a receiver loss comes from the receiver or from the input. It runs the receivers of the selected tiers on a scene whose truth it knows: the time, talker, slot, bits and CACH of each burst, the clock rate and carrier of each talker, and the input level over time. A reference receiver then slices each burst at its true timing, clock rate, carrier and level, and again at the timing that the receiver stamped.

- `DMRGO_GENIE=list go test -run '^TestGenie$' -v ./rf` in v2 lists the scenes. Each scene maps to an item of the rf acceptance list and gives the question it answers.
- `DMRGO_GENIE='<regexp>' go test -run '^TestGenie$' -v ./rf` runs the scenes whose name matches. Add `DMRGO_TESTSEED=<seed>` to take the payloads and noise of a failing run, and `DMRGO_GENIE_ALL=1` to log the bursts that the receiver decodes too.
- Each `GENIE` line gives, for one tier, the count of each verdict and the readings of the receiver at the end of the input: offset, deviation, clock rate and the clock stretch of the lookback replay. The verdicts are:
  - `ok`: the receiver decodes the burst.
  - `phys`: the reference receiver does not decode it either. The input does not hold it.
  - `rx-miss`: the receiver gives no burst, and the reference decodes it.
  - `rx-time`: the reference decodes the burst at its true timing, but not at the timing of the receiver.
  - `rx-bits`: the reference decodes the burst at both timings, and the receiver does not.
  - `rx-cach`: the receiver decodes the burst with a wrong CACH, and the reference reads the CACH correctly.
  - `rx-raw`: the receiver decodes the burst, a voice burst by the Golay codes of its C0 and C1 words, but some of its raw bits differ from the burst on the air, and the reference slices every raw bit correctly. The matrix needs exact raw bits.
  - `extra`: the receiver gives a burst with no burst on the air within 1 ms.
  - `split`: the receiver gives one talker in more than one stream.
- `TestGenieKnownAnswer` checks the reference receiver on clean inputs of each front: audio, I/Q at 48 kHz and I/Q at 9600 Hz.
- To add a scene, build a `genieCase` from a matrix row (`genieRowCase`), from a bench stream (`genieBenchCase`, `genieBBCase`) or from I/Q bytes (`genieIQBytesCase`), and add it to `genieScenes`.

## Generated facts

Facts that come from the code go inside markers in the Markdown, so that the pages change when the code changes. Write the prose by hand and leave the marker body empty; [docgen](https://docgen.mcswain.dev/) fills it. Do not edit a marker body by hand.

```text
<!-- docgen:config pkg=csbk type=RequesterConfig -->
<!-- /docgen:config -->
```

`.docgen.yaml` at the repository root holds the settings: the pages are under `docs`, the module is `v2`, each go command runs with `-p=2`, and `docgen` is the version of docgen that the pages and the Docs workflow use. Package paths are relative to `v2`: `csbk`, `transport/dmrd/dmrdsession`, and `.` for the root package. The last elements of one package path are enough (`dmrdsession`). An argument with spaces takes double quotes: `section="Build tags"`.

The pages use these marker kinds. [Markers](https://docgen.mcswain.dev/latest/reference/markers) gives the arguments and the rules of each.

| Marker | Body |
|---|---|
| `docgen:output` | The standard output of `go run` on an example (`cmd=v2/examples/<dir>`), or the `// Output:` of a Go Example (`test=<pkg>.<ExampleName>`). |
| `docgen:code` | An excerpt of a Go, YAML or C file by `symbol=`, by `region=`, by `lines=`, or the whole file. |
| `docgen:config` | A table of the exported fields of a config struct: type, default and doc text. The default comes from the code that applies it. |
| `docgen:const` | Inline: the value of a constant. Add `print=raw` for a type with a String method when the page needs the number, such as a port. |
| `docgen:errors` | A table of the errors that the doc comments of the constructors and methods of a type name. |
| `docgen:states` | The states, the transitions seen in the tests and an SVG diagram in `states/` next to the page. |
| `docgen:doc` | A doc comment as Markdown, or one `# heading` section of it. |
| `docgen:gomod` | Inline: a line of `v2/go.mod`, or of the `go.mod` at `file=`. |
| `docgen:text` | Text with the values of `vars` in `.docgen.yaml`, such as the docgen version. |

docgen records the transitions of a state machine from its tests: a go build overlay adds a hook to the top of each exported method, and the hook records the state before and after each call. A transition that no test makes is not in the table, so a missing transition needs a test, not a hand edit.

`check` also fails for a `docgen:region` in the repository that no page excerpts, for a diagram in a `states/` directory that no marker writes, and for a link in `docs/` that reaches nothing.

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
