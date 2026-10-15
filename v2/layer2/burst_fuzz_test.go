package layer2_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/layer2"
)

func FuzzBurstDecode(f *testing.F) {
	files, err := filepath.Glob("testdata/*.bin")
	if err != nil {
		f.Fatal(err)
	}
	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			f.Fatal(err)
		}
		for i := 0; i+33 <= len(data); i += 33 {
			f.Add(data[i : i+33])
		}
	}

	f.Fuzz(func(t *testing.T, raw []byte) {
		if len(raw) != 33 {
			return
		}
		var frame [33]byte
		copy(frame[:], raw)

		for _, trunking := range []bool{false, true} {
			var b layer2.Burst
			b.SetTrunkingMode(trunking)
			err := b.DecodeFromBytes(frame)
			first := b.Encode()
			if err != nil {
				continue
			}

			var b2 layer2.Burst
			b2.SetTrunkingMode(trunking)
			_ = b2.DecodeFromBytes(first)
			second := b2.Encode()
			if first != second {
				t.Fatalf("trunking=%v: encode not stable\ninput  %x\nfirst  %x\nsecond %x", trunking, frame, first, second)
			}
		}
	})
}
