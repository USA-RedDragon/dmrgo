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
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		fmt.Fprintln(os.Stderr, "dmrgen: could not determine source file location")
		os.Exit(1)
	}
	repoRoot := filepath.Join(filepath.Dir(thisFile), "..", "..")
	v2Root := filepath.Join(repoRoot, "v2")

	if err := run(v2Root); err != nil {
		fmt.Fprintf(os.Stderr, "dmrgen: %v\n", err)
		os.Exit(1)
	}
}

type sourceFile struct {
	path  string
	start int
	n     int
}

func run(v2Root string) error {
	var files []sourceFile
	pkgs := map[string][]parse.PDUStruct{}
	err := filepath.Walk(v2Root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".go") ||
			strings.HasSuffix(path, "_test.go") ||
			strings.HasSuffix(path, "_gen.go") {
			return nil
		}
		pdus, parseErr := parse.File(path)
		if parseErr != nil {
			return fmt.Errorf("parsing %s: %w", path, parseErr)
		}
		if len(pdus) == 0 {
			return nil
		}
		dir := filepath.Dir(path)
		files = append(files, sourceFile{path: path, start: len(pkgs[dir]), n: len(pdus)})
		pkgs[dir] = append(pkgs[dir], pdus...)
		return nil
	})
	if err != nil {
		return err
	}
	for _, pdus := range pkgs {
		parse.SetUnexportedDecode(pdus)
	}
	for _, sf := range files {
		pdus := pkgs[filepath.Dir(sf.path)][sf.start : sf.start+sf.n]
		f, genErr := emit.GenerateFile(pdus, pdus[0].Package)
		if genErr != nil {
			return fmt.Errorf("generating for %s: %w", sf.path, genErr)
		}
		base := strings.TrimSuffix(filepath.Base(sf.path), ".go")
		outPath := filepath.Join(filepath.Dir(sf.path), base+"_decode_gen.go")
		if saveErr := f.Save(outPath); saveErr != nil {
			return fmt.Errorf("writing %s: %w", outPath, saveErr)
		}
		testPath := filepath.Join(filepath.Dir(sf.path), base+"_string_gen_test.go")
		if saveErr := emit.GenerateTestFile(pdus, pdus[0].Package).Save(testPath); saveErr != nil {
			return fmt.Errorf("writing %s: %w", testPath, saveErr)
		}
		fmt.Printf("dmrgen: generated %s (%d structs)\n", filepath.Base(outPath), len(pdus))
	}
	return nil
}
