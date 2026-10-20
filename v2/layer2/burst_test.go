package layer2_test

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
)

func TestBurst_ReservedDataTypes(t *testing.T) {
	t.Parallel()

	for dt := enums.DataTypeReserved; dt <= 0xF; dt++ {
		t.Run(fmt.Sprintf("DataType%d", dt), func(t *testing.T) {
			t.Parallel()

			frame := must(lcBurst([12]byte{}, dt, 1))
			{
				var burst layer2.Burst
				if err := burst.DecodeFromBytes(frame); err == nil {
					t.Fatalf("data type %d: expected error", dt)
				}
				if burst.Kind != layer2.KindData || burst.SlotType.DataType != dt {
					t.Fatalf("data type %d: got IsData=%v DataType=%d", dt, (burst.Kind == layer2.KindData), burst.SlotType.DataType)
				}
				if burst.Data != nil {
					t.Errorf("data type %d: expected nil Data", dt)
				}
				if got := must(burst.Encode()); got != frame {
					t.Errorf("data type %d: encode mismatch\ngot  %x\nwant %x", dt, got, frame)
				}
			}
		})
	}
}

func loadBursts(t testing.TB, path string) []layer2.RawBurst {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read file %s: %v", path, err)
	}

	if len(data)%33 != 0 {
		t.Fatalf("file size %d is not a multiple of 33", len(data))
	}

	count := len(data) / 33
	bursts := make([]layer2.RawBurst, count)
	for i := 0; i < count; i++ {
		copy(bursts[i][:], data[i*33:(i+1)*33])
	}
	return bursts
}

