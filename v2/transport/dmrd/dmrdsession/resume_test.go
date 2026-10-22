package dmrdsession_test

import (
	"testing"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/transport/dmrd"
	"github.com/USA-RedDragon/dmrgo/v2/transport/dmrd/dmrdsession"
)

func TestMasterResume(t *testing.T) {
	t.Parallel()

	s := time.Second
	m, _ := dmrdsession.NewMaster(&dmrdsession.MasterConfig{Dialect: dmrd.DialectMMDVM, Timeout: 30 * s})
	if out := m.Packet(0, ctl(dmrd.CommandRPTPING, 5), nil); len(out) != 1 || command(t, out[0].Packet, dmrd.DialectMMDVM) != dmrd.CommandMSTNAK {
		t.Fatal("ping before resume")
	}
	out := m.Resume(s, 5, nil)
	if len(out) != 1 || out[0].Kind != dmrdsession.OutputUp || out[0].ID != 5 || m.State() != dmrdsession.StateRunning || m.ID() != 5 {
		t.Fatalf("resume %+v", out)
	}
	if out = m.Packet(2*s, ctl(dmrd.CommandRPTPING, 5), nil); command(t, out[0].Packet, dmrd.DialectMMDVM) != dmrd.CommandMSTPONG {
		t.Fatal("ping after resume")
	}
	if out = m.Resume(10*s, 5, nil); len(out) != 0 {
		t.Fatalf("resume again %+v", out)
	}
	if at, _ := m.Deadline(); at != 40*s {
		t.Fatalf("deadline %v", at)
	}
	out = m.Resume(11*s, 6, nil)
	if len(out) != 2 || out[0].Reason != dmrdsession.ReasonLogin || out[0].ID != 5 || out[1].ID != 6 {
		t.Fatalf("resume of another ID %+v", out)
	}
	if out = m.Tick(41*s, nil); len(out) != 1 || out[0].Reason != dmrdsession.ReasonTimeout {
		t.Fatalf("timeout %+v", out)
	}
	var zero dmrdsession.Master
	if out = zero.Resume(0, 5, nil); len(out) != 0 || zero.State() != dmrdsession.StateIdle {
		t.Fatal("zero Master resumed")
	}
}
