package dmrdsession_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2"
	"github.com/USA-RedDragon/dmrgo/v2/transport/dmrd"
	"github.com/USA-RedDragon/dmrgo/v2/transport/dmrd/auth"
	"github.com/USA-RedDragon/dmrgo/v2/transport/dmrd/dmrdsession"
)

func TestMasterLoginTimeout(t *testing.T) {
	t.Parallel()

	s := time.Second
	m, _ := dmrdsession.NewMaster(&dmrdsession.MasterConfig{Dialect: dmrd.DialectMMDVM})
	m.Packet(0, ctl(dmrd.CommandRPTL, 5), nil)
	if at, _ := m.Deadline(); at != dmrdsession.LoginTimeoutDefault {
		t.Fatalf("login deadline %v", at)
	}
	m.Admit(0, 7, []byte("pw"), nil)
	if at, _ := m.Deadline(); at != dmrdsession.LoginTimeoutDefault {
		t.Fatalf("auth deadline %v", at)
	}
	out := m.Tick(dmrdsession.LoginTimeoutDefault, nil)
	if len(out) != 1 || out[0].Reason != dmrdsession.ReasonTimeout || out[0].State != dmrdsession.StateAuth {
		t.Fatalf("login timeout %+v", out)
	}
	m2, _ := dmrdsession.NewMaster(&dmrdsession.MasterConfig{Dialect: dmrd.DialectMMDVM, LoginTimeout: 5 * s, Timeout: 100 * s})
	tok, _ := auth.Token(dmrd.DialectMMDVM, 7, []byte("pw"))
	m2.Packet(0, ctl(dmrd.CommandRPTL, 5), nil)
	m2.Admit(0, 7, []byte("pw"), nil)
	m2.Packet(4*s, rptk(5, tok), nil)
	if at, _ := m2.Deadline(); at != 9*s {
		t.Fatalf("config deadline %v", at)
	}
	m2.Packet(8*s, mmdvmConfig(5), nil)
	if at, _ := m2.Deadline(); at != 9*s || m2.State() != dmrdsession.StateConfig {
		t.Fatalf("deadline before Accept %v %v", at, m2.State())
	}
	m2.Accept(8*s, nil)
	if at, _ := m2.Deadline(); at != 108*s || m2.State() != dmrdsession.StateRunning {
		t.Fatalf("running deadline %v %v", at, m2.State())
	}
	var ce *dmrgo.ConfigError
	if _, err := dmrdsession.NewMaster(&dmrdsession.MasterConfig{Dialect: dmrd.DialectMMDVM, LoginTimeout: -1}); !errors.As(err, &ce) {
		t.Fatalf("negative LoginTimeout: %v", err)
	}
}

func TestMasterRefuseConfig(t *testing.T) {
	t.Parallel()

	m, _ := dmrdsession.NewMaster(&dmrdsession.MasterConfig{Dialect: dmrd.DialectMMDVM})
	tok, _ := auth.Token(dmrd.DialectMMDVM, 7, []byte("pw"))
	m.Packet(0, ctl(dmrd.CommandRPTL, 5), nil)
	m.Admit(0, 7, []byte("pw"), nil)
	m.Packet(0, rptk(5, tok), nil)
	if out := m.Packet(0, mmdvmConfig(5), nil); len(out) != 1 || out[0].Kind != dmrdsession.OutputConfig || out[0].ID != 5 {
		t.Fatalf("RPTC %+v", out)
	}
	out := m.Refuse(0, nil)
	if len(out) != 2 || command(t, out[0].Packet, dmrd.DialectMMDVM) != dmrd.CommandMSTNAK || out[1].Kind != dmrdsession.OutputDown || out[1].State != dmrdsession.StateConfig || out[1].Reason != dmrdsession.ReasonRefused {
		t.Fatalf("Refuse %+v", out)
	}
	if out = m.Accept(0, nil); len(out) != 0 || m.State() != dmrdsession.StateIdle {
		t.Fatalf("Accept after Refuse %+v %v", out, m.State())
	}
	m.Packet(0, ctl(dmrd.CommandRPTL, 5), nil)
	m.Admit(0, 7, []byte("pw"), nil)
	m.Packet(0, rptk(5, tok), nil)
	m.Packet(0, mmdvmConfig(5), nil)
	out = m.Accept(dmrdsession.LoginTimeoutDefault, nil)
	if len(out) != 1 || out[0].Kind != dmrdsession.OutputDown || out[0].Reason != dmrdsession.ReasonTimeout || m.State() != dmrdsession.StateIdle {
		t.Fatalf("Accept after LoginTimeout %+v %v", out, m.State())
	}
}

func TestMasterLongPassword(t *testing.T) {
	t.Parallel()

	pass := strings.Repeat("p", 200)
	for _, d := range []dmrd.Dialect{dmrd.DialectMMDVM, dmrd.DialectHBRP} {
		l := newLink(t, d, "", pass, pass)
		l.advance(l.now)
		if l.peer.State() != dmrdsession.StateRunning || l.master.State() != dmrdsession.StateRunning {
			t.Fatalf("%v: 200-octet password: states %v %v", d, l.peer.State(), l.master.State())
		}
		l = newLink(t, d, "", pass, pass[:199]+"q")
		l.advance(l.now)
		if l.master.State() == dmrdsession.StateRunning {
			t.Fatalf("%v: a password that differs in octet 200 logged in", d)
		}
	}
}

func TestMasterRepeatedRPTLDoesNotExtendLogin(t *testing.T) {
	t.Parallel()

	s := time.Second
	m, _ := dmrdsession.NewMaster(&dmrdsession.MasterConfig{Dialect: dmrd.DialectMMDVM, LoginTimeout: 10 * s})
	m.Packet(0, ctl(dmrd.CommandRPTL, 5), nil)
	for now := s; now < 10*s; now += s {
		m.Packet(now, ctl(dmrd.CommandRPTL, 5), nil)
	}
	m.Admit(0, 7, []byte("pw"), nil)
	for now := 9 * s; now < 10*s; now += 100 * time.Millisecond {
		m.Packet(now, ctl(dmrd.CommandRPTL, 5), nil)
	}
	if at, _ := m.Deadline(); at != 10*s {
		t.Fatalf("deadline %v", at)
	}
	out := m.Tick(10*s, nil)
	if len(out) != 1 || out[0].Reason != dmrdsession.ReasonTimeout || m.State() != dmrdsession.StateIdle {
		t.Fatalf("half-open login kept %+v %v", out, m.State())
	}
	if out = m.Packet(11*s, ctl(dmrd.CommandRPTL, 5), nil); len(out) != 1 || out[0].Kind != dmrdsession.OutputLogin {
		t.Fatalf("new attempt %+v", out)
	}
	if at, _ := m.Deadline(); at != 21*s {
		t.Fatalf("new attempt deadline %v", at)
	}
}
