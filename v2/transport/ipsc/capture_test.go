package ipsc_test

import (
	"bufio"
	"bytes"
	"encoding/hex"
	"os"
	"strings"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/transport/ipsc"
	"github.com/USA-RedDragon/dmrgo/v2/vocoder"
)

type capLine struct {
	tag string
	p   []byte
}

func readXPR(t *testing.T) []capLine {
	t.Helper()
	f, err := os.Open("testdata/xpr8400.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var out []capLine
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fs := strings.Fields(sc.Text())
		p, err := hex.DecodeString(fs[1])
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, capLine{fs[0], p})
	}
	return out
}

func TestXPR8400PacketsRoundTrip(t *testing.T) {
	t.Parallel()

	for _, c := range readXPR(t) {
		switch ipsc.Opcode(c.p[0]) {
		case ipsc.OpGroupVoice, ipsc.OpPrivateVoice, ipsc.OpGroupData, ipsc.OpPrivateData:
			u, err := ipsc.DecodeUser(c.p)
			if err != nil {
				t.Fatalf("%x: %v", c.p, err)
			}
			out, err := u.AppendBinary(nil)
			if err != nil || !bytes.Equal(out, c.p) {
				t.Fatalf("%x: encode %x %v", c.p, out, err)
			}
			if h := u.Hint(); h.Kind == layer2.KindUnknown && !h.SyncLost && u.Burst != 0x26 {
				t.Errorf("%x: hint %+v", c.p, h)
			}
		case ipsc.OpRegisterRequest, ipsc.OpRegisterReply, ipsc.OpPeerListRequest, ipsc.OpPeerListReply, ipsc.OpAliveRequest, ipsc.OpAliveReply, ipsc.OpDeregisterRequest:
			m, err := ipsc.DecodeControl(c.p)
			if err != nil {
				t.Fatalf("%x: %v", c.p, err)
			}
			out, err := m.AppendBinary(nil)
			if m.Opcode == ipsc.OpPeerListReply {
				l, _ := ipsc.PeerListOf(c.p)
				entries := make([]ipsc.PeerEntry, l.Len())
				for i := range entries {
					entries[i] = l.Entry(i)
				}
				out = ipsc.AppendPeerListReply(nil, m.ID, entries)
			}
			if err != nil || !bytes.Equal(out, c.p) {
				t.Fatalf("%x: encode %x %v", c.p, out, err)
			}
		default:
			r, err := ipsc.DecodeRaw(c.p)
			if err != nil || !bytes.Equal(r.AppendBinary(nil), c.p) {
				t.Fatalf("%x: raw %v", c.p, err)
			}
		}
	}
}

func TestXPR8400Register(t *testing.T) {
	t.Parallel()

	p, _ := hex.DecodeString("900004ce2e6a0000805c04080400")
	c, err := ipsc.DecodeControl(p)
	if err != nil || c.ID != 314926 || !c.Mode.Operational() || !c.Mode.Digital() || c.Mode.TS1() != 2 || c.Mode.TS2() != 2 {
		t.Fatalf("%+v %v", c, err)
	}
	if !c.Flags.Has(ipsc.FlagCSBK) || !c.Flags.Has(ipsc.FlagXNLMaster) || !c.Flags.Has(ipsc.FlagAuth) || !c.Flags.Has(ipsc.FlagData) ||
		!c.Flags.Has(ipsc.FlagVoice) || c.Flags.Has(ipsc.FlagMasterPeer) || c.Version != [4]byte{4, 8, 4, 0} {
		t.Fatalf("flags %x version %x", c.Flags, c.Version)
	}
}

func TestXPR8400CallsRebuild(t *testing.T) {
	t.Parallel()

	var voice, data int
	for _, c := range readXPR(t) {
		if c.tag == "one" {
			continue
		}
		u, err := ipsc.DecodeUser(c.p)
		if err != nil {
			t.Fatal(err)
		}
		if u.Burst&0x7F == ipsc.BurstSyncLost {
			continue
		}
		raw, err := u.RawBurst(7, layer2.SyncSourceBS)
		if err != nil {
			if u.Burst == ipsc.BurstRaw || u.Burst == 0x26 {
				continue
			}
			t.Fatalf("%x: %v", c.p, err)
		}
		h := u.Hint()
		var back ipsc.User
		back.Info = u.Info
		back.Voice.LC = u.Voice.LC
		pos := h.VoiceBurst
		if u.IsVoice() {
			voice++
			frames, _ := raw.AMBEFrames()
			ambe, _ := vocoder.PackAMBEVoice(frames)
			want, _ := vocoder.UnpackAMBEVoiceFlags(u.Voice.AMBE)
			got, _ := vocoder.UnpackAMBEVoiceFlags(ambe)
			if got != want {
				t.Fatalf("%x: AMBE frames changed", c.p)
			}
			if pos == enums.VoiceBurstUnknown {
				pos = enums.VoiceBurstB
			}
		} else {
			data++
			if dt := enums.DataType(u.Burst); dt != enums.DataTypeRate34 {
				pl, res := raw.BPTCPayload()
				if res.Uncorrectable || !bytes.Equal(pl[:], u.Data.Octets[:12]) {
					t.Fatalf("%x: BPTC %x", c.p, pl)
				}
				if dt == enums.DataTypeVoiceLCHeader || dt == enums.DataTypeTerminatorWithLC || dt == enums.DataTypeDataHeader || dt == enums.DataTypeCSBK {
					if !layer2.MMDVMHostAccepts(dt, pl) {
						t.Errorf("%x: %v fails its CRC or parity", c.p, dt)
					}
				}
			}
		}
		h.VoiceBurst = pos
		if err := back.SetRawBurst(raw, h); err != nil {
			t.Fatalf("%x: SetRawBurst %v", c.p, err)
		}
		if u.IsVoice() {
			if back.Voice.AMBE != u.Voice.AMBE && u.Voice.AMBE[6]&0x40 == 0 && u.Voice.AMBE[12]&0x10 == 0 {
				t.Errorf("%x: voice %x, want %x", c.p, back.Voice.AMBE, u.Voice.AMBE)
			}
			if u.Voice.Layout != ipsc.VoiceLayoutA && back.Voice.EMB != u.Voice.EMB {
				t.Errorf("%x: EMB %x, want %x", c.p, back.Voice.EMB, u.Voice.EMB)
			}
		} else if back.Data.Octets != u.Data.Octets {
			t.Errorf("%x: data %x, want %x", c.p, back.Data.Octets, u.Data.Octets)
		}
	}
	if voice == 0 || data == 0 {
		t.Fatalf("voice %d data %d", voice, data)
	}
}
