package bs_test

import (
	"sort"
	"testing"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/bs"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/voice"
)

type emitted struct {
	bs.Output
	at time.Duration
}

func expressRun(t *testing.T, r *bs.Repeater, in []advPkt, end time.Duration, decoded bool) []emitted {
	t.Helper()
	sort.SliceStable(in, func(a, b int) bool { return in[a].at < in[b].at })
	out := make([]bs.Output, 0, bs.MaxOutputs)
	var got []emitted
	keep := func(now time.Duration) {
		if cap(out) != bs.MaxOutputs {
			t.Fatalf("outputs grew to %d", cap(out))
		}
		for i := range out {
			checkHint(t, &out[i])
			got = append(got, emitted{out[i], now})
		}
	}
	b := layer2.NewBurst()
	for now := time.Duration(0); now <= end; now += time.Millisecond {
		for len(in) > 0 && in[0].at <= now {
			h := in[0].h
			h.At = in[0].at
			if decoded {
				var d *layer2.Burst
				if b.DecodeFromBytes(in[0].raw) == nil {
					d = b
				}
				out = r.Decoded(now, d, h, bs.Source{Peer: in[0].peer}, out[:0])
			} else {
				out = r.Inbound(now, &in[0].raw, h, bs.Source{Peer: in[0].peer}, out[:0])
			}
			keep(now)
			in = in[1:]
		}
		for n := 0; ; n++ {
			if n > 1000 {
				t.Fatalf("Tick loop at %v", now)
			}
			at, ok := r.Deadline()
			if !ok || at > now {
				break
			}
			out = r.Tick(now, out[:0])
			keep(now)
		}
	}
	return got
}

func netData(t *testing.T, bursts []layer2.RawBurst, slot enums.Slot, seq0 uint16, stream uint32, start time.Duration) []advPkt {
	t.Helper()
	out := make([]advPkt, len(bursts))
	for i := range bursts {
		out[i] = advPkt{at: start + time.Duration(i)*60*time.Millisecond, raw: bursts[i], peer: 1,
			h: layer2.Hint{Slot: slot, Seq: (seq0 + uint16(i)) & 0xFF, HasSeq: true, SeqBits: 8, Stream: stream, HasStream: stream != 0}}
	}
	return out
}

func payloadOf(raw layer2.RawBurst) [12]byte {
	p, _ := raw.BPTCPayload()
	return p
}

func dataOut(got []emitted, slot enums.Slot) []emitted {
	var d []emitted
	for _, o := range got {
		if o.Kind != bs.OutputBurst || o.Slot != slot || o.Hint.Kind != layer2.KindData {
			continue
		}
		if dt := o.Hint.DataType; dt == enums.DataTypeVoiceLCHeader || dt == enums.DataTypePIHeader || dt == enums.DataTypeTerminatorWithLC || dt == enums.DataTypeIdle {
			continue
		}
		d = append(d, o)
	}
	return d
}

func checkData(t *testing.T, name string, got []emitted, want []advPkt, late func(i int) time.Duration) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s: %d data bursts out, want %d", name, len(got), len(want))
	}
	for i := range want {
		if payloadOf(got[i].Burst) != payloadOf(want[i].raw) {
			t.Fatalf("%s: data burst %d out of order", name, i)
		}
		at := max(got[i].at, got[i].Due)
		if limit := want[i].at + late(i); at > limit {
			t.Fatalf("%s: data burst %d out at %v, arrived %v, limit %v", name, i, at, want[i].at, limit)
		}
	}
}

func testCSBK(t *testing.T) layer2.RawBurst {
	t.Helper()
	c := pdu.CSBK{LastBlock: true, Opcode: pdu.CSBKUnitToUnitVoiceServiceRequest, RawPayload: [8]byte{0, 0, 0, 0, 1, 0x30, 0xB4, 0x3C}}
	return must(layer2.BuildBurst(&c, 1, layer2.SyncSourceMS))
}

func TestExpressNetworkData(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"d-sms.bin", "h-sms.bin", "m-sms.bin"} {
		for _, decoded := range []bool{false, true} {
			in := netData(t, loadBin(t, name), enums.Slot1, 250, 5, time.Second)
			want := append([]advPkt(nil), in...)
			got := expressRun(t, newRepeater(t, &bs.Config{Mode: bs.ModeNetwork, ColorCode: 1}), in, 3*time.Second, decoded)
			checkData(t, name, dataOut(got, enums.Slot1), want, func(int) time.Duration { return 0 })
		}
	}
	in := netData(t, []layer2.RawBurst{testCSBK(t)}, enums.Slot2, 7, 9, time.Second)
	want := append([]advPkt(nil), in...)
	got := expressRun(t, newRepeater(t, &bs.Config{Mode: bs.ModeNetwork, ColorCode: 1}), in, 2*time.Second, false)
	checkData(t, "csbk", dataOut(got, enums.Slot2), want, func(int) time.Duration { return 0 })
}

