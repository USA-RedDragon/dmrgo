package main

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/framer"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/vocoder"
)

type loop struct {
	built Built
	d     Demodulated
	lines []string
}

// runLoop builds a sequence, sends it through Modulate (or ModulateBS),
// ToIQ, channel and Demodulate, and reports the bursts.
func runLoop(t *testing.T, seq string, cfg SeqConfig) loop {
	t.Helper()
	return runLoopStretch(t, seq, cfg, 0)
}

func runLoopStretch(t *testing.T, seq string, cfg SeqConfig, ppm float64) loop {
	t.Helper()
	built, err := Sequence(seq, cfg)
	if err != nil {
		t.Fatal(err)
	}
	return runBuilt(t, seq, built, cfg, ppm)
}

// runBuilt is runLoopStretch for a built sequence.
func runBuilt(t *testing.T, seq string, built Built, cfg SeqConfig, ppm float64) loop {
	t.Helper()
	var fm, env []float64
	want := built.Bursts
	if cfg.BSSourced {
		fm, env = ModulateBS(built.Slots)
		want = nil
		for _, s := range built.Slots {
			want = append(want, s.Burst)
		}
	} else {
		fm, env = Modulate(built.Bursts, Schedule{Slot: 0})
	}
	iq := ToIQ(fm, env, 2e6, 0, 0.9)
	rx := 2.048e6
	if cfg.BSSourced {
		rx = 2.4e6
	}
	d := Demodulate(FromRTL(channel(iq, 2e6/(1+ppm*1e-6), rx, 3000, 25, 5)), rx)
	if len(d.Bursts) != len(want) {
		t.Fatalf("%s: got %d bursts, want %d", seq, len(d.Bursts), len(want))
	}
	for i, r := range d.Bursts {
		if r.Data != want[i] {
			t.Errorf("%s burst %d: bits differ", seq, i)
		}
		if cfg.BSSourced && r.CACH != built.Slots[i].CACH {
			t.Errorf("%s burst %d: CACH differs", seq, i)
		}
	}
	var buf bytes.Buffer
	rep := NewReporter(&buf)
	for _, r := range d.Bursts {
		rep.Burst(r)
	}
	return loop{built, d, strings.Split(buf.String(), "\n")}
}

func (l loop) grep(prefix string) []string {
	var out []string
	for _, s := range l.lines {
		if strings.HasPrefix(s, prefix+" ") {
			out = append(out, s)
		}
	}
	return out
}

func (l loop) expect(t *testing.T, prefix, substr string, n int) {
	t.Helper()
	var got int
	for _, s := range l.grep(prefix) {
		if strings.Contains(s, substr) {
			got++
		}
	}
	if got != n {
		t.Errorf("%d %q lines with %q, want %d\n%s", got, prefix, substr, n, strings.Join(l.grep(prefix), "\n"))
	}
}

func checkAMBE(t *testing.T, l loop, frames int, silence bool) {
	t.Helper()
	got := l.grep("ambe")
	if len(got) != frames {
		t.Fatalf("%d ambe lines, want %d", len(got), frames)
	}
	for i, s := range got {
		f := TestAMBEFrame(i)
		if silence {
			f = vocoder.AMBESilence()
		}
		want := fmt.Sprintf("seq=%d n=%d 0x%013x fec=ok", i, i%3, ambeValue(f))
		if !strings.Contains(s, want) {
			t.Errorf("ambe line %q, want %q", s, want)
		}
	}
	tx := 0
	for _, s := range l.built.Info {
		if strings.HasPrefix(s, "ambe tx ") {
			tx++
		}
	}
	if tx != frames {
		t.Errorf("%d ambe tx lines, want %d", tx, frames)
	}
}

func TestTestAMBEFramesAreDistinct(t *testing.T) {
	t.Parallel()

	seen := map[uint64]bool{ambeValue(vocoder.AMBESilence()): true}
	for i := range 1000 {
		v := ambeValue(TestAMBEFrame(i))
		if seen[v] {
			t.Fatalf("frame %d repeats 0x%013x", i, v)
		}
		seen[v] = true
	}
}

