// Package allocgate holds the tests that fail when the machine code of the
// library can reach the heap allocator outside the rules that let it
// allocate. The gate disassembles the export data of the library and of its
// dependencies with go tool objdump, builds the call graph from the
// relocations, and follows each exported function and method to the
// allocator of the runtime. Under TinyGo it adds the allocation sites that
// tinygo build -print-allocs reports for allapi, a program that references
// the whole API. The rules are patterns, such as an exported New function or
// a function that takes a slice dst and allocates only to grow it, which
// the gate proves from the source, and the README lists them with what
// they allow.
package allocgate