func TestExpressRFData(t *testing.T) {
	t.Parallel()
	in := netData(t, loadBin(t, "d-sms.bin"), enums.Slot1, 0, 5, time.Second)
	want := append([]advPkt(nil), in...)
	for i := range in {
		in[i].at = time.Second
	}
	got := expressRun(t, newRepeater(t, &bs.Config{ColorCode: 1, CACH: true}), in, 3*time.Second, false)
	d := dataOut(got, enums.Slot1)
	checkData(t, "bunched", d, want, func(i int) time.Duration { return time.Duration(i+1) * 60 * time.Millisecond })
	for i := 1; i < len(d); i++ {
		if d[i].Due-d[i-1].Due != 60*time.Millisecond {
			t.Fatalf("burst %d due %v after %v", i, d[i].Due, d[i-1].Due)
		}
	}
	in = netData(t, loadBin(t, "d-sms.bin"), enums.Slot1, 0, 5, time.Second)
	want = append([]advPkt(nil), in...)
	got = expressRun(t, newRepeater(t, &bs.Config{ColorCode: 1, CACH: true}), in, 3*time.Second, false)
	checkData(t, "paced", dataOut(got, enums.Slot1), want, func(int) time.Duration { return 60 * time.Millisecond })
}

func TestExpressReorder(t *testing.T) {
	t.Parallel()
	bursts := loadBin(t, "d-sms.bin")
	in := netData(t, bursts, enums.Slot1, 254, 5, time.Second)
	in[1].at, in[2].at = in[1].at+30*time.Millisecond, in[1].at
	want := append([]advPkt(nil), in...)
	got := expressRun(t, newRepeater(t, &bs.Config{Mode: bs.ModeNetwork, ColorCode: 1}), in, 3*time.Second, false)
	checkData(t, "swap", dataOut(got, enums.Slot1), want, func(i int) time.Duration {
		if i == 2 {
			return 30 * time.Millisecond
		}
		return 0
	})

	in = netData(t, bursts, enums.Slot1, 0, 5, time.Second)
	in = append(in[:3:3], in[4:]...)
	want = append([]advPkt(nil), in...)
	got = expressRun(t, newRepeater(t, &bs.Config{Mode: bs.ModeNetwork, ColorCode: 1}), in, 3*time.Second, false)
	checkData(t, "lost", dataOut(got, enums.Slot1), want, func(i int) time.Duration {
		if i == 3 {
			return 60 * time.Millisecond
		}
		return 0
	})
}

func TestExpressSkippingCounter(t *testing.T) {
	t.Parallel()
	in := netData(t, loadBin(t, "d-sms.bin"), enums.Slot1, 0, 5, time.Second)
	for i := range in {
		in[i].h.Seq = uint16(2*i) & 0xFF
	}
	want := append([]advPkt(nil), in...)
	got := expressRun(t, newRepeater(t, &bs.Config{Mode: bs.ModeNetwork, ColorCode: 1}), in, 3*time.Second, false)
	checkData(t, "skip", dataOut(got, enums.Slot1), want, func(i int) time.Duration {
		if i == 1 {
			return 60 * time.Millisecond
		}
		return 0
	})
}

type voiceOut struct {
	due   time.Duration
	burst layer2.RawBurst
}

func voiceTrace(got []emitted, slot enums.Slot) (trace []voiceOut, term time.Duration) {
	for _, o := range got {
		if o.Kind != bs.OutputBurst || o.Slot != slot {
			continue
		}
		if o.Hint.Kind != layer2.KindVoice && o.Hint.DataType != enums.DataTypeVoiceLCHeader && o.Hint.DataType != enums.DataTypeTerminatorWithLC {
			continue
		}
		trace = append(trace, voiceOut{o.Due, o.Burst})
		if o.Hint.DataType == enums.DataTypeTerminatorWithLC {
			return trace, max(o.at, o.Due)
		}
	}
	return trace, 0
}