func TestLoopbackVoice(t *testing.T) {
	t.Parallel()

	for _, group := range []bool{true, false} {
		cfg := testConfig()
		cfg.Group, cfg.Superframes = group, 3
		l := runLoop(t, "voice", cfg)
		kind := "private"
		if group {
			kind = "group"
		}
		fields := fmt.Sprintf("flco=%d fid=0x00 %s src=3191868 dst=9990 privacy=0", map[bool]int{true: 0, false: 3}[group], kind)
		l.expect(t, "lc", "header "+fields, 1)
		l.expect(t, "lc", "terminator "+fields, 1)
		l.expect(t, "elc", fields+" emergency=0 checksum=ok fec=ok", 3)
		l.expect(t, "emb", "cc=1 pi=0", 15)
		for i := 1; i < 6; i++ {
			pos, _ := enums.VoiceBurstAt(i)
			lcss, _ := pos.LCSS()
			l.expect(t, "emb", fmt.Sprintf("pos=%s cc=1 pi=0 lcss=%q fec=ok", pos, lcss.String()), 3)
		}
		checkAMBE(t, l, 3*18, false)
	}
}

func TestLoopbackVoiceSlowClock(t *testing.T) {
	t.Parallel()

	for _, bs := range []bool{false, true} {
		cfg := testConfig()
		cfg.Superframes, cfg.BSSourced = 4, bs
		l := runLoopStretch(t, "voice", cfg, 500)
		l.expect(t, "elc", "checksum=ok fec=ok", 4)
		checkAMBE(t, l, 4*18, false)
	}
}

// flipSync inverts the first n bits of the SYNC field of b.
func flipSync(b layer2.RawBurst, n int) layer2.RawBurst {
	bits := bit.Unpack264(b)
	for k := framer.SyncOffset; k < framer.SyncOffset+n; k++ {
		bits[k] ^= 1
	}
	return bit.Pack264(bits)
}

// TestLoopbackVoiceDamagedSync damages the SYNC of every burst A of a
// BS-sourced call, and the EMB of one burst B, and checks that the slot
// timing still finds every superframe.
func TestLoopbackVoiceDamagedSync(t *testing.T) {
	t.Parallel()

	cfg := testConfig()
	cfg.BSSourced, cfg.Superframes = true, 4
	built, err := Sequence("voice", cfg)
	if err != nil {
		t.Fatal(err)
	}
	var a []int
	for i, s := range built.Slots {
		if b, err := layer2.NewBurstFromBytes(s.Burst); err == nil && b.Kind == layer2.KindVoice && b.SyncPattern == enums.SyncBSSourcedVoice {
			a = append(a, i)
		}
	}
	if len(a) != 4 {
		t.Fatalf("%d bursts A, want 4", len(a))
	}

	for i, n := range []int{14, 6, 14, 12} {
		built.Slots[a[i]].Burst = flipSync(built.Slots[a[i]].Burst, n)
	}
	built.Slots[a[2]+2].Burst = flipSync(built.Slots[a[2]+2].Burst, 4)
	l := runBuilt(t, "voice", built, cfg, 0)
	checkAMBE(t, l, 4*18, false)
	for _, pos := range []string{"A", "B", "C", "D", "E", "F"} {
		l.expect(t, "ambe", " pos="+pos+" ", 4*3)
	}
	l.expect(t, "emb", "fec=ok", 4*5)
	l.expect(t, "elc", "checksum=ok fec=ok", 4)
}

