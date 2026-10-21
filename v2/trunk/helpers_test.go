package trunk_test

import (
	"os"
	"testing"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/internal/bits"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/framer"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/layer3"
	"github.com/USA-RedDragon/dmrgo/v2/trunk"
)

const testCC = 3

var testSys = pdu.SysCode{Model: pdu.NetworkModelSmall, Net: 12, Site: 3, Par: 2}

const slot = 30 * time.Millisecond

func bsBurst(t testing.TB, d layer2.Data, cc enums.ColorCode) layer2.RawBurst {
	t.Helper()
	raw, err := layer2.BuildBurst(d, cc, layer2.SyncSourceBS)
	if err != nil {
		t.Fatalf("BuildBurst %T: %v", d, err)
	}
	return raw
}

func sysIdent(t testing.TB) uint16 {
	t.Helper()
	v, err := testSys.Pack()
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func aloha(t testing.TB) layer2.RawBurst {
	t.Helper()
	return bsBurst(t, &pdu.CSBK{LastBlock: true, Opcode: pdu.CSBKAloha, AlohaPDU: &pdu.AlohaPDU{
		Version: 1, NRandWait: 2, Reg: true, Backoff: 3, SysIdentCode: sysIdent(t)}}, testCC)
}

func tvGrant(t testing.TB, tg uint32, ch uint16, ts2 bool) layer2.RawBurst {
	t.Helper()
	return bsBurst(t, &pdu.CSBK{LastBlock: true, Opcode: pdu.CSBKTalkgroupVoiceGrant, TalkgroupVoiceGrantPDU: &pdu.TalkgroupVoiceGrantPDU{
		PhysicalChannel: ch, LogicalChannel: ts2, TargetAddress: tg, SourceAddress: 3120001}}, testCC)
}

func pClear(t testing.TB, tg uint32) layer2.RawBurst {
	t.Helper()
	return bsBurst(t, &pdu.CSBK{LastBlock: true, Opcode: pdu.CSBKClear, ClearPDU: &pdu.ClearPDU{
		GroupIndividual: true, TargetAddress: tg, SourceAddress: 3120001}}, testCC)
}

func terminator(t testing.TB, tg uint32) layer2.RawBurst {
	t.Helper()
	lc, err := pdu.NewVoiceLC(true, tg, 3120001, layer3.ServiceOptions{})
	if err != nil {
		t.Fatal(err)
	}
	lc.BurstDataType = enums.DataTypeTerminatorWithLC
	return bsBurst(t, &lc, testCC)
}

func mbcGrant(t testing.TB, tg uint32, cdef layer3.CdefParms, cc enums.ColorCode) [2]layer2.RawBurst {
	t.Helper()
	h := &pdu.MBCHeader{Opcode: pdu.CSBKTalkgroupVoiceGrant, TalkgroupVoiceGrantPDU: &pdu.TalkgroupVoiceGrantPDU{
		PhysicalChannel: trunk.ChannelAbsolute, LogicalChannel: true, TargetAddress: tg, SourceAddress: 3120001}}
	cg := pdu.CGAPContinuation{LastBlock: true, Opcode: pdu.CSBKTalkgroupVoiceGrant, ColorCode: cc, CdefParms: cdef}
	p, err := cg.Encode()
	if err != nil {
		t.Fatal(err)
	}
	cont, err := layer2.BuildDataBurst(p, enums.DataTypeMBCContinuation, testCC, layer2.SyncSourceBS)
	if err != nil {
		t.Fatal(err)
	}
	return [2]layer2.RawBurst{bsBurst(t, h, testCC), cont}
}

func kinds(out []trunk.Output) []trunk.OutputKind {
	k := make([]trunk.OutputKind, len(out))
	for i := range out {
		k[i] = out[i].Kind
	}
	return k
}

func wantKinds(t *testing.T, what string, out []trunk.Output, want ...trunk.OutputKind) {
	t.Helper()
	got := kinds(out)
	if len(got) != len(want) {
		t.Fatalf("%s: outputs %v, want %v", what, got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("%s: outputs %v, want %v", what, got, want)
		}
	}
}

type capture struct {
	at   time.Duration
	raw  layer2.RawBurst
	hint layer2.Hint
	cach *[3]byte
}

func readCapture(t testing.TB, path string) []capture {
	t.Helper()
	stream, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	fr := framer.New(framer.Config{CACH: true, Threshold: -1})
	bursts := fr.Write(stream, len(stream)*8)
	caps := make([]capture, 0, len(bursts))
	for _, b := range bursts {
		c := capture{at: time.Duration(b.Offset) * time.Second / 9600, raw: b.Data, hint: layer2.Hint{Slot: enums.Slot(b.Channel)}}
		if b.HasCACH {
			var cc [3]byte
			bits.Copy(cc[:], 0, stream, int(b.Offset)-layer2.CACHBits, layer2.CACHBits)
			c.cach = &cc
		}
		caps = append(caps, c)
	}
	return caps
}
