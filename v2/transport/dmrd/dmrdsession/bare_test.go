package dmrdsession_test

import (
	"encoding/binary"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/transport/dmrd"
	"github.com/USA-RedDragon/dmrgo/v2/transport/dmrd/dmrdsession"
)

func TestPeerBareMasterMessages(t *testing.T) {
	t.Parallel()

	p, err := dmrdsession.NewPeer(&dmrdsession.PeerConfig{Dialect: dmrd.DialectMMDVM, ID: 5, Password: []byte("pw"), Config: mmdvmConfig(5)})
	if err != nil {
		t.Fatal(err)
	}
	out := p.Tick(0, nil)
	if len(out) != 1 || command(t, out[0].Packet, dmrd.DialectMMDVM) != dmrd.CommandRPTL {
		t.Fatalf("first Tick %+v", out)
	}
	if out = p.Packet(0, []byte("RPTACK"), nil); len(out) != 0 || p.State() != dmrdsession.StateLogin {
		t.Fatalf("RPTACK without the salt %+v %v", out, p.State())
	}
	salted := binary.BigEndian.AppendUint32([]byte("RPTACK"), 7)
	if out = p.Packet(0, salted, nil); len(out) != 1 || command(t, out[0].Packet, dmrd.DialectMMDVM) != dmrd.CommandRPTK {
		t.Fatalf("RPTACK with the salt %+v", out)
	}
	if out = p.Packet(0, []byte("RPTACK"), nil); len(out) != 1 || command(t, out[0].Packet, dmrd.DialectMMDVM) != dmrd.CommandRPTC {
		t.Fatalf("bare RPTACK of the RPTK %+v %v", out, p.State())
	}
	if out = p.Packet(0, []byte("RPTACK"), nil); len(out) != 1 || out[0].Kind != dmrdsession.OutputUp {
		t.Fatalf("bare RPTACK of the RPTC %+v %v", out, p.State())
	}
	if out = p.Packet(0, []byte("MSTCL"), nil); len(out) != 1 || out[0].Kind != dmrdsession.OutputDown || out[0].Reason != dmrdsession.ReasonClosed {
		t.Fatalf("bare MSTCL %+v", out)
	}
	at, _ := p.Deadline()
	if out = p.Tick(at, nil); len(out) != 1 || command(t, out[0].Packet, dmrd.DialectMMDVM) != dmrd.CommandRPTL {
		t.Fatalf("login after MSTCL %+v", out)
	}
	if out = p.Packet(at, []byte("MSTNAK"), nil); len(out) != 1 || out[0].Kind != dmrdsession.OutputDown || out[0].Reason != dmrdsession.ReasonRefused {
		t.Fatalf("bare MSTNAK %+v", out)
	}
	for _, b := range []string{"RPTACKx", "MSTCLxx", "MSTNAKxxx"} {
		if out = p.Packet(at, []byte(b), nil); len(out) != 0 {
			t.Fatalf("%q: %+v", b, out)
		}
	}
}
