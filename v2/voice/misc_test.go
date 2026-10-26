package voice_test

import (
	"testing"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/voice"
)

func TestReceiver_PIHeader(t *testing.T) {
	t.Parallel()

	bursts := transmission(t, groupLC(t, 91, 3191868), 1, func(o *layer2.VoiceTransmitterOptions) {
		o.PIHeader = &pdu.PIHeader{AlgorithmID: 1, KeyID: 2}
	})
	got := feed(t, newReceiver(t, nil), timed(bursts, 0, layer2.Hint{}), -1)
	if k := kinds(got); k != "SP"+repeat("V", 6)+"E" || got[1].Burst != bursts[1] {
		t.Fatalf("events %s", k)
	}
}

func TestReceiver_ColorCode(t *testing.T) {
	t.Parallel()

	bursts := transmission(t, groupLC(t, 91, 3191868), 1, nil)
	cc := enums.ColorCode(2)
	got := feed(t, newReceiver(t, &voice.ReceiverConfig{ColorCode: &cc}), timed(bursts, 0, layer2.Hint{}), -1)
	if len(got) != 0 {
		t.Errorf("color code 1 bursts with ColorCode 2: %s", kinds(got))
	}
}

func TestReceiver_Decoded(t *testing.T) {
	t.Parallel()

	bursts := loadBin(t, "voice.bin")
	want := feed(t, newReceiver(t, nil), timed(bursts, 0, layer2.Hint{}), -1)
	in := timed(bursts, 0, layer2.Hint{})
	for i := range in {
		var b layer2.Burst
		if err := b.DecodeFromBytes(in[i].raw); err != nil {
			t.Fatal(err)
		}
		in[i].decoded = &b
	}
	got := feed(t, newReceiver(t, nil), in, -1)
	if kinds(got) != kinds(want) || positions(got) != positions(want) {
		t.Fatalf("Decoded %s, Burst %s", kinds(got), kinds(want))
	}
	for i := range got {
		if got[i].Burst != want[i].Burst || got[i].Call != want[i].Call {
			t.Fatalf("event %d differs", i)
		}
	}
}

func TestReceiver_LateEntryInHang(t *testing.T) {
	t.Parallel()

	a := transmission(t, groupLC(t, 91, 3191868), 1, nil)
	rx := newReceiver(t, nil)
	end := time.Duration(len(a)) * slot
	in := append(timed(a, 0, layer2.Hint{}), timed(a[2:4], end+time.Second, layer2.Hint{})...)
	got := feed(t, rx, in, end+2*time.Second+slot)
	if k := kinds(got); k != "S"+repeat("V", 6)+"E"+"VV" || rx.State() != voice.StateHang {
		t.Fatalf("events %s, state %v", k, rx.State())
	}
	got = rx.Tick(end+3*time.Second-slot, nil)
	if k := kinds(got); k != "e" || rx.State() != voice.StateIdle {
		t.Fatalf("hang end %s, state %v", k, rx.State())
	}
}

func TestStrings(t *testing.T) {
	t.Parallel()

	tests := []struct{ got, want string }{
		{voice.OriginHeader.String(), "Header"},
		{voice.Origin(9).String(), "Origin(9)"},
		{voice.EndSuperseded.String(), "Superseded"},
		{voice.EndReason(0).String(), "None"},
		{voice.EventTalkerAlias.String(), "TalkerAlias"},
		{voice.EventKind(0).String(), "EventKind(0)"},
		{voice.StateLateEntry.String(), "LateEntry"},
		{voice.State(9).String(), "State(9)"},
	}
	for _, tt := range tests {
		if tt.got != tt.want {
			t.Errorf("%q, want %q", tt.got, tt.want)
		}
	}
	for o := voice.OriginUnknown; o <= voice.OriginExpected; o++ {
		if o.String() == "" {
			t.Error("empty origin name")
		}
	}
	for r := voice.EndTerminator; r <= voice.EndTimeout; r++ {
		if r.String() == "" {
			t.Error("empty reason name")
		}
	}
	for k := voice.EventCallStart; k <= voice.EventSessionEnd; k++ {
		if k.String() == "" {
			t.Error("empty kind name")
		}
	}
	for s := voice.StateIdle; s <= voice.StateHang; s++ {
		if s.String() == "" {
			t.Error("empty state name")
		}
	}
}
