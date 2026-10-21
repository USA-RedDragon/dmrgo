package main

import (
	"bytes"
	"fmt"
	"math"
	"math/cmplx"
	"strings"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/internal/rftx"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

func siteLoop(t *testing.T, built rftx.Built, iq []byte, rate, offsetHz float64, want []rftx.BSSlot) loop {
	t.Helper()
	x := rftx.FromCS8(iq)
	for i := range x {
		x[i] *= cmplx.Exp(complex(0, -2*math.Pi*offsetHz*float64(i)/rate))
	}
	d := demodulate(t, x, rate)
	if len(d.Bursts) != len(want) {
		t.Fatalf("channel at %+.0f Hz: %d bursts, want %d", offsetHz, len(d.Bursts), len(want))
	}
	for i, r := range d.Bursts {
		if r.Data != want[i].Burst || r.CACH != want[i].CACH {
			t.Errorf("channel at %+.0f Hz slot %d: bits differ", offsetHz, i)
		}
	}
	var buf bytes.Buffer
	rep := NewReporter(&buf)
	for _, r := range d.Bursts {
		rep.Burst(r)
	}
	return loop{built, d, strings.Split(buf.String(), "\n")}
}

func TestLoopbackT3Site(t *testing.T) {
	t.Parallel()

	cfg := testConfig()
	cfg.BSSourced, cfg.Superframes = true, 2
	built, err := rftx.Sequence("t3site", cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(built.Carriers) != 1 {
		t.Fatalf("%d extra carriers, want 1", len(built.Carriers))
	}
	const rate = 2e6
	fm, env := rftx.ModulateBS(built.Slots)
	iq := rftx.MixIQ(built.Signals(fm, env, 0), rate, 0.9)

	var got []string
	for _, b := range decodeAll(t, siteLoop(t, built, iq, rate, 0, built.Slots)) {
		switch d := b.Data.(type) {
		case *pdu.CSBK:
			if g := d.TalkgroupVoiceGrantPDU; g != nil {
				got = append(got, fmt.Sprintf("grant ch=%d ts2=%t late=%t %d->%d", g.PhysicalChannel, g.LogicalChannel, g.LateEntry, g.SourceAddress, g.TargetAddress))
			}
		case *pdu.MBCHeader:
			if g := d.TalkgroupVoiceGrantPDU; g != nil {
				got = append(got, fmt.Sprintf("mbc grant ch=0x%x ts2=%t %d->%d", g.PhysicalChannel, g.LogicalChannel, g.SourceAddress, g.TargetAddress))
			}
		case *pdu.MBCContinuation:
			if c := d.AsCGAP(); c.Opcode == pdu.CSBKTalkgroupVoiceGrant {
				got = append(got, fmt.Sprintf("cgap ch=%d rx=%d tx=%d bad=%t", c.CdefParms.Channel, c.CdefParms.RXFrequencyHz(), c.CdefParms.TXFrequencyHz(), c.FEC.Uncorrectable))
			} else if c := d.AsBCAP(); c.Opcode == pdu.CSBKBroadcast {
				got = append(got, fmt.Sprintf("bcap ch=%d rx=%d tx=%d bad=%t", c.CdefParms.Channel, c.CdefParms.RXFrequencyHz(), c.CdefParms.TXFrequencyHz(), c.FEC.Uncorrectable))
			}
		}
	}
	want := []string{
		"bcap ch=1 rx=446006250 tx=441006250 bad=false",
		"bcap ch=2 rx=446031250 tx=441031250 bad=false",
		"grant ch=2 ts2=true late=false 3191868->9990",
		"mbc grant ch=0xfff ts2=true 3191868->9990",
		"cgap ch=2 rx=446031250 tx=441031250 bad=false",
		"grant ch=2 ts2=true late=true 3191868->9990",
	}
	all := strings.Join(got, "\n")
	for _, w := range want {
		if !strings.Contains(all, w) {
			t.Errorf("control channel has no %q in\n%s", w, all)
		}
	}

	tr := siteLoop(t, built, iq, rate, built.Carriers[0].OffsetHz, built.Carriers[0].Slots)
	tr.expect(t, "lc", "ts=2 header flco=0 fid=0x00 group src=3191868 dst=9990", 1)
	tr.expect(t, "lc", "ts=2 terminator flco=0 fid=0x00 group src=3191868 dst=9990", 1)
	tr.expect(t, "elc", "ts=2 flco=0 fid=0x00 group src=3191868 dst=9990", cfg.Superframes)
	checkAMBE(t, tr, cfg.Superframes*18, false)
}
