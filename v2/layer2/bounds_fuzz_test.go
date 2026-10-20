package layer2_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/layer3"
	"github.com/USA-RedDragon/dmrgo/v2/vocoder"
)

func burstsBound(n int) int {
	return (n + layer2.VoiceSlotsPerBurst - 1) / layer2.VoiceSlotsPerBurst
}

func runTransmitterBounds(t *testing.T, data []byte) {
	t.Helper()
	lc, err := pdu.NewVoiceLC(true, 9990, 3191868, layer3.ServiceOptions{})
	if err != nil {
		t.Fatal(err)
	}
	opts := &layer2.VoiceTransmitterOptions{LC: lc, ColorCode: 1, Source: layer2.SyncSourceMS}
	if len(data) > 0 && data[0]&1 == 1 {
		opts.PIHeader = &pdu.PIHeader{AlgorithmID: 1}
	}
	tx, err := layer2.NewVoiceTransmitter(opts)
	if err != nil {
		t.Fatal(err)
	}
	if n := len(tx.Headers()); n > 2 {
		t.Fatalf("Headers returned %d bursts", n)
	}
	for i := 0; i < len(data); i++ {
		op, n := data[i]%4, int(data[i]>>2)%11
		switch op {
		case 0:
			slots := make([][9]byte, n)
			for k := range slots {
				slots[k][0] = byte(i + k)
			}
			out, _ := tx.AppendSlots(nil, slots...)
			if len(out) > burstsBound(n) {
				t.Fatalf("AppendSlots of %d slots returned %d bursts", n, len(out))
			}
		case 1:
			frames := make([]vocoder.AMBEFrame, n)
			for k := range frames {
				frames[k] = vocoder.AMBESilence()
				if i+k < len(data) && data[i+k]&0x80 != 0 {
					frames[k][6] |= 1
				}
			}
			out, _ := tx.AddFrames(frames...)
			if len(out) > burstsBound(n) {
				t.Fatalf("AddFrames of %d frames returned %d bursts", n, len(out))
			}
		case 2:
			out, _ := tx.AppendTerminate(nil)
			if len(out) > layer2.VoiceBurstsPerSuperframe+1 {
				t.Fatalf("AppendTerminate returned %d bursts", len(out))
			}
			if err := tx.Reset(opts); err != nil {
				t.Fatal(err)
			}
		default:
			if n := len(tx.AppendHeaders(nil)); n > 2 {
				t.Fatalf("AppendHeaders returned %d bursts", n)
			}
		}
	}
	out, _ := tx.Terminate()
	if len(out) > layer2.VoiceBurstsPerSuperframe+1 {
		t.Fatalf("Terminate returned %d bursts", len(out))
	}
}

func transmitterBoundSeeds() [][]byte {
	return [][]byte{{}, {1}, {0x2c, 0x29, 0x06}, {0x29, 0x29, 0x29, 0x2a, 0x01, 0x05}, {0xfd, 0x80, 0x11, 0x02, 0x03}}
}

func TestVoiceTransmitterBounds(t *testing.T) {
	t.Parallel()
	for _, s := range transmitterBoundSeeds() {
		runTransmitterBounds(t, s)
	}
}

func FuzzVoiceTransmitterBounds(f *testing.F) {
	for _, s := range transmitterBoundSeeds() {
		f.Add(s)
	}
	f.Fuzz(runTransmitterBounds)
}