func checkVoice(t *testing.T, name string, r *bs.Repeater, v []advPkt, got []emitted, slot enums.Slot) time.Duration {
	t.Helper()
	alone, _ := voiceTrace(expressRun(t, r, append([]advPkt(nil), v...), 6*time.Second, false), slot)
	trace, term := voiceTrace(got, slot)
	if len(alone) == 0 || term == 0 || len(trace) != len(alone) {
		t.Fatalf("%s: %d voice bursts, %d without data, terminator %v", name, len(trace), len(alone), term)
	}
	if trace[0].due > alone[0].due+bs.SlotTime {
		t.Fatalf("%s: voice due %v, %v without data", name, trace[0].due, alone[0].due)
	}
	for i := range alone {
		if trace[i].burst != alone[i].burst || trace[i].due-trace[0].due != alone[i].due-alone[0].due {
			t.Fatalf("%s: voice burst %d due %v, %v without data", name, i, trace[i].due, alone[i].due)
		}
	}
	return term
}

func TestExpressVoiceThenData(t *testing.T) {
	t.Parallel()
	for _, stream := range []uint32{2, 1} {
		for _, mode := range []bs.Mode{bs.ModeNetwork, bs.ModeRF} {
			v := advNetCall(t, 3, 10, 1, 1, time.Second, 3191868)
			last := v[len(v)-1]
			seq0 := (last.h.Seq + 1) & 0xFF
			if stream != 1 {
				seq0 = 100
			}
			d := netData(t, loadBin(t, "d-sms.bin"), enums.Slot1, seq0, stream, last.at+20*time.Millisecond)
			wantD := append([]advPkt(nil), d...)
			cfg := &bs.Config{Mode: mode, ColorCode: 1, CACH: true}
			got := expressRun(t, newRepeater(t, cfg), append(append([]advPkt(nil), v...), d...), 6*time.Second, false)
			term := checkVoice(t, "voice then data", newRepeater(t, cfg), v, got, enums.Slot1)
			out := dataOut(got, enums.Slot1)
			if len(out) != len(wantD) {
				t.Fatalf("stream %d %v: %d data bursts, want %d", stream, mode, len(out), len(wantD))
			}
			for i := range out {
				if payloadOf(out[i].Burst) != payloadOf(wantD[i].raw) {
					t.Fatalf("stream %d %v: data %d out of order", stream, mode, i)
				}
			}
			if first := max(out[0].at, out[0].Due); first < term || first > term+voice.DefaultJitter-20*time.Millisecond+120*time.Millisecond {
				t.Fatalf("stream %d %v: first data out at %v, terminator %v, data arrived %v", stream, mode, first, term, wantD[0].at)
			}
			if mode == bs.ModeNetwork && max(out[0].at, out[0].Due) > term+60*time.Millisecond {
				t.Fatalf("stream %d: first data out at %v, terminator out at %v", stream, max(out[0].at, out[0].Due), term)
			}
		}
	}
}

func TestExpressDataThenVoice(t *testing.T) {
	t.Parallel()
	for _, mode := range []bs.Mode{bs.ModeNetwork, bs.ModeRF} {
		d := netData(t, loadBin(t, "h-sms.bin"), enums.Slot1, 40, 2, time.Second)
		last := d[len(d)-1]
		v := advNetCall(t, 3, 0, 3, 1, last.at+20*time.Millisecond, 3191868)
		wantD := append([]advPkt(nil), d...)
		cfg := &bs.Config{Mode: mode, ColorCode: 1, CACH: true}
		got := expressRun(t, newRepeater(t, cfg), append(append([]advPkt(nil), d...), v...), 6*time.Second, false)
		late := time.Duration(0)
		if mode == bs.ModeRF {
			late = 60 * time.Millisecond
		}
		checkData(t, "data then voice", dataOut(got, enums.Slot1), wantD, func(int) time.Duration { return late })
		checkVoice(t, "data then voice", newRepeater(t, cfg), v, got, enums.Slot1)
	}
}

func TestExpressDataDuringOtherVoice(t *testing.T) {
	t.Parallel()
	v := advNetCall(t, 5, 0, 1, 2, time.Second, 3191868)
	d := netData(t, []layer2.RawBurst{testCSBK(t)}, enums.Slot1, 0, 9, 2*time.Second)
	cfg := &bs.Config{Mode: bs.ModeNetwork, ColorCode: 1}
	got := expressRun(t, newRepeater(t, cfg), append(append([]advPkt(nil), v...), d...), 6*time.Second, false)
	var rejected time.Duration
	for _, o := range got {
		if o.Kind == bs.OutputRejected && o.From.Peer == 1 && rejected == 0 {
			rejected = o.at
		}
	}
	if rejected == 0 || rejected > d[0].at {
		t.Fatalf("CSBK of peer 1 refused at %v, arrived %v", rejected, d[0].at)
	}
	checkVoice(t, "other voice", newRepeater(t, cfg), v, got, enums.Slot1)
}
