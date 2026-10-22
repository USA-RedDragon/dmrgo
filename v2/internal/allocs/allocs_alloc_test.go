//go:build !tinygo

package allocs_test

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/internal/dvsisim"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/framer"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/packet"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/layer3"
	"github.com/USA-RedDragon/dmrgo/v2/vocoder"
	"github.com/USA-RedDragon/dmrgo/v2/vocoder/dvsi"
	"github.com/USA-RedDragon/dmrgo/v2/voice"
)

const testdata = "../../layer2/testdata"

func captures(t *testing.T) map[string][]layer2.RawBurst {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(testdata, "*.bin"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no captures: %v", err)
	}
	out := map[string][]layer2.RawBurst{}
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil || len(data)%33 != 0 {
			t.Fatalf("read %s: %v", f, err)
		}
		var bursts []layer2.RawBurst
		for i := 0; i < len(data); i += 33 {
			bursts = append(bursts, layer2.RawBurst(data[i:i+33]))
		}
		out[filepath.Base(f)] = bursts
	}
	return out
}

func assertZero(t *testing.T, name string, f func()) {
	t.Helper()
	f()
	if n := testing.AllocsPerRun(20, f); n != 0 {
		t.Errorf("%s: %.1f allocations per run, want 0", name, n)
	}
}

func TestAllocsDecodeEncodeCaptures(t *testing.T) {
	for name, bursts := range captures(t) {
		var b layer2.Burst
		assertZero(t, name, func() {
			for _, raw := range bursts {
				if b.DecodeFromBytes(raw) == nil {
					_, _ = b.Encode()
				}
			}
		})
	}
}

func TestAllocsNewBurstFirstDecode(t *testing.T) {
	all := captures(t)
	streams := make([][]layer2.RawBurst, 0, len(all))
	for _, c := range all {
		streams = append(streams, c)
	}
	bursts := make([]*layer2.Burst, len(streams))
	best := ^uint64(0)
	for range 5 {
		for i := range bursts {
			bursts[i] = layer2.NewBurst()
		}
		var before, after runtime.MemStats
		runtime.ReadMemStats(&before)
		for i, c := range streams {
			for _, raw := range c {
				_ = bursts[i].DecodeFromBytes(raw)
			}
		}
		runtime.ReadMemStats(&after)
		best = min(best, after.Mallocs-before.Mallocs)
	}
	if best != 0 {
		t.Errorf("first decodes into NewBurst: %d allocations, want 0", best)
	}
}

func TestAllocsPacketAssembly(t *testing.T) {
	all := captures(t)
	for _, name := range []string{"d-sms.bin", "h-sms.bin", "m-sms.bin"} {
		bursts := all[name]
		var b layer2.Burst
		a := packet.NewAssembler(127)
		assertZero(t, name, func() {
			for _, raw := range bursts {
				if b.DecodeFromBytes(raw) == nil {
					_, _, _ = a.AddBurst(&b)
				}
			}
		})
	}
}

func TestAllocsPacketRetry(t *testing.T) {
	b := packet.NewBuilder(127)
	var h pdu.DataHeader
	if err := h.SetConfirmed(pdu.ConfirmedDataHeader{SAP: 4, LLIDDestination: 2, LLIDSource: 1, FullMessageFlag: true}); err != nil {
		t.Fatal(err)
	}
	p, err := b.Segment(h, make([]byte, 100), enums.DataTypeRate12)
	if err != nil {
		t.Fatal(err)
	}
	var rd packet.ResponseData
	rd.Retry[2], rd.Retry[5] = true, true
	assertZero(t, "retry", func() {
		if r, err := b.Retry(&p, &rd); err != nil || len(r.Blocks) != 2 {
			t.Fatalf("retry: %v", err)
		}
	})
}

func TestAllocsVoiceSuperframe(t *testing.T) {
	var frames []vocoder.AMBEFrame
	var b layer2.Burst
	for _, raw := range captures(t)["parrot_kerchunk.bin"] {
		if b.DecodeFromBytes(raw) == nil && b.Kind == layer2.KindVoice {
			f, _ := b.AMBEFrames()
			frames = append(frames, f[:]...)
		}
	}
	if len(frames) < 3*layer2.VoiceBurstsPerSuperframe {
		t.Fatalf("parrot capture has %d frames", len(frames))
	}
	frames = frames[:3*layer2.VoiceBurstsPerSuperframe]
	lc, err := pdu.NewVoiceLC(true, 91, 3191868, layer3.ServiceOptions{})
	if err != nil {
		t.Fatal(err)
	}
	tx, err := layer2.NewVoiceTransmitter(&layer2.VoiceTransmitterOptions{LC: lc, ColorCode: 1, Silence: vocoder.AMBESilenceSlot()})
	if err != nil {
		t.Fatal(err)
	}
	out := make([]layer2.RawBurst, 0, 2*layer2.VoiceBurstsPerSuperframe)
	var rx layer2.VoiceReceiver
	var rb layer2.Burst
	assertZero(t, "superframe", func() {
		out, _ = tx.AppendFrames(out[:0], frames...)
		for _, raw := range out {
			if rb.DecodeFromBytesWith(raw, &layer2.DecodeOptions{Kind: layer2.KindVoice}) == nil {
				_, _, _ = rx.Receive(&rb, nil)
			}
		}
	})
}

