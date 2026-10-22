package voice_test

import (
	"bufio"
	"encoding/hex"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/layer3"
	"github.com/USA-RedDragon/dmrgo/v2/vocoder"
	"github.com/USA-RedDragon/dmrgo/v2/voice"
)

const slot = 60 * time.Millisecond

type input struct {
	at      time.Duration
	raw     layer2.RawBurst
	hint    layer2.Hint
	lost    bool
	decoded *layer2.Burst
}

func feed(t *testing.T, rx *voice.Receiver, in []input, tick time.Duration) []voice.Event {
	t.Helper()
	var all []voice.Event
	buf := make([]voice.Event, 0, voice.MaxEvents)
	for i := range in {
		switch {
		case in[i].lost:
			buf = rx.Lost(in[i].at, buf[:0])
		case in[i].decoded != nil:
			buf = rx.Decoded(in[i].at, in[i].decoded, in[i].hint, buf[:0])
		default:
			buf = rx.Burst(in[i].at, &in[i].raw, in[i].hint, buf[:0])
		}
		if len(buf) > voice.MaxEvents {
			t.Fatalf("input %d: %d events, more than MaxEvents", i, len(buf))
		}
		all = append(all, buf...)
	}
	if tick >= 0 {
		all = append(all, rx.Tick(tick, buf[:0])...)
	}
	return all
}

func timed(bursts []layer2.RawBurst, start time.Duration, h layer2.Hint) []input {
	out := make([]input, len(bursts))
	for i, b := range bursts {
		out[i] = input{at: start + time.Duration(i)*slot, raw: b, hint: h}
	}
	return out
}

func kinds(events []voice.Event) string {
	var b strings.Builder
	for _, e := range events {
		b.WriteByte(" SVMTPEse"[e.Kind])
	}
	return b.String()
}

func positions(events []voice.Event) string {
	var b strings.Builder
	for _, e := range events {
		if e.Kind != voice.EventVoice {
			continue
		}
		if e.Position == enums.VoiceBurstUnknown {
			b.WriteByte('?')
		} else {
			b.WriteString(e.Position.String())
		}
	}
	return b.String()
}

func lostCounts(events []voice.Event) map[int]int {
	out := map[int]int{}
	n := 0
	for _, e := range events {
		if e.Kind != voice.EventVoice {
			continue
		}
		if e.Lost != 0 {
			out[n] = int(e.Lost)
		}
		n++
	}
	return out
}

func find(events []voice.Event, k voice.EventKind) []voice.Event {
	var out []voice.Event
	for _, e := range events {
		if e.Kind == k {
			out = append(out, e)
		}
	}
	return out
}

func newReceiver(t *testing.T, cfg *voice.ReceiverConfig) *voice.Receiver {
	t.Helper()
	rx, err := voice.NewReceiver(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return rx
}

func groupLC(t *testing.T, target, source uint32) pdu.FullLinkControl {
	t.Helper()
	lc, err := pdu.NewVoiceLC(true, target, source, layer3.ServiceOptions{})
	if err != nil {
		t.Fatal(err)
	}
	return lc
}

func transmission(t *testing.T, lc pdu.FullLinkControl, superframes int, opts func(*layer2.VoiceTransmitterOptions)) []layer2.RawBurst {
	t.Helper()
	o := layer2.VoiceTransmitterOptions{LC: lc, ColorCode: 1, Silence: vocoder.AMBESilenceSlot()}
	if opts != nil {
		opts(&o)
	}
	tx, err := layer2.NewVoiceTransmitter(&o)
	if err != nil {
		t.Fatal(err)
	}
	out := tx.AppendHeaders(nil)
	for i := range superframes * layer2.VoiceBurstsPerSuperframe * layer2.VoiceSlotsPerBurst {
		var s [9]byte
		s[i%72/8] = 0x80 >> (i % 8)
		if out, err = tx.AppendSlots(out, s); err != nil {
			t.Fatal(err)
		}
	}
	if out, err = tx.AppendTerminate(out); err != nil {
		t.Fatal(err)
	}
	return out
}

func loadBin(t testing.TB, name string) []layer2.RawBurst {
	t.Helper()
	data, err := os.ReadFile("../layer2/testdata/" + name)
	if err != nil || len(data)%33 != 0 {
		t.Fatalf("read %s: %v", name, err)
	}
	out := make([]layer2.RawBurst, 0, len(data)/33)
	for i := 0; i < len(data); i += 33 {
		out = append(out, layer2.RawBurst(data[i:i+33]))
	}
	return out
}

func loadGolden(t *testing.T, name string) []layer2.RawBurst {
	t.Helper()
	f, err := os.Open("testdata/golden_bursts.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var out []layer2.RawBurst
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) != 3 || fields[0] != name {
			continue
		}
		b, err := hex.DecodeString(fields[2])
		if err != nil || len(b) != 33 {
			t.Fatalf("%s: %v", sc.Text(), err)
		}
		out = append(out, layer2.RawBurst(b))
	}
	if len(out) == 0 {
		t.Fatalf("no bursts for %s", name)
	}
	return out
}

func loadWeak(t testing.TB) []input {
	t.Helper()
	f, err := os.Open("testdata/xpr8400_bs_voice_weak.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var out []input
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) != 3 || strings.HasPrefix(fields[0], "#") || fields[1] != "2" {
			continue
		}
		ms, err := strconv.ParseFloat(fields[0], 64)
		b, err2 := hex.DecodeString(fields[2])
		if err != nil || err2 != nil || len(b) != 33 {
			t.Fatalf("%s: %v %v", sc.Text(), err, err2)
		}
		out = append(out, input{at: time.Duration(ms * float64(time.Millisecond)), raw: layer2.RawBurst(b), hint: layer2.Hint{Slot: enums.Slot2}})
	}
	return out
}
