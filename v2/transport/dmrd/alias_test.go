package dmrd_test

import (
	"bytes"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/transport/dmrd"
)

func TestTalkerAliasForms(t *testing.T) {
	t.Parallel()

	local := []byte{'D', 'M', 'R', 'A', 0x30, 0xB4, 0x3C, 0, 'K', 'I', '5', 'V', 'M', 'F', ' '}
	l, err := dmrd.DecodeLocalTalkerAlias(local)
	if err != nil || l.Source != 3191868 || l.Type != 0 || string(l.Data[:]) != "KI5VMF " {
		t.Fatalf("%+v %v", l, err)
	}
	m := dmrd.TalkerAlias{Repeater: 3149999, Source: l.Source, Type: l.Type, Data: l.Data}
	p, err := m.AppendBinary(nil)
	if err != nil || len(p) != 19 || !bytes.Equal(p[8:], local[4:]) || !bytes.Equal(p[4:8], []byte{0x00, 0x30, 0x10, 0xAF}) {
		t.Fatalf("%x %v", p, err)
	}
	if got, err := dmrd.DecodeTalkerAlias(p); err != nil || got != m {
		t.Fatalf("%+v %v", got, err)
	}
	if _, err := dmrd.DecodeTalkerAlias(p[:15]); err == nil {
		t.Error("15 octets: no error")
	}
}

func TestPositionForms(t *testing.T) {
	t.Parallel()

	local := dmrd.LocalPosition{Source: 3191868, Data: [7]byte{1, 2, 3, 4, 5, 6, 7}}
	p, err := local.AppendBinary(nil)
	if err != nil || len(p) != 14 || string(p[:4]) != "DMRG" {
		t.Fatalf("%x %v", p, err)
	}
	if got, err := dmrd.DecodeLocalPosition(p); err != nil || got != local {
		t.Fatalf("%+v %v", got, err)
	}
	m := dmrd.Position{Repeater: 7, Source: local.Source, Data: local.Data}
	q, err := m.AppendBinary(nil)
	if err != nil || len(q) != 18 || !bytes.Equal(q[8:], p[4:]) {
		t.Fatalf("%x %v", q, err)
	}
	if got, err := dmrd.DecodePosition(q); err != nil || got != m {
		t.Fatalf("%+v %v", got, err)
	}
}
