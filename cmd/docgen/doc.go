// Command docgen fills the generated parts of the Markdown pages under docs/
// from the code of v2. A page marks each generated part with an open and a
// close comment:
//
//	<!-- dmrgo:const pkg=csbk name=DefaultIdleSearch --><!-- /dmrgo:const -->
//
// docgen writes the body between the two comments. Prose stays outside the
// markers and docgen does not change it.
//
// Usage:
//
//	go run -C cmd/docgen . -update
//	go run -C cmd/docgen . -check
//
// -update rewrites the body of every marker. -check rewrites nothing, lists
// each marker whose body differs from what -update writes, and exits 1 when
// there is one. Both exit 1 on an error, for example a marker that names a
// type that does not exist. Package paths are relative to v2 ("csbk",
// "transport/dmrd/dmrdsession"; "." is the root package).
//
// # Marker kinds
//
//   - dmrgo:output example=<dir> runs go run ./examples/<dir> in v2 and
//     writes its standard output.
//   - dmrgo:output test=<pkg>.<ExampleName> runs the Example with go test
//     and writes its // Output: comment. "<pkg>.Example" is the package
//     Example.
//   - dmrgo:code file=<repo path> symbol=<Name> writes one func, method
//     (Type.Method), type, or const or var block, without its doc comment
//     and, for an Example, without its output comment.
//   - dmrgo:code file=<YAML file> symbol=<key.key> writes the block of
//     the key at that dotted path, such as jobs.test of a workflow. A last
//     segment key[A:B] writes the items of the list at key from the item
//     with name A to the item with name B, and key[A] the one item A.
//   - dmrgo:code file=<C file> symbol=<Name> writes the #define of Name or
//     the definition of the function Name.
//   - dmrgo:code file=<repo path> writes the whole file.
//   - dmrgo:code file=<repo path> lines=<a>-<b> writes those lines of the
//     file. For a Go file the lines must hold whole declarations or
//     statements. When the file changes, -check and -update fail: they
//     give the new range when the old excerpt moved, and otherwise ask for
//     a check of the range.
//   - dmrgo:config pkg=<path> type=<Config> writes a table of the exported
//     fields: name, type, default and doc text.
//   - dmrgo:const pkg=<path> name=<Name> writes the value of a constant, or
//     of a package-level var with a constant initial value, exported or not.
//     A time.Duration prints as Go prints it (60ms, 2m0s).
//   - dmrgo:errors pkg=<path> type=<Type> writes a table of the errors that
//     the doc comments of the constructors and methods of the type name.
//   - dmrgo:states pkg=<path> type=<Type> writes the states and the
//     transitions of a state machine and an SVG diagram of them.
//   - dmrgo:doc pkg=<path> [name=<Name>] [section=<heading>] writes a doc
//     comment as Markdown: the package doc, or the doc of a type, func,
//     method (Type.Method), const or var, or one "# heading" section of it.
//     A pkg= that names a directory outside v2, such as cmd/rxbench, reads
//     the package doc of that directory.
//   - dmrgo:gomod field=<go|toolchain|module> [file=<repo path>] writes
//     that line of v2/go.mod, or of the go.mod at file=.
//   - dmrgo:flags cmd=<dir> runs go run on the command in that directory
//     (relative to the repository root) with -h and writes the usage.
//
// Arguments that hold spaces take double quotes: section="Build tags".
//
// # Defaults
//
// The Default column of dmrgo:config comes from the code that applies the
// default, not from the doc text. docgen follows each config field through
// plain copies in the package: assignments, composite literal keys and
// arguments of calls to functions of the package. It then looks for an
// if statement or a tagless switch case that tests a copy for 0, "" or nil
// and assigns a value to a copy in its body. That value, and its constant
// value when it has one, is the default. When docgen finds no such code and
// the doc text states a default ("0 means", "nil means"), the column says
// that the default is not machine-readable. Otherwise the column says zero
// value.
//
// # States and transitions
//
// The states are the constants of the state type, in value order, with
// their doc comments. The type= of the marker is the machine type, when it
// has a State method that returns an integer type, or the state type, when
// one method of one type of the package returns it. That method can take
// one argument; docgen then reads the state for each exported constant of
// the argument type (bs.Repeater.SlotState for each enums.Slot). A machine
// with no State method gets its state from its exported methods that take
// no argument and return a bool or a (value, bool), and from Len() (0 or
// not).
//
// The transitions come from the tests, not from the source text. docgen
// compiles the package tests and Examples with a go build overlay that
// adds one statement to the top of each exported method of the machine
// type:
//
//	if __dgInT(r) { defer __dgOutT(r, "Method") }
//
// and one file with the hook functions. The hook reads the state when the
// outer method call starts and again when it returns, and when they differ
// it records the pair of states and the method name. Calls that a method
// makes to other exported methods of the same machine count as part of the
// outer call. The overlay exists only in artifacts/docgen and only for that
// go test run, so the library source and its builds do not change.
//
// This keeps the table true when the code changes: a new state, a new
// transition or a method that no longer moves the machine shows up in the
// next run, and -check fails until the page is updated. A transition that
// no test makes is not in the table, and the state table says which states
// no test reached. Tests that fail only because of the hook (for example
// allocation counts) are listed in the log, and the transitions they made
// before they failed still count.
//
// The diagram has one box per state in value order. A transition to a
// later state is an arc above the boxes and a transition to an earlier
// state is an arc below them, each labeled with the method names. The
// diagram is an SVG file next to the page, in states/<pkg>-<type>.svg,
// so pandoc and GitHub render it without JavaScript.
//
// # Environment
//
// docgen runs go with GOPROXY=off, GOFLAGS=-p=2 and GOMAXPROCS=2, so it
// needs the module cache and no network.
package main
