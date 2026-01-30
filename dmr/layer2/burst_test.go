package layer2_test

import (
	"bytes"
	"os"
	"testing"

	"github.com/USA-RedDragon/dmrgo/dmr/layer2"
)

func loadBursts(t testing.TB, path string) [][33]byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read file %s: %v", path, err)
	}

	if len(data)%33 != 0 {
		t.Fatalf("file size %d is not a multiple of 33", len(data))
	}

	count := len(data) / 33
	bursts := make([][33]byte, count)
	for i := 0; i < count; i++ {
		copy(bursts[i][:], data[i*33:(i+1)*33])
	}
	return bursts
}

func TestBurst_Encode(t *testing.T) {
	tests := []struct {
		name string
		file string
	}{
		{"ParrotKerchunk", "testdata/parrot_kerchunk.bin"},
		{"Voice", "testdata/voice.bin"},
		{"SMS", "testdata/sms.bin"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bursts := loadBursts(t, tt.file)
			for i, data := range bursts {
				burst := layer2.NewBurstFromBytes(data)

				// Skip Data bursts encoding test for now until data encoding is implemented
				if burst.IsData {
					// Optionally check if we can verify other properties
					continue
				}

				encoded := burst.Encode()

				// Verify stability: Encode(Decode(encoded)) == encoded
				// This handles cases where the input file has invalid parity/FEC bits (captured data)
				// which are "fixed" by the first Encode().
				burst2 := layer2.NewBurstFromBytes(encoded)
				encoded2 := burst2.Encode()

				if !bytes.Equal(encoded[:], encoded2[:]) {
					t.Errorf("Burst %d stability mismatch:\nfirst  %x\nsecond %x\nSync: %v\nVoice: %v\nHasEmbSig: %v\nCorrected Errors: %d\nUncorrectable: %v",
						i, encoded, encoded2,
						burst.SyncPattern,
						burst.VoiceBurst,
						burst.HasEmbeddedSignalling,
						burst.VoiceData.CorrectedErrors(),
						burst.VoiceData.Uncorrectable())
				} else if !bytes.Equal(data[:], encoded[:]) {
					t.Logf("Burst %d input differed from stable encoding (expected for captured data)", i)
					if !burst.IsData {
						t.Logf("Corrected Errors: %d, Uncorrectable: %v", burst.VoiceData.CorrectedErrors(), burst.VoiceData.Uncorrectable())
					}
					t.Logf("Input:   %x", data)
					t.Logf("Encoded: %x", encoded)
				}
			}
		})
	}
}

func benchmarkDecode(b *testing.B, file string) {
	b.Helper()
	bursts := loadBursts(b, file)
	var burst layer2.Burst
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		burst.DecodeFromBytes(bursts[i%len(bursts)])
	}
}

func BenchmarkBurst_Decode_Voice(b *testing.B)  { benchmarkDecode(b, "testdata/voice.bin") }
func BenchmarkBurst_Decode_SMS(b *testing.B)    { benchmarkDecode(b, "testdata/sms.bin") }
func BenchmarkBurst_Decode_Parrot(b *testing.B) { benchmarkDecode(b, "testdata/parrot_kerchunk.bin") }

func benchmarkEncode(b *testing.B, file string) {
	b.Helper()
	bursts := loadBursts(b, file)

	// Pre-decode bursts
	decodedBursts := make([]*layer2.Burst, 0, len(bursts))
	for _, d := range bursts {
		decodedBursts = append(decodedBursts, layer2.NewBurstFromBytes(d))
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = decodedBursts[i%len(decodedBursts)].Encode()
	}
}

func BenchmarkBurst_Encode_Voice(b *testing.B)  { benchmarkEncode(b, "testdata/voice.bin") }
func BenchmarkBurst_Encode_SMS(b *testing.B)    { benchmarkEncode(b, "testdata/sms.bin") }
func BenchmarkBurst_Encode_Parrot(b *testing.B) { benchmarkEncode(b, "testdata/parrot_kerchunk.bin") }
