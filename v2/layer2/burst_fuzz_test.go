package layer2_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
)

// ambiguousSync reports a decode whose SYNC classification can change on
// re-encode: the first centre matched a SYNC pattern with bit errors, or a
// voice burst B-F re-encoded with an embedded field within the SYNC match
// threshold. A damaged SYNC burst and such a voice burst look the same.
func ambiguousSync(first, second *layer2.Burst) bool {
	if first.SyncPattern != enums.SyncEmbeddedSignalling {
		return first.SyncDistance > 0
	}
	return second.SyncPattern != enums.SyncEmbeddedSignalling
}

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

		{
			var b layer2.Burst
			err := b.DecodeFromBytes(frame)
			first, encErr := b.Encode()
			if err != nil || encErr != nil {
				return
			}

			var b2 layer2.Burst
			_ = b2.DecodeFromBytes(first)
			second, err := b2.Encode()
			if ambiguousSync(&b, &b2) {
				return
			}
			if err != nil {
				t.Fatalf("re-encode failed: %v", err)
			}
			if first != second {
				t.Fatalf("encode not stable\ninput  %x\nfirst  %x\nsecond %x", frame, first, second)
			}
		}
	})
}
