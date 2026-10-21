package hytera_test

import (
	"bufio"
	"encoding/hex"
	"os"
	"strings"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/transport/hytera"
)

type callFrame struct {
	group bool
	f     hytera.Frame
}

func readCalls(t *testing.T) []callFrame {
	t.Helper()
	f, err := os.Open("testdata/calls.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var out []callFrame
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fs := strings.Fields(sc.Text())
		if len(fs) != 2 || fs[0][0] == '#' {
			continue
		}
		p, err := hex.DecodeString(fs[1])
		if err != nil {
			t.Fatal(err)
		}
		fr, err := hytera.DecodeFrame(p)
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, callFrame{group: fs[0] == "group", f: fr})
	}
	return out
}

func TestFrameCallFromCaptures(t *testing.T) {
	t.Parallel()

	frames := readCalls(t)
	if len(frames) != 5 {
		t.Fatalf("%d frames", len(frames))
	}
	for i, c := range frames {
		group, target, source := c.f.Call()
		if group != c.group || target != c.f.Destination || source != c.f.Source {
			t.Errorf("frame %d, CallType %#x: Call %v %d %d, want group %v", i, c.f.CallType, group, target, source, c.group)
		}
		voice := c.f
		voice.SlotType = hytera.SlotTypeVoiceB
		if g, _, _ := voice.Call(); g != (c.f.CallType == 0x01) {
			t.Errorf("frame %d as a voice burst, CallType %#x: group %v", i, c.f.CallType, g)
		}
	}
}

func TestFrameSetCall(t *testing.T) {
	t.Parallel()

	var fr hytera.Frame
	if err := fr.SetCall(true, 262, 2628347); err != nil || fr.CallType != 0x01 || fr.Destination != 262 || fr.Source != 2628347 {
		t.Fatalf("group: %v %+v", err, fr)
	}
	if err := fr.SetCall(false, 65000, 2145007); err != nil || fr.CallType != 0x00 || fr.Destination != 65000 {
		t.Fatalf("private: %v %+v", err, fr)
	}
	if g, _, _ := fr.Call(); g {
		t.Fatal("private call reads as group")
	}
	want := fr
	for _, c := range []struct{ target, source uint32 }{{1 << 24, 1}, {1, 1 << 24}} {
		if err := fr.SetCall(true, c.target, c.source); err == nil || fr != want {
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
