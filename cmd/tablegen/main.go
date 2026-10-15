// Command tablegen computes the FEC and vocoder lookup tables from their
// definitions and writes them to the tables_gen.go file of each table
// package under v2.
//
// Usage:
//
//	go run -C cmd/tablegen .
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

	_, thisFile, _, _ := runtime.Caller(0) //nolint:dogsled
	v2Root := filepath.Join(filepath.Dir(thisFile), "..", "..", "v2")

	if err := generate.WriteAll(tables, v2Root); err != nil {
		fmt.Fprintf(os.Stderr, "generate failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("OK: generated files written")
}
