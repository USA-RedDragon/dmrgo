package main

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
)

func TestRun(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	if err := run(&buf, filepath.Join("..", "..", "layer2", "testdata")); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, want := range []string{
		"capture: 79 bursts (2 data, 77 voice), color code 1",
		"call: Unit to Unit Voice Channel User 3191868 -> 9990",
		"burst FEC: injected 67, corrected 67, uncorrectable false",
		"AMBE FEC: injected 77, corrected 80, uncorrectable false",
		"mmdvmhost accepts: 2/2 data bursts",
		"embedded LC: 13/13 match the header",
		"regenerated: 80 bursts from 231 AMBE frames, clean true, frames match true, 48 identical to capture",
		"rewrite target 9990 -> 91: accepted before seal false, after seal true, equals LCBytes.Codeword true",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
}
