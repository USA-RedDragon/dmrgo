package data_test

import (
	"errors"
	"testing"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2"
	"github.com/USA-RedDragon/dmrgo/v2/data"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
)

func TestSenderZeroRateIsRate12(t *testing.T) {
	t.Parallel()
	zero := message(t, 30)
	zero.Rate = 0
	a, b := newSender(t, &data.SenderConfig{Slot: enums.Slot1, Preambles: -1}), newSender(t, &data.SenderConfig{Slot: enums.Slot1, Preambles: -1})
	if err := a.Send(0, zero); err != nil {
		t.Fatal(err)
	}
	if err := b.Send(0, message(t, 30)); err != nil {
		t.Fatal(err)
	}
	got, want := drain(t, a, 2*time.Second), drain(t, b, 2*time.Second)
	if len(got) != len(want) {
		t.Fatalf("%d outputs, want %d", len(got), len(want))
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("output %d: %+v, want %+v", i, got[i], want[i])
		}
	}
	if zero.Rate != 0 {
		t.Errorf("Send changed m.Rate to %v", zero.Rate)
	}
	if message(t, 30).Rate != enums.DataTypeRate12 {
		t.Fatal("message() is not rate 1/2")
	}
}

func TestSenderNilMessage(t *testing.T) {
	t.Parallel()
	s := newSender(t, &data.SenderConfig{Slot: enums.Slot1, Preambles: -1})
	if err := s.Send(0, nil); !errors.Is(err, dmrgo.ErrNilArgument) {
		t.Errorf("Send(nil): %v", err)
	}
	if s.State() != data.SenderIdle {
		t.Errorf("state %v after Send(nil)", s.State())
	}
}
