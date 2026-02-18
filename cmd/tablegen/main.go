// Command tablegen computes all FEC lookup tables from their mathematical
// definitions and writes Go source files.  It supports two modes:
//
//   - verify: compute tables and compare byte-for-byte against existing
//     hardcoded values (imported at compile time).  Exit non-zero on mismatch.
//   - generate: write *_tables_gen.go files with "// Code generated … DO NOT
//     EDIT." headers, ready to replace the hardcoded originals.
//
// Usage:
//
//	go run ./cmd/tablegen
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/USA-RedDragon/dmrgo/cmd/tablegen/compute"
	"github.com/USA-RedDragon/dmrgo/cmd/tablegen/generate"
)

func main() {
	tables := compute.All()

	// Resolve the dmr/ directory relative to this source file's location.
	_, thisFile, _, _ := runtime.Caller(0) //nolint:dogsled
	dmrRoot := filepath.Join(filepath.Dir(thisFile), "..", "..", "dmr")

	if err := generate.WriteAll(tables, dmrRoot); err != nil {
		fmt.Fprintf(os.Stderr, "generate failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("OK: generated files written")
}
