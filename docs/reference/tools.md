# Tools

This page lists the commands of the repository, what each one does, and its flags.

`v2/cmd/dmrrf` is a command for users of the library. The commands under `cmd/` build the library and its docs; each one is a module of its own, run from the root of a clone with `go run -C cmd/<name> .`. [Tooling](../project/tooling.md) shows when to run each generator and check.

## dmrrf

[dmrrf](../../v2/cmd/dmrrf/README.md) writes DMR test transmissions and decodes DMR from the samples of a software radio or an FM radio:

<!-- dmrgo:doc pkg=cmd/dmrrf -->
Command dmrrf builds DMR transmissions as I/Q or FM modulator audio and decodes DMR from I/Q or FM discriminator audio. It reads and writes files and pipes in the sample formats of common software radios; it does not drive a radio.
<!-- /dmrgo:doc -->

Build it from the `v2` directory with `go build -o dmrrf ./cmd/dmrrf`. It has three subcommands. Its README gives the sample formats, the sequences of `tx`, the output lines of `rx` and examples for common radios.

`dmrrf tx` writes a transmission as I/Q or modulator audio:

<!-- dmrgo:code file=v2/cmd/dmrrf/main.go lines=99-134 -->
```go
out := fs.String("o", "", "output file, or - for standard output")
format := fs.String("format", "", formatUsage)
seq := fs.String("seq", "data12", strings.Join(rftx.Sequences(), ", "))
cw := fs.Float64("cw", 0, "write an unmodulated carrier of this many seconds instead (I/Q only)")
rate := fs.Float64("rate", 0, "sample rate in Hz (default 48000 for audio; required for I/Q)")
tune := tuningFlags(fs, "carrier frequency in Hz, for -ppm and the example command", "frequency offset of the carrier from the tuned frequency in Hz (I/Q only)")
level := fs.Float64("level", 0.9, "peak amplitude, 0 to 1")
slot := fs.Int("slot", 0, "TDMA slot 0 or 1, or -1 for back-to-back bursts")
cc := enums.ColorCode(1)
fs.Func("cc", "color code, 0 to 15 (default 1)", func(s string) error {
	v, err := strconv.ParseUint(s, 10, 8)
	if err != nil {
		return err
	}
	cc, err = enums.NewColorCode(uint8(v))
	return err
})
src := idFlag(fs, "src", "source radio ID")
dst := idFlag(fs, "dst", "destination radio or talkgroup ID")
bs := idFlag(fs, "bs", "BS (repeater) address for the wakeup CSBK")
group := fs.Bool("group", false, "send to a talkgroup instead of a radio")
wakeups := fs.Int("wakeups", 3, "wakeup CSBKs before the sequence, or idle frames before a -bs-sourced stream")
msg := fs.String("msg", "DMRGO TEST", "message text")
repeat := fs.Float64("repeat", 0, "repeat the sequence, padded to whole 60 ms frames, for this many seconds")
stretch := fs.Float64("stretch", 0, "slow the symbol clock by this many ppm so repeats sweep across the repeater's slot timing")
superframes := fs.Int("superframes", 3, "voice superframes (bursts A to F) of the voice and pi sequences")
silence := fs.Bool("ambe-silence", false, "send AMBE+2 silence instead of the AMBE test pattern")
lateEntry := fs.Bool("late-entry", false, "leave out the voice LC and PI headers, so the call is known only from the embedded LC")
preambles := fs.Int("preambles", 4, "preamble CSBKs of the preamble and sms sequences")
dd := uint8(enums.DDUTF16LE)
fs.Func("dd", "Defined Data format of the sms sequence, 0 to 63; the text is always UTF-16LE (default 21, UTF-16LE)", func(s string) error {
	v, err := strconv.ParseUint(s, 10, 6)
	dd = uint8(v)
	return err
})
bsSourced := fs.Bool("bs-sourced", false, "send a continuous BS outbound stream (CACH, Short LC, idle bursts on the other slot) like a repeater")
```
<!-- /dmrgo:code -->

`dmrrf rx` decodes I/Q or discriminator audio:

<!-- dmrgo:code file=v2/cmd/dmrrf/main.go lines=510-516 -->
```go
in := fs.String("i", "", "input file, or - for standard input")
format := fs.String("format", "", formatUsage)
rate := fs.Float64("rate", 0, "sample rate in Hz (default from a WAV header)")
tune := tuningFlags(fs, "tuned frequency in Hz, for -ppm and the ppm figure", "frequency offset of the carrier from the tuned frequency in Hz, as with an offset tune (I/Q only)")
tier := fs.Int("tier", 3, "receiver tier 0 to 3: 0 discriminator, 1 with the soft stages, 2 coherent, 3 with the phase in the trellis state; audio defaults to the rf default")
ambeSpec := fs.String("ambe", "", ambeUsage)
pcmPath := fs.String("pcm", "", "write the decoded AMBE+2 voice as 8 kHz 16-bit little-endian PCM to this file, or - for standard output (needs -ambe)")
```
<!-- /dmrgo:code -->