func TestAllocsVoiceTransmitterReset(t *testing.T) {
	lc, err := pdu.NewVoiceLC(true, 91, 3191868, layer3.ServiceOptions{})
	if err != nil {
		t.Fatal(err)
	}
	opts := &layer2.VoiceTransmitterOptions{LC: lc, ColorCode: 1, Silence: vocoder.AMBESilenceSlot(), PIHeader: &pdu.PIHeader{AlgorithmID: 1}}
	var tx layer2.VoiceTransmitter
	assertZero(t, "reset", func() {
		if err := tx.Reset(opts); err != nil {
			t.Fatal(err)
		}
	})
	bad := &layer2.VoiceTransmitterOptions{Source: layer2.SyncSource(9)}
	assertZero(t, "reset with an unknown source", func() {
		if err := tx.Reset(bad); err == nil {
			t.Fatal("no error")
		}
	})
}

func TestAllocsVoiceReceiver(t *testing.T) {
	all := captures(t)
	rx, err := voice.NewReceiver(nil)
	if err != nil {
		t.Fatal(err)
	}
	out := make([]voice.Event, 0, voice.MaxEvents)
	var now time.Duration
	for _, name := range []string{"voice.bin", "parrot_kerchunk.bin"} {
		bursts := all[name]
		assertZero(t, name, func() {
			for i := range bursts {
				now += 60 * time.Millisecond
				out = rx.Burst(now, &bursts[i], layer2.Hint{}, out[:0])
			}
			now += 10 * time.Second
			out = rx.Tick(now, out[:0])
		})
	}
}

func TestAllocsVoiceTransmitter(t *testing.T) {
	tx, err := voice.NewTransmitter(&voice.TransmitterConfig{ColorCode: 1, Silence: vocoder.AMBESilenceSlot(), Queue: 3 * 20})
	if err != nil {
		t.Fatal(err)
	}
	gps := pdu.FLCGPSInfo{Latitude: 41.75}
	opts := &voice.CallOptions{PIHeader: &pdu.PIHeader{AlgorithmID: 1}, TalkerAlias: "KI5VMF", GPS: &gps}
	out := make([]voice.Output, 0, voice.MaxOutputs)
	var now time.Duration
	drain := func() {
		for {
			at, ok := tx.Deadline()
			if !ok {
				return
			}
			now = max(now, at)
			out = tx.Tick(now, out[:0])
		}
	}
	assertZero(t, "source", func() {
		if err := tx.Start(now, voice.Call{Group: true, Target: 91, Source: 3191868}, opts); err != nil {
			t.Fatal(err)
		}
		for range 3 * 12 {
			if tx.Room() == 0 {
				out = tx.Tick(now, out[:0])
				now += 60 * time.Millisecond
			}
			_ = tx.Frame(vocoder.AMBESilence())
		}
		tx.Stop()
		drain()
	})
	rx, err := voice.NewReceiver(nil)
	if err != nil {
		t.Fatal(err)
	}
	ev := make([]voice.Event, 0, voice.MaxEvents)
	bursts := captures(t)["voice.bin"]
	assertZero(t, "relay", func() {
		for i := range bursts {
			now += 60 * time.Millisecond
			ev = rx.Burst(now, &bursts[i], layer2.Hint{}, ev[:0])
			for j := range ev {
				_ = tx.Relay(now, &ev[j])
			}
			out = tx.Tick(now, out[:0])
		}
		drain()
		now += 10 * time.Second
		ev = rx.Tick(now, ev[:0])
	})
}

func TestAllocsVocoder(t *testing.T) {
	var slots [][9]byte
	var b layer2.Burst
	for _, raw := range captures(t)["voice.bin"] {
		if b.DecodeFromBytes(raw) == nil && b.Kind == layer2.KindVoice {
			slots = append(slots, b.VoiceSlots[:]...)
		}
	}
	assertZero(t, "voice.bin", func() {
		for i := 0; i+3 <= len(slots); i += 3 {
			var frames [3]vocoder.AMBEFrame
			for j := range frames {
				frames[j], _ = vocoder.DecodeAMBE72(slots[i+j])
				_, _ = vocoder.EncodeAMBE72(frames[j])
			}
			payload, _ := vocoder.PackAMBEVoice(frames)
			_ = vocoder.UnpackAMBEVoice(payload)
		}
	})
}

func TestAllocsFramer(t *testing.T) {
	var stream []byte
	for _, raw := range captures(t)["voice.bin"] {
		stream = append(stream, raw[:]...)
	}
	f, err := framer.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	dst := make([]framer.Burst, 0, 16)
	assertZero(t, "voice.bin", func() {
		for i := 0; i < len(stream); i += 13 {
			chunk := stream[i:min(i+13, len(stream))]
			dst = f.AppendBursts(dst[:0], chunk, 8*len(chunk))
		}
	})
}

func TestAllocsDVSI(t *testing.T) {
	pairs := dvsisim.Pairs(4)
	for _, opt := range []dvsisim.Options{{}, {Chunk: 3}, {Datagram: true}} {
		d := dvsisim.New(pairs, opt)
		c, err := dvsi.New(d, nil)
		if err != nil {
			t.Fatal(err)
		}
		pcm := make([]int16, 0, dvsi.FrameSamples)
		pkt := make([]byte, 0, dvsi.MaxPacketLen)
		assertZero(t, "dvsi", func() {
			for _, p := range pairs {
				pcm, _ = c.Decode(pcm[:0], p.Slot)
				_, _ = c.Encode(p.PCM[:])
				pkt, _ = dvsi.AppendSpeech(pkt[:0], 1, p.PCM[:], true)
				q, _, _ := dvsi.Parse(pkt, true)
				pcm, _, _ = q.AppendSamples(pcm[:0])
				pkt, _ = dvsi.AppendChannel(pkt[:0], 2, p.Slot[:], dvsi.FrameBits, false)
				q, _, _ = dvsi.Parse(pkt, false)
				pkt, _, _, _ = q.AppendChannelData(pkt[:0])
			}
		})
		if len(d.Errs) > 0 {
			t.Errorf("%+v: device errors %q", opt, d.Errs)
		}
	}
}
