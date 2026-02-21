// Command dmrgen generates decode/encode functions for DMR PDU structs
// that are annotated with dmr struct tags.
//
// Usage:
//
//	go run -C cmd/dmrgen .
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/USA-RedDragon/dmrgo/cmd/dmrgen/emit"
	"github.com/USA-RedDragon/dmrgo/cmd/dmrgen/parse"
)

func main() {
	// Resolve the code root from the location of this source file.
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		fmt.Fprintln(os.Stderr, "dmrgen: could not determine source file location")
		os.Exit(1)
	}
	repoRoot := filepath.Join(filepath.Dir(thisFile), "..", "..")
	v2Root := filepath.Join(repoRoot, "v2")

	err := filepath.Walk(v2Root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		// Only process .go files; skip tests and generated files.
		if !strings.HasSuffix(path, ".go") ||
			strings.HasSuffix(path, "_test.go") ||
			strings.HasSuffix(path, "_gen.go") {
			return nil
		}

		pdus, parseErr := parse.ParseFile(path)
		if parseErr != nil {
			return fmt.Errorf("parsing %s: %w", path, parseErr)
		}
		if len(pdus) == 0 {
			return nil
		}

		f, genErr := emit.GenerateFile(pdus, pdus[0].Package)
		if genErr != nil {
			return fmt.Errorf("generating for %s: %w", path, genErr)
		}

		base := strings.TrimSuffix(filepath.Base(path), ".go")
		outPath := filepath.Join(filepath.Dir(path), base+"_decode_gen.go")

		if saveErr := f.Save(outPath); saveErr != nil {
			return fmt.Errorf("writing %s: %w", outPath, saveErr)
		}

		fmt.Printf("dmrgen: generated %s (%d structs)\n", filepath.Base(outPath), len(pdus))
		return nil
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "dmrgen: %v\n", err)
		os.Exit(1)
	}
}
