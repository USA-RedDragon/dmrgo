package main

import (
	"bufio"
	"bytes"
	"encoding/hex"
	"fmt"
	"math"
	"os"
	"strings"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/internal/rftx"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/rf"
)

const captureHeader = `# Bursts that dmrrf rx recovered from an RTL-SDR capture of the outbound
# stream of a Motorola XPR 8400 repeater (CC 7, ID 314926) on 2026-10-15:
# 424.075 MHz, 2.048 Msps, run dmrgo-bs-voice/rec1, file repeater-bs-voice.cu8.
# The repeater sent a group voice call from 3191868 to TG 9 on TS2, 5
# superframes of the AMBE test pattern of dmrrf (rftx.TestAMBEFrame) that came to it
# over the network.
# Columns: the sample of the first burst bit at 48 kHz, the 24 CACH bits
# before the burst in transmit order, the SYNC distance or -1 for a burst
# placed by timing, the burst. Regenerate with
# DMRRF_CAPTURE=<path to repeater-bs-voice.cu8> go test -run TestCaptureXPR8400 .
`

const capturePath = "testdata/xpr8400_bs_voice.txt"

func writeCapture(t *testing.T, iq string) {
	t.Helper()
	b, err := os.ReadFile(iq)
	if err != nil {
		t.Fatal(err)
	}
	var sb strings.Builder
	sb.WriteString(captureHeader)
	for _, r := range demodulate(t, rftx.FromCU8(b), 2.048e6).Bursts {
		fmt.Fprintf(&sb, "%d %06x %d %x\n", int(math.Round(r.Seconds*basebandRate)), r.CACH, r.Distance, r.Data)
	}
	if err := os.WriteFile(capturePath, []byte(sb.String()), 0o600); err != nil {
		t.Fatal(err)
	}
}

func readCapture(t *testing.T) []rf.Burst {
	t.Helper()
	f, err := os.Open(capturePath)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var out []rf.Burst
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if strings.HasPrefix(sc.Text(), "#") {
			continue
		}
		var sample, distance int
		var cach uint64
		var burst string
		if _, err := fmt.Sscanf(sc.Text(), "%d %x %d %s", &sample, &cach, &distance, &burst); err != nil {
			t.Fatalf("bad line %q: %v", sc.Text(), err)
		}
		r := rf.Burst{Seconds: float64(sample) / basebandRate, Distance: distance, HasCACH: true, BS: true}
		if n, err := hex.Decode(r.Data[:], []byte(burst)); err != nil || n != len(r.Data) {
			t.Fatalf("bad burst %q: %v", burst, err)
		}
		r.CACH = [3]byte{byte(cach >> 16), byte(cach >> 8), byte(cach)}
		out = append(out, r)
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestCaptureXPR8400(t *testing.T) {
	t.Parallel()

	if iq := os.Getenv("DMRRF_CAPTURE"); iq != "" {
		writeCapture(t, iq)
	}
	bursts := readCapture(t)
	if len(bursts) != 162 {
		t.Fatalf("%d bursts, want 162", len(bursts))
	}
	var buf bytes.Buffer
	rep := NewReporter(&buf)
	for _, r := range bursts {
		rep.Burst(r)
	}
	l := loop{lines: strings.Split(buf.String(), "\n")}

	const fields = "ts=2 header flco=0 fid=0x00 group src=3191868 dst=9 privacy=0 emergency=0 fec=ok"
	l.expect(t, "lc", fields, 3)
	l.expect(t, "lc", strings.Replace(fields, "header", "terminator", 1), 33)
	l.expect(t, "elc", "ts=2 flco=0 fid=0x00 group src=3191868 dst=9 privacy=0 emergency=0 checksum=ok fec=ok", 5)
	l.expect(t, "cach", "fec=ok", 162)
	l.expect(t, "slc", fmt.Sprintf("slco=1 activity ts1=0x0 hash1=0x00 ts2=0x%x hash2=0x3f fec=ok", uint8(enums.ActivityGroupVoice)), 34)
	l.expect(t, "slc", "slco=0", 6)

	checkAMBEPattern(t, l, 90)
	checkEMB(t, l, 5)
}

func checkAMBEPattern(t *testing.T, l loop, n int) {
	t.Helper()
	ambe := l.grep("ambe")
	if len(ambe) != n {
		t.Fatalf("%d ambe lines, want %d", len(ambe), n)
	}
	for i, s := range ambe {
		pos, _ := enums.VoiceBurstAt(i / 3 % layer2.VoiceBurstsPerSuperframe)
		want := fmt.Sprintf("ts=2 pos=%s seq=%d n=%d 0x%013x fec=", pos, i, i%3, rftx.AMBEValue(rftx.TestAMBEFrame(i)))
		if !strings.Contains(s, want) || strings.HasSuffix(s, "fec=bad") {
			t.Errorf("ambe line %q, want %q", s, want)
		}
	}
}

func checkEMB(t *testing.T, l loop, n int) {
	t.Helper()
	emb := l.grep("emb")
	if len(emb) != 5*n {
		t.Fatalf("%d emb lines, want %d", len(emb), 5*n)
	}
	for i, s := range emb {
		pos, _ := enums.VoiceBurstAt(i%5 + 1)
		lcss, _ := pos.LCSS()
		want := fmt.Sprintf("ts=2 pos=%s cc=7 pi=0 lcss=%q fec=ok", pos, lcss.String())
		if !strings.Contains(s, want) {
			t.Errorf("emb line %q, want %q", s, want)
		}
	}
}

func TestCaptureXPR8400Weak(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	rep := NewReporter(&buf)
	d := weakDemod(t)
	for _, r := range d.Bursts {
		rep.Burst(r)
	}
	l := loop{d: d, lines: strings.Split(buf.String(), "\n")}
	const fields = "ts=2 header flco=0 fid=0x00 group src=3191868 dst=9 privacy=0 emergency=0 fec=ok"
	l.expect(t, "lc", fields, 3)
	l.expect(t, "lc", strings.Replace(fields, "header", "terminator", 1), 1)
	l.expect(t, "elc", "ts=2 flco=0 fid=0x00 group src=3191868 dst=9 privacy=0 emergency=0 checksum=ok fec=ok", 1)
	checkAMBEPattern(t, l, 18)
	checkEMB(t, l, 1)
}
