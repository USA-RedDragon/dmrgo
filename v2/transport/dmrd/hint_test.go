package dmrd_test

import (
	"strconv"
	"testing"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/transport/dmrd"
	"github.com/USA-RedDragon/dmrgo/v2/vocoder"
	"github.com/USA-RedDragon/dmrgo/v2/voice"
)

func TestSetHintFromTransmitter(t *testing.T) {
	t.Parallel()

	tx, err := voice.NewTransmitter(&voice.TransmitterConfig{Slot: enums.Slot1})
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Start(0, voice.Call{Group: true, Target: 91, Source: 3120001}, nil); err != nil {
		t.Fatal(err)
	}
	var out []voice.Output
	kinds := map[layer2.Kind]int{}
	fed := 0
	for now := time.Duration(0); now < 2*time.Second; now += 10 * time.Millisecond {
		for fed < 36 && tx.Room() > 0 {
			if err := tx.Frame(vocoder.AMBESilence()); err != nil {
				t.Fatal(err)
			}
			if fed++; fed == 36 {
				tx.Stop(now)
			}
		}
		out = tx.Tick(now, out[:0])
		for i := range out {
			o := &out[i]
			if o.Kind != voice.OutputBurst {
				continue
			}
			d := dmrd.Data{Source: 3120001, Destination: 91, Burst: o.Burst}
			h := o.Hint
			h.Slot = enums.Slot2
			if err := d.SetHint(h); err != nil {
				t.Fatalf("%+v: %v", o.Hint, err)
			}
			p, err := d.AppendBinary(nil)
			if err != nil {
				t.Fatal(err)
			}
			back, err := dmrd.DecodeData(p)
			if err != nil {
				t.Fatal(err)
			}
			got := back.Hint(0)
			if got.Slot != enums.Slot2 || got.Kind != o.Hint.Kind || got.VoiceBurst != o.Hint.VoiceBurst ||
				got.HasDataType != o.Hint.HasDataType || got.DataType != o.Hint.DataType {
				t.Fatalf("hint %+v, sent %+v", got, o.Hint)
			}
			kinds[o.Hint.Kind]++
		}
	}
	if kinds[layer2.KindVoice] != 12 || kinds[layer2.KindData] < 2 {
		t.Fatalf("bursts %v", kinds)
	}
}

func TestSetHintNil(t *testing.T) {
	t.Parallel()

	var d *dmrd.Data
	if err := d.SetHint(layer2.Hint{Kind: layer2.KindVoice, VoiceBurst: enums.VoiceBurstA}); err == nil {
		t.Fatal("nil Data: no error")
	}
}

func TestHintLostAfterMMDVMHostHeader(t *testing.T) {
	t.Parallel()

	var call []dmrd.Data
	for _, c := range readCapture(t, "mmdvmhost-dmrgateway.txt") {
		if c.dir != "host>gw" || dmrd.Classify(c.p, dmrd.DialectLocal) != dmrd.CommandDMRD {
			continue
		}
		d, err := dmrd.DecodeData(c.p)
		if err != nil {
			t.Fatal(err)
		}
		call = append(call, d)
	}
	if len(call) != 8 || call[0].Seq != 0 {
		t.Fatalf("%d DMRD packets from MMDVMHost, first sequence number %d", len(call), call[0].Seq)
	}
	for _, c := range []struct {
		name string
		keep []int
		seq  []uint8
		want string
	}{
		{"call", []int{0, 1, 2, 3, 4, 5, 6, 7}, nil, "A0B0C0D0E0F0"},
		{"burst A lost", []int{0, 2, 3, 4, 5, 6, 7}, nil, "B1C0D0E0F0"},
		{"late entry at burst E", []int{0, 5, 6, 7}, []uint8{0, 1, 2, 3}, "E0F0"},
		{"late entry at burst E, E lost", []int{0, 6, 7}, []uint8{0, 2, 3}, "F1"},
		{"one counter for both slots, as ipsc2hbpc", []int{0, 1, 2, 3, 4, 5, 6, 7}, []uint8{0, 2, 4, 6, 8, 10, 12, 14}, "A0B0C0D0E0F0"},
	} {
		var tr layer2.PositionTracker
		got := ""
		for i, k := range c.keep {
			d := call[k]
			if c.seq != nil {
				d.Seq = c.seq[i]
			}
			h := d.Hint(0)
			lost, _ := tr.Place(&d.Burst, &h)
			if j, ok := h.VoiceBurst.Index(); ok && h.Kind == layer2.KindVoice {
				got += string("ABCDEF"[j]) + strconv.Itoa(lost)
			}
		}
		if got != c.want {
			t.Errorf("%s: %s, want %s", c.name, got, c.want)
		}
	}
}