// TestLoopbackMSVoiceDamagedSync damages the SYNC of every burst A of an
// MS-sourced call, and checks that the slot timing from the voice LC header
// to the terminator finds every superframe. Then it ends a call after burst
// C with a damaged terminator SYNC, which must not decode as burst D.
func TestLoopbackMSVoiceDamagedSync(t *testing.T) {
	t.Parallel()

	cfg := testConfig()
	cfg.Superframes = 3
	built, err := Sequence("voice", cfg)
	if err != nil {
		t.Fatal(err)
	}
	const header = 2
	for sf := range 3 {
		i := header + 1 + sf*layer2.VoiceBurstsPerSuperframe
		if b, err := layer2.NewBurstFromBytes(built.Bursts[i]); err != nil || b.SyncPattern != enums.SyncMSSourcedVoice {
			t.Fatalf("burst %d is not a burst A", i)
		}
		built.Bursts[i] = flipSync(built.Bursts[i], 10)
	}
	l := runBuilt(t, "voice", built, cfg, 0)
	checkAMBE(t, l, 3*18, false)
	l.expect(t, "elc", "checksum=ok fec=ok", 3)
	l.expect(t, "lc", "terminator", 1)

	built, err = Sequence("voice", cfg)
	if err != nil {
		t.Fatal(err)
	}
	n := len(built.Bursts)
	term := flipSync(built.Bursts[n-1], 6)
	built.Bursts = append(built.Bursts[:n-4], term)
	built.Info = built.Info[:len(built.Info)-9]
	l = runBuilt(t, "voice", built, cfg, 0)
	checkAMBE(t, l, 2*18+9, false)
	l.expect(t, "lc", "terminator", 1)
}

func TestLoopbackVoiceLateEntry(t *testing.T) {
	t.Parallel()

	cfg := testConfig()
	cfg.Group, cfg.Superframes, cfg.LateEntry = true, 2, true
	l := runLoop(t, "voice", cfg)
	fields := "flco=0 fid=0x00 group src=3191868 dst=9990 privacy=0"
	l.expect(t, "lc", "header", 0)
	l.expect(t, "lc", "terminator "+fields, 1)
	l.expect(t, "elc", fields+" emergency=0 checksum=ok fec=ok", 2)
}

func TestLoopbackVoiceSilence(t *testing.T) {
	t.Parallel()

	cfg := testConfig()
	cfg.Superframes, cfg.AMBESilence = 1, true
	checkAMBE(t, runLoop(t, "voice", cfg), 18, true)
}

func TestLoopbackPI(t *testing.T) {
	t.Parallel()

	cfg := testConfig()
	cfg.Group, cfg.Superframes = true, 2
	l := runLoop(t, "pi", cfg)
	l.expect(t, "pi", "algid=0x21 mfid=0x10 keyid=0x01 mi=0x12345678 dst=9990 fec=ok", 1)
	if !strings.HasPrefix(l.built.Info[0], "pi algid=0x21 mfid=0x10 keyid=0x01 mi=0x12345678") {
		t.Errorf("tx info %q", l.built.Info[0])
	}
	l.expect(t, "lc", "privacy=1", 2)
	l.expect(t, "elc", "privacy=1 emergency=0 checksum=ok", 2)
	checkAMBE(t, l, 2*18, false)
}

func TestLoopbackResponses(t *testing.T) {
	t.Parallel()

	cfg := testConfig()
	l := runLoop(t, "ack", cfg)
	l.expect(t, "response", "ACK class=0 type=1 status=0 3191868 -> 9990", 1)
	l = runLoop(t, "sr", cfg)
	l.expect(t, "response", "SACK class=2 type=0 status=0 3191868 -> 9990 retry=1,4,6", 1)
	if !strings.HasPrefix(l.built.Info[0], "sr retry=1,4,6 of 8 blocks, C_RDATA ad") {
		t.Errorf("tx info %q", l.built.Info[0])
	}
}

func TestLoopbackPreamble(t *testing.T) {
	t.Parallel()

	cfg := testConfig()
	cfg.Preambles = 3
	l := runLoop(t, "preamble", cfg)
	var cbf []int
	for _, r := range l.d.Bursts {
		b, err := layer2.NewBurstFromBytes(r.Data)
		if err != nil {
			t.Fatal(err)
		}
		if c, ok := b.Data.(*pdu.CSBK); ok && c.Opcode == pdu.CSBKPreamble {
			p := c.PreamblePDU
			if c.FEC.Uncorrectable || !p.Data || p.TargetAddress != cfg.Dest || p.SourceAddress != cfg.Source {
				t.Errorf("preamble %+v %+v", c, p)
			}
			cbf = append(cbf, int(p.CSBKBlocksToFollow))
		}
	}
	data, err := dataBursts("data12", cfg)
	if err != nil {
		t.Fatal(err)
	}
	n := len(data)
	if fmt.Sprint(cbf) != fmt.Sprint([]int{n + 2, n + 1, n}) {
		t.Errorf("CSBK blocks to follow %v, data bursts %d", cbf, n)
	}
	var done bool
	for _, s := range l.lines {
		done = done || strings.Contains(s, fmt.Sprintf("packet complete: %q", cfg.Message))
	}
	if !done {
		t.Error("data packet not complete")
	}
}

