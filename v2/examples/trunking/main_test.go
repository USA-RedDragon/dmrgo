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
		"outbound CSBKs: 7 round trips ok",
		"C_ALOHA SYScode 0x6acd: model 1 net 85 site 19 par 1",
		"Burst.Encode matches BuildBurst",
		"TD_GRANT_MI ch 77 tg 9 src 3120001",
		"inbound C_RAND PDU ok, sync MS-sourced data",
		"ch 0xFFF tg 9: TX 451.1250 MHz RX 456.1250 MHz",
		"CACH Short LC: 8 C_SYS_Parms/P_SYS_Parms round trips ok",
		"all checks passed",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
}
