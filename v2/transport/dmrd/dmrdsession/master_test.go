package dmrdsession_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/transport/dmrd"
	"github.com/USA-RedDragon/dmrgo/v2/transport/dmrd/dmrdsession"
)

func TestMasterAliasAndPosition(t *testing.T) {
	t.Parallel()

	l := newLink(t, dmrd.DialectMMDVM, "", "pw", "pw")
	l.advance(l.now)
	a := dmrd.TalkerAlias{Repeater: 3129100}
	alias, _ := a.AppendBinary(nil)
	g := dmrd.Position{Repeater: 3129100}
	pos, _ := g.AppendBinary(nil)
	for _, p := range [][]byte{alias, pos} {
		out := l.master.Packet(l.now, p, nil)
		if len(out) != 1 || out[0].Kind != dmrdsession.OutputData {
			t.Fatalf("%q: %v", p[:4], kinds(out))
		}
		if out = l.peer.Packet(l.now, p, nil); len(out) != 1 || out[0].Kind != dmrdsession.OutputData {
			t.Fatalf("peer %q: %v", p[:4], kinds(out))
		}
	}
	g.Repeater = 5
	other, _ := g.AppendBinary(nil)
	if out := l.master.Packet(l.now, other, nil); len(out) != 1 || command(t, out[0].Packet, dmrd.DialectMMDVM) != dmrd.CommandMSTNAK {
		t.Fatal("position of another repeater")
	}
	if out := l.master.Packet(l.now, alias[:10], nil); len(out) != 0 {
		t.Fatal("short alias")
	}
	if out := l.master.Packet(l.now, make([]byte, dmrdsession.MaxPacket+1), nil); len(out) != 0 {
		t.Fatal("long packet")
	}
}
