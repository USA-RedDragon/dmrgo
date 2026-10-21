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
	m := dmrd.TalkerAlias{Repeater: 3149999, Source: l.Source, Type: l.Type, Data: l.Data, Size: 19}
	p, err := m.AppendBinary(nil)
	if err != nil || len(p) != 19 || !bytes.Equal(p[8:], local[4:]) || !bytes.Equal(p[4:8], []byte{0x00, 0x30, 0x10, 0xAF}) {
		t.Fatalf("%x %v", p, err)
	}
	if got, err := dmrd.DecodeTalkerAlias(p); err != nil || got != m {
		t.Fatalf("%+v %v", got, err)
	}
	short, err := dmrd.DecodeTalkerAlias(p[:15])
	if err != nil || short.Size != 15 || short.Repeater != 3149999 || short.Source != 3191868 || short.Data != [7]byte{'K', 'I', '5'} {
		t.Fatalf("DMRGateway form: %+v %v", short, err)
	}
	if q, err := short.AppendBinary(nil); err != nil || !bytes.Equal(q, p[:15]) {
		t.Fatalf("DMRGateway form encodes %x %v", q, err)
	}
	for _, n := range []int{14, 16, 18, 20} {
		if _, err := dmrd.DecodeTalkerAlias(append(p[:0:0], make([]byte, n)...)); err == nil {
			t.Errorf("%d octets: no error", n)
		}
	}
	bad := m
	bad.Size = 17
	if _, err := bad.AppendBinary(nil); err == nil {
		t.Error("Size 17: no error")
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
	m := dmrd.Position{Repeater: 7, Source: local.Source, Data: local.Data, Size: 18}
	q, err := m.AppendBinary(nil)
	if err != nil || len(q) != 18 || !bytes.Equal(q[8:], p[4:]) {
		t.Fatalf("%x %v", q, err)
	}
	if got, err := dmrd.DecodePosition(q); err != nil || got != m {
		t.Fatalf("%+v %v", got, err)
	}
	short, err := dmrd.DecodePosition(q[:14])
	if err != nil || short.Size != 14 || short.Repeater != 7 || short.Source != 3191868 || short.Data != [7]byte{1, 2, 3} {
		t.Fatalf("DMRGateway form: %+v %v", short, err)
	}
	if r, err := short.AppendBinary(nil); err != nil || !bytes.Equal(r, q[:14]) {
		t.Fatalf("DMRGateway form encodes %x %v", r, err)
	}
}
