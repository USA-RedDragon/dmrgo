# Allocation

The hot path does not allocate. After construction, decoding, encoding, building, receiving and assembling bursts and packets make no heap allocations, with gc (Go 1.24 or later) and with TinyGo. Errors are static values, so a failed call does not allocate either.

On TinyGo this holds only when the values that you pass by pointer are not local variables of your function. TinyGo moves such a local to the heap, 12 to 96 bytes per call, for `layer2.BuildBurst(&lc, ...)`, `packet.AppendBursts(dst, &p, ...)`, `layer2.VoiceTransmitter.Reset(&opts)` and `packet.Assembler.Add` of a local block. Keep those values in package-level variables or in a struct that you allocate once:

```go
var lc pdu.FullLinkControl // package level, not a local of send

func send() (layer2.RawBurst, error) {
	return layer2.BuildBurst(&lc, 1, layer2.SyncSourceBS)
}
```

The table lists every place where the library can allocate. [v2/internal/allocgate](../v2/internal/allocgate) generates it and fails when this page differs.

<!-- allocgate:begin -->

| What may allocate | When | Functions |
| --- | --- | ---: |
| The initialization of a package-level variable, such as a lookup table or a joined sentinel error | Once, when the program starts. | 7 |
| A String, GoString or Error method | When the text is built, for a person to read. | 296 |
| A function of internal/text, which formats the values of String and Error methods | When the text is built, for a person to read. | 7 |
| An exported package-level New function | Once, at construction: it allocates what it returns, sized up front. | 47 |
| An exported function or method that takes a slice dst and returns a slice of the same type | Only to grow dst when dst has no room. The caller sizes dst. A machine built from other machines may also pass each of them an unexported field sliced to [:0] that its New makes with capacity MaxX of that machine's package, whose fuzz test checks that bound. | 273 |
| An exported X with an exported twin AppendX or XInto that does not allocate | X allocates the slice, string or struct it returns; the twin writes into storage of the caller. | 9 |
| An exported X whose whole body is return Twin(nil, its parameters), where Twin allocates only to grow dst | X allocates the slice it returns; Twin with room in dst does not. | 3 |
| A type assertion or type switch to an interface type, and a goroutine that blocks | Once for each call site and dynamic type, as the runtime fills the cache of the assertion or switch, and when no sudog is free for a goroutine that blocks on a mutex or a channel. The runtime keeps both. | 4 |
| A method of an interface that the library declares for the caller to implement, such as the Read and Write of a dvsi.Port | When the method of the caller allocates. The code of the library on the path does not. | 2 |
| The runtime print functions, which the runtime calls on its debug paths | Never: they format into fixed buffers that the gate cannot prove are large enough. | 1 |

The counts are of exported functions and methods, for linux/amd64 with cgo. `DMRGO_README=write go test -run TestHeapAllocations ./internal/allocgate` in v2 writes the table.

<!-- allocgate:end -->

- The doc of each exported function that a rule covers gives when it allocates.
- Do not copy a machine or a `layer2.Burst` after first use. They keep pointers into their own storage, and `go vet` reports a copy.
- A `layer2.Burst` keeps its decoded `Data` until the next decode into it. `packet.Builder` and `packet.Assembler` keep their results the same way.
- Three checks keep this true, with no list of functions to maintain:
  - [v2/internal/allocgate](../v2/internal/allocgate) follows the machine code of each exported function to the heap allocator.
  - The same package adds each allocation site that `tinygo build -print-allocs` reports for the pico and for `riscv-qemu`.
  - [v2/internal/allocs](../v2/internal/allocs) measures allocations of the main calls with `testing.AllocsPerRun`, and under TinyGo with `runtime.MemStats`.
