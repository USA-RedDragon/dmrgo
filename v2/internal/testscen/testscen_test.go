package testscen

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/internal/rftx"
	"github.com/USA-RedDragon/dmrgo/v2/internal/testseed"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
)

func TestSameSeedSameScenario(t *testing.T) {
	t.Parallel()
	a, b, c := Of(42), Of(42), Of(43)
	if a != b || a == c {
		t.Fatalf("seed 42 draws %+v then %+v, seed 43 %+v", a, b, c)
	}
	cfg := rftx.SeqConfig{Message: "0123456789"}
	if a.Apply(cfg) != b.Apply(cfg) {
		t.Fatal("one scenario applies two messages")
	}
}

func TestScenariosAreValidDMR(t *testing.T) {
	t.Parallel()
	g := testseed.New(t)
	for range 64 {
		s := Draw(g)
		for _, a := range []uint32{s.Source, s.Dest, s.BS} {
			if a < 1 || a > MaxAddress {
				t.Fatalf("address %d out of range", a)
			}
		}
		cfg := s.ApplyAll(rftx.SeqConfig{Wakeups: 1, Superframes: 1, Message: "0123456789ABCDEF"})
		if len(cfg.Message) != 16 {
			t.Fatalf("message length %d, want 16", len(cfg.Message))
		}
		for _, seq := range []string{"voice", "data12", "data34", "data1"} {
			b, err := rftx.Sequence(seq, cfg)
			if err != nil {
				t.Fatalf("%s of %+v: %v", seq, cfg, err)
			}
			for i, r := range b.Bursts {
				if _, err := layer2.NewBurstFromBytes(r); err != nil {
					t.Fatalf("%s burst %d does not decode: %v", seq, i, err)
				}
			}
		}
		r, err := CSBK(g, s.ColorCode, layer2.SyncSourceBS)
		if err != nil {
			t.Fatal(err)
		}
		b, err := layer2.NewBurstFromBytes(r)
		if err != nil || b.SlotType.DataType != enums.DataTypeCSBK {
			t.Fatalf("CSBK burst decodes as %v, %v", b, err)
		}
	}
}

func TestVoiceSeedChangesTheVoice(t *testing.T) {
	t.Parallel()
	if rftx.VoiceFrame(0, 5) != rftx.TestAMBEFrame(5) || rftx.VoiceFrame(Of(1).VoiceSeed, 5) == rftx.TestAMBEFrame(5) {
		t.Fatal("VoiceSeed 0 is not the test pattern, or another seed is")
	}
}
