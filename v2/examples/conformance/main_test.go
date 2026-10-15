package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
)

func TestRun(t *testing.T) {
	var buf bytes.Buffer
	if err := run(&buf, filepath.Join("..", "..", "layer2", "testdata")); err != nil {
		t.Fatalf("run: %v\n%s", err, buf.String())
	}
	out := buf.String()
	for _, want := range []string{
		"voice.bin      79     77     2          79        2       2      13          0    PASS",
		"d-sms.bin      11      0    11          11       11       6       0          0    PASS",
		"all captures conform",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "FAIL") {
		t.Errorf("output has a failure:\n%s", out)
	}
}

func TestBadCRCFails(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "layer2", "testdata", "d-sms.bin"))
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i+burstSize <= len(raw); i += burstSize {
		rb := layer2.RawBurst(raw[i : i+burstSize])
		if st := rb.SlotType(); st.DataType != enums.DataTypeDataHeader {
			continue
		}
		p, _ := rb.BPTCPayload()
		p[0] ^= 0x80
		rb.SetBPTCPayload(p)
		copy(raw[i:], rb[:])
		tl := checkCapture("d-sms.bin", raw)
		if !strings.Contains(strings.Join(tl.failures, "\n"), "Data Header CRC-CCITT fails") {
			t.Fatalf("bad CRC not caught: %q", tl.failures)
		}
		return
	}
	t.Fatal("no data header in d-sms.bin")
}
