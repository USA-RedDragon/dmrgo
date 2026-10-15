package main

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
)

func TestRun(t *testing.T) {
	var buf bytes.Buffer
	if err := run(&buf, filepath.Join("..", "..", "layer2", "testdata")); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, want := range []string{
		"voice in: 79 bursts, 231 slots, Unit to Unit Voice Channel User 3191868 -> 9990",
		"voice out: 80 bursts, 234 slots, Group Voice Channel User 3100999 -> 91",
		"ok header and terminator carry the new IDs and pass MMDVMHostAccepts",
		"ok embedded LC carries the new IDs in 13/13 superframes",
		"ok 231 voice slots forwarded unchanged, 3 silence slots of padding",
		"ok sms: 5 CSBK preambles and 1 data headers rewritten to 3100999 -> 91",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
}
