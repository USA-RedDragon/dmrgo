package session_test

import (
	"errors"
	"testing"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/transport/dmrd"
	"github.com/USA-RedDragon/dmrgo/v2/transport/dmrd/auth"
	"github.com/USA-RedDragon/dmrgo/v2/transport/dmrd/session"
)

func TestMasterLoginTimeout(t *testing.T) {
	t.Parallel()

	s := time.Second
	m, _ := session.NewMaster(&session.MasterConfig{Dialect: dmrd.DialectMMDVM})
	m.Packet(0, ctl(dmrd.CommandRPTL, 5), nil)
	if at, _ := m.Deadline(); at != session.LoginTimeoutDefault {
		t.Fatalf("login deadline %v", at)
	}
	m.Admit(s, 7, []byte("pw"), nil)
	if at, _ := m.Deadline(); at != s+session.LoginTimeoutDefault {
		t.Fatalf("auth deadline %v", at)
	}
	out := m.Tick(s+session.LoginTimeoutDefault, nil)
	if len(out) != 1 || out[0].Reason != session.ReasonTimeout || out[0].State != session.StateAuth {
		t.Fatalf("login timeout %+v", out)
	}
	m2, _ := session.NewMaster(&session.MasterConfig{Dialect: dmrd.DialectMMDVM, LoginTimeout: 5 * s, Timeout: 100 * s})
	tok, _ := auth.Token(dmrd.DialectMMDVM, 7, []byte("pw"))
	m2.Packet(0, ctl(dmrd.CommandRPTL, 5), nil)
	m2.Admit(0, 7, []byte("pw"), nil)
	m2.Packet(4*s, rptk(5, tok), nil)
	if at, _ := m2.Deadline(); at != 9*s {
		t.Fatalf("config deadline %v", at)
	}
	m2.Packet(8*s, mmdvmConfig(5), nil)
	if at, _ := m2.Deadline(); at != 108*s || m2.State() != session.StateRunning {
		t.Fatalf("running deadline %v %v", at, m2.State())
	}
	var ce *session.ConfigError
	if _, err := session.NewMaster(&session.MasterConfig{Dialect: dmrd.DialectMMDVM, LoginTimeout: -1}); !errors.As(err, &ce) {
		t.Fatalf("negative LoginTimeout: %v", err)
	}
}