func TestBurst_Encode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		file string
	}{
		{"ParrotKerchunk", "testdata/parrot_kerchunk.bin"},
		{"Voice", "testdata/voice.bin"},
		{"Motorola SMS", "testdata/m-sms.bin"},
		{"Hytera SMS", "testdata/h-sms.bin"},
		{"DMR Standard SMS", "testdata/d-sms.bin"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			bursts := loadBursts(t, tt.file)
			for i, data := range bursts {
				burst, err := layer2.NewBurstFromBytes(data)
				if err != nil {
					t.Fatalf("NewBurstFromBytes failed for burst %d: %v", i, err)
				}
				t.Logf("burst: %v", burst.String())

				encoded := must(burst.Encode())

				burst2, err := layer2.NewBurstFromBytes(encoded)
				if err != nil {
					t.Fatalf("NewBurstFromBytes failed for burst %d: %v", i, err)
				}
				encoded2 := must(burst2.Encode())

				if !bytes.Equal(encoded[:], encoded2[:]) {
					t.Errorf("Burst %d stability mismatch:\nfirst  %x\nsecond %x\nSync: %v\nVoice: %v\nHasEmbSig: %v",
						i, encoded, encoded2,
						burst.SyncPattern,
						burst.VoiceBurst,
						burst.HasEmbeddedSignalling())
				} else if !bytes.Equal(data[:], encoded[:]) {
					t.Logf("Burst %d input differed from stable encoding (expected for captured data)", i)
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
		err := burst.DecodeFromBytes(bursts[i%len(bursts)])
		if err != nil {
			b.Fatalf("DecodeFromBytes failed: %v", err)
		}
	}
}

func BenchmarkBurst_Decode_Voice(b *testing.B) { benchmarkDecode(b, "testdata/voice.bin") }

func BenchmarkBurst_Decode_Motorola_SMS(b *testing.B) { benchmarkDecode(b, "testdata/m-sms.bin") }
func BenchmarkBurst_Decode_Hytera_SMS(b *testing.B)   { benchmarkDecode(b, "testdata/h-sms.bin") }
func BenchmarkBurst_Decode_DMR_SMS(b *testing.B)      { benchmarkDecode(b, "testdata/d-sms.bin") }
func BenchmarkBurst_Decode_Parrot(b *testing.B)       { benchmarkDecode(b, "testdata/parrot_kerchunk.bin") }

func benchmarkEncode(b *testing.B, file string) {
	b.Helper()
	bursts := loadBursts(b, file)

	decodedBursts := make([]*layer2.Burst, 0, len(bursts))
	for _, d := range bursts {
		burst, err := layer2.NewBurstFromBytes(d)
		if err != nil {
			b.Fatalf("NewBurstFromBytes failed: %v", err)
		}
		decodedBursts = append(decodedBursts, burst)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = decodedBursts[i%len(decodedBursts)].Encode()
	}
}

func BenchmarkBurst_Encode_Voice(b *testing.B)        { benchmarkEncode(b, "testdata/voice.bin") }
func BenchmarkBurst_Encode_Motorola_SMS(b *testing.B) { benchmarkEncode(b, "testdata/m-sms.bin") }
func BenchmarkBurst_Encode_Hytera_SMS(b *testing.B)   { benchmarkEncode(b, "testdata/h-sms.bin") }
func BenchmarkBurst_Encode_DMR_SMS(b *testing.B)      { benchmarkEncode(b, "testdata/d-sms.bin") }
func BenchmarkBurst_Encode_Parrot(b *testing.B)       { benchmarkEncode(b, "testdata/parrot_kerchunk.bin") }

func flipBurstBits(frame [33]byte, positions ...int) [33]byte {
	for _, pos := range positions {
		frame[pos/8] ^= 0x80 >> (pos % 8)
	}
	return frame
}

func TestBurst_SyncTolerance(t *testing.T) {
	t.Parallel()

	frame := must(lcBurst([12]byte{}, enums.DataTypeIdle, 1))

	noisy := flipBurstBits(frame, layer2.SyncStart, layer2.SyncStart+11, layer2.SyncStart+30, layer2.SyncEnd-1)
	burst, err := layer2.NewBurstFromBytes(noisy)
	if err != nil {
		t.Fatal(err)
	}
	if burst.Kind != layer2.KindData || burst.SyncPattern != enums.SyncBSSourcedData || burst.SyncDistance != 4 {
		t.Fatalf("4 sync bit errors: IsData=%v SyncPattern=%s SyncDistance=%d", (burst.Kind == layer2.KindData), burst.SyncPattern.String(), burst.SyncDistance)
	}
	if got := must(burst.Encode()); got != frame {
		t.Errorf("encode did not restore clean sync\ngot  %x\nwant %x", got, frame)
	}

	noisy = flipBurstBits(noisy, layer2.SyncStart+20)
	burst, _ = layer2.NewBurstFromBytes(noisy)
	if (burst.Kind == layer2.KindData) || burst.SyncPattern != enums.SyncNone || !burst.HasEmbeddedSignalling() {
		t.Errorf("5 sync bit errors: IsData=%v SyncPattern=%s", (burst.Kind == layer2.KindData), burst.SyncPattern.String())
	}
}

func TestBurst_SyncTolerance_EMBPositions(t *testing.T) {
	t.Parallel()

	frame := must(lcBurst([12]byte{}, enums.DataTypeIdle, 1))
	diff := uint64(enums.SyncBSSourcedData ^ enums.SyncMSSourcedData)
	for i := 0; i < 48; i++ {
		if (diff>>(47-i))&1 == 1 {
			frame = flipBurstBits(frame, layer2.SyncStart+i)
		}
	}

	burst, err := layer2.NewBurstFromBytes(flipBurstBits(frame, 115, 155))
	if err != nil {
		t.Fatal(err)
	}
	if burst.Kind != layer2.KindData || burst.SyncPattern != enums.SyncMSSourcedData || burst.SyncDistance != 2 {
		t.Fatalf("IsData=%v SyncPattern=%s SyncDistance=%d", (burst.Kind == layer2.KindData), burst.SyncPattern.String(), burst.SyncDistance)
	}
	if burst.SlotType.DataType != enums.DataTypeIdle {
		t.Errorf("DataType=%v", burst.SlotType.DataType)
	}
	if got := must(burst.Encode()); got != frame {
		t.Errorf("encode did not restore clean sync\ngot  %x\nwant %x", got, frame)
	}
}

func TestBurst_SyncTolerance_EmbeddedVoiceBursts(t *testing.T) {
	t.Parallel()

	files, err := filepath.Glob("testdata/*.bin")
	if err != nil {
		t.Fatal(err)
	}
	embedded := 0
	minDistance := 49
	for _, file := range files {
		for i, data := range loadBursts(t, file) {
			var burst layer2.Burst
			_ = burst.DecodeFromBytes(data)
			if burst.HasEmbeddedSignalling() {
				embedded++
				minDistance = min(minDistance, burst.SyncDistance)
			} else if burst.SyncDistance != 0 {
				t.Errorf("%s burst %d: %s matched at distance %d", file, i, burst.SyncPattern.String(), burst.SyncDistance)
			}
		}
	}
	if embedded == 0 {
		t.Fatal("no embedded signalling bursts in testdata")
	}
	t.Logf("%d embedded signalling bursts, nearest SYNC distance %d", embedded, minDistance)
}
