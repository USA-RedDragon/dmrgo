package dmrd_test

import (
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

	tx, err := voice.NewTransmitter(nil)
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
				tx.Stop()
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
