package lrrp_test

import (
	"bytes"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/mototrbo/lrrp"
)

func TestRequestMaxInfoAge(t *testing.T) {
	t.Parallel()

	m := new(lrrp.Message)
	if err := lrrp.DecodeMessageInto(m, unhex(t, "090e22040000000151"+"42822c"+"62343178")); err != nil {
		t.Fatal(err)
	}
	q := &m.Request
	if m.Document != lrrp.DocumentTriggeredRequest || !q.HasMaxInfoAge || q.MaxInfoAge != 300 || !q.AccuracyAndTime || !q.Speed ||
		!q.Periodic || !q.HasInterval || q.Interval != 120 || !bytes.Equal(q.RequestID, []byte{0, 0, 0, 1}) {
		t.Fatalf("lrrpdec request %s", m.String())
	}
	out, err := m.AppendBinary(nil)
	if err != nil {
		t.Fatal(err)
	}
	if want := unhex(t, "090e220400000001"+"343178"+"51"+"42822c"+"62"); !bytes.Equal(out, want) {
		t.Errorf("encodes to %x, want %x", out, want)
	}
	back := new(lrrp.Message)
	if err := lrrp.DecodeMessageInto(back, out); err != nil || back.Request.MaxInfoAge != 300 || !back.Request.HasMaxInfoAge {
		t.Errorf("round trip %s, %v", back.String(), err)
	}
	q.HasMaxInfoAge = false
	if out, err := m.AppendBinary(nil); err != nil || bytes.Contains(out, []byte{0x42, 0x82}) {
		t.Errorf("without HasMaxInfoAge: %x %v", out, err)
	}
}
