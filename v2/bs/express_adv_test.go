package bs_test

import (
	"sort"
	"testing"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/bs"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

func events(got []emitted, slot enums.Slot) []string {
	var ev []string
	for _, o := range got {
		if o.Slot != slot {
			continue
		}
		switch o.Kind {
		case bs.OutputCallStart:
			ev = append(ev, "start:"+o.Service.String())
		case bs.OutputCallEnd:
			ev = append(ev, "end:"+o.Service.String()+":"+o.End.String())
		case bs.OutputRejected:
			ev = append(ev, "rej:"+o.Reason.String())
		case bs.OutputBurst, bs.OutputKey, bs.OutputDekey:
		}
	}
	return ev
}

func peerCSBK(t *testing.T, n byte) layer2.RawBurst {
	t.Helper()
	c := pdu.CSBK{LastBlock: true, Opcode: pdu.CSBKUnitToUnitVoiceServiceRequest, RawPayload: [8]byte{0, 0, 0, n, 1, 0x30, 0xB4, 0x3C}}
	return must(layer2.BuildBurst(&c, 1, layer2.SyncSourceMS))
}

func TestExpressDupOfHeld(t *testing.T) {
	t.Parallel()
	for _, mode := range []bs.Mode{bs.ModeNetwork, bs.ModeRF} {
		d := netData(t, loadBin(t, "d-sms.bin"), enums.Slot1, 10, 5, time.Second)
		want := append([]advPkt(nil), d...)
		d[3].at += 90 * time.Millisecond
		dup := d[4]
		dup.at += 5 * time.Millisecond
		got := expressRun(t, newRepeater(t, &bs.Config{Mode: mode, ColorCode: 1, CACH: true}), append(d, dup), 4*time.Second, false)
		late := 90 * time.Millisecond
		if mode == bs.ModeRF {
			late = 60 * time.Duration(len(want)) * time.Millisecond
		}
		checkData(t, mode.String(), dataOut(got, enums.Slot1), want, func(int) time.Duration { return late })
	}
}

func TestExpressDataTailBeforeVoice(t *testing.T) {
	t.Parallel()
	for _, mode := range []bs.Mode{bs.ModeNetwork, bs.ModeRF} {
		d := netData(t, loadBin(t, "d-sms.bin"), enums.Slot1, 50, 5, time.Second)
		want := append([]advPkt(nil), d...)
		v := advNetCall(t, 2, 200, 6, 1, d[len(d)-1].at-20*time.Millisecond, 3191870)
		got := expressRun(t, newRepeater(t, &bs.Config{Mode: mode, ColorCode: 1, CACH: true}), append(append([]advPkt(nil), d...), v...), 6*time.Second, false)
		late := time.Duration(0)
		if mode == bs.ModeRF {
			late = 60 * time.Millisecond
		}
		checkData(t, mode.String(), dataOut(got, enums.Slot1), want, func(int) time.Duration { return late })
		ev := events(got, enums.Slot1)
		if len(ev) < 2 || ev[1] != "end:Data:None" {
			t.Fatalf("%v: events %v", mode, ev)
		}
	}
}

func TestExpressBehindQueuedVoice(t *testing.T) {
	t.Parallel()
	for _, mode := range []bs.Mode{bs.ModeNetwork, bs.ModeRF} {
		b := advNetCall(t, 3, 100, 22, 2, time.Second, 3191869)
		a := advNetCall(t, 3, 10, 11, 1, b[len(b)-1].at-300*time.Millisecond, 3191868)
		last := a[len(a)-1]
		d := netData(t, loadBin(t, "d-sms.bin"), enums.Slot1, (last.h.Seq+1)&0xFF, 11, last.at+20*time.Millisecond)
		got := expressRun(t, newRepeater(t, &bs.Config{Mode: mode, ColorCode: 1, CACH: true}), append(append(append([]advPkt(nil), b...), a...), d...), 8*time.Second, false)
		for _, e := range events(got, enums.Slot1) {
			if e == "end:Voice:DataSync" || e == "end:Voice:Superseded" || e == "end:Data:Superseded" {
				t.Fatalf("%v: events %v", mode, events(got, enums.Slot1))
			}
		}
	}
}

func TestExpressKeysFull(t *testing.T) {
	t.Parallel()
	d := netData(t, loadBin(t, "d-sms.bin"), enums.Slot1, 0, 5, time.Second)
	want := append([]advPkt(nil), d...)
	d[3].at += 90 * time.Millisecond
	in := append([]advPkt(nil), d...)
	at := d[4].at + time.Millisecond
	for p := uint32(2); p <= 5; p++ {
		for _, seq := range []uint16{2, 4} {
			in = append(in, advPkt{at: at, raw: peerCSBK(t, byte(p)+byte(seq)*16), peer: p, h: layer2.Hint{Slot: enums.Slot1, Seq: seq, HasSeq: true, SeqBits: 8, Stream: 100 + p, HasStream: true}})
			at += time.Millisecond
		}
	}
	got := dataOut(expressRun(t, newRepeater(t, &bs.Config{Mode: bs.ModeNetwork, ColorCode: 1}), in, 4*time.Second, false), enums.Slot1)
	n := map[[12]byte]int{}
	for _, o := range got {
		n[payloadOf(o.Burst)]++
	}
	for i := range want {
		if n[payloadOf(want[i].raw)] != 1 {
			t.Fatalf("block %d out %d times", i, n[payloadOf(want[i].raw)])
		}
	}
}

func TestExpressSkipsReset(t *testing.T) {
	t.Parallel()
	a := netData(t, loadBin(t, "d-sms.bin"), enums.Slot1, 0, 0, time.Second)
	a = append(a[:4:4], a[5:]...)
	b := netData(t, loadBin(t, "h-sms.bin"), enums.Slot1, 20, 0, 10*time.Second)
	want := append([]advPkt(nil), b...)
	b[2].at, b[3].at = b[3].at, b[3].at-20*time.Millisecond
	got := dataOut(expressRun(t, newRepeater(t, &bs.Config{Mode: bs.ModeNetwork, ColorCode: 1}), append(append([]advPkt(nil), a...), b...), 15*time.Second, false), enums.Slot1)
	if len(got) != len(a)+len(b) {
		t.Fatalf("%d data out, want %d", len(got), len(a)+len(b))
	}
	checkData(t, "second message", got[len(a):], want, func(int) time.Duration { return 60 * time.Millisecond })
}

func TestExpressClockReset(t *testing.T) {
	t.Parallel()
	for _, stream := range []uint32{0, 5} {
		a := netData(t, loadBin(t, "d-sms.bin"), enums.Slot1, 0, stream, 100*time.Second)
		b := netData(t, loadBin(t, "h-sms.bin"), enums.Slot1, 20, stream, 20*time.Second)
		r := newRepeater(t, &bs.Config{Mode: bs.ModeNetwork, ColorCode: 1})
		got := expressRun(t, r, a, 102*time.Second, false)
		want := append([]advPkt(nil), b...)
		sort.SliceStable(b, func(i, j int) bool { return b[i].at < b[j].at })
		got = append(got, expressRun(t, r, b, 22*time.Second, false)...)
		d := dataOut(got, enums.Slot1)
		if len(d) != len(a)+len(b) {
			t.Fatalf("stream %d: %d data out, want %d", stream, len(d), len(a)+len(b))
		}
		checkData(t, "after the reset", d[len(a):], want, func(int) time.Duration { return 60 * time.Millisecond })
	}
}
