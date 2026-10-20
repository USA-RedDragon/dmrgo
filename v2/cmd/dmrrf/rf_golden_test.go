package main

import (
	"bufio"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/internal/rftx"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
)

type rfRun struct {
	name, seq string
	cfg       func(*rftx.SeqConfig)
}

func rfRuns() []rfRun {
	return []rfRun{
		{"wakeup", "wakeup", func(c *rftx.SeqConfig) { c.Wakeups = 1 }},
		{"data12", "data12", func(c *rftx.SeqConfig) { c.Dest = 3191868 }},
		{"data34", "data34", func(c *rftx.SeqConfig) { c.Dest = 3191868 }},
		{"data34u", "data34u", func(c *rftx.SeqConfig) { c.Dest, c.Group = 9, true }},
		{"preamble", "preamble", func(c *rftx.SeqConfig) { c.Dest, c.Preambles = 3191868, 2 }},
		{"voice", "voice", func(c *rftx.SeqConfig) { c.Dest, c.Group, c.Superframes = 9, true, 1 }},
		{"voice-late", "voice", func(c *rftx.SeqConfig) { c.Dest, c.Group, c.Superframes, c.LateEntry = 91, true, 2, true }},
		{"pi", "pi", func(c *rftx.SeqConfig) { c.Dest, c.Group, c.Superframes = 9, true, 1 }},
		{"resp-data", "data12", func(c *rftx.SeqConfig) { c.Dest = 3140598 }},
		{"ack", "ack", func(c *rftx.SeqConfig) { c.Source, c.Dest, c.Wakeups = 3140598, 3191868, 0 }},
		{"sr", "sr", func(c *rftx.SeqConfig) { c.Source, c.Dest, c.Wakeups = 3140598, 3191868, 0 }},
	}
}

func rfConfig() rftx.SeqConfig {
	return rftx.SeqConfig{ColorCode: 7, Source: 3191868, BS: 314926, Wakeups: 1, Message: "DMRGO TEST", Superframes: 3, Preambles: 4}
}

func rfBursts(t *testing.T) map[string][]string {
	t.Helper()
	out := map[string][]string{}
	for _, r := range rfRuns() {
		cfg := rfConfig()
		r.cfg(&cfg)
		built, err := rftx.Sequence(r.seq, cfg)
		if err != nil {
			t.Fatalf("%s: %v", r.name, err)
		}
		for _, b := range built.Bursts {
			out[r.name] = append(out[r.name], hex.EncodeToString(b[:]))
		}
	}
	return out
}

func TestRFGoldenBursts(t *testing.T) {
	t.Parallel()

	got := rfBursts(t)
	const path = "testdata/rf_golden.txt"
	if os.Getenv("DMRRF_UPDATE_GOLDEN") == "1" {
		var sb strings.Builder
		for _, r := range rfRuns() {
			for i, h := range got[r.name] {
				fmt.Fprintf(&sb, "%s %d %s\n", r.name, i, h)
			}
		}
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(sb.String()), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	want := map[string][]string{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var name, h string
		var i int
		if _, err := fmt.Sscan(sc.Text(), &name, &i, &h); err != nil {
			t.Fatalf("bad line %q: %v", sc.Text(), err)
		}
		want[name] = append(want[name], h)
	}
	for _, r := range rfRuns() {
		if len(got[r.name]) != len(want[r.name]) {
			t.Errorf("%s: %d bursts, want %d", r.name, len(got[r.name]), len(want[r.name]))
			continue
		}
		for i := range want[r.name] {
			if got[r.name][i] != want[r.name][i] {
				t.Errorf("%s burst %d:\n got %s\nwant %s", r.name, i, got[r.name][i], want[r.name][i])
			}
		}
	}
}

func TestRFRelayedPayloads(t *testing.T) {
	t.Parallel()

	got := rfBursts(t)
	bptc := func(name string, i int) string {
		b, _ := hex.DecodeString(got[name][i])
		p, res := layer2.RawBurst(b).BPTCPayload()
		if res.Uncorrectable {
			t.Fatalf("%s burst %d: BPTC uncorrectable", name, i)
		}
		return hex.EncodeToString(p[:])
	}
	rate34 := func(name string, i int) string {
		b, _ := hex.DecodeString(got[name][i])
		p, res := layer2.RawBurst(b).Rate34Payload()
		if res.Uncorrectable {
			t.Fatalf("%s burst %d: trellis uncorrectable", name, i)
		}
		return hex.EncodeToString(p[:])
	}
	for _, c := range []struct {
		what, got, want string
	}{
		{"wakeup CSBK", bptc("wakeup", 0), "b800000004ce2e30b43cd6a8"},
		{"data12 header", bptc("data12", 1), "43a630b43c30b43c820019d1"},
		{"data12 block 1", bptc("data12", 2), "0107444d52474f2054455354"},
		{"data12 block 2", bptc("data12", 3), "02c3000000000000d0281b13"},
		{"data34 block, IPSC rotates it left by 2", rate34("data34", 2), "002e444d52474f20544553540000a3ecda7c"},
		{"data34u header", bptc("data34u", 1), "82a400000930b43c81006a6d"},
		{"data34u block, IPSC rotates it left by 2", rate34("data34u", 2), "444d52474f205445535400000000bd3e6a6c"},
		{"preamble 1", bptc("preamble", 1), "bd00800430b43c30b43cac55"},
		{"preamble 2", bptc("preamble", 2), "bd00800330b43c30b43cb511"},
		{"voice LC header", bptc("voice", 1), "00000000000930b43ce9f8ed"},
		{"voice terminator", bptc("voice", len(got["voice"])-1), "00000000000930b43ce6f7e2"},
		{"pi LC header", bptc("pi", 1), "00004000000930b43ce175ec"},
		{"pi header", bptc("pi", 2), "21100112345678000009246e"},
		{"pi terminator", bptc("pi", len(got["pi"])-1), "00004000000930b43cee7ae3"},
		{"data header answered by a response", bptc("resp-data", 1), "43a62febf630b43c8200a5eb"},
		{"ACK response header", bptc("ack", 0), "01a030b43c2febf60008327b"},
		{"SACK response header", bptc("sr", 0), "01a030b43c2febf6018011ca"},
		{"SACK retry bitmap", bptc("sr", 1), "adffffffffffffff25b85442"},
	} {
		if c.got != c.want {
			t.Errorf("%s: %s, repeater relayed %s", c.what, c.got, c.want)
		}
	}
}