// activeBlocks counts the Short LCs of a BS-sourced stream whose four slots
// carry part of the sequence.
func activeBlocks(l loop, cfg SeqConfig) int {
	busy := func(f int) bool { return f >= cfg.Wakeups && f-cfg.Wakeups < len(l.built.Bursts) }
	var n int
	for f := 0; f < len(l.built.Slots)/2; f += 2 {
		if busy(f) || busy(f+1) {
			n++
		}
	}
	return n
}

// TestReportChannelFromTiming gives some bursts a CACH whose TC is wrong but
// decodes clean, as two bit errors do, and checks that each burst still goes
// to the channel of its slot time.
func TestReportChannelFromTiming(t *testing.T) {
	t.Parallel()

	cfg := testConfig()
	cfg.BSSourced, cfg.Slot, cfg.Superframes = true, 1, 2
	l := runLoop(t, "voice", cfg)
	for i := 3; i < len(l.d.Bursts); i += 5 {
		r := &l.d.Bursts[i]
		c := layer2.DecodeCACH(layer2.CACHDeinterleave(r.CACH))
		c.TACT.TDMAChannel = !c.TACT.TDMAChannel
		bits, err := c.Encode()
		if err != nil {
			t.Fatal(err)
		}
		r.CACH = layer2.CACHInterleave(bits)
	}
	var buf bytes.Buffer
	rep := NewReporter(&buf)
	for _, r := range l.d.Bursts {
		rep.Burst(r)
	}
	l.lines = strings.Split(buf.String(), "\n")
	l.expect(t, "elc", "ts=2 flco=3", 2)
	l.expect(t, "lc", "ts=2 header", 1)
	checkAMBE(t, l, 2*18, false)
}

func decodeAll(t *testing.T, l loop) []*layer2.Burst {
	t.Helper()
	out := make([]*layer2.Burst, 0, len(l.d.Bursts))
	for _, r := range l.d.Bursts {
		b, err := layer2.NewBurstFromBytes(r.Data)
		if err != nil {
			t.Fatal(err)
		}
		if b.FEC.Aggregate().Uncorrectable {
			t.Errorf("burst %s: FEC failed", b)
		}
		out = append(out, b)
	}
	return out
}

func TestLoopbackReverseChannel(t *testing.T) {
	t.Parallel()

	cfg := testConfig()
	b := decodeAll(t, runLoop(t, "rc", cfg))
	rc := b[len(b)-1]
	if rc.Kind != layer2.KindReverseChannel || rc.ReverseChannel.RCCommand != enums.RCCeaseTransmissionCommand || rc.EmbeddedSignalling.ColorCode != cfg.ColorCode {
		t.Errorf("RC burst %s", rc)
	}
}

