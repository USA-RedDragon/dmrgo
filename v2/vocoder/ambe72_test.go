package vocoder_test

import (
	"encoding/hex"
	"math/rand"
	"os"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/internal/bits"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/vocoder"
)

func voiceSlots(t testing.TB) [][9]byte {
	t.Helper()
	data, err := os.ReadFile("../layer2/testdata/voice.bin")
	if err != nil {
		t.Fatal(err)
	}
	var slots [][9]byte
	for off := 0; off+33 <= len(data); off += 33 {
		var raw [33]byte
		copy(raw[:], data[off:off+33])
		burst, err := layer2.NewBurstFromBytes(raw)
		if err != nil {
			t.Fatalf("burst at %d: %v", off, err)
		}
		if burst.Kind == layer2.KindData {
			continue
		}
		var voice [27]byte
		bits.Copy(voice[:], 0, raw[:], 0, 108)
		bits.Copy(voice[:], 108, raw[:], 156, 108)
		for i := range 3 {
			slots = append(slots, [9]byte(voice[i*9:(i+1)*9]))
		}
	}
	if len(slots) == 0 {
		t.Fatal("no voice bursts in voice.bin")
	}
	return slots
}

func TestAMBE72RoundTrip(t *testing.T) {
	t.Parallel()

	clean := 0
	for i, slot := range voiceSlots(t) {
		info, res := vocoder.DecodeAMBE72(slot)
		if res.Uncorrectable {
			continue
		}
		reencoded, err := vocoder.EncodeAMBE72(info)
		if err != nil {
			t.Fatalf("slot %d: %v", i, err)
		}
		if res.ErrorsCorrected == 0 {
			clean++
			if reencoded != slot {
				t.Errorf("slot %d: clean slot did not re-encode to itself", i)
			}
		}
		info2, res2 := vocoder.DecodeAMBE72(reencoded)
		if info2 != info || res2.ErrorsCorrected != 0 || res2.Uncorrectable {
			t.Errorf("slot %d: re-encoded slot decode = %+v", i, res2)
		}
		if res2.BitsChecked != 47 {
			t.Errorf("slot %d: BitsChecked = %d, want 47", i, res2.BitsChecked)
		}
	}
	if clean == 0 {
		t.Error("no clean slots in voice.bin")
	}
}

func TestAMBE72CorrectsErrors(t *testing.T) {
	t.Parallel()

	rng := rand.New(rand.NewSource(1))
	aPos := []int{0, 4, 8}
	bPos := []int{25, 29, 33}
	for range 200 {
		var info vocoder.AMBEFrame
		for i := range 49 {
			bits.SetBit(info[:], i, byte(rng.Intn(2)))
		}
		slot, err := vocoder.EncodeAMBE72(info)
		if err != nil {
			t.Fatal(err)
		}
		for _, p := range append(aPos, bPos...) {
			slot[p/8] ^= 0x80 >> (p % 8)
		}
		got, res := vocoder.DecodeAMBE72(slot)
		if got != info || res.Uncorrectable || res.ErrorsCorrected != 6 {
			t.Fatalf("decode with 3+3 errors: equal=%v result=%+v", got == info, res)
		}
	}
}

func TestAMBESilence(t *testing.T) {
	t.Parallel()

	slot, err := vocoder.EncodeAMBE72(vocoder.AMBESilence())
	if err != nil {
		t.Fatal(err)
	}
	if slot != vocoder.AMBESilenceSlot() {
		t.Fatal("AMBESilenceSlot differs from EncodeAMBE72(AMBESilence())")
	}
	if got := hex.EncodeToString(slot[:]); got != "b9e881526173002a6b" {
		t.Fatalf("EncodeAMBE72(AMBESilence()) = %s, want b9e881526173002a6b", got)
	}
	if f, res := vocoder.DecodeAMBE72(slot); f != vocoder.AMBESilence() || res.HasErrors() {
		t.Fatalf("DecodeAMBE72 = %v %+v", f, res)
	}
}
