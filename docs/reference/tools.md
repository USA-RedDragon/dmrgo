# Tools

This page lists the commands of the repository, what each one does, and its flags.

`v2/cmd/dmrrf` is a command for users of the library. The commands under `cmd/` build the library and its docs; each one is a module of its own, run from the root of a clone with `go run -C cmd/<name> .`. [Tooling](../project/tooling.md) shows when to run each generator and check.

## dmrrf

[dmrrf](../../v2/cmd/dmrrf/README.md) writes DMR test transmissions and decodes DMR from the samples of a software radio or an FM radio:

<!-- dmrgo:doc pkg=cmd/dmrrf -->
Command dmrrf builds DMR transmissions as I/Q or FM modulator audio and decodes DMR from I/Q or FM discriminator audio. It reads and writes files and pipes in the sample formats of common software radios; it does not drive a radio.
<!-- /dmrgo:doc -->

Build it from the `v2` directory with `go build -o dmrrf ./cmd/dmrrf`. It has three subcommands. Its README gives the sample formats, the sequences of `tx`, the output lines of `rx` and examples for common radios.

`dmrrf tx` writes a transmission as I/Q or modulator audio. `dmrrf tx -h` lists its flags.

`dmrrf rx` decodes I/Q or discriminator audio. `dmrrf rx -h` lists its flags.

`dmrrf cal` gives the frequency offset of an unmodulated carrier in I/Q. `dmrrf cal -h` lists its flags.

## rxbench

`cmd/rxbench` writes the sensitivity tables and graphs of the [Receiver](../rx/README.md) datasheet from the sweep data of `v2/rf`. With `-update-readme` it first runs the long sweep, then writes the tables and graphs. With `-check-readme` it renders the committed data and fails when the page or a graph differs. Give one of the two.

## rxsweep

`cmd/rxsweep` measures many variants of the receiver of `v2/rf` and finds the Pareto front of sensitivity against cost. A space file lists the variants. `rxsweep run` measures each variant and appends the results to an output directory, so a run that stops continues where it was. `rxsweep report` reads the results and writes the tables, the Pareto fronts and the graphs.

`rxsweep run -h` and `rxsweep report -h` list the flags of each subcommand.

## speccheck

`cmd/speccheck` checks `spec_matrix.yaml` against the code. It runs the tests that the file lists for each clause of ETSI TS 102 361 and gives each clause a status: verified, failing, untested or unimplemented. It also checks that each Go file under `v2` that is not a test and not generated is in the file. With `-update-readme` it writes the table of [ETSI coverage](../etsi.md). With `-check-readme` it fails when that table is out of date, or when a relative link in `README.md` or `docs/` names a file or heading that does not exist.

## tablegen

`cmd/tablegen` computes the FEC and vocoder lookup tables from their definitions and writes the `tables_gen.go` file of each table package under `v2`. It has no flags.

## dmrgen

`cmd/dmrgen` writes the decode and encode functions of each PDU struct that has `dmr` field tags, into the `*_decode_gen.go` files. It has no flags.

## docgen

`cmd/docgen` fills the generated parts of the pages under `docs/` from the code of `v2`: example output, code excerpts, constants, config tables, error tables, state tables and doc comments. Give one of `-update` and `-check`.

<!-- dmrgo:code file=cmd/docgen/main.go symbol=defineFlags -->
```go
func defineFlags(fl *flag.FlagSet) options {
	return options{
		update: fl.Bool("update", false, "rewrite the body of each marker in docs/**/*.md"),
		check:  fl.Bool("check", false, "exit 1 and list each marker whose body differs from what docgen writes"),
		root:   fl.String("root", "", "repository root (default: two directories above the docgen source)"),
		docs:   fl.String("docs", "docs", "directory of the Markdown pages, relative to the root"),
	}
}
```
<!-- /dmrgo:code -->
