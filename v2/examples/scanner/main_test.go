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
		"TS1 late entry at voice burst",
		"TS1 call: private 3191868 -> 9990 (embedded LC)",
		"TS1 call end: private 3191868 -> 9990, 74 voice bursts",
		"TS2 call: group 3141592 -> 3100 (Voice LC Header)",
		`TS2 talker alias: "KF5XYZ Jacob"`,
		"TS2 GPS: lat 30.2672 lon -97.7431",
		"TS2 call end: group 3141592 -> 3100, 72 voice bursts",
		"framer: first burst at bit 349, 151 bursts",
		"TS2: 72 voice bursts (A=12 B=12 C=12 D=12 E=12 F=12 unknown=0), 0 embedded LC errors",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
	if strings.Count(out, "TS2 GPS:") != 1 {
		t.Errorf("GPS position printed more than once:\n%s", out)
	}
}