`dmrrf cal` gives the frequency offset of an unmodulated carrier in I/Q:

<!-- dmrgo:code file=v2/cmd/dmrrf/main.go lines=607-610 -->
```go
in := fs.String("i", "", "input file of I/Q, or - for standard input")
format := fs.String("format", "", formatUsage)
rate := fs.Float64("rate", 0, "sample rate in Hz (default from a WAV header)")
tune := tuningFlags(fs, "frequency the carrier should be on, in Hz", "frequency offset of the carrier from the tuned frequency in Hz, as with an offset tune")
```
<!-- /dmrgo:code -->

## rxbench

`cmd/rxbench` writes the sensitivity tables and graphs of the [Receiver](../rx/README.md) datasheet from the sweep data of `v2/rf`. With `-update-readme` it first runs the long sweep, then writes the tables and graphs. With `-check-readme` it renders the committed data and fails when the page or a graph differs. Give one of the two.

<!-- dmrgo:code file=cmd/rxbench/main.go lines=40-42 -->
```go
update := flag.Bool("update-readme", false, "run the long sweep, then rewrite the block of docs/rx/README.md and the graphs")
check := flag.Bool("check-readme", false, "fail when the block of docs/rx/README.md or a graph differs from the committed data")
noSweep := flag.Bool("no-sweep", false, "with -update-readme, render the committed data without the sweep")
```
<!-- /dmrgo:code -->

## rxsweep

`cmd/rxsweep` measures many variants of the receiver of `v2/rf` and finds the Pareto front of sensitivity against cost. A space file lists the variants. `rxsweep run` measures each variant and appends the results to an output directory, so a run that stops continues where it was. `rxsweep report` reads the results and writes the tables, the Pareto fronts and the graphs.

The flags of `rxsweep run`:

<!-- dmrgo:code file=cmd/rxsweep/main.go lines=64-69 -->
```go
space := fs.String("space", "", "space file of the variants")
out := fs.String("out", "", "output directory")
jobs := fs.Int("jobs", 2, "worker processes at once; each gets an equal share of the cores")
only := fs.String("only", "", "comma-separated variant names to run; all when empty")
noCost := fs.Bool("no-cost", false, "skip the cost phase")
m4 := fs.String("m4", "", "command that measures a variant on a Cortex-M4F: it gets the patched v2 directory and the variant JSON file, and prints one JSON object")
```
<!-- /dmrgo:code -->

The flags of `rxsweep report`:

<!-- dmrgo:code file=cmd/rxsweep/main.go lines=88-89 -->
```go
out := fs.String("out", "", "output directory of rxsweep run")
floor := fs.String("floor", "", "floor file; default v2/rf/testdata/rxbench_floor.json of this checkout")
```
<!-- /dmrgo:code -->

## speccheck

`cmd/speccheck` checks `spec_matrix.yaml` against the code. It runs the tests that the file lists for each clause of ETSI TS 102 361 and gives each clause a status: verified, failing, untested or unimplemented. It also checks that each Go file under `v2` that is not a test and not generated is in the file. With `-update-readme` it writes the table of [ETSI coverage](etsi.md). With `-check-readme` it fails when that table is out of date, or when a relative link in `README.md` or `docs/` names a file or heading that does not exist.

<!-- dmrgo:code file=cmd/speccheck/main.go lines=105-106 -->
```go
updateReadme := flag.Bool("update-readme", false, "Write the Markdown coverage report into docs/reference/etsi.md between the ETSI COVERAGE markers")
checkReadme := flag.Bool("check-readme", false, "Fail if the coverage section in docs/reference/etsi.md is not up to date or a relative link in README.md or docs/ is broken")
```
<!-- /dmrgo:code -->

## tablegen

`cmd/tablegen` computes the FEC and vocoder lookup tables from their definitions and writes the `tables_gen.go` file of each table package under `v2`. It has no flags.

## dmrgen

`cmd/dmrgen` writes the decode and encode functions of each PDU struct that has `dmr` field tags, into the `*_decode_gen.go` files. It has no flags.

## docgen

`cmd/docgen` fills the generated parts of the pages under `docs/` from the code of `v2`: example output, code excerpts, constants, config tables, error tables, state tables and doc comments. Give one of `-update` and `-check`.

<!-- dmrgo:code file=cmd/docgen/main.go lines=23-26 -->
```go
update := fl.Bool("update", false, "rewrite the body of each marker in docs/**/*.md")
check := fl.Bool("check", false, "exit 1 and list each marker whose body differs from what docgen writes")
root := fl.String("root", "", "repository root (default: two directories above the docgen source)")
docs := fl.String("docs", "docs", "directory of the Markdown pages, relative to the root")
```
<!-- /dmrgo:code -->
