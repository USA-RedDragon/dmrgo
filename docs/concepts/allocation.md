# Allocation

This page states where dmrgo allocates heap memory, how to call it so that TinyGo does not allocate in your code, and which tests hold the rule.

## The rule

After construction, the hot path makes no heap allocations, with gc and with TinyGo. The hot path is each call that decodes, encodes, builds, receives or assembles bursts and packets. Construction is the New function of a type, such as [layer2.NewBurstFromBytes](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/layer2#NewBurstFromBytes), [voice.NewReceiver](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/voice#NewReceiver) or [packet.NewAssembler](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/layer2/packet#NewAssembler). Each error is a static value, so a call that fails does not allocate. See [Errors and the never-panic rule](errors.md).

The doc comment of each exported function states when that function allocates.

## Size the slices that you pass

A state machine appends its outputs to a slice `dst` that you give it, and allocates only to grow `dst`. Make `dst` once, with the capacity that its package gives, and pass it again sliced to `[:0]` on each call. The [voice.Receiver](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/voice#Receiver) of [Follow a voice call](../start/03-follow-a-voice-call.md) does this:

<!-- dmrgo:code file=v2/voice/example_test.go lines=27-27 -->
<!-- /dmrgo:code -->

[voice.MaxEvents](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/voice#MaxEvents) is <!-- dmrgo:const pkg=voice name=MaxEvents --><!-- /dmrgo:const -->. A slice with that capacity does not grow. The other machines have the same constants, such as `data.MaxEvents` and `data.MaxOutputs`.

A function that returns a new slice or string allocates it. Each such function has a twin that writes into storage that you give it, such as an `AppendX` or `XInto` form.

## Keep pointer arguments out of local variables on TinyGo

TinyGo moves a local variable to the heap when its address goes into the library. This occurs for these calls:

- the `Data` of [layer2.BuildBurst](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/layer2#BuildBurst);
- the `Packet` of [packet.AppendBursts](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/layer2/packet#AppendBursts);
- the options of [layer2.VoiceTransmitter.Reset](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/layer2#VoiceTransmitter.Reset);
- a block for [packet.Assembler.Add](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/layer2/packet#Assembler.Add).

Put such a value in a package-level variable, or in a struct that you allocate once. This excerpt keeps the Full LC of `layer2.BuildBurst` at package level:

<!-- dmrgo:code file=v2/layer2/example_test.go lines=195-199 -->
<!-- /dmrgo:code -->

Under TinyGo, the test `TestTinyGoCallerStorage` in [v2/internal/allocs](../../v2/internal/allocs) holds this rule. It logs the bytes per call that each of these calls allocates when its value is a local.

## Do not copy machines and bursts

Do not copy a state machine or a [layer2.Burst](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/layer2#Burst) after its first use. Each one holds pointers into its own storage, and `go vet` reports a copy. A `layer2.Burst` keeps its decoded `Data` until the next decode into it. [packet.Builder](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/layer2/packet#Builder) and [packet.Assembler](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/layer2/packet#Assembler) keep their results in the same way.

## What may allocate

The table lists every place where the library can allocate. [v2/internal/allocgate](../../v2/internal/allocgate) generates it, and fails when this page differs.

<!-- allocgate:begin -->

| What may allocate | When | Functions |
| --- | --- | ---: |
| The initialization of a package-level variable, such as a lookup table or a joined sentinel error | Once, when the program starts. | 7 |
| A String, GoString or Error method | When the text is built, for a person to read. | 296 |
| A function of internal/text, which formats the values of String and Error methods | When the text is built, for a person to read. | 7 |
| An exported package-level New function | Once, at construction: it allocates what it returns, sized up front. | 48 |
| An exported function or method that takes a slice dst and returns a slice of the same type | Only to grow dst when dst has no room. The caller sizes dst. A machine built from other machines may also pass each of them an unexported field sliced to [:0] that its New makes with capacity MaxX of that machine's package, whose fuzz test checks that bound. | 273 |
| An exported X with an exported twin AppendX or XInto that does not allocate | X allocates the slice, string or struct it returns; the twin writes into storage of the caller. | 9 |
| An exported X whose whole body is return Twin(nil, its parameters), where Twin allocates only to grow dst | X allocates the slice it returns; Twin with room in dst does not. | 3 |
| A type assertion or type switch to an interface type, and a goroutine that blocks | Once for each call site and dynamic type, as the runtime fills the cache of the assertion or switch, and when no sudog is free for a goroutine that blocks on a mutex or a channel. The runtime keeps both. | 4 |
| A method of an interface that the library declares for the caller to implement, such as the Read and Write of a dvsi.Port | When the method of the caller allocates. The code of the library on the path does not. | 2 |
| The runtime print functions, which the runtime calls on its debug paths | Never: they format into fixed buffers that the gate cannot prove are large enough. | 1 |

The counts are of exported functions and methods, for linux/amd64 with cgo. `DMRGO_README=write go test -run TestHeapAllocations ./internal/allocgate` in v2 writes the table.

<!-- allocgate:end -->

## The checks

Three checks hold the rule. None of them has a list of functions to keep up to date.

- [v2/internal/allocgate](../../v2/internal/allocgate) follows the machine code of each exported function to the heap allocator of the runtime.
- The same package adds each allocation site that `tinygo build -print-allocs` reports for the pico and for `riscv-qemu`.
- [v2/internal/allocs](../../v2/internal/allocs) counts the allocations of the main calls with `testing.AllocsPerRun`, and under TinyGo with `runtime.MemStats`.

[Tooling](../tooling.md) gives the commands that write and check the table.

Next: [Tiers](tiers.md).
