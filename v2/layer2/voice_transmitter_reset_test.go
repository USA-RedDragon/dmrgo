package layer2_test

import (
	"slices"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/internal/bits"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/vocoder"
)

func transmit(t *testing.T, tx *layer2.VoiceTransmitter, n int) []layer2.RawBurst {
	t.Helper()
	out := tx.AppendHeaders(nil)
	for i := range n {
		var slot [9]byte
		bits.SetBit(slot[:], i%72, 1)
		var err error
		if out, err = tx.AppendSlots(out, slot); err != nil {
			t.Fatal(err)
		}
	}
	out, err := tx.AppendTerminate(out)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestVoiceTransmitter_Reset(t *testing.T) {
	t.Parallel()

	first := &layer2.VoiceTransmitterOptions{LC: groupLC(enums.FIDStandardized), ColorCode: 1, Silence: vocoder.AMBESilenceSlot()}
	lc2 := groupLC(enums.FIDStandardized)
	mustSet(lc2.SetGroupVoice(pdu.FLCGroupVoice{GroupAddress: 9, SourceAddress: 3120001}))
	second := &layer2.VoiceTransmitterOptions{LC: lc2, ColorCode: 7, Source: layer2.SyncSourceMS, Silence: vocoder.AMBESilenceSlot(),
		PIHeader: &pdu.PIHeader{AlgorithmID: 1, KeyID: 2}}
	want := transmit(t, must(layer2.NewVoiceTransmitter(second)), 40)

	for _, slots := range []int{40, 7} {
		tx := must(layer2.NewVoiceTransmitter(first))
		if slots == 40 {
			transmit(t, tx, slots)
		} else {
			var slot [9]byte
			for range slots {
				if _, err := tx.AppendSlots(nil, slot); err != nil {
					t.Fatal(err)
				}
			}
		}
		if err := tx.Reset(second); err != nil {
			t.Fatal(err)
		}
		if got := transmit(t, tx, 40); !slices.Equal(got, want) {
			t.Errorf("after %d slots of a first call: the second call differs from a new VoiceTransmitter", slots)
		}
	}

	var zero layer2.VoiceTransmitter
	if err := zero.Reset(second); err != nil {
		t.Fatal(err)
	}
	if got := transmit(t, &zero, 40); !slices.Equal(got, want) {
		t.Error("Reset of the zero VoiceTransmitter differs from a new VoiceTransmitter")
	}
}

func TestVoiceTransmitter_ResetError(t *testing.T) {
	t.Parallel()

	opts := &layer2.VoiceTransmitterOptions{LC: groupLC(enums.FIDStandardized), ColorCode: 1, Silence: vocoder.AMBESilenceSlot()}
	want := transmit(t, must(layer2.NewVoiceTransmitter(opts)), 12)
	tx := must(layer2.NewVoiceTransmitter(opts))
	if err := tx.Reset(&layer2.VoiceTransmitterOptions{Source: layer2.SyncSource(9)}); err == nil {
		t.Fatal("unknown sync source: no error")
	}
	if got := transmit(t, tx, 12); !slices.Equal(got, want) {
		t.Error("a failed Reset changed the VoiceTransmitter")
	}
}
