package data_test

import (
	"errors"
	"testing"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2"
	"github.com/USA-RedDragon/dmrgo/v2/data"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
)

func TestSenderSharedMethodSet(t *testing.T) {
	t.Parallel()

	var zero data.Sender
	if err := zero.Send(0, message(t, 10)); !errors.Is(err, dmrgo.ErrZeroValue) {
		t.Errorf("zero Sender Send: %v, want ErrZeroValue", err)
	}
	if out := zero.Lost(0, nil); len(out) != 0 {
		t.Errorf("zero Sender Lost: %+v", out)
	}
	var none *data.Sender
	if out := none.Lost(0, nil); out != nil {
		t.Errorf("nil Sender Lost: %+v", out)
	}
	for _, c := range []struct {
		name string
		cfg  data.SenderConfig
	}{
		{"Lead", data.SenderConfig{Lead: -time.Millisecond}},
		{"Slot", data.SenderConfig{Slot: enums.Slot(3)}},
	} {
		var ce *dmrgo.ConfigError
		if _, err := data.NewSender(&c.cfg); !errors.As(err, &ce) || ce.Field != c.name {
			t.Errorf("%s: %v, want a ConfigError for %s", c.name, err, c.name)
		}
	}

	const lead = 25 * time.Millisecond
	s, err := data.NewSender(&data.SenderConfig{Slot: enums.Slot2, Lead: lead, Preambles: -1, Access: enums.AccessImpolite})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Send(0, nil); !errors.Is(err, dmrgo.ErrNilArgument) {
		t.Errorf("Send nil: %v, want ErrNilArgument", err)
	}
	const now = time.Second
	if err := s.Send(now, message(t, 10)); err != nil {
		t.Fatal(err)
	}
	at, ok := s.Deadline()
	if !ok || at != now {
		t.Fatalf("Deadline %v %v, want %v", at, ok, now)
	}
	out := s.Tick(at, nil)
	if len(out) != 1 || out[0].Kind != data.OutputBurst || out[0].Due != now+lead || out[0].Hint.Slot != enums.Slot2 {
		t.Fatalf("first burst %+v, want Due %v on Slot2", out, now+lead)
	}
	if out[0].Hint.Kind != layer2.KindData {
		t.Errorf("Hint kind %v", out[0].Hint.Kind)
	}
	if next, _ := s.Deadline(); next != out[0].Due+60*time.Millisecond-lead {
		t.Errorf("next Deadline %v, want %v", next, out[0].Due+60*time.Millisecond-lead)
	}
	if out := s.Lost(next(s), nil); len(out) != 1 || out[0].Kind != data.OutputBurst {
		t.Errorf("Lost at the Deadline: %+v, want the next burst", out)
	}
}

func next(s *data.Sender) time.Duration {
	at, _ := s.Deadline()
	return at
}
