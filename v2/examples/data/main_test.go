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
		`d-sms.bin: Unconfirmed 3009999 -> 9990, Rate 1/2, 56 octets, CRC 0f89b330, text "TEST N0CALL"`,
		`h-sms.bin: Defined Short 3009999 -> 9990, Rate 1/2, 26 octets, CRC 5dc543c8, text "TEST N0CALL"`,
		`m-sms.bin: Unconfirmed 3009999 -> 9990, Rate 1/2, 62 octets, CRC 63bee077, text "TEST N0CALL"`,
		`unconfirmed: 6 bursts, BF=5 POC=9, port 5016, text "Hello from dmrgo, unconfirmed SMS test": ok`,
		"confirmed: 4 bursts, missing [1], SACK requests [1]",
		`confirmed: retry done, "LIP: 41.878,-87.629,hdg=90,v=12", ACK 3002702 -> 3002701: ok`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
}
