package data_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/data"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
)

func TestNilBurst(t *testing.T) {
	t.Parallel()

	rx, err := data.NewReceiver(nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, h := range []layer2.Hint{{}, {SyncLost: true}, {End: layer2.EndStrong}} {
		if ev := rx.Burst(0, nil, h, nil); len(ev) != 0 {
			t.Errorf("Receiver %+v: %d events in Idle", h, len(ev))
		}
	}
	tx, err := data.NewSender(&data.SenderConfig{Source: layer2.SyncSourceMS, ColorCode: 1})
	if err != nil {
		t.Fatal(err)
	}
	if out := tx.Burst(0, nil, layer2.Hint{SyncLost: true}, nil); len(out) != 0 {
		t.Errorf("Sender.Burst: %d outputs in Idle", len(out))
	}
	if out := tx.Decoded(0, nil, layer2.Hint{}, nil); len(out) != 0 {
		t.Errorf("Sender.Decoded: %d outputs in Idle", len(out))
	}
}