func TestLoopbackTier3(t *testing.T) {
	t.Parallel()

	cfg := testConfig()
	cfg.BSSourced, cfg.Slot = true, 0
	l := runLoop(t, "tier3", cfg)
	var got []string
	for _, b := range decodeAll(t, l) {
		switch d := b.Data.(type) {
		case *pdu.CSBK:
			got = append(got, fmt.Sprintf("csbk %d", d.Opcode))
		case *pdu.MBCHeader:
			g := d.TalkgroupVoiceGrantPDU
			got = append(got, fmt.Sprintf("mbc %d %d->%d ch 0x%x", d.Opcode, g.SourceAddress, g.TargetAddress, g.PhysicalChannel))
		case *pdu.MBCContinuation:
			c := d.AsCGAP()
			got = append(got, fmt.Sprintf("cgap %v %d MHz %d", c.FEC.Uncorrectable, c.CdefParms.TXMHz, c.CdefParms.TXKHz))
		}
	}
	want := fmt.Sprintf("[csbk %d csbk %d csbk %d mbc %d 3191868->9990 ch 0xfff cgap false 446 MHz 50]",
		pdu.CSBKAloha, pdu.CSBKTalkgroupVoiceGrant, pdu.CSBKMove, pdu.CSBKTalkgroupVoiceGrant)
	if fmt.Sprint(got) != want {
		t.Errorf("got %v\nwant %s", got, want)
	}
	l.expect(t, "slc", fmt.Sprintf("activity ts1=0x%x hash1=0x%02x ts2=0x0", uint8(enums.ActivityIndividualCSBK), pdu.AddressHash(cfg.Dest)), activeBlocks(l, cfg))
	l.expect(t, "slc", "activity ts1=0x0 hash1=0x00 ts2=0x0 hash2=0x00 fec=ok", len(l.built.Slots)/4-activeBlocks(l, cfg))
}

func TestLoopbackBSSourced(t *testing.T) {
	t.Parallel()

	for _, slot := range []int{0, 1} {
		cfg := testConfig()
		cfg.BSSourced, cfg.Slot, cfg.Group, cfg.Superframes = true, slot, true, 2
		l := runLoop(t, "voice", cfg)
		ts := fmt.Sprintf("ts=%d", slot+1)
		l.expect(t, "elc", ts+" flco=0 fid=0x00 group src=3191868 dst=9990 privacy=0 emergency=0 checksum=ok", 2)
		l.expect(t, "emb", fmt.Sprintf("%s pos=B cc=1 pi=0 lcss=%q fec=ok", ts, enums.LCSSFirstFragment.String()), 2)
		l.expect(t, "lc", ts+" header", 1)
		l.expect(t, "lc", ts+" terminator", 1)
		checkAMBE(t, l, 2*18, false)
		n := len(l.built.Slots)
		if n%4 != 0 {
			t.Errorf("%d slots, want a multiple of 4", n)
		}
		l.expect(t, "cach", "fec=ok", n)
		l.expect(t, "cach", "tc=0", n/2)
		l.expect(t, "cach", fmt.Sprintf("at=1 tc=%d", slot), len(l.built.Bursts))
		l.expect(t, "cach", fmt.Sprintf("at=1 tc=%d", 1-slot), 0)
		for _, lcss := range []enums.LCSS{enums.LCSSFirstFragment, enums.LCSSLastFragment} {
			l.expect(t, "cach", fmt.Sprintf("lcss=%q", lcss.String()), n/4)
		}
		l.expect(t, "cach", fmt.Sprintf("lcss=%q", enums.LCSSContinuationFragment.String()), n/2)
		hash := pdu.AddressHash(cfg.Dest)
		au := fmt.Sprintf("slco=1 activity ts1=0x0 hash1=0x00 ts2=0x8 hash2=0x%02x fec=ok", hash)
		if slot == 0 {
			au = fmt.Sprintf("slco=1 activity ts1=0x8 hash1=0x%02x ts2=0x0 hash2=0x00 fec=ok", hash)
		}
		l.expect(t, "slc", au, activeBlocks(l, cfg))
		l.expect(t, "slc", "activity ts1=0x0 hash1=0x00 ts2=0x0 hash2=0x00 fec=ok", n/4-activeBlocks(l, cfg))
	}
	cfg := testConfig()
	cfg.BSSourced, cfg.Slot = true, 1
	l := runLoop(t, "data12", cfg)
	var done bool
	for _, s := range l.lines {
		done = done || strings.Contains(s, fmt.Sprintf("packet complete: %q", cfg.Message))
	}
	if !done {
		t.Error("BS-sourced data packet not complete")
	}
	if _, err := Sequence("wakeup", cfg); err == nil {
		t.Error("BS-sourced wakeup accepted")
	}
	cfg.Slot = -1
	if _, err := Sequence("voice", cfg); err == nil {
		t.Error("BS-sourced stream with slot -1 accepted")
	}
}
