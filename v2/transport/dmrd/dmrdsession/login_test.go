package dmrdsession_test

import (
	"errors"
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
	m.Admit(7, []byte("pw"), nil)
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
	m2.Admit(7, []byte("pw"), nil)
	m2.Packet(4*s, rptk(5, tok), nil)
	if at, _ := m2.Deadline(); at != 9*s {
		t.Fatalf("config deadline %v", at)
	}
	m2.Packet(8*s, mmdvmConfig(5), nil)
	if at, _ := m2.Deadline(); at != 108*s || m2.State() != dmrdsession.StateRunning {
		t.Fatalf("running deadline %v %v", at, m2.State())
	}
	var ce *dmrgo.ConfigError
	if _, err := dmrdsession.NewMaster(&dmrdsession.MasterConfig{Dialect: dmrd.DialectMMDVM, LoginTimeout: -1}); !errors.As(err, &ce) {
		t.Fatalf("negative LoginTimeout: %v", err)
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
	m.Admit(7, []byte("pw"), nil)
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
