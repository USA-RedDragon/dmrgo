package layer2_test

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/elements"
)

func TestBurst_ReservedDataTypes(t *testing.T) {
	for dt := elements.DataTypeReserved; dt <= 0xF; dt++ {
		t.Run(fmt.Sprintf("DataType%d", dt), func(t *testing.T) {
			frame := must(layer2.BuildLCDataBurst([12]byte{}, dt, 1))
			for _, trunking := range []bool{false, true} {
				var burst layer2.Burst
				burst.SetTrunkingMode(trunking)
				if err := burst.DecodeFromBytes(frame); err == nil {
					t.Fatalf("data type %d: expected error", dt)
				}
				if !burst.IsData || burst.SlotType.DataType != dt {
					t.Fatalf("data type %d: got IsData=%v DataType=%d", dt, burst.IsData, burst.SlotType.DataType)
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
		{"Motorola SMS", "testdata/m-sms.bin"},
		{"Hytera SMS", "testdata/h-sms.bin"},
		{"DMR Standard SMS", "testdata/d-sms.bin"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bursts := loadBursts(t, tt.file)
			for i, data := range bursts {
				burst, err := layer2.NewBurstFromBytes(data)
				if err != nil {
					t.Fatalf("NewBurstFromBytes failed for burst %d: %v", i, err)
				}
				t.Logf("burst: %v", burst.ToString())

				encoded := must(burst.Encode())

				// Verify stability: Encode(Decode(encoded)) == encoded
				// This handles cases where the input file has invalid parity/FEC bits (captured data)
				// which are "fixed" by the first Encode().
				burst2, err := layer2.NewBurstFromBytes(encoded)
				if err != nil {
					t.Fatalf("NewBurstFromBytes failed for burst %d: %v", i, err)
				}
				encoded2 := must(burst2.Encode())

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

	// Pre-decode bursts
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
	frame := must(layer2.BuildLCDataBurst([12]byte{}, elements.DataTypeIdle, 1))

	noisy := flipBurstBits(frame, layer2.SyncStart, layer2.SyncStart+11, layer2.SyncStart+30, layer2.SyncEnd-1)
	burst, err := layer2.NewBurstFromBytes(noisy)
	if err != nil {
		t.Fatal(err)
	}
	if !burst.IsData || burst.SyncPattern != enums.BsSourcedData || burst.SyncDistance != 4 {
		t.Fatalf("4 sync bit errors: IsData=%v SyncPattern=%s SyncDistance=%d", burst.IsData, enums.SyncPatternToName(burst.SyncPattern), burst.SyncDistance)
	}
	if got := must(burst.Encode()); got != frame {
		t.Errorf("encode did not restore clean sync\ngot  %x\nwant %x", got, frame)
	}

	noisy = flipBurstBits(noisy, layer2.SyncStart+20)
	burst, _ = layer2.NewBurstFromBytes(noisy)
	if burst.IsData || burst.SyncPattern != enums.EmbeddedSignallingPattern || !burst.HasEmbeddedSignalling {
		t.Errorf("5 sync bit errors: IsData=%v SyncPattern=%s", burst.IsData, enums.SyncPatternToName(burst.SyncPattern))
	}
}

// TestBurst_SyncTolerance_ValidEMB checks a voice burst B-F whose centre is
// 4 bits from BS-sourced data SYNC. Its EMB is a valid QR(16,7,6) codeword,
// so it must stay embedded signalling. FuzzBurstDecode found this frame.
func TestBurst_SyncTolerance_ValidEMB(t *testing.T) {
	raw, err := hex.DecodeString("3230307030303030703230307030ff57d75df5f130303070323030703030303070")
	if err != nil {
		t.Fatal(err)
	}
	var frame [33]byte
	copy(frame[:], raw)

	burst, err := layer2.NewBurstFromBytes(frame)
	if err != nil {
		t.Fatal(err)
	}
	if burst.IsData || !burst.HasEmbeddedSignalling || burst.SyncPattern != enums.EmbeddedSignallingPattern {
		t.Fatalf("IsData=%v HasEmbeddedSignalling=%v SyncPattern=%s", burst.IsData, burst.HasEmbeddedSignalling, enums.SyncPatternToName(burst.SyncPattern))
	}
	if burst.SyncDistance != 4 || burst.FEC.EMB.ErrorsCorrected != 0 {
		t.Errorf("SyncDistance=%d EMB FEC %+v", burst.SyncDistance, burst.FEC.EMB)
	}
	if got := must(burst.Encode()); got != frame {
		t.Errorf("encode mismatch\ngot  %x\nwant %x", got, frame)
	}
}

// TestBurst_SyncTolerance_EmbeddedVoiceBursts checks that tolerant SYNC
// matching does not take a voice burst B-F (EMB and embedded signalling in
// the centre) as a SYNC burst. The captures have clean SYNC fields, so any
// SYNC match at a non-zero distance would be a misclassified B-F burst.
func TestBurst_SyncTolerance_EmbeddedVoiceBursts(t *testing.T) {
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
			if burst.HasEmbeddedSignalling {
				embedded++
				minDistance = min(minDistance, burst.SyncDistance)
			} else if burst.SyncDistance != 0 {
				t.Errorf("%s burst %d: %s matched at distance %d", file, i, enums.SyncPatternToName(burst.SyncPattern), burst.SyncDistance)
			}
		}
	}
	if embedded == 0 {
		t.Fatal("no embedded signalling bursts in testdata")
	}
	t.Logf("%d embedded signalling bursts, nearest SYNC distance %d", embedded, minDistance)
}
