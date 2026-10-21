package hytera_test

import (
	"bufio"
	"encoding/hex"
	"os"
	"strings"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/transport/hytera"
)

func TestFrameCallFromCaptures(t *testing.T) {
	t.Parallel()

	f, err := os.Open("testdata/calls.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	n := 0
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || line[0] == '#' {
			continue
		}
		p, err := hex.DecodeString(line)
		if err != nil {
			t.Fatal(err)
		}
		fr, err := hytera.DecodeFrame(p)
		if err != nil {
			t.Fatal(err)
		}
		h := fr.Hint()
		b := layer2.NewBurst()
		if err := b.DecodeFromBytesWith(fr.Burst, &layer2.DecodeOptions{Kind: h.Kind, DataType: &h.DataType}); err != nil {
			t.Fatal(err)
		}
		lc, ok := b.Data.(*pdu.FullLinkControl)
		if !ok || lc.FEC.Uncorrectable || lc.FLCO != enums.FLCOGroupVoiceChannelUser {
			t.Fatalf("%s: not a group voice LC: %+v", line[:16], b.Data)
		}
		lcTarget, lcSource, _ := lc.Addresses()
		group, target, source := fr.Call()
		if !group || target != lcTarget || source != lcSource {
			t.Errorf("CallType %#x: Call %v %d %d, LC group %d %d", fr.CallType, group, target, source, lcTarget, lcSource)
		}
		n++
	}
	if n != 2 {
		t.Fatalf("%d frames", n)
	}
}

func TestFrameSetCall(t *testing.T) {
	t.Parallel()

	fr := hytera.Frame{CallType: 0x01}
	if err := fr.SetCall(true, 262, 2628347); err != nil || fr.CallType != 0x00 || fr.Destination != 262 || fr.Source != 2628347 {
		t.Fatalf("SetCall: %v %+v", err, fr)
	}
	want := fr
	for _, c := range []struct {
		group          bool
		target, source uint32
	}{{false, 1, 1}, {true, 1 << 24, 1}, {true, 1, 1 << 24}} {
		if err := fr.SetCall(c.group, c.target, c.source); err == nil || fr != want {
			t.Errorf("%+v: %v %+v", c, err, fr)
		}
	}
	if g, _, _ := (&hytera.Frame{CallType: 0x0A}).Call(); g {
		t.Error("CallType 0x0A is a group call")
	}
	var nf *hytera.Frame
	if g, tg, src := nf.Call(); g || tg != 0 || src != 0 {
		t.Error("nil Frame Call")
	}
	if err := nf.SetCall(true, 1, 1); err == nil {
		t.Error("nil Frame SetCall: no error")
	}
}
