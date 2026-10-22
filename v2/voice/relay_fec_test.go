package voice_test

import (
	"math/rand/v2"
	"testing"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	refambe "github.com/USA-RedDragon/dmrgo/v2/internal/fecref/ambe"
	"github.com/USA-RedDragon/dmrgo/v2/internal/unpacked"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/vocoder"
	"github.com/USA-RedDragon/dmrgo/v2/voice"
)

func flipAMBE(s [9]byte, a, b, c int) [9]byte {
	for i := range a {
		s[i] ^= 0x80
	}
	for i := range b {
		s[3+i] ^= 0x40
	}
	for i := range c {
		s[i] ^= 0x10
	}
	return s
}

func TestRegression_RelayRegeneratesAMBEFEC(t *testing.T) {
	t.Parallel()

	r := rand.New(rand.NewPCG(1, 2))
	for _, fid := range []enums.FID{enums.FIDStandardized, enums.FIDMotorola, 0xC2} {
		tx := newTransmitter(t, &voice.TransmitterConfig{Queue: voice.MaxQueue})
		c := rcall()
		lc := c.LC
		lc.SetFID(fid)
		c.LC, c.FID = lc, fid
		if err := tx.Relay(0, &voice.Event{Kind: voice.EventCallStart, Call: c}); err != nil {
			t.Fatal(err)
		}
		var frames [][3]vocoder.AMBEFrame
		var sent, want [][3][9]byte
		out := tx.Tick(0, nil)
		for i := 1; i <= 12; i++ {
			var f [3]vocoder.AMBEFrame
			var in, w [3][9]byte
			for j := range f {
				for k := range 6 {
					f[j][k] = byte(r.Uint32())
				}
				f[j][6] = byte(r.Uint32()) & 0x80
				clean, err := vocoder.EncodeAMBE72(f[j])
				if err != nil {
					t.Fatal(err)
				}
				switch j {
				case 0:
					in[j] = flipAMBE(clean, 3, 2, 1)
					w[j] = flipAMBE(clean, 0, 0, 1)
				case 1:
					in[j] = flipAMBE(clean, 1, 3, 0)
					w[j] = clean
				case 2:
					in[j] = flipAMBE(clean, 4-i%2, 3*(i%2)+i%3*(1-i%2), 0)
					w[j] = in[j]
				}
			}
			if fid != enums.FIDStandardized && fid != enums.FIDMotorola {
				w = in
			}
			b := vburst(t, (i-1)%6)
			b.SetVoiceSlots(in)
			frames, sent, want = append(frames, f), append(sent, in), append(want, w)
			now := time.Duration(i) * slot
			if err := tx.Relay(now, &voice.Event{Kind: voice.EventVoice, Call: c, Burst: b, Position: enums.VoiceBurstA}); err != nil {
				t.Fatal(err)
			}
			out = append(out, tx.Tick(now, nil)...)
		}
		var got [][3][9]byte
		for _, o := range out {
			if o.Kind == voice.OutputBurst && o.Hint.Kind == layer2.KindVoice {
				got = append(got, o.Burst.VoiceSlots())
			}
		}
		if len(got) != len(want) {
			t.Fatalf("FID %v: %d voice bursts, want %d", fid, len(got), len(want))
		}
		for i := range got {
			for j := range got[i] {
				if got[i][j] != want[i][j] {
					t.Errorf("FID %v burst %d slot %d: relayed %x for %x, want %x", fid, i, j, got[i][j], sent[i][j], want[i][j])
				}
				if fid != enums.FIDStandardized || j == 2 {
					continue
				}
				var bits [72]unpacked.Bit
				copy(bits[:], unpacked.Unpack(got[i][j][:]))
				info, res := refambe.Decode(bits)
				wantInfo, _ := vocoder.DecodeAMBE72(want[i][j])
				if [7]byte(unpacked.Pack(info[:])) != [7]byte(wantInfo) || res.ErrorsCorrected != 0 || res.Uncorrectable {
					t.Errorf("burst %d slot %d: reference decode %x %+v, want %x without errors", i, j, unpacked.Pack(info[:]), res, wantInfo)
				}
				if j == 1 && wantInfo != frames[i][j] {
					t.Errorf("burst %d slot %d: decodes to %x, sent %x", i, j, wantInfo, frames[i][j])
				}
			}
		}
	}
}
