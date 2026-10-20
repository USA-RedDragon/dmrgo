package voice_test

import (
	"encoding/binary"
	"encoding/hex"
	"testing"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/vocoder"
)

var (
	ipscCall = []string{
		"800004ce2e3230b43c00000902000058f320805dfb283bbf02a40000000001c0000a800a006000000000000930b43ce9f8ed00714694",
		"800004ce2e3230b43c00000902000058f320805dfb293bbf0484000000008a1440541cbd8ee6d793cd54372cbe82e8c4004a2a78",
		"800004ce2e3230b43c00000902000058f320805dfb2a3bbf0664000000008a1906dc54392640f6312d4a350e936cf863a3f517500000090072",
		"800004ce2e3230b43c00000902000058f320805dfb2b3bbf0844000000008a19064d5f0fa2997082675922d5678bc4820e4c5618060a060676",
		"800004ce2e3230b43c00000902000058f320805dfb2c3bbf0a24000000008a190624461b4658531d9ba79627a12937002adef7b0090c240c76",
		"800004ce2e3230b43c00000902000058f320805dfb2d3bbf0c04000000008a2216d01ff05dedbdaeaab3075525e078b0066268c80c2b2b2400000000000930b43c74",
		"800004ce2e3230b43c00000902000058f320805dfb2e3bbf0de4000000008a19069fcbcb8ec055b04b8eaa424aa8b8815bfbfc300000000070",
		"800004ce2e3230b43c00000902000058f360805efb2f3bbf0fc40000000002c0000a800a006000000000000930b43ce6f7e20072467f",
	}
	ipscLostA = []string{
		"800004ce2e3230b43c00000902000058f320805dfea03bc583a40000000001c0000a800a006000000000000930b43ce9f8ed00714751",
		"800004ce2e3230b43c00000902000058f320805dfea13bc585840000000001c0000a800a006000000000000930b43ce9f8ed00714751",
		"800004ce2e3230b43c00000902000058f320805dfea23bc58764000000008a1906dc54392640f6312d4a350e936cf863a3f517500000090072",
		"800004ce2e3230b43c00000902000058f320805dfea33bc58944000000008a19064d5f0fa2997082675922d5678bc4820e4c5618060a060676",
		"800004ce2e3230b43c00000902000058f320805dfea43bc58b24000000008a190624461b4658531d9ba79627a12937002adef7b0090c240c76",
		"800004ce2e3230b43c00000902000058f320805dfea53bc58d04000000008a2216d01ff05dedbdaeaab3075525e078b0066268c80c2b2b2400000000000930b43c74",
		"800004ce2e3230b43c00000902000058f320805dfea63bc58ee4000000008a19069fcbcb8ec055b04b8eaa424aa8b8815bfbfc300000000070",
		"800004ce2e3230b43c00000902000058f360805efea73bc590c40000000002c0000a800a006000000000000930b43ce6f7e20072470e",
	}
)

func ipscInputs(t *testing.T, packets []string, noEMB bool) []input {
	t.Helper()
	out := make([]input, 0, len(packets))
	for i, p := range packets {
		d, err := hex.DecodeString(p)
		if err != nil {
			t.Fatal(err)
		}
		in := input{at: time.Duration(i) * slot}
		in.hint = layer2.Hint{Slot: enums.Slot2, Seq: binary.BigEndian.Uint16(d[20:22]), HasSeq: true}
		switch d[30] & 0x7F {
		case 0x01, 0x02:
			dt := enums.DataTypeVoiceLCHeader
			if d[30]&0x7F == 0x02 {
				dt = enums.DataTypeTerminatorWithLC
			}
			cw, err := pdu.LCBytes(d[38:47]).Codeword(dt)
			if err != nil {
				t.Fatal(err)
			}
			if in.raw, err = layer2.BuildDataBurst(cw, dt, 7, layer2.SyncSourceBS); err != nil {
				t.Fatal(err)
			}
		case 0x0a:
			frames := vocoder.UnpackAMBEVoice([19]byte(d[33:52]))
			if len(d) == 52 || d[32]&0x40 != 0 {
				in.hint.Kind, in.hint.VoiceBurst = layer2.KindVoice, enums.VoiceBurstA
				if in.raw, err = layer2.BuildVoiceBurst(frames, enums.VoiceBurstA, pdu.EmbeddedSignalling{}, [4]byte{}, layer2.SyncSourceBS); err != nil {
					t.Fatal(err)
				}
				break
			}
			if len(d) == 66 {
				in.hint.Kind, in.hint.VoiceBurst = layer2.KindVoice, enums.VoiceBurstE
			}
			e := d[len(d)-1]
			emb := pdu.EmbeddedSignalling{ColorCode: enums.ColorCode(e >> 4), PreemptionAndPowerControlIndicator: e&0x08 != 0, LCSS: enums.LCSS(e >> 1 & 3)}
			if in.raw, err = layer2.BuildVoiceBurst(frames, enums.VoiceBurstB, emb, [4]byte(d[52:56]), layer2.SyncSourceBS); err != nil {
				t.Fatal(err)
			}
			noErr(in.raw.SetEMB(emb))
			if noEMB {
				b := new(layer2.Burst)
				if err := b.DecodeFromBytesWith(in.raw, &layer2.DecodeOptions{Kind: layer2.KindVoice, VoiceBurst: in.hint.VoiceBurst}); err != nil {
					t.Fatal(err)
				}
				b.EmbeddedSignalling.FEC.Uncorrectable = true
				in.decoded = b
			}
		default:
			t.Fatalf("packet %d: burst type %#x", i, d[30])
		}
		out = append(out, in)
	}
	return out
}

func TestReceiver_IPSCLostBurstA(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		packets   []string
		noEMB     bool
		positions string
		dups      uint16
		lost      uint16
	}{
		{"call", ipscCall, false, "ABCDEF", 0, 0},
		{"lost A", ipscLostA, false, "BCDEF", 1, 1},
		{"lost A, EMB not used", ipscLostA, true, "BCDEF", 1, 1},
	}
	for _, tt := range tests {
		got := feed(t, newReceiver(t, nil), ipscInputs(t, tt.packets, tt.noEMB), -1)
		if p := positions(got); p != tt.positions {
			t.Errorf("%s: positions %s, want %s", tt.name, p, tt.positions)
		}
		end := got[len(got)-1]
		if k := kinds(got); k[0] != 'S' || end.Reason == 0 || end.Call.Target != 9 || end.Call.Source != 3191868 ||
			end.Stats.Lost != tt.lost || end.Stats.Duplicates != tt.dups {
			t.Errorf("%s: events %s, end %v %+v", tt.name, k, end.Reason, end.Stats)
		}
	}
}
