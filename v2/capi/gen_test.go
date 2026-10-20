package main

import (
	"bytes"
	"os"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/capi/internal/capigen"
)

func TestGenerated(t *testing.T) {
	t.Parallel()

	lock, err := os.ReadFile("abi.txt")
	if err != nil {
		t.Fatal(err)
	}
	out, err := capigen.Generate(".", lock, capigen.Options{})
	if out != nil {
		for _, b := range out.Broken {
			t.Log("changed:", b)
		}
	}
	if err != nil {
		t.Fatal(err)
	}
	for name, want := range out.Files() {
		got, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, want) {
			t.Errorf("%s is out of date; run go generate ./... in the root module", name)
		}
	}
	mapped := 0
	for _, e := range out.Exports {
		if e.Err != "" {
			t.Errorf("%s %s has no C counterpart: %s", e.Kind, e.Go, e.Err)
			continue
		}
		mapped++
	}
	t.Logf("%d of %d exports mapped", mapped, len(out.Exports))
}
