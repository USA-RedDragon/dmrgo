package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/packet"
	"github.com/USA-RedDragon/dmrgo/v2/rf"
	"github.com/USA-RedDragon/dmrgo/v2/sms"
)

func TestSMSSequenceDecodes(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "sms.cu8")
	runText(t, stdio{}, "tx", "-seq", "sms", "-msg", "Grüße 73", "-o", path, "-rate", "256000")
	iq, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	d, err := DemodulateBytes(iq, 256e3, false, rf.Tier3)
	if err != nil {
		t.Fatal(err)
	}
	asm := packet.NewAssembler(127)
	texts := 0
	for _, r := range d.Bursts {
		var b layer2.Burst
		if err := b.DecodeFromBytes(r.Data); err != nil {
			continue
		}
		m, ok, err := asm.AddBurst(&b)
		if err != nil {
			t.Fatal(err)
		}
		if !ok {
			continue
		}
		_, text, err := sms.Decode(nil, &m.Header, m.Data)
		if err != nil {
			t.Fatalf("sms.Decode: %v", err)
		}
		if text.DD != enums.DDUTF16LE || string(text.Body) != "Grüße 73" {
			t.Errorf("sms.Decode: DD %v, text %q", text.DD, text.Body)
		}
		texts++
	}
	if texts != 1 {
		t.Errorf("%d texts, want 1", texts)
	}
}
